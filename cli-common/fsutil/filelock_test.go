// filelock_test.go Locking test cases for the cross-process file lock:
// concurrent mutual exclusion (at most one in the critical section at a
// time), giving up when the lock is held and there is no budget (fail-open is
// the caller's decision — this API only returns false), stale-lock breaking,
// and release handing the lock back. All fixtures are constructed
// programmatically.
package fsutil

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithFileLockMutualExclusion locks down concurrent mutual exclusion: 32
// goroutines each hold the lock to run the critical section (with a short
// pause to widen the race window), all of them acquire the lock successfully
// (queued within budget), and at no point are more than 1 inside the critical
// section at once (counter max concurrency == 1).
func TestWithFileLockMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "guard.lock")
	const workers = 32
	var inSection, maxInSection int64
	var entered int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			held := WithFileLock(lockPath, LockOptions{
				StaleAge:      10 * time.Second,
				WaitBudget:    5 * time.Second,
				RetryInterval: time.Millisecond,
			}, func() {
				cur := atomic.AddInt64(&inSection, 1)
				// Track max concurrency: under mutual exclusion it is always 1
				for {
					max := atomic.LoadInt64(&maxInSection)
					if cur <= max || atomic.CompareAndSwapInt64(&maxInSection, max, cur) {
						break
					}
				}
				time.Sleep(time.Millisecond) // lengthen the critical section to widen the race window
				atomic.AddInt64(&inSection, -1)
				atomic.AddInt64(&entered, 1)
			})
			if !held {
				t.Error("all workers should acquire the lock within budget")
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt64(&entered); got != workers {
		t.Fatalf("all %d workers should run the critical section, got %d", workers, got)
	}
	if got := atomic.LoadInt64(&maxInSection); got != 1 {
		t.Fatalf("max concurrency inside the critical section should be 1 under mutual exclusion, got %d", got)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("the lock file should be handed back after the critical section: %v", err)
	}
}

// TestWithFileLockContentionSingleWinner locks down the contention policy:
// when the lock is held by someone else (fresh, below the stale threshold)
// and there is no retry budget (WaitBudget=0), the second party fails to take
// the lock, returns false, does not run fn, and does not touch the other
// party's lock file — the failure policy (proceed or refuse) is the caller's
// decision; this API only faithfully returns false.
func TestWithFileLockContentionSingleWinner(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "guard.lock")
	const foreignToken = "someone-else-token"
	if err := os.WriteFile(lockPath, []byte(foreignToken), 0o600); err != nil {
		t.Fatal(err)
	}
	invoked := false
	held := WithFileLock(lockPath, LockOptions{
		StaleAge:   10 * time.Second,
		WaitBudget: 0, // no budget: give up as soon as the lock is held
	}, func() { invoked = true })
	if held {
		t.Fatal("should return false when the lock is held by someone else and there is no budget")
	}
	if invoked {
		t.Fatal("must not run the critical section without holding the lock")
	}
	if data, err := os.ReadFile(lockPath); err != nil || string(data) != foreignToken {
		t.Fatalf("must not touch the other party's lock file without holding the lock: %v %q", err, data)
	}
}

// TestWithFileLockStaleBroken locks down stale-lock breaking: when the lock
// file's mtime is past the threshold (residue of a crashed holder), the stale
// lock is removed and re-acquired, fn runs normally, and the new lock is
// released and handed back after the critical section.
func TestWithFileLockStaleBroken(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "guard.lock")
	if err := os.WriteFile(lockPath, []byte("crashed-holder"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-(10*time.Second + time.Second))
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	invoked := false
	held := WithFileLock(lockPath, LockOptions{
		StaleAge:   10 * time.Second,
		WaitBudget: 0, // breaking a stale lock does not consume budget: success even with no retry budget
	}, func() { invoked = true })
	if !held {
		t.Fatal("a stale lock should be broken and the lock acquired")
	}
	if !invoked {
		t.Fatal("the critical section should run after acquiring the lock")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("the lock file should be handed back after the critical section (the new lock, acquired after breaking the stale one, must also be released): %v", err)
	}
}

// TestWithFileLockFreshLockNotBrokenWithinStaleAge locks down the stale
// threshold boundary: a fresh lock (mtime below the threshold) is not broken;
// the budgetless party gives up and the lock file is left as-is.
func TestWithFileLockFreshLockNotBrokenWithinStaleAge(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "guard.lock")
	if err := os.WriteFile(lockPath, []byte("fresh-holder"), 0o600); err != nil {
		t.Fatal(err)
	}
	held := WithFileLock(lockPath, LockOptions{
		StaleAge:   time.Hour,
		WaitBudget: 0,
	}, func() {})
	if held {
		t.Fatal("a fresh lock (far below the threshold) must not be broken")
	}
	if data, err := os.ReadFile(lockPath); err != nil || string(data) != "fresh-holder" {
		t.Fatalf("a fresh lock must not be touched: %v %q", err, data)
	}
}
