package autoupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// withTestHome isolates HOME (the skills lock and the local skills-authority
// directory both resolve from HOME) and clears XDG_STATE_HOME.
func withTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	return home
}

// writeSkillLock writes the npx skills global lock file under the given HOME.
func writeSkillLock(t *testing.T, home, skill, source, sourceURL string) {
	t.Helper()
	dir := filepath.Join(home, ".agents")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	lock := skillLockFile{
		Version: 3,
		Skills: map[string]skillLockEntry{
			skill: {Source: source, SourceType: "git", SourceURL: sourceURL, SkillPath: skill + "/"},
		},
	}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".skill-lock.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// installNpxShim puts a fake npx at the front of PATH: it records arguments and
// environment and emulates successful update output. Returns the marker file
// path. Unix only.
func installNpxShim(t *testing.T, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell shim is Unix only")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	shim := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"" + marker + "\"\n" +
		"printf 'DISABLE_TELEMETRY=%s\\n' \"$DISABLE_TELEMETRY\" >> \"" + marker + "\"\n" +
		"echo '" + output + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "npx"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

func baseSkillOpts() Options {
	return Options{CommandName: "testcli", RepoSubdir: "test-cli", SkillName: "myskill", SkillRepoMarker: "acme/skills", Interval: time.Hour}
}

func skillState(t *testing.T, command string) state {
	t.Helper()
	st, err := loadState(command + ".skill")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSkillSyncNoLockSkips(t *testing.T) {
	withTestCache(t)
	withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	marker := installNpxShim(t, "✓ Updated skill")

	if err := runSkillSync(baseSkillOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_skipped_no_lock" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
	if st.LastCheck.IsZero() {
		t.Fatal("skips should still write last_check for throttling")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("npx must not be invoked without a lock file")
	}
}

func TestSkillSyncEligibleRunsNpx(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	writeSkillLock(t, home, "myskill", "ssh://git@gitlab.example.com/acme/skills.git", "ssh://git@gitlab.example.com/acme/skills.git")
	marker := installNpxShim(t, "✓ Updated skill")

	if err := runSkillSync(baseSkillOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_updated" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal("npx should have been invoked")
	}
	content := string(data)
	for _, want := range []string{"--yes skills update myskill -g -y", "DISABLE_TELEMETRY=1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("unexpected npx invocation: %q, missing %q", content, want)
		}
	}
}

func TestSkillSyncUpToDateDetection(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	writeSkillLock(t, home, "myskill", "ssh://git@gitlab.example.com/acme/skills.git", "")
	installNpxShim(t, "✓ All global skills are up to date")

	if err := runSkillSync(baseSkillOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_up_to_date" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
}

func TestSkillSyncSkipsOnAuthoritativeMachine(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	writeSkillLock(t, home, "myskill", "ssh://git@gitlab.example.com/acme/skills.git", "")
	if err := os.MkdirAll(filepath.Join(home, ".config", "skills-authority", "myskill"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := installNpxShim(t, "✓ Updated skill")

	if err := runSkillSync(baseSkillOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_skipped_authoritative_machine" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("npx must not be invoked when a local skills authority exists")
	}
}

func TestSkillSyncSourceMismatchSkips(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	writeSkillLock(t, home, "myskill", "https://github.com/someone/other-skills.git", "https://github.com/someone/other-skills.git")
	marker := installNpxShim(t, "✓ Updated skill")

	if err := runSkillSync(baseSkillOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_skipped_source_mismatch" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("npx must not be invoked when the source does not match")
	}
}

func TestSkillSyncNotTrackedSkips(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	writeSkillLock(t, home, "other-skill", "ssh://git@gitlab.example.com/acme/skills.git", "")
	marker := installNpxShim(t, "✓ Updated skill")

	if err := runSkillSync(baseSkillOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_skipped_not_tracked" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("npx must not be invoked for an untracked skill")
	}
}

func TestSkillSyncDisabledByEnv(t *testing.T) {
	withTestCache(t)
	withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "1")

	if !skillSyncDisabled() {
		t.Fatal("MC_CLI_NO_SKILL_SYNC=1 should disable")
	}
	if skillSyncDue(baseSkillOpts()) {
		t.Fatal("not due while disabled")
	}
}

func TestSkillSyncDueThrottle(t *testing.T) {
	withTestCache(t)
	withTestHome(t)
	opts := baseSkillOpts()

	if !skillSyncDue(opts) {
		t.Fatal("due when there is no state")
	}

	now := time.Now().UTC()
	if err := saveState("testcli.skill", state{LastAttempt: now, LastCheck: now, LastResult: "skill_updated"}); err != nil {
		t.Fatal(err)
	}
	if skillSyncDue(opts) {
		t.Fatal("not due within the interval")
	}

	if err := saveState("testcli.skill", state{LastAttempt: now, LastResult: "skill_error", Error: "boom"}); err != nil {
		t.Fatal(err)
	}
	if skillSyncDue(opts) {
		t.Fatal("not due within the failure backoff")
	}
	old := now.Add(-2 * errorBackoff)
	if err := saveState("testcli.skill", state{LastAttempt: old, LastResult: "skill_error", Error: "boom"}); err != nil {
		t.Fatal(err)
	}
	if !skillSyncDue(opts) {
		t.Fatal("due after the backoff expires")
	}
}

func TestAcquireSkillSyncLock(t *testing.T) {
	withTestCache(t)

	release, ok, err := acquireSkillSyncLock()
	if err != nil || !ok {
		t.Fatalf("first acquisition should succeed: ok=%v err=%v", ok, err)
	}

	_, ok2, err2 := acquireSkillSyncLock()
	if err2 != nil || ok2 {
		t.Fatalf("second acquisition while held should fail: ok=%v err=%v", ok2, err2)
	}

	release()
	release3, ok3, err3 := acquireSkillSyncLock()
	if err3 != nil || !ok3 {
		t.Fatalf("re-acquisition after release should succeed: ok=%v err=%v", ok3, err3)
	}
	release3()
}

func TestWorkerEnvPassesSkillName(t *testing.T) {
	// Worker environment protocol: spawnWorker passes envSkill through.
	env := workerEnv(Options{CommandName: "testcli", RepoSubdir: "test-cli", SkillName: "myskill"})
	found := ""
	prefix := envSkill + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			found = strings.TrimPrefix(e, prefix)
		}
	}
	if found != "myskill" {
		t.Fatalf("worker environment should contain %s=myskill, got %q", envSkill, found)
	}
}

// Empty SkillRepoMarker: the source repository is not checked, and any tracked
// skill with the same name participates in the sync.
func TestSkillSyncEmptyMarkerAcceptsAnySource(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("MC_CLI_NO_SKILL_SYNC", "")
	writeSkillLock(t, home, "myskill", "https://github.com/someone/other-skills.git", "")
	installNpxShim(t, "✓ Updated skill")

	opts := baseSkillOpts()
	opts.SkillRepoMarker = ""
	if err := runSkillSync(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := skillState(t, "testcli")
	if st.LastResult != "skill_updated" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
}

// Worker environment protocol: GitURL and the skill repository marker must be
// passed through to the subprocess intact.
func TestWorkerEnvPassesGitURLAndMarker(t *testing.T) {
	env := workerEnv(Options{
		CommandName:     "testcli",
		RepoSubdir:      "test-cli",
		GitURL:          "ssh://git@gitlab.example.com/acme/cli.git",
		SkillRepoMarker: "acme/skills",
	})
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		envGitURL + "=ssh://git@gitlab.example.com/acme/cli.git",
		envSkillMarker + "=acme/skills",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("worker environment missing %q", want)
		}
	}
}
