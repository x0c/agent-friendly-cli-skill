// Package fsutil provides file-system utilities shared by the CLIs in this
// repository: atomic write via same-directory temp file + rename, a
// cross-process file lock based on O_EXCL (critical-section serialization),
// and a single-instance lock with a PID liveness probe.
//
// Design boundaries (driven by real use cases from in-repo CLIs — a query
// budget guard and its log tail poller, a deploy command and its skill
// generator — no speculative generalization):
//   - Directory creation (MkdirAll and its permissions) always stays with the
//     caller: each call site has its own directory permission policy
//     (0700/0755) and its own failure wording, so this package only handles
//     file-level operations under the assumption that the directory exists;
//   - No unified default failure policy: WithFileLock only reports "whether
//     the lock was actually held" — after the lock turns out to be
//     unavailable, whether to proceed anyway (fail-open, the soft-guard
//     policy) or to refuse (fail-closed) is explicitly decided by the caller.
//     AcquireInstanceLock instead returns an error per the inherent
//     semantics of a single-instance lock (preemption is refused); callers
//     can recognize the error type and wrap it with their own wording;
//   - User-visible wording belongs to the caller: every failure is classified
//     by stage into *OpError (Op identifies the failing stage, Err is the
//     underlying error), and Error() passes the underlying error text
//     straight through, so a caller that wraps it with its own message or
//     surfaces it as-is never gets doubled-up wording.
package fsutil

import (
	"errors"
)

// Failure-stage identifiers for atomic write and lock operations (values of
// *OpError.Op). The same stage carries the same semantics across operations
// (e.g. OpWrite/OpClose); lock-specific stages carry a lock prefix.
const (
	// OpTempCreate atomic write: creating the same-directory temp file failed
	OpTempCreate = "temp_create"
	// OpChmod atomic write: setting the temp file permissions failed
	OpChmod = "chmod"
	// OpWrite writing file content failed (atomic-write temp file / lock-file token)
	OpWrite = "write"
	// OpClose closing the file failed
	OpClose = "close"
	// OpRename atomic write: renaming the temp file onto the target path failed
	OpRename = "rename"
	// OpLockCreate lock: O_CREATE|O_EXCL atomic creation of the lock file failed (not "already exists")
	OpLockCreate = "lock_create"
	// OpLockRead lock: reading the existing lock file content failed (not "no longer exists")
	OpLockRead = "lock_read"
	// OpLockRemove lock: reclaiming the stale lock file failed (not "no longer exists")
	OpLockRemove = "lock_remove"
)

// OpError is a file-system operation failure classified by stage: Op is the
// failing stage (see the constants above), Err is the underlying error.
// Error() passes the underlying error text straight through — a caller that
// wraps it with its own wording (fmt.Errorf "%w") or surfaces it as-is never
// gets doubled-up wording; use errors.As with Op to distinguish stages.
type OpError struct {
	Op  string // failing-stage identifier
	Err error  // underlying error
}

// Error passes the underlying error text through unchanged, keeping the
// caller's wording policy intact.
func (e *OpError) Error() string { return e.Err.Error() }

// Unwrap exposes the underlying error so errors.Is / errors.As can traverse it.
func (e *OpError) Unwrap() error { return e.Err }

// ErrLockAttemptsExhausted means single-instance-lock preemption retries ran
// out (backstop for stale-lock reclamation racing with concurrent creation):
// the text is the user-visible message, surfaced as-is by callers.
var ErrLockAttemptsExhausted = errors.New("lock acquisition retry attempts exhausted (stale-lock reclamation and concurrent creation race), please retry later")
