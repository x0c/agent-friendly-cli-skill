// instancelock.go Single-instance lock (fail-closed): only one holder is
// allowed per resource at a time; a second instance's preemption must be
// refused (e.g. the single-poller guard of a log tail command). Unlike the
// critical-section semantics of WithFileLock — where an unavailable lock
// means wait or proceed — the single-instance lock refuses to start when it
// cannot acquire, hence this API returns an error rather than a bool.
package fsutil

import (
	"bytes"
	"fmt"
	"os"
)

// HolderState is the verdict on an existing lock holder, returned by
// HolderProbe, which decides how the preemptor handles the lock file.
type HolderState int

const (
	// HolderAlive means the holder is alive: the lock cannot be preempted,
	// return *InstanceLockBusyError (Holder is the holder identity produced
	// by the probe callback, for the caller to build its error message).
	HolderAlive HolderState = iota
	// HolderDead means the holder is dead: reclaim the stale lock and retry
	// preemption; a reclamation failure is reported as OpLockRemove (the
	// holder is confirmed dead, so failing to clean up is an anomaly that
	// must surface).
	HolderDead
	// HolderCorrupt means the lock content is corrupt: treat it as a stale
	// lock, reclaim and retry; ignore reclamation failure (the content is
	// already untrustworthy — best-effort cleanup followed by retrying
	// creation is the recovery path).
	HolderCorrupt
)

// InstanceLockOptions holds the parameters of the single-instance lock.
type InstanceLockOptions struct {
	// Self is this instance's holder credential written into the lock file
	// (a caller-defined content contract, e.g. a JSON-serialized PID + start
	// time). Required.
	Self []byte
	// HolderProbe decides the state of the existing lock holder: content is
	// the lock file's current content; return (holder identity, state).
	// Required.
	HolderProbe func(content []byte) (holder any, state HolderState)
	// StillSelf verifies before release that the lock file still belongs to
	// this instance: the lock is deleted only when it returns true (guarding
	// against deleting a lock newly created by another preemptor); return
	// true when the content cannot be parsed (anomalous residue is reclaimed
	// too). Defaults to "content byte-equal to Self".
	StillSelf func(content []byte) bool
	// MaxAttempts caps preemption retries (backstop for stale-lock
	// reclamation racing with concurrent creation); <= 0 defaults to 8.
	MaxAttempts int
}

// InstanceLockBusyError means the lock is held by a live holder (preemption
// refused). Holder is the holder identity returned by HolderProbe; callers
// build their own error message from it (e.g. extracting the PID).
type InstanceLockBusyError struct {
	Holder any
}

// Error returns an internal diagnostic text; the user-visible message is
// constructed by the caller from Holder.
func (e *InstanceLockBusyError) Error() string {
	return fmt.Sprintf("lock is held by a live holder (fail-closed: preemption refused), holder identity: %v", e.Holder)
}

// AcquireInstanceLock preempts the single-instance lock at lockPath: atomic
// O_CREATE|O_EXCL creation (0600) with opts.Self as the content; when held,
// the content is read and handed to opts.HolderProbe — alive →
// *InstanceLockBusyError, dead/corrupt → reclaim the stale lock and retry.
// On success it returns a release function (idempotent; before deleting it
// verifies via opts.StillSelf that the lock still belongs to this instance,
// guarding against deleting a lock rebuilt by another; a lock file that no
// longer exists is treated as already released).
//
// Failure policy: underlying failures of create/write/close/read/reclaim are
// classified as *OpError (see the Op constants) with the underlying error
// text passed through, for the caller to wrap with its own wording; when the
// retry cap is exhausted it returns ErrLockAttemptsExhausted. Directory
// creation is the caller's responsibility (see the package comment).
func AcquireInstanceLock(lockPath string, opts InstanceLockOptions) (release func(), err error) {
	stillSelf := opts.StillSelf
	if stillSelf == nil {
		stillSelf = func(content []byte) bool { return bytes.Equal(content, opts.Self) }
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		f, crErr := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if crErr == nil {
			_, wErr := f.Write(opts.Self)
			clErr := f.Close()
			if wErr != nil {
				_ = os.Remove(lockPath) // don't leave a half-written lock for the next instance
				return nil, &OpError{Op: OpWrite, Err: wErr}
			}
			if clErr != nil {
				_ = os.Remove(lockPath)
				return nil, &OpError{Op: OpClose, Err: clErr}
			}
			var released bool
			return func() {
				if released {
					return
				}
				released = true
				content, rerr := os.ReadFile(lockPath)
				if rerr != nil {
					return // lock no longer exists (cleaned up abnormally) — treat as released
				}
				if !stillSelf(content) {
					return // the lock was rebuilt by someone else — leave it alone
				}
				_ = os.Remove(lockPath)
			}, nil
		}
		if !os.IsExist(crErr) {
			return nil, &OpError{Op: OpLockCreate, Err: crErr}
		}
		// Lock held: read the content and decide whether the holder is alive
		raw, rErr := os.ReadFile(lockPath)
		if rErr != nil {
			if os.IsNotExist(rErr) {
				continue // race: the holder just released — retry creation directly
			}
			return nil, &OpError{Op: OpLockRead, Err: rErr}
		}
		holder, state := opts.HolderProbe(raw)
		switch state {
		case HolderAlive:
			return nil, &InstanceLockBusyError{Holder: holder}
		case HolderDead:
			// Holder dead: reclaim the stale lock, then retry preemption
			if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return nil, &OpError{Op: OpLockRemove, Err: rmErr}
			}
		default:
			// Corrupt content is treated as a stale lock: reclaim and retry
			// (reclaim failure ignored, see the HolderCorrupt comment)
			_ = os.Remove(lockPath)
		}
	}
	return nil, ErrLockAttemptsExhausted
}
