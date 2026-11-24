package session

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestRestoreJarMatchingOrigin(t *testing.T) {
	cookies := []*http.Cookie{{Name: "sid", Value: "v", Path: "/"}}
	jar, origin, err := RestoreJar("https://tsp.example.com/", "https://tsp.example.com", cookies)
	if err != nil {
		t.Fatal(err)
	}
	if origin.String() != "https://tsp.example.com" {
		t.Fatalf("parsed server address mismatch: %s", origin.String())
	}
	got := jar.Cookies(origin)
	if len(got) != 1 || got[0].Name != "sid" {
		t.Fatalf("cookies should be restored when CookieOrigin matches: %v", got)
	}
}

func TestRestoreJarMismatchedOrigin(t *testing.T) {
	cookies := []*http.Cookie{{Name: "sid", Value: "v", Path: "/"}}
	// The origin the cookies are bound to differs from the server address:
	// they must not be restored, to prevent cross-origin reuse.
	jar, otherOrigin, err := RestoreJar("https://other.example.com", "https://tsp.example.com", cookies)
	if err != nil {
		t.Fatal(err)
	}
	if got := jar.Cookies(otherOrigin); len(got) != 0 {
		t.Fatalf("cookies should not be restored when origins differ: %v", got)
	}
}

func TestEnsureValidSkipsLogin(t *testing.T) {
	loginCalled := false
	login := func(context.Context) error { loginCalled = true; return nil }
	if err := Ensure(context.Background(), func(context.Context) (bool, error) { return true, nil }, login); err != nil {
		t.Fatal(err)
	}
	if loginCalled {
		t.Fatal("login should be skipped when the session is valid")
	}
}

func TestEnsureInvalidTriggersLogin(t *testing.T) {
	loginCalled := false
	login := func(context.Context) error { loginCalled = true; return nil }
	if err := Ensure(context.Background(), func(context.Context) (bool, error) { return false, nil }, login); err != nil {
		t.Fatal(err)
	}
	if !loginCalled {
		t.Fatal("login should run when the session is invalid")
	}
}

func TestEnsurePropagatesErrors(t *testing.T) {
	sentinel := errors.New("validation failed")
	if err := Ensure(context.Background(), func(context.Context) (bool, error) { return false, sentinel }, nil); !errors.Is(err, sentinel) {
		t.Fatalf("validation errors should propagate: %v", err)
	}
	loginErr := errors.New("login failed")
	if err := Ensure(context.Background(), func(context.Context) (bool, error) { return false, nil }, func(context.Context) error { return loginErr }); !errors.Is(err, loginErr) {
		t.Fatalf("login errors should propagate: %v", err)
	}
}
