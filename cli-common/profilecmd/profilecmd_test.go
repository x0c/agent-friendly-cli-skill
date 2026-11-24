package profilecmd

// Command-layer unit tests: driven by an in-memory fakeStore and a simple
// fakeView, never touching real storage. Byte-precise envelope assertions,
// together with the projects' testdata/golden baselines, form the migration
// safety net. TTY cases rely on script(1) to allocate a pseudo-terminal and
// run only on unix platforms.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/profile"
)

// fakeProfile is the test profile shape (simulating a credential-backed
// field set).
type fakeProfile struct {
	Name       string
	Server     string
	Username   string
	Password   string
	LoginAt    string
	CookieLeft bool // simulates whether session cookies are left behind
}

// fakeStore is an in-memory Store: map + root; Save counts calls for the
// dry-run does-not-persist assertions.
type fakeStore struct {
	root      profile.Root
	profiles  map[string]*fakeProfile
	saveCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{profiles: map[string]*fakeProfile{}}
}

func (s *fakeStore) LoadRoot() (*profile.Root, error) {
	root := s.root
	if root.SchemaVersion == "" {
		root.SchemaVersion = profile.SchemaVersion
	}
	return &root, nil
}

func (s *fakeStore) SaveRoot(root *profile.Root) error {
	s.root = *root
	return nil
}

func (s *fakeStore) Load(name string) (*fakeProfile, error) {
	if p, ok := s.profiles[name]; ok {
		copied := *p
		return &copied, nil
	}
	// Mirrors the profile package's not-found message format, which the
	// string-based detection in this package matches against.
	return nil, fmt.Errorf("profile %q not found", name)
}

func (s *fakeStore) Save(p *fakeProfile) error {
	s.saveCalls++
	copied := *p
	s.profiles[p.Name] = &copied
	return nil
}

func (s *fakeStore) List() ([]string, error) {
	names := make([]string, 0, len(s.profiles))
	for name := range s.profiles {
		names = append(names, name)
	}
	// Sorted lexicographically, matching the real Store's behavior.
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names, nil
}

func (s *fakeStore) Remove(name string) error {
	delete(s.profiles, name)
	return nil
}

// fakeView is the test View: a credential-backed presentation shape.
type fakeView struct{}

func (fakeView) Public(p *fakeProfile, current bool) map[string]any {
	return map[string]any{
		"name":         p.Name,
		"server":       p.Server,
		"username":     p.Username,
		"has_password": p.Password != "",
		"current":      current,
	}
}

func (fakeView) ShowLine(p *fakeProfile) string {
	return fmt.Sprintf("%s @ %s (%s)", p.Name, p.Server, p.Username)
}

func (fakeView) ListLine(p *fakeProfile, current bool) string {
	mark := " "
	if current {
		mark = "*"
	}
	session := "logged out"
	if p.LoginAt != "" && p.CookieLeft {
		session = "logged in"
	}
	return fmt.Sprintf("%s %-8s %-20s %s", mark, p.Name, p.Server, session)
}

// testReset records how many times ResetSession was called.
type testReset struct{ calls int }

// newTestCommand assembles the full profile command family with the fake
// Store/View.
func newTestCommand(store *fakeStore, cred *CredentialConfig, mutate func(*AddConfig[*fakeProfile]), reset *testReset) *cobra.Command {
	addCfg := AddConfig[*fakeProfile]{
		DefaultServer:      "https://default.example.com",
		Short:              "Add a test profile",
		ServerFlagHelp:     "test server address; defaults to the test environment",
		UsernameFlagHelp:   "login username",
		ReplaceFlagHelp:    "confirm overwriting a same-name profile with different settings",
		InvalidProfileHint: "provide --username; --server is optional",
		ConflictMessage:    "a profile with the same name already exists with different settings",
		BuildProfile: func(name, server, username string) *fakeProfile {
			return &fakeProfile{Name: name, Server: server, Username: username}
		},
		Validate: func(p *fakeProfile, password string, requirePassword bool) error {
			if p.Username == "" {
				return fmt.Errorf("--username is required")
			}
			if requirePassword {
				p.Password = password
				if strings.TrimSpace(p.Password) == "" {
					return fmt.Errorf("password not provided; set FAKE_PASSWORD or enter it in the terminal")
				}
			}
			return nil
		},
		EqualStored: func(a, b *fakeProfile) bool {
			return a.Name == b.Name && a.Server == b.Server && a.Username == b.Username && a.Password == b.Password
		},
		ResetSession: nil,
		SavedMessage: func(p *fakeProfile, replaced bool) string {
			return fmt.Sprintf("Saved profile %q", p.Name)
		},
	}
	if reset != nil {
		addCfg.ResetSession = func(p *fakeProfile) {
			reset.calls++
			p.LoginAt = ""
			p.CookieLeft = false
		}
	}
	if mutate != nil {
		mutate(&addCfg)
	}
	// Wrap in a fakecli root command, simulating a real CLI's command tree.
	root := &cobra.Command{Use: "fakecli", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(NewCommand(Config[*fakeProfile]{
		CommandName: "fakecli",
		Short:       "Manage test profiles",
		ShowShort:   "Show the non-sensitive summary of a profile",
		Store:       store,
		View:        fakeView{},
		Add:         addCfg,
		Credential:  cred,
	}))
	return root
}

// runFake executes one command in machine mode, returning the exit code and
// stdout.
func runFake(t *testing.T, root *cobra.Command, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := appframe.Execute(root, args, &stdout, &stderr)
	return code, stdout.String()
}

func credentialConfig() *CredentialConfig {
	return &CredentialConfig{
		EnvVars:                   []string{"FAKE_PASSWORD", "FAKE_ALIAS_PASSWORD"},
		Prompt:                    "Enter the test password: ",
		ValidateAfterPasswordHint: "set FAKE_PASSWORD, or enter the password interactively in the terminal",
	}
}

// ===== add, credential-backed =====

func TestAddCredentialCreated(t *testing.T) {
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 0 {
		t.Fatalf("create should succeed, exit code %d: %s", code, out)
	}
	want := `{"ok":true,"data":{"profile":{"current":false,"has_password":true,"name":"test","server":"https://a.example.com","username":"alice"},"status":"created"},"error":null,"meta":{"schema_version":"v1"}}` + "\n"
	if out != want {
		t.Fatalf("created envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
	if store.saveCalls != 1 {
		t.Fatalf("Save should be called exactly once, got %d", store.saveCalls)
	}
}

func TestAddCredentialUnchanged(t *testing.T) {
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://a.example.com", Username: "alice", Password: "secret"}
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 0 {
		t.Fatalf("unchanged should succeed, exit code %d: %s", code, out)
	}
	if !strings.Contains(out, `"status":"unchanged"`) {
		t.Fatalf("expected unchanged: %s", out)
	}
	if store.saveCalls != 0 {
		t.Fatalf("unchanged must not persist, Save called %d times", store.saveCalls)
	}
}

func TestAddCredentialConflict(t *testing.T) {
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://a.example.com", Username: "bob", Password: "old"}
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 5 {
		t.Fatalf("conflict should exit with code 5, got %d", code)
	}
	want := `{"ok":false,"data":null,"error":{"code":"profile_conflict","message":"a profile with the same name already exists with different settings","hint":"pass --replace explicitly to overwrite once confirmed","retryable":false},"meta":{"schema_version":"v1"}}` + "\n"
	if out != want {
		t.Fatalf("conflict envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
	if store.saveCalls != 0 {
		t.Fatalf("conflict must not persist")
	}
}

func TestAddCredentialReplaceResetsSession(t *testing.T) {
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://a.example.com", Username: "bob", Password: "old", LoginAt: "2026-01-01T00:00:00Z", CookieLeft: true}
	reset := &testReset{}
	root := newTestCommand(store, credentialConfig(), nil, reset)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice", "--replace")
	if code != 0 {
		t.Fatalf("replace should succeed, exit code %d: %s", code, out)
	}
	if !strings.Contains(out, `"status":"replaced"`) {
		t.Fatalf("expected replaced: %s", out)
	}
	if reset.calls != 1 {
		t.Fatalf("ResetSession should be called exactly once when replacing a changed config, got %d", reset.calls)
	}
	saved := store.profiles["test"]
	if saved.LoginAt != "" || saved.CookieLeft {
		t.Fatalf("session should be cleared after replace: %+v", saved)
	}
}

func TestAddCredentialReplaceUnchangedSkipsReset(t *testing.T) {
	// Identical config but an explicit --replace: takes the unchanged branch
	// and does not trigger ResetSession.
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://a.example.com", Username: "alice", Password: "secret", LoginAt: "2026-01-01T00:00:00Z", CookieLeft: true}
	reset := &testReset{}
	root := newTestCommand(store, credentialConfig(), nil, reset)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice", "--replace")
	if code != 0 || !strings.Contains(out, `"status":"unchanged"`) {
		t.Fatalf("expected unchanged: code=%d out=%s", code, out)
	}
	if reset.calls != 0 {
		t.Fatalf("unchanged must not trigger ResetSession, got %d", reset.calls)
	}
}

// ===== add, password reading =====

func TestAddPasswordEnvVarPriority(t *testing.T) {
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", "primary")
	t.Setenv("FAKE_ALIAS_PASSWORD", "alias")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 0 {
		t.Fatalf("create should succeed: %s", out)
	}
	if store.profiles["test"].Password != "primary" {
		t.Fatalf("EnvVars order is the priority, the primary name should win: %q", store.profiles["test"].Password)
	}
}

func TestAddPasswordAliasFallback(t *testing.T) {
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", "")
	t.Setenv("FAKE_ALIAS_PASSWORD", "alias")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 0 {
		t.Fatalf("alias fallback create should succeed: %s", out)
	}
	if store.profiles["test"].Password != "alias" {
		t.Fatalf("the compatibility alias should take effect as fallback: %q", store.profiles["test"].Password)
	}
}

func TestAddPasswordMissingNonInteractive(t *testing.T) {
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", "")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 2 {
		t.Fatalf("missing_password should exit with code 2, got %d", code)
	}
	want := `{"ok":false,"data":null,"error":{"code":"missing_password","message":"non-interactive mode requires FAKE_PASSWORD to be set","hint":"provide the password via an environment variable; it never appears on the command line","retryable":false},"meta":{"schema_version":"v1"}}` + "\n"
	if out != want {
		t.Fatalf("missing_password envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
}

func TestAddPasswordValidateAfterPassword(t *testing.T) {
	// A whitespace-only password from the environment: reading succeeds but
	// Validate(true) blocks it, taking the ValidateAfterPasswordHint path.
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), nil, nil)
	t.Setenv("FAKE_PASSWORD", " ")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com", "--username", "alice")
	if code != 2 {
		t.Fatalf("validation failure should exit with code 2, got %d", code)
	}
	want := `{"ok":false,"data":null,"error":{"code":"missing_password","message":"password not provided; set FAKE_PASSWORD or enter it in the terminal","hint":"set FAKE_PASSWORD, or enter the password interactively in the terminal","retryable":false},"meta":{"schema_version":"v1"}}` + "\n"
	if out != want {
		t.Fatalf("Validate(true) envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
}

// ===== add, dry-run (the dry-run-enabled shape) =====

func TestAddDryRunWouldCreate(t *testing.T) {
	store := newFakeStore()
	cred := credentialConfig()
	cred.PasswordSource = func() string {
		if os.Getenv("FAKE_PASSWORD") != "" {
			return "environment"
		}
		return "interactive"
	}
	root := newTestCommand(store, cred, func(cfg *AddConfig[*fakeProfile]) {
		cfg.DryRun = true
		cfg.EnvFlagHelp = "environment alias: test or prod"
		cfg.DryRunFlagHelp = "preview only; do not write any files"
		cfg.ResolveServer = func(serverFlag, envFlag string) (string, error) {
			if strings.TrimSpace(envFlag) != "" {
				if envFlag != "test" {
					return "", fmt.Errorf("unknown environment %q", envFlag)
				}
				return "https://test.example.com", nil
			}
			return serverFlag, nil
		}
	}, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--env", "test", "--username", "alice", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run should succeed, exit code %d: %s", code, out)
	}
	want := `{"ok":true,"data":{"password_source":"environment","profile":{"current":false,"has_password":false,"name":"test","server":"https://test.example.com","username":"alice"},"status":"would_create"},"error":null,"meta":{"schema_version":"v1","dry_run":true}}` + "\n"
	if out != want {
		t.Fatalf("dry-run would_create envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
	if store.saveCalls != 0 {
		t.Fatalf("dry-run must not persist, Save called %d times", store.saveCalls)
	}
}

func TestAddDryRunWouldReplace(t *testing.T) {
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://test.example.com", Username: "bob", Password: "old"}
	cred := credentialConfig()
	cred.PasswordSource = func() string { return "interactive" }
	root := newTestCommand(store, cred, func(cfg *AddConfig[*fakeProfile]) {
		cfg.DryRun = true
		cfg.ResolveServer = func(serverFlag, envFlag string) (string, error) {
			return serverFlag, nil
		}
	}, nil)
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://test.example.com", "--username", "alice", "--replace", "--dry-run")
	if code != 0 || !strings.Contains(out, `"status":"would_replace"`) {
		t.Fatalf("expected would_replace: code=%d out=%s", code, out)
	}
}

func TestAddDryRunConflictStillIntercepts(t *testing.T) {
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://test.example.com", Username: "bob", Password: "old"}
	cred := credentialConfig()
	root := newTestCommand(store, cred, func(cfg *AddConfig[*fakeProfile]) {
		cfg.DryRun = true
		cfg.ResolveServer = func(serverFlag, envFlag string) (string, error) { return serverFlag, nil }
	}, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://test.example.com", "--username", "alice", "--dry-run")
	if code != 5 || !strings.Contains(out, `"code":"profile_conflict"`) {
		t.Fatalf("the dry-run path should still intercept conflict: code=%d out=%s", code, out)
	}
}

func TestAddResolveServerInvalidEnv(t *testing.T) {
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), func(cfg *AddConfig[*fakeProfile]) {
		cfg.InvalidEnvHint = "use --env test or --env prod"
		cfg.ResolveServer = func(serverFlag, envFlag string) (string, error) {
			if strings.TrimSpace(envFlag) != "" {
				return "", fmt.Errorf("unknown environment %q; available: prod, test", envFlag)
			}
			return serverFlag, nil
		}
	}, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--env", "bogus", "--username", "alice")
	if code != 2 {
		t.Fatalf("invalid_env should exit with code 2, got %d", code)
	}
	want := `{"ok":false,"data":null,"error":{"code":"invalid_env","message":"unknown environment \"bogus\"; available: prod, test","hint":"use --env test or --env prod","retryable":false},"meta":{"schema_version":"v1"}}` + "\n"
	if out != want {
		t.Fatalf("invalid_env envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
}

func TestAddResolveServerEnvOverridesServer(t *testing.T) {
	store := newFakeStore()
	root := newTestCommand(store, credentialConfig(), func(cfg *AddConfig[*fakeProfile]) {
		cfg.ResolveServer = func(serverFlag, envFlag string) (string, error) {
			if strings.TrimSpace(envFlag) != "" {
				return "https://env.example.com", nil
			}
			return serverFlag, nil
		}
	}, nil)
	t.Setenv("FAKE_PASSWORD", "secret")
	code, out := runFake(t, root, "profile", "add", "test", "--env", "test", "--server", "https://ignored.example.com", "--username", "alice")
	if code != 0 {
		t.Fatalf("create should succeed: %s", out)
	}
	if store.profiles["test"].Server != "https://env.example.com" {
		t.Fatalf("--env should override --server: %q", store.profiles["test"].Server)
	}
}

// ===== add, no-auth =====

func TestAddNoCredential(t *testing.T) {
	store := newFakeStore()
	reset := &testReset{}
	root := newTestCommand(store, nil, func(cfg *AddConfig[*fakeProfile]) {
		cfg.Short = "Add a profile (stores the server address only)"
		cfg.ReplaceFlagHelp = "confirm overwriting a same-name profile with a different address"
		cfg.InvalidProfileHint = "provide a valid name and --server"
		cfg.ConflictMessage = "a profile with the same name already exists with a different server address"
		// No-auth shape: Validate ignores the password parameters, unchanged
		// compares Server only, and no ResetSession is injected.
		cfg.Validate = func(p *fakeProfile, password string, requirePassword bool) error { return nil }
		cfg.EqualStored = func(a, b *fakeProfile) bool { return a.Server == b.Server }
		cfg.ResetSession = nil
		cfg.SavedMessage = func(p *fakeProfile, replaced bool) string {
			return fmt.Sprintf("Saved profile %q (%s)", p.Name, p.Server)
		}
	}, nil)
	// Creation works without the --username flag and without password
	// environment variables.
	flag := root.Commands()[0].Flags().Lookup("username")
	if flag != nil {
		t.Fatal("the no-auth shape must not register --username")
	}
	code, out := runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com")
	if code != 0 {
		t.Fatalf("create should succeed: %s", out)
	}
	if !strings.Contains(out, `"status":"created"`) {
		t.Fatalf("expected created: %s", out)
	}
	// Same server → unchanged (compares Server only).
	code, out = runFake(t, root, "profile", "add", "test", "--server", "https://a.example.com")
	if code != 0 || !strings.Contains(out, `"status":"unchanged"`) {
		t.Fatalf("same address should be unchanged: code=%d out=%s", code, out)
	}
	// Different server without --replace → conflict.
	code, out = runFake(t, root, "profile", "add", "test", "--server", "https://b.example.com")
	if code != 5 || !strings.Contains(out, `"a profile with the same name already exists with a different server address"`) {
		t.Fatalf("expected conflict: code=%d out=%s", code, out)
	}
	// --replace → replaced, without triggering session cleanup.
	code, out = runFake(t, root, "profile", "add", "test", "--server", "https://b.example.com", "--replace")
	if code != 0 || !strings.Contains(out, `"status":"replaced"`) {
		t.Fatalf("expected replaced: code=%d out=%s", code, out)
	}
	if reset.calls != 0 {
		t.Fatalf("the no-auth shape must not trigger ResetSession, got %d", reset.calls)
	}
}

// ===== list / show / use =====

func TestListTwoCurrent(t *testing.T) {
	store := newFakeStore()
	store.root.CurrentProfile = "alpha"
	store.profiles["alpha"] = &fakeProfile{Name: "alpha", Server: "https://a.example.com", Username: "alice", Password: "s", LoginAt: "2026-01-01T00:00:00Z", CookieLeft: true}
	store.profiles["beta"] = &fakeProfile{Name: "beta", Server: "https://b.example.com", Username: "bob", Password: "s"}
	root := newTestCommand(store, nil, nil, nil)
	code, out := runFake(t, root, "profile", "list")
	if code != 0 {
		t.Fatalf("list should succeed: %s", out)
	}
	var env struct {
		Data struct {
			Profiles       []map[string]any `json:"profiles"`
			CurrentProfile string           `json:"current_profile"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if env.Data.CurrentProfile != "alpha" || len(env.Data.Profiles) != 2 {
		t.Fatalf("data mismatch: %s", out)
	}
	if env.Data.Profiles[0]["name"] != "alpha" || env.Data.Profiles[0]["current"] != true {
		t.Fatalf("the current marker should be injected into Public: %s", out)
	}
}

func TestShowExistsAndNotFound(t *testing.T) {
	store := newFakeStore()
	store.root.CurrentProfile = "alpha"
	store.profiles["alpha"] = &fakeProfile{Name: "alpha", Server: "https://a.example.com", Username: "alice"}
	root := newTestCommand(store, nil, nil, nil)
	code, out := runFake(t, root, "profile", "show", "alpha")
	if code != 0 || !strings.Contains(out, `"current":true`) {
		t.Fatalf("show should carry the current marker: code=%d out=%s", code, out)
	}
	code, out = runFake(t, root, "profile", "show", "nope")
	if code != 3 {
		t.Fatalf("show of a missing profile should exit with code 3, got %d", code)
	}
	want := `{"ok":false,"data":null,"error":{"code":"profile_not_found","message":"profile \"nope\" not found","hint":"run fakecli profile list to see the saved profiles","retryable":false},"meta":{"schema_version":"v1"}}` + "\n"
	if out != want {
		t.Fatalf("profile_not_found envelope not byte-identical:\n got: %s\nwant: %s", out, want)
	}
}

func TestUseExistsAndNotFound(t *testing.T) {
	store := newFakeStore()
	store.profiles["alpha"] = &fakeProfile{Name: "alpha", Server: "https://a.example.com", Username: "alice"}
	root := newTestCommand(store, nil, nil, nil)
	code, out := runFake(t, root, "profile", "use", "alpha")
	if code != 0 || !strings.Contains(out, `"current_profile":"alpha"`) {
		t.Fatalf("use should update current: code=%d out=%s", code, out)
	}
	if store.root.CurrentProfile != "alpha" {
		t.Fatalf("root should have been updated: %+v", store.root)
	}
	code, out = runFake(t, root, "profile", "use", "nope")
	if code != 3 || !strings.Contains(out, `"code":"profile_not_found"`) {
		t.Fatalf("use of a missing profile should be profile_not_found/3: code=%d out=%s", code, out)
	}
}

// ===== remove =====

func TestRemoveYes(t *testing.T) {
	store := newFakeStore()
	store.root.CurrentProfile = "test"
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://a.example.com"}
	root := newTestCommand(store, nil, nil, nil)
	code, out := runFake(t, root, "profile", "remove", "test", "--yes")
	if code != 0 || !strings.Contains(out, `"status":"removed"`) {
		t.Fatalf("remove --yes should succeed: code=%d out=%s", code, out)
	}
	if _, ok := store.profiles["test"]; ok {
		t.Fatal("the profile should have been removed")
	}
	if store.root.CurrentProfile != "" {
		t.Fatalf("current should be cleared after removing the current profile: %q", store.root.CurrentProfile)
	}
}

// TTY subprocess sentinel case: the parent test uses script(1) to allocate a
// pseudo-terminal, re-runs this test binary, and drives remove's interactive
// confirmation branch (both the cancelled and the confirmed-yes paths).
func TestRemoveTTYChild(t *testing.T) {
	mode := os.Getenv("MC_REMOVE_TTY_MODE")
	if mode == "" {
		t.Skip("only restarted as a TTY subprocess by the parent test")
	}
	store := newFakeStore()
	store.profiles["test"] = &fakeProfile{Name: "test", Server: "https://a.example.com"}
	root := newTestCommand(store, nil, nil, nil)
	code := appframe.Execute(root, []string{"profile", "remove", "test"}, os.Stdout, os.Stderr)
	fmt.Fprintf(os.Stdout, "\nMC_REMOVE_TTY_EXIT:%d\n", code)
}

// runRemoveTTY runs the remove interactive case inside a pseudo-terminal
// (mode decides the input).
func runRemoveTTY(t *testing.T, mode, stdinInput string) (int, string) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the pseudo-terminal case relies on script(1) and runs on unix platforms only")
	}
	scriptPath, err := exec.LookPath("script")
	if err != nil {
		t.Skipf("script(1) is missing in this environment: %v", err)
	}
	childArgs := []string{"-test.run=TestRemoveTTYChild", "-test.timeout=60s"}
	var cmdArgs []string
	if runtime.GOOS == "darwin" {
		cmdArgs = append([]string{"-q", "/dev/null", os.Args[0]}, childArgs...)
	} else {
		cmdArgs = []string{"-q", "-c", os.Args[0] + " " + strings.Join(childArgs, " "), "/dev/null"}
	}
	command := exec.Command(scriptPath, cmdArgs...)
	command.Env = append(os.Environ(), "MC_REMOVE_TTY_MODE="+mode)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("failed to start the TTY subprocess: %v", err)
	}
	if _, err := stdin.Write([]byte(stdinInput)); err != nil {
		t.Fatalf("failed to write TTY input: %v", err)
	}
	// Do not close stdin proactively: avoid script(1) receiving an early
	// EOF and closing the pseudo-terminal.
	if err := command.Wait(); err != nil {
		t.Logf("TTY subprocess exited: %v", err)
	}
	text := strings.ReplaceAll(output.String(), "\r\n", "\n")
	idx := strings.Index(text, "\nMC_REMOVE_TTY_EXIT:")
	if idx < 0 {
		t.Fatalf("the TTY subprocess is missing the exit-code sentinel: %q", text)
	}
	code := strings.TrimSpace(strings.SplitN(text[idx+len("\nMC_REMOVE_TTY_EXIT:"):], "\n", 2)[0])
	var codeInt int
	fmt.Sscanf(code, "%d", &codeInt)
	return codeInt, text[:idx]
}

func TestRemoveCancelledAtTTY(t *testing.T) {
	code, out := runRemoveTTY(t, "cancel", "no\n")
	if code != 1 {
		t.Fatalf("cancelled should exit with code 1, got %d: %s", code, out)
	}
	if !strings.Contains(out, `Delete profile "test"? Type yes to continue: `) || !strings.Contains(out, "Error: Deletion cancelled") {
		t.Fatalf("cancelled human message mismatch: %q", out)
	}
}

func TestRemoveConfirmYesAtTTY(t *testing.T) {
	code, out := runRemoveTTY(t, "confirm", "yes\n")
	if code != 0 {
		t.Fatalf("confirming yes should delete successfully, exit code %d: %s", code, out)
	}
	if !strings.Contains(out, `Deleted profile "test"`) {
		t.Fatalf("deletion success message mismatch: %q", out)
	}
}

// ===== MapProfileNotFound =====

func TestMapProfileNotFound(t *testing.T) {
	mapped := MapProfileNotFound(fmt.Errorf("profile %q not found", "x"), "fakecli")
	var appErr *output.AppError
	if !errors.As(mapped, &appErr) {
		t.Fatalf("should map to an AppError: %v", mapped)
	}
	if appErr.Detail.Code != "profile_not_found" || appErr.Exit != output.ExitNotFound {
		t.Fatalf("envelope mismatch: %+v exit code %d", appErr.Detail, appErr.Exit)
	}
	if appErr.Detail.Message != `profile "x" not found` || appErr.Detail.Hint != "run fakecli profile list to see the saved profiles" {
		t.Fatalf("message/hint mismatch: %+v", appErr.Detail)
	}
	passed := MapProfileNotFound(fmt.Errorf("some other error"), "fakecli")
	if passed.Error() != "some other error" {
		t.Fatalf("non-not-found errors should pass through untouched: %v", passed)
	}
}
