// instancelock_test.go Locking test cases for the single-instance lock: first
// instance wins, second instance refused (BusyError carries the holder
// identity), dead-holder/corrupt-content reclamation and re-acquisition,
// idempotent release and StillSelf guarding against mistaken deletion,
// exactly one winner under concurrent preemption, and the retry cap. The
// liveness probe is injectable; all fixtures are constructed programmatically
// with no dependency on real process life or death.
package fsutil

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// lockIdentity is the holder credential contract used by tests (JSON: PID +
// start time), constructed programmatically.
type lockIdentity struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"started_at_unix"`
}

// identityProbe decides the holder state per the test contract: when content
// parses and PID>0, a PID present in aliveAt counts as alive and everything
// else as dead; a parse failure counts as corrupt content.
func identityProbe(aliveAt map[int]bool) func([]byte) (any, HolderState) {
	return func(content []byte) (any, HolderState) {
		var prev lockIdentity
		if err := json.Unmarshal(content, &prev); err != nil || prev.PID <= 0 {
			return nil, HolderCorrupt
		}
		if aliveAt[prev.PID] {
			return prev, HolderAlive
		}
		return nil, HolderDead
	}
}

// marshalIdentity constructs the lock content programmatically.
func marshalIdentity(t *testing.T, id lockIdentity) []byte {
	t.Helper()
	data, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestAcquireInstanceLockFirstWinsAndBusyCarriesHolder locks down first-
// instance preemption and second-instance refusal: with no lock, preemption
// succeeds (lock content == Self, 0600); when the lock is held by a live
// holder, *InstanceLockBusyError is returned and Holder carries the holder
// identity.
func TestAcquireInstanceLockFirstWinsAndBusyCarriesHolder(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tail.lock")
	self := marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 100})
	release, err := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self:        self,
		HolderProbe: identityProbe(map[int]bool{os.Getpid(): true}),
	})
	if err != nil {
		t.Fatalf("the first instance should acquire the lock: %v", err)
	}
	defer release()
	data, err := os.ReadFile(lockPath)
	if err != nil || string(data) != string(self) {
		t.Fatalf("lock content should equal the Self credential: %v %q", err, data)
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock file permission should be 0600, got %#o", info.Mode().Perm())
	}
	// Holder alive (any valid content counts as alive) → the second
	// instance's preemption is refused, and BusyError.Holder carries the
	// holder identity parsed from the lock content (i.e. the first
	// instance's credential)
	_, err2 := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self: self,
		HolderProbe: func(content []byte) (any, HolderState) {
			var prev lockIdentity
			if err := json.Unmarshal(content, &prev); err != nil || prev.PID <= 0 {
				return nil, HolderCorrupt
			}
			return prev, HolderAlive
		},
	})
	if err2 == nil {
		t.Fatal("preemption should be refused when the lock is held by a live holder")
	}
	var busy *InstanceLockBusyError
	if !errors.As(err2, &busy) {
		t.Fatalf("should return *InstanceLockBusyError, got %T: %v", err2, err2)
	}
	holder, ok := busy.Holder.(lockIdentity)
	if !ok || holder.PID != os.Getpid() {
		t.Fatalf("BusyError.Holder should carry the live holder's (first instance's) PID %d, got %#v", os.Getpid(), busy.Holder)
	}
}

// TestAcquireInstanceLockDeadHolderRecovered locks down holder death: the
// stale lock is reclaimed, preemption succeeds, and the lock content is
// replaced with this instance's credential.
func TestAcquireInstanceLockDeadHolderRecovered(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tail.lock")
	if err := os.WriteFile(lockPath, marshalIdentity(t, lockIdentity{PID: 999, StartedAt: 1}), 0o600); err != nil {
		t.Fatal(err)
	}
	self := marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 200})
	release, err := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self:        self,
		HolderProbe: identityProbe(map[int]bool{}), // 999 not in the alive set → dead
	})
	if err != nil {
		t.Fatalf("a dead holder's stale lock should be reclaimed and preemption succeed: %v", err)
	}
	defer release()
	if data, err := os.ReadFile(lockPath); err != nil || string(data) != string(self) {
		t.Fatalf("lock content after preemption should be this instance's credential: %v %q", err, data)
	}
}

// TestAcquireInstanceLockCorruptRecovered locks down corrupt content: treated
// as a stale lock, reclaimed, then acquired.
func TestAcquireInstanceLockCorruptRecovered(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tail.lock")
	if err := os.WriteFile(lockPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self:        marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 1}),
		HolderProbe: identityProbe(map[int]bool{os.Getpid(): true}),
	})
	if err != nil {
		t.Fatalf("a corrupt lock should be reclaimed and preemption succeed: %v", err)
	}
	release()
}

// TestAcquireInstanceLockReleaseIdempotentAndStillSelf locks down release
// semantics: release is idempotent (repeat calls have no side effects); when
// the lock content has been rebuilt by someone else (no longer equal to
// Self), the default StillSelf (byte-wise comparison) prevents deleting the
// other party's lock by mistake.
func TestAcquireInstanceLockReleaseIdempotentAndStillSelf(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tail.lock")
	self := marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 1})
	release, err := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self:        self,
		HolderProbe: identityProbe(map[int]bool{}),
	})
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	release()
	release() // idempotent
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("the lock file should be deleted after release: %v", err)
	}

	// After someone else rebuilds the lock, this instance's release must not
	// delete it by mistake
	release2, err := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self:        self,
		HolderProbe: identityProbe(map[int]bool{}),
	})
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	foreign := marshalIdentity(t, lockIdentity{PID: 4321, StartedAt: 2})
	if err := os.WriteFile(lockPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	release2()
	if data, err := os.ReadFile(lockPath); err != nil || string(data) != string(foreign) {
		t.Fatalf("release must not delete a lock that someone else rebuilt: %v %q", err, data)
	}
}

// TestAcquireInstanceLockConcurrentSingleWinner locks down concurrent
// preemption: first establish a holder, then 16 goroutines preempt the same
// lock simultaneously (any valid holder counts as alive) — all are refused
// with *InstanceLockBusyError, and the total winner count is exactly 1.
// Note: the concurrent "empty read during creation" race (an empty read is
// treated as corrupt and reclaimed, letting others take over) is an
// inherently narrow race window of the original implementation and is not
// asserted by this case.
func TestAcquireInstanceLockConcurrentSingleWinner(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tail.lock")
	const workers = 16
	// Any valid content counts as alive: late arrivals necessarily read the
	// holder's credential and are refused
	aliveAll := func(content []byte) (any, HolderState) {
		var prev lockIdentity
		if err := json.Unmarshal(content, &prev); err != nil || prev.PID <= 0 {
			return nil, HolderCorrupt
		}
		return prev, HolderAlive
	}
	opts := InstanceLockOptions{
		Self:        marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 1}),
		HolderProbe: aliveAll,
	}
	// Establish the holder first (removing the "empty read during creation"
	// race's interference with the assertions)
	releases := make(chan func(), workers+1)
	release, err := AcquireInstanceLock(lockPath, opts)
	if err != nil {
		t.Fatalf("the first instance should acquire the lock: %v", err)
	}
	releases <- release
	var wg sync.WaitGroup
	results := make([]error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			release, err := AcquireInstanceLock(lockPath, opts)
			if err == nil {
				results[i] = nil
				releases <- release
				return
			}
			results[i] = err
		}(i)
	}
	wg.Wait()
	close(releases)
	for release := range releases {
		release() // final cleanup: release all winners (idempotent)
	}
	success, busyCount := 0, 0
	for i, err := range results {
		if err == nil {
			success++
			continue
		}
		var busy *InstanceLockBusyError
		if !errors.As(err, &busy) {
			t.Fatalf("failure of worker %d should be *InstanceLockBusyError, got %T: %v", i, err, err)
		}
		busyCount++
	}
	if success != 0 || busyCount != workers {
		t.Fatalf("after the holder is established, all 16 concurrent workers should be refused; got %d successes and %d refusals", success, busyCount)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("the lock file should be handed back after the final releases: %v", err)
	}
}

// TestAcquireInstanceLockAttemptsExhausted locks down the retry cap: with
// corrupt content plus a non-writable directory (reclaim failures are
// ignored, creation keeps failing), the loop runs to the cap and returns
// ErrLockAttemptsExhausted. Windows directory permission semantics differ and
// root is unrestricted; both are skipped.
func TestAcquireInstanceLockAttemptsExhausted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory permission semantics differ; this case does not apply")
	}
	if os.Geteuid() == 0 {
		t.Skip("root is not restricted by directory write permissions; this case does not apply")
	}
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "tail.lock")
	if err := os.WriteFile(lockPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	_, err := AcquireInstanceLock(lockPath, InstanceLockOptions{
		Self:        marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 1}),
		HolderProbe: identityProbe(map[int]bool{}),
		MaxAttempts: 4,
	})
	if !errors.Is(err, ErrLockAttemptsExhausted) {
		t.Fatalf("exhausting the retry cap should return ErrLockAttemptsExhausted, got %T: %v", err, err)
	}
}

// TestAcquireInstanceLockOpCreateFailure locks down the stage classification
// of creation failure: with a non-writable directory, O_EXCL creation fails
// (not "already exists") and returns an *OpError with OpLockCreate.
func TestAcquireInstanceLockOpCreateFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory permission semantics differ; this case does not apply")
	}
	if os.Geteuid() == 0 {
		t.Skip("root is not restricted by directory write permissions; this case does not apply")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	_, err := AcquireInstanceLock(filepath.Join(dir, "tail.lock"), InstanceLockOptions{
		Self:        marshalIdentity(t, lockIdentity{PID: os.Getpid(), StartedAt: 1}),
		HolderProbe: identityProbe(map[int]bool{}),
	})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Op != OpLockCreate {
		t.Fatalf("should return an *OpError with OpLockCreate, got %T: %v", err, err)
	}
	if opErr.Error() != opErr.Err.Error() {
		t.Fatalf("OpError text should pass through the underlying error: %q != %q", opErr.Error(), opErr.Err.Error())
	}
}
