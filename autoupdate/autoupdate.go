// Package autoupdate provides silent background self-updates for CLI tools.
// It pulls prebuilt artifacts from a git repository's dist/ directory, compares
// the SHA256 with the local executable, and performs an atomic replacement on
// mismatch; failures are written to local state only and never disturb
// stdout/stderr.
//
// Entry-point convention (every CLI's Execute must follow it):
//
//	if autoupdate.MaybeWorker() { return } // or os.Exit(0)
//	autoupdate.Start(autoupdate.Options{...})
package autoupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	defaultBranch   = "master"
	defaultInterval = 24 * time.Hour
	errorBackoff    = time.Hour
	gitTimeout      = 3 * time.Minute

	envWorker         = "MC_CLI_AUTOUPDATE_WORKER"
	envCmd            = "MC_CLI_AUTOUPDATE_CMD"
	envSubdir         = "MC_CLI_AUTOUPDATE_SUBDIR"
	envGitURL         = "MC_CLI_AUTOUPDATE_GIT_URL"
	envBranch         = "MC_CLI_AUTOUPDATE_BRANCH"
	envVersion        = "MC_CLI_AUTOUPDATE_VERSION"
	envAllowDowngrade = "MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE"
	envSkill          = "MC_CLI_AUTOUPDATE_SKILL"
	envSkillMarker    = "MC_CLI_AUTOUPDATE_SKILL_MARKER"
)

// Options describes the self-update configuration of one CLI.
type Options struct {
	CommandName     string        // e.g. mytool, deploy
	RepoSubdir      string        // e.g. mytool-cli (used to build the dist path)
	GitURL          string        // required: git repository hosting the prebuilt artifacts (e.g. ssh://git@gitlab.example.com/acme/cli.git); when empty, self-update does not run and the reason is written to the state file
	Branch          string        // defaults to master
	Interval        time.Duration // check interval after success; defaults to 24h
	LocalVersion    string        // local binary version (git describe injected via ldflags, e.g. v0.1.0-55-g12fa79d)
	SkillName       string        // companion skill name (npx skills distribution name); empty disables skill sync
	SkillRepoMarker string        // skill sync eligibility criterion: substring that the lock entry's source/sourceUrl must contain (proving it comes from your organization's skills repository); empty skips the source check
}

// Update predicate: prefer comparing the versions from dist/manifest.json
// (forward-only: skip when the remote version is NOT HIGHER than local, which
// prevents a stale, not-yet-rebuilt dist on master from pulling the machine back
// to an older artifact). Only when the manifest is deterministically absent on
// the remote (git show explicitly reports the path does not exist — legacy
// projects that never rebuilt dist) or the manifest version is unparseable does
// it degrade to plain SHA256 alignment (recorded as updated_no_manifest).
// Manifest network failures, timeouts and parse failures always fail-closed
// (no replacement this round, state error). MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE=1
// forces plain-alignment semantics.

// state persists the result of the last check, used for throttling.
type state struct {
	LastCheck   time.Time `json:"last_check,omitempty"`
	LastAttempt time.Time `json:"last_attempt"`
	LastResult  string    `json:"last_result"`
	Error       string    `json:"error,omitempty"`
	LocalHash   string    `json:"local_hash,omitempty"`
	RemoteHash  string    `json:"remote_hash,omitempty"`
}

// updateSession fetches the artifact and the dist manifest from the same commit
// within one pull session; replaceable in tests.
type updateSession func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error)

var lookupExecutable = os.Executable

var runUpdateFetch updateSession = gitFetchUpdate

var resolveCacheDir = func() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "mc-cli-autoupdate"), nil
}

var startOnce sync.Map

// MaybeWorker, when the current process is a self-update worker, performs one
// update and returns true (the caller should exit immediately).
func MaybeWorker() bool {
	if os.Getenv(envWorker) != "1" {
		return false
	}
	opts := Options{
		CommandName:     os.Getenv(envCmd),
		RepoSubdir:      os.Getenv(envSubdir),
		GitURL:          os.Getenv(envGitURL),
		Branch:          os.Getenv(envBranch),
		LocalVersion:    os.Getenv(envVersion),
		SkillName:       os.Getenv(envSkill),
		SkillRepoMarker: os.Getenv(envSkillMarker),
	}
	// Skill sync and binary self-update are independent tasks: each keeps its
	// own throttle and its own state.
	normalized := normalize(opts)
	if normalized.SkillName != "" && !skillSyncDisabled() && skillSyncDue(normalized) {
		_ = runSkillSync(normalized)
	}
	_ = RunOnce(opts)
	return true
}

// Start spawns a detached subprocess to run the check/update and returns
// immediately; it never blocks and never touches stdout/stderr.
func Start(opts Options) {
	opts = normalize(opts)
	if opts.CommandName == "" || opts.RepoSubdir == "" {
		return
	}
	if os.Getenv(envWorker) == "1" {
		return
	}
	if disabled(opts.CommandName) {
		return
	}
	if _, loaded := startOnce.LoadOrStore(opts.CommandName, struct{}{}); loaded {
		return
	}
	// Spawn the worker when either the binary or the skill check is due; the
	// worker applies its own throttling per task.
	skillWanted := opts.SkillName != "" && !skillSyncDisabled() && skillSyncDue(opts)
	if !dueForCheck(opts) && !skillWanted {
		return
	}
	_ = spawnWorker(opts)
}

// RunOnce performs one synchronous check (used by the worker subprocess and
// tests); still writes nothing to stdout/stderr.
func RunOnce(opts Options) error {
	opts = normalize(opts)
	if disabled(opts.CommandName) {
		return errors.New(msgAutoUpdateDisabled)
	}
	return runOnce(opts)
}

func normalize(opts Options) Options {
	if opts.Branch == "" {
		opts.Branch = defaultBranch
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}
	return opts
}

func disabled(commandName string) bool {
	if truthy(os.Getenv("MC_CLI_NO_AUTO_UPDATE")) {
		return true
	}
	if truthy(os.Getenv("CI")) {
		return true
	}
	envKey := strings.ToUpper(strings.ReplaceAll(commandName, "-", "_")) + "_NO_AUTO_UPDATE"
	return truthy(os.Getenv(envKey))
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func dueForCheck(opts Options) bool {
	st, err := loadState(opts.CommandName)
	if err != nil || st.LastAttempt.IsZero() {
		return true
	}
	if successResult(st.LastResult) {
		return time.Since(st.LastCheck) >= opts.Interval
	}
	if st.LastResult == "skipped_not_writable" {
		return time.Since(st.LastAttempt) >= opts.Interval
	}
	return time.Since(st.LastAttempt) >= errorBackoff
}

// successResult reports whether a result counts as successful (throttled by
// Interval and advancing last_check); all other results (including error and
// skipped_manifest_mismatch) go through errorBackoff.
func successResult(r string) bool {
	switch r {
	case "up_to_date", "updated", "updated_no_manifest", "skipped_remote_older", "skipped_no_git_url":
		return true
	}
	return false
}

func runOnce(opts Options) (err error) {
	result := "error"
	skipReason := ""
	noManifest := false
	var localHash, remoteHash string
	attempt := time.Now().UTC()
	defer func() {
		msg := ""
		if err != nil {
			msg = err.Error()
		} else if skipReason != "" {
			msg = skipReason
		}
		st := state{
			LastAttempt: attempt,
			LastResult:  result,
			Error:       msg,
			LocalHash:   localHash,
			RemoteHash:  remoteHash,
		}
		if successResult(result) {
			st.LastCheck = attempt
		} else if prev, loadErr := loadState(opts.CommandName); loadErr == nil {
			st.LastCheck = prev.LastCheck
		}
		_ = saveState(opts.CommandName, st)
	}()

	// GitURL unset: skip instead of failing — a configuration error does not
	// heal itself, so it is throttled by the normal Interval instead of sharing
	// the error bucket with transient network failures (only those need the
	// short 1h backoff).
	if opts.GitURL == "" {
		result = "skipped_no_git_url"
		return nil
	}

	exe, err := lookupExecutable()
	if err != nil {
		return fmt.Errorf(msgLocateExeFailed, err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf(msgResolveExePathFailed, err)
	}
	_ = os.Remove(exe + ".old")

	if !dirWritable(filepath.Dir(exe)) {
		result = "skipped_not_writable"
		return errors.New(msgDirNotWritable)
	}

	localHash, err = fileSHA256(exe)
	if err != nil {
		return fmt.Errorf(msgLocalHashFailed, err)
	}

	artifactRel := artifactRelativePath(opts.RepoSubdir, opts.CommandName)
	tmpRoot, err := os.MkdirTemp("", "mc-cli-autoupdate-*")
	if err != nil {
		return fmt.Errorf(msgTempDirFailed, err)
	}
	defer os.RemoveAll(tmpRoot)

	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	manifestRel := filepath.ToSlash(filepath.Join(opts.RepoSubdir, "dist", "manifest.json"))
	fetched, err := runUpdateFetch(ctx, opts, artifactRel, manifestRel, tmpRoot)
	if err != nil {
		// Any failure to fetch the artifact or the manifest (network / timeout /
		// corrupted repository) fails closed: no replacement this round, error
		// returned, state error, errorBackoff applied.
		return err
	}
	remotePath := fetched.artifactPath
	remoteHash, err = fileSHA256(remotePath)
	if err != nil {
		return fmt.Errorf(msgRemoteHashFailed, err)
	}
	if localHash == remoteHash {
		result = "up_to_date"
		return nil
	}

	// Version guard (forward-only): read the remote dist manifest and refuse the
	// replacement when the remote version is NOT HIGHER than local. Only when
	// the manifest is deterministically absent on the remote (git show
	// explicitly reports the path does not exist — legacy projects that never
	// rebuilt dist) does it degrade to plain hash alignment, with the state
	// recorded as updated_no_manifest for observability; network failures,
	// timeouts and manifest parse failures always fail closed, preventing a
	// failed fetch from being treated as "no manifest" and overwriting a locally
	// built, higher version with a stale remote artifact.
	// MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE=1 forces plain-alignment semantics.
	if !truthy(os.Getenv(envAllowDowngrade)) {
		if fetched.manifestMissing {
			// Deterministically absent: degrade to plain alignment and allow the
			// replacement, but keep the state distinguishable.
			noManifest = true
		} else {
			var manifest distManifest
			if err := json.Unmarshal(fetched.manifestBytes, &manifest); err != nil {
				return fmt.Errorf(msgManifestParseFailed, err)
			}
			if want := manifest.Artifacts[filepath.Base(remotePath)]; want != "" && want != remoteHash {
				result = "skipped_manifest_mismatch"
				return fmt.Errorf(msgManifestHashMismatch, filepath.Base(remotePath))
			}
			if !shouldReplace(opts.LocalVersion, manifest.Version) {
				result = "skipped_remote_older"
				skipReason = fmt.Sprintf(msgRemoteVersionNotNewer,
					manifest.Version, opts.LocalVersion, envAllowDowngrade)
				return nil
			}
		}
	}

	if err := replaceExecutable(exe, remotePath); err != nil {
		return fmt.Errorf(msgReplaceExeFailed, err)
	}
	result = "updated"
	if noManifest {
		result = "updated_no_manifest"
	}
	return nil
}

func artifactRelativePath(repoSubdir, commandName string) string {
	name := fmt.Sprintf("%s-%s-%s", commandName, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.ToSlash(filepath.Join(repoSubdir, "dist", name))
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// dirWritable tests replaceability by creating a temp file in the same
// directory, avoiding any O_RDWR on a running Windows exe.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".autoupdate-writetest-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func statePath(commandName string) (string, error) {
	dir, err := resolveCacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, commandName+".json"), nil
}

func loadState(commandName string) (state, error) {
	path, err := statePath(commandName)
	if err != nil {
		return state{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state{}, err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return state{}, err
	}
	return st, nil
}

func saveState(commandName string, st state) error {
	path, err := statePath(commandName)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func spawnWorker(opts Options) error {
	exe, err := lookupExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Env = workerEnv(opts)
	detachProcess(cmd)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// workerEnv builds the worker subprocess environment (drops any stale worker
// marker from the current environment, then injects the configuration).
func workerEnv(opts Options) []string {
	return append(filterEnv(os.Environ(), envWorker),
		envWorker+"=1",
		envCmd+"="+opts.CommandName,
		envSubdir+"="+opts.RepoSubdir,
		envGitURL+"="+opts.GitURL,
		envBranch+"="+opts.Branch,
		envVersion+"="+opts.LocalVersion,
		envSkill+"="+opts.SkillName,
		envSkillMarker+"="+opts.SkillRepoMarker,
	)
}

func filterEnv(env []string, dropKey string) []string {
	prefix := dropKey + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}
