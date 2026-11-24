package tokenrefresher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/secretfile"
)

// testCred is the test credential struct (mimicking the on-disk credentials
// of the legacy CLIs).
type testCred struct {
	BaseURL   string `json:"base_url"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Token     string `json:"token"`
	UID       string `json:"uid"`
	ExpiresAt int64  `json:"expires_at"`
}

var (
	errNeedLogin = errors.New("login required: no local credential found or it has expired")
	errUnsafe    = errors.New("credential file permissions are unsafe")
)

// writeFile writes a 0600 credential file.
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testFile(path string) File {
	// Half-width colon prefixes (one legacy CLI's style).
	return File{
		Path:         path,
		ErrNeedLogin: errNeedLogin,
		ErrUnsafe:    errUnsafe,
		ParsePrefix:  "failed to parse the credential file: ",
		CheckPrefix:  "failed to check the credential file: ",
		ReadPrefix:   "failed to read the credential file: ",
	}
}

func TestLoadStored_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	writeFile(t, path, `{"base_url":"https://yapi.example.com","token":"t"}`)
	cred, err := LoadStored[testCred](testFile(path))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if cred.BaseURL != "https://yapi.example.com" || cred.Token != "t" {
		t.Fatalf("wrong fields: %+v", cred)
	}
}

func TestLoadStored_NotExist(t *testing.T) {
	_, err := LoadStored[testCred](testFile(filepath.Join(t.TempDir(), "cookie.json")))
	if !errors.Is(err, errNeedLogin) {
		t.Fatalf("a missing file should map to ErrNeedLogin: %v", err)
	}
}

func TestLoadStored_Unsafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	writeFile(t, path, `{}`)
	// Loosen permissions to 0644: secretfile must reject the file.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadStored[testCred](testFile(path))
	if !errors.Is(err, errUnsafe) {
		t.Fatalf("unsafe permissions should map to ErrUnsafe: %v", err)
	}
}

func TestLoadStored_SyntaxError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	writeFile(t, path, `{broken`)
	_, err := LoadStored[testCred](testFile(path))
	if err == nil {
		t.Fatal("a JSON syntax error went unreported")
	}
	if err, ok := err.(error); !ok || len(err.Error()) == 0 {
		t.Fatal("error text is empty")
	}
	// The wording prefix (colon included) must be preserved.
	if got := err.Error(); got[:len("failed to parse the credential file:")] != "failed to parse the credential file:" {
		t.Fatalf("parse-failure wording prefix lost: %q", got)
	}
}

func TestLoadStored_FullWidthPrefix(t *testing.T) {
	// Injection of a full-width-colon prefix (another legacy CLI's style).
	path := filepath.Join(t.TempDir(), "credential.json")
	writeFile(t, path, `{broken`)
	f := testFile(path)
	f.ParsePrefix = "failed to parse the credential file："
	_, err := LoadStored[testCred](f)
	if err == nil {
		t.Fatal("no error reported")
	}
	if got := err.Error(); got[:len("failed to parse the credential file：")] != "failed to parse the credential file：" {
		t.Fatalf("full-width-colon prefix lost: %q", got)
	}
}

func TestStale(t *testing.T) {
	now := time.Now().Unix()
	cases := []struct {
		expiresAt int64
		want      bool
	}{
		{0, false},        // no expiry set
		{-1, false},       // negative value
		{now + 61, false}, // beyond the margin
		{now + 59, true},  // within the 60-second margin
		{now - 10, true},  // already expired
	}
	for _, c := range cases {
		if got := Stale(c.expiresAt); got != c.want {
			t.Errorf("Stale(%d) = %v, want %v", c.expiresAt, got, c.want)
		}
	}
}

// makeOpts builds an Options with a full set of injected closures for the
// Ensure/Relogin tests.
func makeOpts(path string, state *map[string]int, loginErr error) Options[testCred] {
	return Options[testCred]{
		Load: func() (*testCred, error) {
			cred, err := LoadStored[testCred](testFile(path))
			if err != nil {
				return nil, err
			}
			if cred.Token == "" || Stale(cred.ExpiresAt) {
				return nil, errNeedLogin
			}
			return cred, nil
		},
		LoadForRelogin: func() (*testCred, error) {
			cred, err := LoadStored[testCred](testFile(path))
			if err != nil {
				return nil, err
			}
			if cred.BaseURL == "" || cred.Email == "" || cred.Password == "" {
				return nil, errNeedLogin
			}
			return cred, nil
		},
		NeedLogin: errNeedLogin,
		Login: func(cred *testCred) (Session, error) {
			if loginErr != nil {
				return Session{}, loginErr
			}
			(*state)["login"]++
			return Session{Token: "new-token", ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix(), Extra: map[string]string{"uid": "42"}}, nil
		},
		Apply: func(cred *testCred, sess Session) {
			cred.Token = sess.Token
			cred.ExpiresAt = sess.ExpiresAt
			if uid, ok := sess.Extra["uid"]; ok {
				cred.UID = uid
			}
		},
		Save: func(cred *testCred) error {
			(*state)["save"]++
			return secretfile.Write(path, cred, secretfile.OwnerOnly)
		},
	}
}

func TestEnsure_FreshCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	writeFile(t, path, fmt.Sprintf(`{"base_url":"u","email":"e","password":"p","token":"t","expires_at":%d}`, time.Now().Add(time.Hour).Unix()))
	state := map[string]int{}
	cred, relogged, err := Ensure(makeOpts(path, &state, nil))
	if err != nil || relogged {
		t.Fatalf("a valid credential must not trigger re-login: %v relogged=%v", err, relogged)
	}
	if cred.Token != "t" {
		t.Fatalf("credential was unexpectedly modified: %+v", cred)
	}
	if state["login"] != 0 {
		t.Fatalf("a valid credential must not trigger login")
	}
}

func TestEnsure_ReexpiredRelogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	// The token has already expired (within the margin).
	writeFile(t, path, fmt.Sprintf(`{"base_url":"u","email":"e","password":"p","token":"old","expires_at":%d}`, time.Now().Unix()))
	state := map[string]int{}
	cred, relogged, err := Ensure(makeOpts(path, &state, nil))
	if err != nil {
		t.Fatalf("automatic re-login failed: %v", err)
	}
	if !relogged {
		t.Fatal("an expired credential must trigger re-login")
	}
	if cred.Token != "new-token" || cred.UID != "42" {
		t.Fatalf("write-back incorrect: %+v", cred)
	}
	// Verify the saved file on disk.
	saved, err := LoadStored[testCred](testFile(path))
	if err != nil || saved.Token != "new-token" {
		t.Fatalf("credential not persisted correctly: %v %+v", err, saved)
	}
	if state["login"] != 1 || state["save"] != 1 {
		t.Fatalf("wrong login/save counts: %v", state)
	}
}

func TestEnsure_OtherLoadErrorPassthrough(t *testing.T) {
	// Load returns an error other than NeedLogin: pass it through, no re-login.
	opts := Options[testCred]{
		Load:           func() (*testCred, error) { return nil, errors.New("some other error") },
		LoadForRelogin: func() (*testCred, error) { return &testCred{}, nil },
		NeedLogin:      errNeedLogin,
		Login:          func(*testCred) (Session, error) { t.Fatal("must not log in"); return Session{}, nil },
		Apply:          func(*testCred, Session) {},
		Save:           func(*testCred) error { return nil },
	}
	_, _, err := Ensure(opts)
	if err == nil || err.Error() != "some other error" {
		t.Fatalf("non-NeedLogin errors must pass through: %v", err)
	}
}

func TestEnsure_LoginFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	writeFile(t, path, `{"base_url":"u","email":"e","password":"p","token":"","expires_at":0}`)
	loginErr := errors.New("login failed: wrong password")
	_, _, err := Ensure(makeOpts(path, &map[string]int{}, loginErr))
	if err == nil || !errors.Is(err, loginErr) {
		t.Fatalf("a login failure must pass through: %v", err)
	}
}

func TestRelogin_Standalone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookie.json")
	writeFile(t, path, `{"base_url":"u","email":"e","password":"p"}`)
	state := map[string]int{}
	cred, err := LoadStored[testCred](testFile(path))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := Relogin(makeOpts(path, &state, nil), cred)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Token != "new-token" || state["save"] != 1 {
		t.Fatalf("wrong Relogin result: %+v %v", sess, state)
	}
}
