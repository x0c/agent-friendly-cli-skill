// Skill sync: silently align the local agent's companion skill with the latest
// version from your organization's skills repository, in the background.
//
// Design points:
//   - Updates are fully delegated to npx skills (the vercel-labs/skills CLI);
//     no per-agent-directory sync logic is implemented here. It keeps a global
//     lock file (~/.agents/.skill-lock.json) recording sources and content
//     hashes, and its update semantics are "reinstall only when the remote
//     directory hash differs from the locked one", which is naturally idempotent.
//   - Eligibility is decided solely by that lock file: the target skill is
//     tracked and its source points to your organization's skills repository,
//     proving this machine manages the skill through npx skills (machines that
//     installed the skill via `npx skills add`).
//   - Local-authority exemption: skip the sync when a local skills-authority
//     directory exists (e.g. when skills are authored locally), so unpushed
//     local updates are never overwritten by older repository content.
//   - Mutual exclusion: several CLI workers running npx skills update
//     concurrently contend for the same lock JSON (read-modify-write); an
//     exclusive lock file under the cache directory serializes them, and stale
//     locks are broken automatically after a timeout.
//   - Results are written only to a local state file (<command>.skill.json);
//     stdout/stderr are never touched.
package autoupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	envNoSkillSync    = "MC_CLI_NO_SKILL_SYNC"
	skillSyncTimeout  = 5 * time.Minute
	skillLockStaleAge = 10 * time.Minute // how long a mutex lock left behind by a killed worker counts as stale
)

// skillLockFile mirrors the npx skills global lock file
// (~/.agents/.skill-lock.json); only the fields this module cares about are
// parsed.
type skillLockFile struct {
	Version int                       `json:"version"`
	Skills  map[string]skillLockEntry `json:"skills"`
}

type skillLockEntry struct {
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
	SourceURL  string `json:"sourceUrl"`
	SkillPath  string `json:"skillPath"`
}

// skillSyncDisabled reports whether skill sync has been explicitly disabled.
func skillSyncDisabled() bool {
	return truthy(os.Getenv(envNoSkillSync))
}

// skillSyncDue reports whether a skill sync check is due.
func skillSyncDue(opts Options) bool {
	if opts.SkillName == "" || skillSyncDisabled() {
		return false
	}
	opts = normalize(opts)
	st, err := loadState(opts.CommandName + ".skill")
	if err != nil || st.LastAttempt.IsZero() {
		return true
	}
	if st.LastResult == "skill_error" {
		return time.Since(st.LastAttempt) >= errorBackoff
	}
	// All other results (up_to_date/updated/skipped_*) are throttled by the
	// normal interval.
	return time.Since(st.LastCheck) >= opts.Interval
}

// runSkillSync performs one synchronous skill check/update and records state;
// invoked by the worker subprocess and by tests.
func runSkillSync(opts Options) (err error) {
	opts = normalize(opts)
	attempt := time.Now().UTC()
	result := "skill_error"
	var msg string
	defer func() {
		st := state{LastAttempt: attempt, LastResult: result, Error: msg}
		if result != "skill_error" {
			st.LastCheck = attempt
		}
		_ = saveState(opts.CommandName+".skill", st)
	}()

	eligible, reason, err := skillEligible(opts)
	if err != nil {
		msg = err.Error()
		return err
	}
	if !eligible {
		result = "skill_skipped_" + reason
		return nil
	}

	release, ok, err := acquireSkillSyncLock()
	if err != nil {
		msg = err.Error()
		return err
	}
	if !ok {
		// Another worker is currently updating (lock JSON read-modify-write
		// contention); give up this round. Once it finishes, the local skill is
		// already up to date, and the next check happens naturally after the
		// interval.
		result = "skill_skipped_locked"
		return nil
	}
	defer release()

	npxPath, err := exec.LookPath("npx")
	if err != nil {
		result = "skill_skipped_no_npx"
		msg = msgNpxNotFound
		return nil
	}

	out, err := runSkillsUpdate(npxPath, opts.SkillName)
	if err != nil {
		msg = fmt.Sprintf(msgNpxUpdateFailed, err, tailString(out, 400))
		return err
	}
	// Exit code 0 covers three outcomes: updated, already up to date, or
	// skipped (lock entry missing a hash, upstream deleted, etc.). All count as
	// a successful check and are throttled by the normal interval; for skips,
	// the output tail is stored in the state for troubleshooting.
	lower := strings.ToLower(out)
	switch {
	case strings.Contains(lower, "up to date"):
		result = "skill_up_to_date"
	case strings.Contains(lower, "updated"):
		result = "skill_updated"
	default:
		result = "skill_noop"
		msg = msgNpxNoop + tailString(out, 400)
	}
	return nil
}

// skillEligible decides whether this machine should auto-sync the given skill.
// It returns (participates, skip reason, error); the reason is embedded in the
// state result (skill_skipped_<reason>). When opts.SkillRepoMarker is empty the
// source repository is not checked, and any tracked skill with the same name
// participates in the sync.
func skillEligible(opts Options) (bool, string, error) {
	if opts.SkillName == "" {
		return false, "no_skill_config", nil
	}
	path, err := skillLockPath()
	if err != nil {
		return false, "", fmt.Errorf(msgSkillLockPathFailed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No npx skills lock file: this machine did not receive the skill
			// via npx skills distribution; do not participate.
			return false, "no_lock", nil
		}
		return false, "", fmt.Errorf(msgReadSkillLockFailed, err)
	}
	var lock skillLockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		// Matches the skills CLI behavior: a corrupted lock is treated as empty.
		return false, "no_lock", nil
	}
	entry, ok := lock.Skills[opts.SkillName]
	if !ok {
		return false, "not_tracked", nil
	}
	if opts.SkillRepoMarker != "" &&
		!strings.Contains(entry.Source, opts.SkillRepoMarker) && !strings.Contains(entry.SourceURL, opts.SkillRepoMarker) {
		// A skill with the same name comes from another repository: never
		// update a skill that belongs to another repository.
		return false, "source_mismatch", nil
	}
	if dir := localAuthoritySkillDir(opts.SkillName); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			// Local-authority exemption: a local skills-authority directory
			// exists (e.g. skills are authored locally and may hold unpushed
			// updates); skip the sync so repository content cannot overwrite
			// them.
			return false, "authoritative_machine", nil
		}
	}
	return true, "", nil
}

// skillLockPath mirrors the skills CLI global lock location:
// $XDG_STATE_HOME/skills/.skill-lock.json, otherwise
// ~/.agents/.skill-lock.json.
func skillLockPath() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "skills", ".skill-lock.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agents", ".skill-lock.json"), nil
}

// localAuthoritySkillDir returns the skill's directory inside the local skills
// authority (~/.config/skills-authority). It is only an exemption marker for
// the sync, not a dependency.
func localAuthoritySkillDir(skillName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "skills-authority", skillName)
}

// acquireSkillSyncLock serializes concurrent npx skills update runs with an
// exclusively created lock file (workers of several CLIs updating different
// skills still contend for the same lock JSON). Returning ok=false without the
// lock is not an error; stale locks older than skillLockStaleAge are broken
// automatically and retried once.
func acquireSkillSyncLock() (release func(), ok bool, err error) {
	dir, err := resolveCacheDir()
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	path := filepath.Join(dir, "skills-update.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > skillLockStaleAge {
			_ = os.Remove(path)
			f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
		if err != nil {
			return nil, false, nil
		}
	}
	_, _ = f.WriteString(time.Now().UTC().Format(time.RFC3339))
	_ = f.Close()
	return func() { _ = os.Remove(path) }, true, nil
}

// runSkillsUpdate executes npx skills update <skill> -g -y. stdin is closed
// (interactive prompts would hang), output is captured into a buffer (never
// touching the terminal), and DISABLE_TELEMETRY=1 is set.
func runSkillsUpdate(npxPath, skillName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), skillSyncTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, npxPath, "--yes", "skills", "update", skillName, "-g", "-y")
	cmd.Env = append(os.Environ(), "DISABLE_TELEMETRY=1")
	cmd.Stdin = nil
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// tailString returns at most the last n characters of the output, used for
// error summaries in the state file.
func tailString(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
