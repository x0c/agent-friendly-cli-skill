// reactive.go implements the reactive relogin orchestration:
// authentication failure -> relogin once -> a single replay of the original
// request. The trigger predicate and the relogin closure are injected by
// the project; the orchestration itself is lock-free and
// transport-agnostic (all HTTP / WebSocket differences are encapsulated
// inside the request closure), and it is decoupled from the eager half
// (tokenrefresher.go).

package tokenrefresher

// ReactiveOptions holds the injected parameters of the reactive relogin
// orchestration; both are provided by the project.
type ReactiveOptions struct {
	// IsAuthFailure decides whether a request's error counts as an
	// "expired login session". Projects often need to fold "is relogin
	// possible" into the predicate — e.g. mytool must not trigger a
	// relogin when its tokenRefresher is nil; the caller composes that
	// into this predicate itself.
	// Production safety constraint: the predicate must only accept
	// *explicit* authentication failures (such as HTTP 401/403 or the
	// platform's expired-login envelope). Network errors or unknown
	// outcomes must never be treated as a hit, otherwise a write request
	// whose outcome is unknown would be blindly replayed.
	IsAuthFailure func(err error) bool
	// Relogin logs in again and makes the new credentials visible to
	// subsequent requests. The locking strategy is injected entirely by
	// the implementation (one project injects a lock-free implementation,
	// another injects one guarded by a mutex); the orchestration itself is
	// lock-free — it adds no lock and removes none.
	Relogin func() error
}

// OnAuthFailure performs "request -> authentication-failure check ->
// relogin once -> replay once". do must be a stateless request closure:
// every invocation rebuilds the request from scratch and carries the
// latest credentials at that moment (for transports with a connection
// lifecycle such as WebSocket, a replay means a full redial — that
// difference is encapsulated inside the closure). Semantics (must not be
// relaxed):
//   - First-attempt success, or the predicate not hitting -> return as is,
//     zero replays;
//   - Relogin failure -> return the relogin error as is, no replay;
//   - Replay still failing (whatever the error) -> return as is, never a
//     second retry (prevents an infinite loop on bad credentials; the
//     existing tests in every project lock this semantic down);
//   - Either injected function being nil -> treated as not enabled;
//     return the first error as is.
//
// The replay limit of 1 is hardcoded in this function — no retry-count
// parameter is offered, structurally ruling out any relaxation.
func OnAuthFailure(opts ReactiveOptions, do func() error) error {
	err := do()
	if err == nil || opts.IsAuthFailure == nil || opts.Relogin == nil || !opts.IsAuthFailure(err) {
		return err
	}
	if reloginErr := opts.Relogin(); reloginErr != nil {
		return reloginErr
	}
	return do()
}
