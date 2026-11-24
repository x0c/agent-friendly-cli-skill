package autoupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// updateFetch is the result of one pull session: the artifact has been written
// to disk and the manifest comes from the same remote commit.
type updateFetch struct {
	artifactPath    string // on-disk path of the fetched artifact
	manifestBytes   []byte // dist/manifest.json content; invalid when manifestMissing=true
	manifestMissing bool   // git show explicitly reports the manifest path does not exist in the target commit (legacy projects that never rebuilt dist)
}

// errGitPathMissing means git show explicitly reports that the target path does
// not exist in the target commit (deterministically absent). Only this error
// lets the caller degrade to the no-manifest path; every other error (network,
// timeout, corrupted repository) fails closed.
var errGitPathMissing = errors.New(msgGitShowPathMissing)

// gitFetchUpdate maintains a bare shallow clone under the cache directory and,
// after a single fetch, exports both the artifact and the dist manifest from
// the same commit via `git show`, guaranteeing both come from one commit (two
// independent fetches could otherwise observe different versions). Most private
// GitLab deployments do not support blob:none, so partial clone is not assumed;
// the first clone may still be slow, later fetches are incremental.
func gitFetchUpdate(ctx context.Context, opts Options, artifactRel, manifestRel, destDir string) (updateFetch, error) {
	if opts.GitURL == "" {
		return updateFetch{}, errors.New(msgNoGitURL)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return updateFetch{}, fmt.Errorf(msgGitNotFound, err)
	}
	cacheRoot, err := resolveCacheDir()
	if err != nil {
		return updateFetch{}, err
	}
	bareDir := filepath.Join(cacheRoot, "repo.git")
	if err := ensureBareRepo(ctx, opts, bareDir); err != nil {
		return updateFetch{}, err
	}
	if err := gitRun(ctx, bareDir,
		"fetch", "--depth", "1", "origin", "+refs/heads/"+opts.Branch+":refs/heads/"+opts.Branch,
	); err != nil {
		return updateFetch{}, fmt.Errorf(msgGitFetchFailed, err)
	}

	outPath, err := gitShowToFile(ctx, bareDir, opts.Branch, artifactRel, destDir)
	if err != nil {
		return updateFetch{}, err
	}

	manifest, err := gitShowBytes(ctx, bareDir, opts.Branch, manifestRel)
	if err != nil {
		if errors.Is(err, errGitPathMissing) {
			// Deterministically absent (legacy projects that never rebuilt dist):
			// let the caller degrade to the no-manifest path, with a
			// distinguishable state.
			return updateFetch{artifactPath: outPath, manifestMissing: true}, nil
		}
		// Network interruption / timeout / corrupted repository: propagate the
		// error so the caller fails closed; a failed manifest fetch must never
		// be silently equated with "the remote really has no manifest".
		return updateFetch{}, err
	}
	return updateFetch{artifactPath: outPath, manifestBytes: manifest}, nil
}

// gitShowToFile exports a single artifact file from the bare repository to
// destDir via git show.
func gitShowToFile(ctx context.Context, bareDir, ref, artifactRel, destDir string) (string, error) {
	outPath := filepath.Join(destDir, filepath.Base(artifactRel))
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "show", ref+":"+artifactRel)
	cmd.Env = sanitizedGitEnv()
	cmd.Stdout = out
	cmd.Stderr = nil
	runErr := cmd.Run()
	_ = out.Close()
	if runErr != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf(msgGitShowFailed, ref, artifactRel, runErr)
	}
	return outPath, nil
}

// gitShowBytes reads a single file's content from the bare repository; when the
// target path does not exist in the target commit it returns an error wrapping
// errGitPathMissing, and every other error (timeout, corrupted repository, etc.)
// is returned as-is.
func gitShowBytes(ctx context.Context, bareDir, ref, rel string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "show", ref+":"+rel)
	cmd.Env = sanitizedGitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if gitStderrPathMissing(stderr.String()) {
			return nil, fmt.Errorf(msgGitShowFailed, ref, rel, errGitPathMissing)
		}
		return nil, fmt.Errorf(msgGitShowFailedWithStderr, ref, rel, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// gitStderrPathMissing decides whether git show's stderr is a deterministic
// "path does not exist" class of error. Actual git error forms (verified
// locally):
//   - fatal: path 'x' does not exist in 'master' (with tree-ish)
//   - fatal: Path 'x' does not exist (neither on disk nor in the index) (without tree-ish)
//
// Only errors of this class count as the manifest being deterministically
// absent; network and timeout errors happen during fetch and never reach this
// check.
func gitStderrPathMissing(stderr string) bool {
	return strings.Contains(stderr, "does not exist")
}

func ensureBareRepo(ctx context.Context, opts Options, bareDir string) error {
	if st, err := os.Stat(filepath.Join(bareDir, "HEAD")); err == nil && !st.IsDir() {
		if cachedOrigin(bareDir) == opts.GitURL {
			return nil
		}
		// The cached clone's origin does not match the current GitURL (a machine
		// may host CLIs pointing at different repositories): never keep fetching
		// from the stale origin — rebuild the whole cache to avoid pulling
		// artifacts from the wrong repository.
	}
	_ = os.RemoveAll(bareDir)
	parent := filepath.Dir(bareDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	// Bare shallow clone without checkout; single blobs are read later via show.
	return gitRun(ctx, "",
		"clone", "--bare", "--depth", "1", "--branch", opts.Branch, opts.GitURL, bareDir,
	)
}

// cachedOrigin returns the cached bare repository's origin URL, or an empty
// string when unavailable (which triggers a rebuild).
func cachedOrigin(bareDir string) string {
	cmd := exec.Command("git", "--git-dir="+bareDir, "remote", "get-url", "origin")
	cmd.Env = sanitizedGitEnv()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRun(ctx context.Context, gitDir string, args ...string) error {
	full := args
	if gitDir != "" {
		full = append([]string{"--git-dir=" + gitDir}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = sanitizedGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sanitizedGitEnv removes environment variables that could hijack the clone
// URL / SSH transport from the git subprocess's environment (e.g. insteadOf
// pointing at a malicious repository).
func sanitizedGitEnv() []string {
	dropPrefixes := []string{
		"GIT_CONFIG=",
		"GIT_CONFIG_GLOBAL=",
		"GIT_CONFIG_SYSTEM=",
		"GIT_CONFIG_COUNT=",
		"GIT_SSH=",
		"GIT_SSH_COMMAND=",
		"GIT_PROXY_COMMAND=",
		"GIT_HTTP_PROXY=",
		"GIT_HTTPS_PROXY=",
		"ALL_PROXY=",
		"HTTP_PROXY=",
		"HTTPS_PROXY=",
		"http_proxy=",
		"https_proxy=",
		"all_proxy=",
	}
	base := os.Environ()
	out := make([]string, 0, len(base)+4)
	for _, e := range base {
		skip := false
		for _, p := range dropPrefixes {
			if strings.HasPrefix(e, p) {
				skip = true
				break
			}
		}
		if strings.HasPrefix(e, "GIT_CONFIG_KEY_") || strings.HasPrefix(e, "GIT_CONFIG_VALUE_") {
			skip = true
		}
		if skip {
			continue
		}
		out = append(out, e)
	}
	out = append(out,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return out
}
