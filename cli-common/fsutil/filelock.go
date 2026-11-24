// filelock.go Cross-process file lock: O_CREATE|O_EXCL atomically creates a
// lock file to serialize a critical section, suitable for mutual exclusion on
// shared state files that are read, decided on, and written back (e.g. budget
// acquisition by a query guard).
package fsutil

import (
	"fmt"
	"os"
	"time"
)

// LockOptions holds the parameters of the cross-process file lock.
type LockOptions struct {
	// StaleAge is the stale-lock threshold: when the lock file's mtime is
	// older than this, the lock is treated as the residue of a crashed
	// holder — remove it and take over; <= 0 means never break a stale lock
	// (any existing lock can only wait for its holder to release it).
	StaleAge time.Duration
	// WaitBudget is the total backoff-retry budget for lock acquisition
	// (the stale-lock breaking branch is excluded — breaking and
	// re-acquiring does not consume budget); <= 0 means a single attempt:
	// if the lock is held and not stale, give up immediately (a stale lock
	// can still be broken).
	WaitBudget time.Duration
	// RetryInterval is the per-retry backoff interval after a failed
	// acquisition; <= 0 defaults to 2ms (fail fast, yield fast — critical
	// sections are millisecond-scale, so under contention the budget is not
	// slept away).
	RetryInterval time.Duration
	// MaxAttempts caps the total number of iterations (including the
	// stale-lock breaking branch), preventing a pathological
	// "break-reacquire-go-stale-again" loop from spinning forever;
	// <= 0 defaults to 1024.
	MaxAttempts int
}

// WithFileLock serializes the critical section fn behind an O_CREATE|O_EXCL
// lock file (0600), returning whether fn actually ran while holding the lock.
// The lock content is a "PID-nanosecond timestamp" token: goroutines within
// the same process never overlap either; before release, the lock content is
// verified to still be our own token, guarding against deleting the lock of
// the new holder that took over after our lock was broken as stale.
//
// The failure policy must be explicitly chosen by the caller (this function
// has no default of its own): returning false means the lock was not held and
// fn was not executed — a soft-guard scenario (fail-open, e.g. a query
// guard) proceeds on this basis without reading or writing the shared state;
// scenarios that require mutual exclusion (fail-closed) refuse. Lock
// unavailability includes: retry budget exhausted, deterministic failure of
// the directory or permissions, and the token being unwritable into the lock
// file (without the token the lock cannot be released safely, so it is handed
// back and treated as not held).
func WithFileLock(lockPath string, opts LockOptions, fn func()) bool {
	retry := opts.RetryInterval
	if retry <= 0 {
		retry = 2 * time.Millisecond
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1024
	}
	// Token = PID + nanosecond timestamp: the release check uses it to tell
	// whether the lock is still ours.
	token := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	deadline := time.Now().Add(opts.WaitBudget)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, werr := f.WriteString(token); werr != nil {
				// Without the token the lock cannot be released safely —
				// do not hold it: hand it back and treat as not held.
				f.Close()
				os.Remove(lockPath)
				return false
			}
			f.Close()
			defer func() {
				// Release check: delete only if the lock content is still
				// our own token. If a stale-lock breaker took over, the
				// content differs — never touch another's lock.
				if data, rerr := os.ReadFile(lockPath); rerr == nil && string(data) == token {
					os.Remove(lockPath)
				}
			}()
			fn()
			return true
		}
		if !os.IsExist(err) {
			return false // deterministic failure (permissions etc.): retrying is pointless
		}
		// Lock held: a stale lock (mtime past the threshold, residue of a
		// crashed holder) may be broken — retry immediately.
		if opts.StaleAge > 0 {
			if info, serr := os.Stat(lockPath); serr == nil && time.Since(info.ModTime()) > opts.StaleAge {
				os.Remove(lockPath)
				continue
			}
		}
		if !time.Now().Before(deadline) {
			return false // budget exhausted
		}
		time.Sleep(retry)
	}
	return false
}
