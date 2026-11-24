// Package tokenrefresher consolidates the shared skeleton for
// token-authenticated CLIs (several legacy CLIs among them): safe credential
// file reading, error sentinel mapping, expiry checking, and eager automatic
// re-login.
//
// Each project differs in its credential JSON structure, login endpoint and
// expiry calculation, so the credential type and the login closure are fully
// parameterized and injected; error wording (down to half-width vs full-width
// colon differences) and sentinel errors are also injected by the project,
// keeping the behavior locked by existing compat tests byte-for-byte stable.
//
// Design red lines:
//   - No locks: the eager re-login of the legacy CLIs was lock-free, and the
//     shared package preserves that lock-free semantics.
//   - Reactive 401/403 retry (the SetTokenRefresher approach of one legacy
//     CLI) is deeply coupled to the transport layer and used by that CLI
//     alone; it is out of this package's scope and stays in the project.
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

// ExpiryMargin is the lead margin (in seconds) for expiry checking: a token
// is treated as expired 60 seconds before it actually expires, so a request
// at the boundary never receives a token that is about to become invalid.
const ExpiryMargin int64 = 60

// File describes how the credential file is read, plus the injected error
// wording. Prefix texts must carry their own colon (one legacy CLI uses a
// half-width ": " while another uses a full-width "："); the shared package
// makes no concatenation decisions of its own — it extracts the underlying
// error via errors.As and reproduces each project's existing wording with %w
// semantics.
type File struct {
	Path         string // full path to the credential file
	ErrNeedLogin error  // sentinel for a missing file (wording differs per project)
	ErrUnsafe    error  // sentinel for unsafe permissions or symlink substitution
	ParsePrefix  string // prefix for JSON parse failure wording (colon included)
	CheckPrefix  string // prefix for file-check (lstat) failure wording (colon included)
	ReadPrefix   string // prefix for other read failures (colon included)
}

// LoadStored safely reads the credential file and maps errors to the
// injected wording; it does not check expiry. Reads go through the shared
// secretfile package, which rejects symlinks, non-regular files, and files
// whose permissions include group/other bits. A missing file maps to
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

// Stale reports whether the credential has expired (including the
// ExpiryMargin lead). expiresAt <= 0 means never expires and returns false.
// Project-specific conditions such as "an empty token requires re-login" are
// composed by the caller.
func Stale(expiresAt int64) bool {
	return expiresAt > 0 && time.Now().Unix() >= expiresAt-ExpiryMargin
}

// Session is the outcome of one successful anonymous login: the new token,
// its expiry, and project-specific fields to write back (e.g., the uid one
// legacy CLI carries).
type Session struct {
	Token     string
	ExpiresAt int64
	Extra     map[string]string
}

// Options holds the orchestration parameters for eager automatic re-login,
// all injected by the project.
type Options[C any] struct {
	Load           func() (*C, error)             // read the credential and check expiry (expired → NeedLogin sentinel)
	LoadForRelogin func() (*C, error)             // read the credential without the expiry check, and validate the fields required for re-login
	NeedLogin      error                          // the "re-login required" sentinel, matched with errors.Is
	Login          func(cred *C) (Session, error) // anonymous login using the account and password in the credential
	Apply          func(cred *C, sess Session)    // write the login outcome back into the credential (token/uid/expiry)
	Save           func(cred *C) error            // persist the updated credential
}

// Ensure runs the eager orchestration: with a valid credential it returns
// directly (relogged=false); with an expired one it reads the re-login
// credential, logs in anonymously, writes back and saves, then returns
// (relogged=true).
// The orchestration itself is lock-free; callers that need serialization
// add locks inside the injected functions.
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

// Relogin performs "anonymous login → write back → save" on the given
// credential, letting the eager orchestration and reactive refresh share the
// same login details.
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
