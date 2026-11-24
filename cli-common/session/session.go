// Package session provides the shared orchestration for cookie-session
// clients: restoring the cookie jar only when the CookieOrigin matches the
// server address (preventing cross-origin reuse) and the "validate, else
// log in" session-ensuring flow. Building the login request, judging success
// and detecting session expiry are platform details left to each CLI.
package session

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// RestoreJar creates a cookie jar and restores the saved cookies only when
// CookieOrigin matches the server address, preventing cookies from being
// reused across origins. It returns the jar and the parsed server address.
func RestoreJar(server, cookieOrigin string, cookies []*http.Cookie) (*cookiejar.Jar, *url.URL, error) {
	origin, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil {
		return nil, nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, fmt.Errorf(msgCreateCookieJarFailed, err)
	}
	if cookieOrigin == origin.Scheme+"://"+origin.Host {
		jar.SetCookies(origin, cookies)
	}
	return jar, origin, nil
}

// Ensure makes sure a valid session exists: if validation passes it returns
// immediately, otherwise it logs in once. valid and login are injected by each
// CLI (platform details).
func Ensure(ctx context.Context, valid func(context.Context) (bool, error), login func(context.Context) error) error {
	ok, err := valid(ctx)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return login(ctx)
}
