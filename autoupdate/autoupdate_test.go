package autoupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func withTestCache(t *testing.T) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	resolveCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() {
		resolveCacheDir = func() (string, error) {
			cache, err := os.UserCacheDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(cache, "mc-cli-autoupdate"), nil
		}
	})
}

// stubUpdateFetch swaps the fetch session test point and restores the real
// implementation after the test.
func stubUpdateFetch(t *testing.T, fn updateSession) {
	t.Helper()
	old := runUpdateFetch
	runUpdateFetch = fn
	t.Cleanup(func() { runUpdateFetch = old })
}

func TestDisabledEnv(t *testing.T) {
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "1")
	if !disabled("mytool") {
		t.Fatal("expected MC_CLI_NO_AUTO_UPDATE=1 to disable")
	}
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "true")
	if !disabled("mytool") {
		t.Fatal("expected CI=true to disable")
	}
	t.Setenv("CI", "")
	t.Setenv("MYTOOL_NO_AUTO_UPDATE", "yes")
	if !disabled("mytool") {
		t.Fatal("expected MYTOOL_NO_AUTO_UPDATE to disable")
	}
}

func TestArtifactRelativePath(t *testing.T) {
	got := artifactRelativePath("mytool-cli", "mytool")
	wantSuffix := "mytool-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		wantSuffix += ".exe"
	}
	if !strings.HasPrefix(got, "mytool-cli/dist/") || !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("got %q, want mytool-cli/dist/%s", got, wantSuffix)
	}
}

func TestRunOnceUpToDate(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	content := []byte("same-binary-content")
	if err := os.WriteFile(exe, content, 0o755); err != nil {
		t.Fatal(err)
	}

	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		p := filepath.Join(destDir, "remote")
		if err := os.WriteFile(p, content, 0o755); err != nil {
			return updateFetch{}, err
		}
		return updateFetch{artifactPath: p, manifestMissing: true}, nil
	})

	err := RunOnce(Options{CommandName: "testcli", RepoSubdir: "test-cli", GitURL: "https://example.com/repo.git", Interval: time.Second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, err := loadState("testcli")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != "up_to_date" {
		t.Fatalf("result=%s error=%s", st.LastResult, st.Error)
	}
	if st.LastCheck.IsZero() {
		t.Fatal("last_check should be written on success")
	}
}

// Deterministically absent (git show explicitly reports the manifest path does
// not exist — legacy projects that never rebuilt dist): degrade to plain hash
// alignment and allow the replacement, but the state must stay distinguishable
// (updated_no_manifest).
func TestRunOnceUpdatedNoManifest(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv(envAllowDowngrade, "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	newContent := []byte("new-binary-content")
	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		p := filepath.Join(destDir, "remote")
		if err := os.WriteFile(p, newContent, 0o755); err != nil {
			return updateFetch{}, err
		}
		return updateFetch{artifactPath: p, manifestMissing: true}, nil
	})

	err := RunOnce(Options{CommandName: "testcli2", RepoSubdir: "test-cli", GitURL: "https://example.com/repo.git", Interval: time.Second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newContent) {
		t.Fatalf("deterministically absent manifest should degrade to plain alignment and replace, got %q", got)
	}
	st, err := loadState("testcli2")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != "updated_no_manifest" {
		t.Fatalf("result=%s error=%s, want updated_no_manifest", st.LastResult, st.Error)
	}
	if st.LastCheck.IsZero() {
		t.Fatal("updated_no_manifest counts as success; last_check should be written")
	}
}

func TestErrorDoesNotAdvanceLastCheck(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	cmd := "errcli"
	prev := time.Now().UTC().Add(-48 * time.Hour)
	if err := saveState(cmd, state{LastCheck: prev, LastAttempt: prev, LastResult: "up_to_date"}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		return updateFetch{}, context.DeadlineExceeded
	})

	_ = RunOnce(Options{CommandName: cmd, RepoSubdir: "x", GitURL: "https://example.com/repo.git"})
	st, err := loadState(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != "error" {
		t.Fatalf("result=%s", st.LastResult)
	}
	if !st.LastCheck.Equal(prev) {
		t.Fatalf("failure must not advance last_check: got %v want %v", st.LastCheck, prev)
	}
	if st.LastAttempt.Before(prev) {
		t.Fatal("last_attempt should be updated")
	}
}

// Reproduces the case where the artifact fetch succeeds but the manifest read
// times out, and verifies the fix. Before the fix: the manifest failure was
// swallowed as nil, the run degraded to no-manifest and replaced anyway, so a
// locally newer build was overwritten by a stale remote artifact and recorded
// as updated. After the fix: the whole session fails, fail-closed — no
// replacement, state error, errorBackoff.
func TestRunOnceManifestFetchFailureFailsClosed(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv(envAllowDowngrade, "")
	cmd := "audit-cli"
	prev := time.Now().UTC().Add(-48 * time.Hour)
	if err := saveState(cmd, state{LastCheck: prev, LastAttempt: prev, LastResult: "up_to_date"}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "audit-cli")
	if err := os.WriteFile(exe, []byte("local new build v0.2.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		// The artifact is written successfully, but the manifest read times
		// out: the whole session is treated as failed.
		p := filepath.Join(destDir, filepath.Base(artifactRel))
		if err := os.WriteFile(p, []byte("remote old build v0.1.0"), 0o755); err != nil {
			return updateFetch{}, err
		}
		return updateFetch{}, fmt.Errorf("git show %s failed: %w", manifestRel, context.DeadlineExceeded)
	})

	err := RunOnce(Options{CommandName: cmd, RepoSubdir: "audit-cli", GitURL: "https://example.com/repo.git", LocalVersion: "v0.2.0"})
	if err == nil {
		t.Fatal("manifest fetch failure should return an error (fail-closed)")
	}
	got, rerr := os.ReadFile(exe)
	if rerr != nil || string(got) != "local new build v0.2.0" {
		t.Fatalf("local binary must not be replaced when the manifest fetch fails: %v %q", rerr, got)
	}
	st, serr := loadState(cmd)
	if serr != nil {
		t.Fatal(serr)
	}
	if st.LastResult != "error" {
		t.Fatalf("LastResult = %q, want error", st.LastResult)
	}
	if !st.LastCheck.Equal(prev) {
		t.Fatalf("failure must not advance last_check: got %v want %v", st.LastCheck, prev)
	}
	// errorBackoff applies: no check due within 1h.
	if dueForCheck(Options{CommandName: cmd, Interval: 24 * time.Hour}) {
		t.Fatal("not due within 1h after a manifest fetch failure")
	}
}

// A corrupted manifest (JSON parse failure) also fails closed: no replacement,
// state error.
func TestRunOnceManifestParseFailureFailsClosed(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv(envAllowDowngrade, "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	if err := os.WriteFile(exe, []byte("local-newer"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		p := filepath.Join(destDir, filepath.Base(artifactRel))
		if err := os.WriteFile(p, []byte("remote-older"), 0o755); err != nil {
			return updateFetch{}, err
		}
		return updateFetch{artifactPath: p, manifestBytes: []byte("{broken-json")}, nil
	})

	err := RunOnce(Options{CommandName: "parsefail-cli", RepoSubdir: "x", GitURL: "https://example.com/repo.git", LocalVersion: "v0.2.0"})
	if err == nil {
		t.Fatal("manifest parse failure should return an error (fail-closed)")
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "local-newer" {
		t.Fatalf("no replacement on manifest parse failure, got %q", got)
	}
	st, _ := loadState("parsefail-cli")
	if st.LastResult != "error" {
		t.Fatalf("LastResult = %q, want error", st.LastResult)
	}
}

func TestDueForCheckErrorBackoff(t *testing.T) {
	withTestCache(t)
	cmd := "backoff-cli"
	if err := saveState(cmd, state{
		LastAttempt: time.Now().UTC(),
		LastResult:  "error",
		Error:       "network",
	}); err != nil {
		t.Fatal(err)
	}
	if dueForCheck(Options{CommandName: cmd, Interval: 24 * time.Hour}) {
		t.Fatal("not due within 1h after an error")
	}
}

// updated_no_manifest counts as success: throttled by Interval (not
// errorBackoff) and advances last_check.
func TestDueForCheckUpdatedNoManifest(t *testing.T) {
	withTestCache(t)
	cmd := "nomanifest-cli"
	if err := saveState(cmd, state{
		LastCheck:   time.Now().UTC(),
		LastAttempt: time.Now().UTC(),
		LastResult:  "updated_no_manifest",
	}); err != nil {
		t.Fatal(err)
	}
	if dueForCheck(Options{CommandName: cmd, Interval: 24 * time.Hour}) {
		t.Fatal("not due within Interval after updated_no_manifest")
	}
	if !successResult("updated_no_manifest") {
		t.Fatal("updated_no_manifest should count as a success result")
	}
}

func TestStartDisabled(t *testing.T) {
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "1")
	Start(Options{CommandName: "disabled-cli-" + t.Name(), RepoSubdir: "x", Interval: time.Nanosecond})
}

func TestMaybeWorker(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv(envWorker, "1")
	t.Setenv(envCmd, "workercli")
	t.Setenv(envSubdir, "worker-cli")
	t.Setenv(envGitURL, "https://example.com/repo.git")
	t.Cleanup(func() {
		_ = os.Unsetenv(envWorker)
		_ = os.Unsetenv(envCmd)
		_ = os.Unsetenv(envSubdir)
		_ = os.Unsetenv(envGitURL)
	})

	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	content := []byte("worker-bin")
	if err := os.WriteFile(exe, content, 0o755); err != nil {
		t.Fatal(err)
	}
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		p := filepath.Join(destDir, "remote")
		if err := os.WriteFile(p, content, 0o755); err != nil {
			return updateFetch{}, err
		}
		return updateFetch{artifactPath: p, manifestMissing: true}, nil
	})

	if !MaybeWorker() {
		t.Fatal("worker environment should return true")
	}
	st, err := loadState("workercli")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != "up_to_date" {
		t.Fatalf("result=%s err=%s", st.LastResult, st.Error)
	}
}

func TestSanitizedGitEnvDropsProxyAndSSH(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "evil")
	t.Setenv("HTTPS_PROXY", "http://evil")
	t.Setenv("GIT_CONFIG_GLOBAL", "/tmp/evil")
	env := sanitizedGitEnv()
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") || strings.HasPrefix(e, "HTTPS_PROXY=") {
			t.Fatalf("should have been dropped: %s", e)
		}
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") {
		t.Fatal("missing GIT_TERMINAL_PROMPT=0")
	}
	if !strings.Contains(joined, "GIT_CONFIG_GLOBAL=/dev/null") {
		t.Fatal("missing GIT_CONFIG_GLOBAL=/dev/null")
	}
}

// Classification of git show "path does not exist" stderr: only errors of this
// class count as the manifest being deterministically absent.
func TestGitStderrPathMissing(t *testing.T) {
	cases := []struct {
		stderr string
		want   bool
	}{
		{"fatal: path 'mytool-cli/dist/manifest.json' does not exist in 'master'", true},
		{"fatal: Path 'x' does not exist (neither on disk nor in the index)", true},
		{"fatal: unable to access 'https://example.com/repo.git': Failed to connect", false},
		{"error: did not receive expected object", false},
		{"fatal: invalid object name 'master'", false},
		{"", false},
	}
	for _, c := range cases {
		if got := gitStderrPathMissing(c.stderr); got != c.want {
			t.Fatalf("gitStderrPathMissing(%q) = %v, want %v", c.stderr, got, c.want)
		}
	}
}

// GitURL unset: gitFetchUpdate returns a self-explanatory error immediately,
// without running any git operation.
func TestGitFetchUpdateRequiresGitURL(t *testing.T) {
	withTestCache(t)
	_, err := gitFetchUpdate(context.Background(),
		Options{CommandName: "testcli", RepoSubdir: "test-cli"},
		"test-cli/dist/tool-"+runtime.GOOS+"-"+runtime.GOARCH,
		"test-cli/dist/manifest.json", t.TempDir())
	if err == nil {
		t.Fatal("an empty GitURL should be an error")
	}
	if !strings.Contains(err.Error(), "GitURL") {
		t.Fatalf("error should be self-explanatory (mention GitURL): %v", err)
	}
}

// A real local git repository (cloned via file path, no network, no
// credentials) exercises gitFetchUpdate: the missing-manifest detection goes
// through real git show stderr; the artifact and the manifest are read from the
// same commit.
func TestGitFetchUpdateLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available locally, skipping")
	}
	withTestCache(t)

	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(filepath.Join(src, "test-cli", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	artifactRel := "test-cli/dist/tool-linux-amd64"
	if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(artifactRel)), []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = src
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, out)
		}
	}
	git("init", ".")
	// Make sure the target branch is master (default branch names differ
	// across git versions).
	if out, err := exec.Command("git", "-C", src, "checkout", "-b", "master").CombinedOutput(); err != nil && !strings.Contains(string(out), "already exists") {
		t.Fatalf("failed to create the master branch: %v (%s)", err, out)
	}
	git("config", "user.email", "t@t.local")
	git("config", "user.name", "t")
	git("add", ".")
	git("commit", "-m", "init")

	opts := Options{CommandName: "testcli", RepoSubdir: "test-cli", GitURL: src, Branch: "master"}

	// Scenario 1: manifest missing → deterministically absent
	// (manifestMissing), artifact written normally.
	dest := t.TempDir()
	f, err := gitFetchUpdate(context.Background(), opts, artifactRel, "test-cli/dist/manifest.json", dest)
	if err != nil {
		t.Fatalf("missing manifest should degrade and succeed: %v", err)
	}
	if !f.manifestMissing {
		t.Fatal("manifestMissing should be set when the manifest does not exist")
	}
	got, err := os.ReadFile(f.artifactPath)
	if err != nil || string(got) != "v1" {
		t.Fatalf("artifact content mismatch: %v %q", err, got)
	}

	// Scenario 2: after committing a manifest → the bare repository is reused
	// with an incremental fetch, reading the manifest from the same commit.
	manifestRel := "test-cli/dist/manifest.json"
	m := distManifest{
		Command:   "testcli",
		Version:   "v0.1.0-1-gabc1234",
		Artifacts: map[string]string{"tool-linux-amd64": sha256Hex(t, "v1")},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(manifestRel)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "add manifest")

	dest2 := t.TempDir()
	f2, err := gitFetchUpdate(context.Background(), opts, artifactRel, manifestRel, dest2)
	if err != nil {
		t.Fatalf("fetch with manifest failed: %v", err)
	}
	if f2.manifestMissing {
		t.Fatal("manifestMissing must not be set when the manifest exists")
	}
	var parsed distManifest
	if err := json.Unmarshal(f2.manifestBytes, &parsed); err != nil {
		t.Fatalf("corrupted manifest content: %v", err)
	}
	if parsed.Version != m.Version {
		t.Fatalf("manifest version mismatch: got %s want %s", parsed.Version, m.Version)
	}
	got2, err := os.ReadFile(f2.artifactPath)
	if err != nil || string(got2) != "v1" {
		t.Fatalf("artifact content mismatch: %v %q", err, got2)
	}
}

// ---- Version guard (forward-only) ----

func TestParseDescribe(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want describedVersion
	}{
		{"v0.1.0-55-g12fa79d", true, describedVersion{0, 1, 0, 55}},
		{"0.2.3-1-gabc", true, describedVersion{0, 2, 3, 1}},
		{"v1.0.0", true, describedVersion{1, 0, 0, 0}},
		{"dev", false, describedVersion{}},
		{"0.1.0-dev", false, describedVersion{}},
		{"12fa79d", false, describedVersion{}},
		{"v0.1.0-abc-g12f", false, describedVersion{}},
	}
	for _, c := range cases {
		got, ok := parseDescribe(c.in)
		if ok != c.ok {
			t.Fatalf("parseDescribe(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
		if ok && got != c.want {
			t.Fatalf("parseDescribe(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestShouldReplace(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		{"v0.1.0-40-g7bee563", "v0.1.0-55-g12fa79d", true},  // remote higher → replace
		{"v0.1.0-55-g12fa79d", "v0.1.0-40-g7bee563", false}, // remote lower → skip
		{"v0.1.0-55-g12fa79d", "v0.1.0-55-g12fa79d", false}, // same version, different hash → no replacement
		{"v0.1.0", "v0.1.0-3-gabc", true},                   // commits after the tag → replace
		{"v0.1.0-3-gabc", "v0.1.0", false},                  // remote sits on the tag → skip
		{"v0.1.0-99-gbbb", "v0.2.0", true},                  // new tag → replace
		{"v0.2.0-1-gaaa", "v0.1.0-99-gbbb", false},          // remote tag lower → skip
		{"dev", "v0.1.0-40-g7bee563", true},                 // local version unknown → align
		{"v0.1.0-55-g12fa79d", "dev", true},                 // remote version unknown → align
	}
	for _, c := range cases {
		if got := shouldReplace(c.local, c.remote); got != c.want {
			t.Fatalf("shouldReplace(%q, %q) = %v, want %v", c.local, c.remote, got, c.want)
		}
	}
}

// runOnceGuardFixture builds a fake fetch session with a manifest: the artifact
// content is remoteBody and the manifest version is manifestVersion. The
// manifest JSON is constructed programmatically (json.Marshal); hand-writing
// full literals is forbidden.
func runOnceGuardFixture(t *testing.T, remoteBody, manifestVersion, artifactSha string) {
	t.Helper()
	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		p := filepath.Join(destDir, filepath.Base(artifactRel))
		if err := os.WriteFile(p, []byte(remoteBody), 0o755); err != nil {
			return updateFetch{}, err
		}
		m := distManifest{Command: opts.CommandName, Version: manifestVersion,
			BuildCommit: "abc1234", Head: "abc1234",
			Artifacts: map[string]string{filepath.Base(artifactRelativePath(opts.RepoSubdir, opts.CommandName)): artifactSha}}
		data, err := json.Marshal(m)
		if err != nil {
			return updateFetch{}, err
		}
		return updateFetch{artifactPath: p, manifestBytes: data}, nil
	})
}

func TestRunOnceSkipsRemoteOlder(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE", "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "mytool")
	// local = v0.1.0-55 (higher), remote dist = v0.1.0-40 (lower)
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })
	_ = os.WriteFile(exe, []byte("local-newer"), 0o755)

	remoteBody := "remote-older"
	sha := sha256Hex(t, remoteBody)
	runOnceGuardFixture(t, remoteBody, "v0.1.0-40-g7bee563", sha)

	if err := RunOnce(Options{CommandName: "mytool", RepoSubdir: "mytool-cli", GitURL: "https://example.com/repo.git", LocalVersion: "v0.1.0-55-g12fa79d"}); err != nil {
		t.Fatalf("RunOnce should not fail: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "local-newer" {
		t.Fatalf("must not replace the local binary when the remote version is not higher than local, got %q", got)
	}
	st, _ := loadState("mytool")
	if st.LastResult != "skipped_remote_older" {
		t.Fatalf("LastResult = %q, want skipped_remote_older", st.LastResult)
	}
}

func TestRunOnceUpdatesWhenRemoteNewer(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE", "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "mytool")
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })
	_ = os.WriteFile(exe, []byte("local-older"), 0o755)

	remoteBody := "remote-newer"
	sha := sha256Hex(t, remoteBody)
	runOnceGuardFixture(t, remoteBody, "v0.1.0-55-g12fa79d", sha)

	if err := RunOnce(Options{CommandName: "mytool", RepoSubdir: "mytool-cli", GitURL: "https://example.com/repo.git", LocalVersion: "v0.1.0-40-g7bee563"}); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "remote-newer" {
		t.Fatalf("should replace when the remote version is higher than local, got %q", got)
	}
	st, _ := loadState("mytool")
	if st.LastResult != "updated" {
		t.Fatalf("LastResult = %q, want updated", st.LastResult)
	}
}

func TestRunOnceAllowDowngradeEnvForcesAlign(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE", "1")
	dir := t.TempDir()
	exe := filepath.Join(dir, "mytool")
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })
	_ = os.WriteFile(exe, []byte("local-newer"), 0o755)

	remoteBody := "remote-older"
	sha := sha256Hex(t, remoteBody)
	runOnceGuardFixture(t, remoteBody, "v0.1.0-40-g7bee563", sha)

	if err := RunOnce(Options{CommandName: "mytool", RepoSubdir: "mytool-cli", GitURL: "https://example.com/repo.git", LocalVersion: "v0.1.0-55-g12fa79d"}); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "remote-older" {
		t.Fatalf("ALLOW_DOWNGRADE=1 should align with the remote artifact, got %q", got)
	}
}

func TestRunOnceManifestHashMismatchSkips(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv("MC_CLI_AUTO_UPDATE_ALLOW_DOWNGRADE", "")
	dir := t.TempDir()
	exe := filepath.Join(dir, "mytool")
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })
	_ = os.WriteFile(exe, []byte("local"), 0o755)

	remoteBody := "remote-tampered"
	runOnceGuardFixture(t, remoteBody, "v0.1.0-99-gdeadbee", "0000000000000000000000000000000000000000000000000000000000000000")

	err := RunOnce(Options{CommandName: "mytool", RepoSubdir: "mytool-cli", GitURL: "https://example.com/repo.git", LocalVersion: "v0.1.0-40-g7bee563"})
	if err == nil {
		t.Fatal("manifest hash mismatch should be an error")
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "local" {
		t.Fatalf("no replacement on manifest hash mismatch, got %q", got)
	}
	st, _ := loadState("mytool")
	if st.LastResult != "skipped_manifest_mismatch" {
		t.Fatalf("LastResult = %q, want skipped_manifest_mismatch", st.LastResult)
	}
}

func sha256Hex(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// GitURL unset: RunOnce records skipped_no_git_url (not error) and throttles by
// Interval instead of the 1h backoff.
func TestRunOnceEmptyGitURLSkipsWithDedicatedState(t *testing.T) {
	withTestCache(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	if err := RunOnce(Options{CommandName: "nogiturl-cli", RepoSubdir: "x", Interval: 24 * time.Hour}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st, err := loadState("nogiturl-cli")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != "skipped_no_git_url" {
		t.Fatalf("result=%s error=%s, want skipped_no_git_url", st.LastResult, st.Error)
	}
	if st.LastCheck.IsZero() {
		t.Fatal("skipped_no_git_url should advance last_check")
	}
	if dueForCheck(Options{CommandName: "nogiturl-cli", Interval: 24 * time.Hour}) {
		t.Fatal("not due within Interval")
	}
}

// Read-side round trip of MaybeWorker rebuilding Options from environment
// variables: GitURL and the skill marker must reach the fetch session and the
// eligibility check intact (the write side is covered by
// TestWorkerEnvPassesGitURLAndMarker; a broken read side compiles fine and
// gives no production signal, so a test must lock it down).
func TestMaybeWorkerEnvRoundTrip(t *testing.T) {
	withTestCache(t)
	home := withTestHome(t)
	t.Setenv("MC_CLI_NO_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	t.Setenv(envWorker, "1")
	t.Setenv(envCmd, "rtcli")
	t.Setenv(envSubdir, "rt-cli")
	t.Setenv(envGitURL, "ssh://git@gitlab.example.com/acme/cli.git")
	t.Setenv(envBranch, "master")
	t.Setenv(envVersion, "v0.1.0")
	t.Setenv(envSkill, "myskill")
	t.Setenv(envSkillMarker, "acme/skills")
	t.Cleanup(func() {
		for _, k := range []string{envWorker, envCmd, envSubdir, envGitURL, envBranch, envVersion, envSkill, envSkillMarker} {
			_ = os.Unsetenv(k)
		}
	})
	// A lock entry whose source lacks the marker substring: after the marker
	// round-trips through env, the sync should be rejected.
	writeSkillLock(t, home, "myskill", "https://github.com/someone/other-skills.git", "")

	var gotGitURL string
	stubUpdateFetch(t, func(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
		gotGitURL = opts.GitURL
		p := filepath.Join(destDir, "remote")
		if err := os.WriteFile(p, []byte("rt-bin"), 0o755); err != nil {
			return updateFetch{}, err
		}
		return updateFetch{artifactPath: p, manifestMissing: true}, nil
	})
	dir := t.TempDir()
	exe := filepath.Join(dir, "tool")
	_ = os.WriteFile(exe, []byte("rt-bin"), 0o755)
	lookupExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { lookupExecutable = os.Executable })

	if !MaybeWorker() {
		t.Fatal("worker environment should return true")
	}
	if gotGitURL != "ssh://git@gitlab.example.com/acme/cli.git" {
		t.Fatalf("fetch session received GitURL=%q", gotGitURL)
	}
	st, err := loadState("rtcli")
	if err != nil {
		t.Fatal(err)
	}
	if st.LastResult != "up_to_date" {
		t.Fatalf("result=%s err=%s", st.LastResult, st.Error)
	}
	skillSt, err := loadState("rtcli.skill")
	if err != nil {
		t.Fatal(err)
	}
	if skillSt.LastResult != "skill_skipped_source_mismatch" {
		t.Fatalf("skill result=%s, want skill_skipped_source_mismatch", skillSt.LastResult)
	}
}

// Cache isolation: when two CLIs with different GitURLs share a cache
// directory, the cached repository must be rebuilt per GitURL and must not keep
// the previous repository's origin (preventing artifacts being pulled from the
// wrong repository).
func TestGitFetchUpdateRebuildsCacheOnOriginMismatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available locally, skipping")
	}
	withTestCache(t)

	makeRepo := func(content string) string {
		src := filepath.Join(t.TempDir(), "src")
		if err := os.MkdirAll(filepath.Join(src, "test-cli", "dist"), 0o755); err != nil {
			t.Fatal(err)
		}
		rel := "test-cli/dist/tool-linux-amd64"
		if err := os.WriteFile(filepath.Join(src, filepath.FromSlash(rel)), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
		git := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = src
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v failed: %v (%s)", args, err, out)
			}
		}
		git("init", ".")
		if out, err := exec.Command("git", "-C", src, "checkout", "-b", "master").CombinedOutput(); err != nil && !strings.Contains(string(out), "already exists") {
			t.Fatalf("failed to create the master branch: %v (%s)", err, out)
		}
		git("config", "user.email", "t@t.local")
		git("config", "user.name", "t")
		git("add", ".")
		git("commit", "-m", "init")
		return src
	}

	repoA := makeRepo("from-A")
	repoB := makeRepo("from-B")

	f1, err := gitFetchUpdate(context.Background(),
		Options{CommandName: "cachecli", RepoSubdir: "test-cli", GitURL: repoA, Branch: "master"},
		"test-cli/dist/tool-linux-amd64", "test-cli/dist/manifest.json", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(f1.artifactPath)
	if string(got) != "from-A" {
		t.Fatalf("first fetch should come from A: %q", got)
	}

	f2, err := gitFetchUpdate(context.Background(),
		Options{CommandName: "cachecli", RepoSubdir: "test-cli", GitURL: repoB, Branch: "master"},
		"test-cli/dist/tool-linux-amd64", "test-cli/dist/manifest.json", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(f2.artifactPath)
	if string(got2) != "from-B" {
		t.Fatalf("after the GitURL changes, the cache should be rebuilt and fetch from B: %q", got2)
	}
}
