// Package tokenrefresher consolidates the shared skeleton for token-based
// CLIs: "safe credential-file read → error sentinel mapping → expiry check
// → eager auto-relogin".
//
// Each project has its own credential JSON shape, login endpoint, and
// expiry computation, so the credential type and the login closures are
// fully injected; error copy (including half-width vs full-width colon
// differences between adopters) and sentinel errors are injected per
// project as well, keeping the behavior locked by each project's compat
// tests byte-identical.
//
// Design red lines:
//   - No locks: both early adopters' eager relogin was lock-free, and the
//     common package preserves the lock-free semantics;
//   - The reactive half (auth failure → one relogin → one replay) lives in
//     reactive.go of this package: the predicate, the relogin closure, and
//     the lock strategy are all injected; the orchestration is lock-free
//     and transport-agnostic, with HTTP vs WebSocket differences
//     encapsulated inside the request closures.
package tokenrefresher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/secretfile"
)

// ExpiryMargin is the lead margin (in seconds) for the expiry check: a
// token is treated as expired 60 seconds before it actually expires, so a
// borderline request never receives a token about to lapse.
const ExpiryMargin int64 = 60

// File describes how a credential file is read and which error copy is
// injected. The prefix copy must carry its own colon (adopters differ: one
// uses a half-width ": " while another uses a full-width colon); the
// common package makes no concatenation decisions — it extracts the
// underlying error via errors.As and restores each project's existing copy
// with %w semantics.
type File struct {
	Path         string // full path to the credential file
	ErrNeedLogin error  // sentinel returned when the file does not exist (per-project copy)
	ErrUnsafe    error  // sentinel returned for unsafe permissions or symlink substitution
	ParsePrefix  string // copy prefix for JSON parse failures (colon included)
	CheckPrefix  string // copy prefix for the lstat check failure (colon included)
	ReadPrefix   string // copy prefix for any other read failure (colon included)
}

// LoadStored safely reads the credential file and maps errors to the
// injected copy; it does not judge expiry. Before reading, the common
// secretfile package validates the file: symlinks, non-regular files, and
// permissions with group/other bits are rejected. A missing file maps to
// ErrNeedLogin; every unsafe state maps uniformly to the injected ErrUnsafe
// sentinel (matching each project's existing semantics).
func LoadStored[C any](f File) (*C, error) {
	var c C
	if err := secretfile.Read(f.Path, &c, secretfile.OwnerOnly); err != nil {
		var unsafe *secretfile.UnsafeError
		if errors.As(err, &unsafe) {
			return nil, f.ErrUnsafe
		}
		if os.IsNotExist(err) {
			return nil, f.ErrNeedLogin
		}
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) {
			return nil, fmt.Errorf("%s%w", f.ParsePrefix, syntaxErr)
		}
		if errors.As(err, &typeErr) {
			return nil, fmt.Errorf("%s%w", f.ParsePrefix, typeErr)
		}
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) && pathErr.Op == "lstat" {
			return nil, fmt.Errorf("%s%w", f.CheckPrefix, pathErr)
		}
		return nil, fmt.Errorf("%s%w", f.ReadPrefix, err)
	}
	return &c, nil
}

// Stale reports whether the credential has expired (with the ExpiryMargin
// lead). expiresAt <= 0 means never expires and returns false.
// Project-specific conditions such as "empty token means relogin needed"
// are composed by the caller.
func Stale(expiresAt int64) bool {
	return expiresAt > 0 && time.Now().Unix() >= expiresAt-ExpiryMargin
}

// Session is the outcome of one successful anonymous login: the new token,
// its expiry, and project-specific backfill fields (e.g. an
// adopter-specific uid).
type Session struct {
	Token     string
	ExpiresAt int64
	Extra     map[string]string
}

// Options are the orchestration parameters for eager auto-relogin, all
// injected by the adopting project.
type Options[C any] struct {
	Load           func() (*C, error)             // read the credential and judge expiry (expiry returns the NeedLogin sentinel)
	LoadForRelogin func() (*C, error)             // read the credential without the expiry check, validating fields required for relogin
	NeedLogin      error                          // the "relogin needed" sentinel, matched via errors.Is
	Login          func(cred *C) (Session, error) // anonymous login with the account/password from the credential
	Apply          func(cred *C, sess Session)    // write the login outcome back into the credential (token/uid/expiry)
	Save           func(cred *C) error            // persist the backfilled credential
}

// Ensure runs the eager orchestration: a still-valid credential returns
// immediately (relogged=false); an expired one is re-read, logged in
// anonymously, backfilled, and saved (relogged=true).
// The orchestration itself is lock-free; callers that need serialization
// add locks inside their injected functions.
func Ensure[C any](opts Options[C]) (cred *C, relogged bool, err error) {
	cred, err = opts.Load()
	if err == nil {
		return cred, false, nil
	}
	if !errors.Is(err, opts.NeedLogin) {
		return nil, false, err
	}
	cred, err = opts.LoadForRelogin()
	if err != nil {
		return nil, false, err
	}
	if _, err = Relogin(opts, cred); err != nil {
		return nil, false, err
	}
	return cred, true, nil
}

// Relogin performs "anonymous login → backfill → save" for the given
// credential; the eager orchestration and the reactive refresh share this
// single login detail.
func Relogin[C any](opts Options[C], cred *C) (Session, error) {
	sess, err := opts.Login(cred)
	if err != nil {
		return Session{}, err
	}
	opts.Apply(cred, sess)
	if err := opts.Save(cred); err != nil {
		return Session{}, err
	}
	return sess, nil
}
