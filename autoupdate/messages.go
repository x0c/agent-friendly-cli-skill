// All runtime messages of this package are declared here: changing wording means
// touching only this file; localization scenarios only change this file's contents.
package autoupdate

var (
	// Binary self-update (autoupdate.go / replace_*.go)
	msgAutoUpdateDisabled    = "self-update is disabled"
	msgLocateExeFailed       = "failed to locate the executable: %w"
	msgResolveExePathFailed  = "failed to resolve the executable path: %w"
	msgDirNotWritable        = "the executable's directory is not writable; skipping self-update"
	msgLocalHashFailed       = "failed to hash the local executable: %w"
	msgTempDirFailed         = "failed to create a temp directory: %w"
	msgRemoteHashFailed      = "failed to hash the fetched artifact: %w"
	msgManifestParseFailed   = "failed to parse the dist manifest: %w"
	msgManifestHashMismatch  = "remote artifact hash does not match the dist manifest (%s); skipping replacement"
	msgRemoteVersionNotNewer = "remote dist version %s is not higher than local %s; skipping replacement (set %s=1 to force plain alignment)"
	msgReplaceExeFailed      = "failed to replace the executable: %w"
	msgTempFileFailed        = "failed to create a temp file: %w"
	msgMoveOldExeFailed      = "failed to move the old executable out of the way: %w"

	// Artifact / manifest fetching (fetch.go)
	msgGitShowPathMissing      = "git show reports the target path does not exist"
	msgNoGitURL                = "self-update source GitURL is not configured: pass the artifact repository address explicitly in autoupdate.Options; skipping this self-update"
	msgGitNotFound             = "git not found; cannot self-update: %w"
	msgGitFetchFailed          = "git fetch failed: %w"
	msgGitShowFailed           = "git show %s:%s failed: %w"
	msgGitShowFailedWithStderr = "git show %s:%s failed: %w (%s)"

	// Skill sync (skillsync.go)
	msgNpxNotFound         = "npx not found (Node.js is required)"
	msgNpxUpdateFailed     = "npx skills update failed: %v; output tail: %s"
	msgNpxNoop             = "npx made no changes; output tail: "
	msgSkillLockPathFailed = "failed to resolve the skills lock file path: %w"
	msgReadSkillLockFailed = "failed to read the skills lock file: %w"
)
