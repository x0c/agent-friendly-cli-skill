// The login command demonstrates wiring rest + session + prompt for login.
// The login endpoint in this template is a placeholder: when adapting to a
// real platform, replace loginEndpoint and implement the login-success check
// (cookie name / redirect / response-body signature); the rest of the
// skeleton stays unchanged.
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/rest"
	csession "github.com/x0c/agent-friendly-cli-skill/cli-common/session"
)

// newLoginCommand logs in to the template platform with the saved username
// and password and persists the session cookies. When the session expires,
// every command re-logs in automatically via session.Ensure (see the doctor
// command wiring).
func newLoginCommand() *cobra.Command {
	var name string
	command := &cobra.Command{Use: "login", Short: "Log in and save the session cookies", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadProfile(name)
		if err != nil {
			return err
		}
		if err := login(context.Background(), p); err != nil {
			return err
		}
		data := map[string]any{"name": p.Name, "server": p.Server, "logged_in": true}
		return appframe.Respond(cmd, data, "Login succeeded; session cookies saved", output.Meta{})
	}}
	command.Flags().StringVar(&name, "name", "", "Profile name")
	return command
}

// login performs the real login: this placeholder only demonstrates the rest
// request skeleton and the cookie-persistence hooks.
func login(ctx context.Context, p *Profile) error {
	client := rest.New(rest.Options{
		BaseURL:   p.Server,
		Timeout:   30 * time.Second,
		Jar:       nil, // injected by withSessionClient via RestoreJar
		UserAgent: "mcskel",
	})
	_ = client // TODO(adapt-platform): submit the login form with client.NewRequest + client.Do
	_ = ctx
	// TODO(adapt-platform): after a successful login, persist the cookies from
	// the Jar via p.MarkLoggedIn(...) and store.Save(p, false); see the file
	// header comment for how to detect login success.
	return fmt.Errorf("login_not_implemented: the template is not wired to a real platform; replace this function before use")
}

// sessionValid / ensureSession demonstrate the standard orchestration of
// session validation with automatic re-login: session.Ensure validates first,
// and re-invokes login once if the session has expired.
func sessionValid(ctx context.Context, p *Profile, client *rest.Client) (bool, error) {
	req, err := client.NewRequest(ctx, http.MethodGet, "/api/session/status", nil, "")
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	// TODO(adapt-platform): judge by platform semantics (200 and not a login page = valid).
	return resp.StatusCode == http.StatusOK, nil
}

func ensureSession(ctx context.Context, p *Profile) error {
	client, err := withSessionClient(p)
	if err != nil {
		return err
	}
	return csession.Ensure(ctx,
		func(ctx context.Context) (bool, error) { return sessionValid(ctx, p, client) },
		func(ctx context.Context) error { return login(ctx, p) },
	)
}

// withSessionClient builds a rest client with restored cookies.
func withSessionClient(p *Profile) (*rest.Client, error) {
	jar, _, err := csession.RestoreJar(p.Server, p.CookieOrigin, profileCookies(p))
	if err != nil {
		return nil, err
	}
	return rest.New(rest.Options{BaseURL: p.Server, Timeout: 30 * time.Second, Jar: jar, UserAgent: "mcskel"}), nil
}

// profileCookies converts persisted cookies back to http.Cookie (copy this
// function into your CLI if needed).
func profileCookies(p *Profile) []*http.Cookie {
	result := make([]*http.Cookie, 0, len(p.Cookies))
	for _, c := range p.Cookies {
		if c.Name != "" {
			result = append(result, &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HttpOnly: c.HTTPOnly})
		}
	}
	return result
}
