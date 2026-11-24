package tokenrefresher

import (
	"errors"
	"testing"
)

// Sentinels dedicated to the reactive-orchestration tests: they simulate
// the three error kinds — expired login session, unknown network outcome,
// and relogin failure.
var (
	reactiveErrAuth    = errors.New("login session expired")
	reactiveErrNetwork = errors.New("network outcome unknown")
	reactiveErrRelogin = errors.New("relogin failed")
)

// isAuthLike is the minimal authentication-failure predicate: it accepts
// only the explicit authentication sentinel; the unknown-network sentinel
// never hits.
func isAuthLike(err error) bool { return err == reactiveErrAuth }

func TestOnAuthFailureFirstSuccessZeroReplay(t *testing.T) {
	doCalls, reloginCalls := 0, 0
	err := OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
		Relogin:       func() error { reloginCalls++; return nil },
	}, func() error {
		doCalls++
		return nil
	})
	if err != nil {
		t.Fatalf("first-attempt success should return nil, got: %v", err)
	}
	if doCalls != 1 || reloginCalls != 0 {
		t.Fatalf("first-attempt success must not relogin or replay: %d requests, %d relogins", doCalls, reloginCalls)
	}
}

func TestOnAuthFailureReloginOnceReplaySucceeds(t *testing.T) {
	doCalls, reloginCalls := 0, 0
	err := OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
		Relogin:       func() error { reloginCalls++; return nil },
	}, func() error {
		doCalls++
		if doCalls == 1 {
			return reactiveErrAuth
		}
		return nil
	})
	if err != nil {
		t.Fatalf("successful replay should return nil, got: %v", err)
	}
	if doCalls != 2 || reloginCalls != 1 {
		t.Fatalf("must relogin exactly once and replay exactly once: %d requests, %d relogins", doCalls, reloginCalls)
	}
}

func TestOnAuthFailureReloginFailureReturnsWithoutReplay(t *testing.T) {
	doCalls, reloginCalls := 0, 0
	err := OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
		Relogin:       func() error { reloginCalls++; return reactiveErrRelogin },
	}, func() error {
		doCalls++
		return reactiveErrAuth
	})
	if !errors.Is(err, reactiveErrRelogin) {
		t.Fatalf("relogin failure should return the relogin error as is, got: %v", err)
	}
	if doCalls != 1 || reloginCalls != 1 {
		t.Fatalf("no replay after relogin failure: %d requests, %d relogins", doCalls, reloginCalls)
	}
}

func TestOnAuthFailureNoSecondRetryAfterFailedReplay(t *testing.T) {
	doCalls, reloginCalls := 0, 0
	err := OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
		Relogin:       func() error { reloginCalls++; return nil },
	}, func() error {
		doCalls++
		return reactiveErrAuth
	})
	if !errors.Is(err, reactiveErrAuth) {
		t.Fatalf("a replay that still fails authentication should return that error as is, got: %v", err)
	}
	if doCalls != 2 || reloginCalls != 1 {
		t.Fatalf("no second retry after the single replay: %d requests, %d relogins", doCalls, reloginCalls)
	}
}

func TestOnAuthFailureNetworkUnknownNoReplay(t *testing.T) {
	doCalls, reloginCalls := 0, 0
	err := OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
		Relogin:       func() error { reloginCalls++; return nil },
	}, func() error {
		doCalls++
		return reactiveErrNetwork
	})
	if !errors.Is(err, reactiveErrNetwork) {
		t.Fatalf("unknown network outcome should be returned as is, got: %v", err)
	}
	if doCalls != 1 || reloginCalls != 0 {
		t.Fatalf("unknown network outcome must not refresh or replay: %d requests, %d relogins", doCalls, reloginCalls)
	}
}

func TestOnAuthFailurePredicateMissNonAuthErrorZeroReplay(t *testing.T) {
	doCalls, reloginCalls := 0, 0
	businessErr := errors.New("business error")
	err := OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
		Relogin:       func() error { reloginCalls++; return nil },
	}, func() error {
		doCalls++
		return businessErr
	})
	if !errors.Is(err, businessErr) {
		t.Fatalf("a non-authentication error should be returned as is, got: %v", err)
	}
	if doCalls != 1 || reloginCalls != 0 {
		t.Fatalf("a non-authentication error must not be replayed: %d requests, %d relogins", doCalls, reloginCalls)
	}
}

func TestOnAuthFailureMissingInjectionPassthrough(t *testing.T) {
	// IsAuthFailure nil: treated as not enabled; the first error is
	// returned as is.
	doCalls, reloginCalls := 0, 0
	err := OnAuthFailure(ReactiveOptions{
		Relogin: func() error { reloginCalls++; return nil },
	}, func() error {
		doCalls++
		return reactiveErrAuth
	})
	if !errors.Is(err, reactiveErrAuth) {
		t.Fatalf("missing predicate should pass the first error through as is, got: %v", err)
	}
	if doCalls != 1 || reloginCalls != 0 {
		t.Fatalf("missing predicate must not relogin or replay: %d requests, %d relogins", doCalls, reloginCalls)
	}

	// Relogin nil: likewise treated as not enabled.
	doCalls, reloginCalls = 0, 0
	err = OnAuthFailure(ReactiveOptions{
		IsAuthFailure: isAuthLike,
	}, func() error {
		doCalls++
		return reactiveErrAuth
	})
	if !errors.Is(err, reactiveErrAuth) {
		t.Fatalf("missing relogin should pass the first error through as is, got: %v", err)
	}
	if doCalls != 1 || reloginCalls != 0 {
		t.Fatalf("missing relogin must not relogin or replay: %d requests, %d relogins", doCalls, reloginCalls)
	}
}
