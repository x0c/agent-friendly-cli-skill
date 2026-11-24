package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testProfile simulates a CLI-side composed profile: an embedded Base plus
// project-specific fields.
type testProfile struct {
	Base
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
}

func newTestStore(t *testing.T, envVar, appName string, legacy []string) *Store[testProfile] {
	t.Helper()
	dir := t.TempDir()
	// t.TempDir may create 0755 depending on umask; tighten it to 0700 to
	// match a real config directory.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envVar, dir)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate from the real user config
	return NewStore(StoreOptions[testProfile]{
		AppName:    appName,
		EnvVar:     envVar,
		LegacyDirs: legacy,
		Validate: func(p *testProfile, requirePassword bool) error {
			return p.Validate(ValidateOptions{
				Noun:            "MYTOOL",
				DefaultServer:   "https://default.example.com",
				PasswordEnvVar:  "TEST_PASSWORD",
				RequirePassword: requirePassword,
				SkipUsername:    true,
				Extra: func() error {
					if strings.TrimSpace(p.Cluster) == "" {
						return errTest("--cluster is required")
					}
					return nil
				},
			})
		},
		PrepareSave: func(p *testProfile) error { return p.PrepareSave() },
		ProfileName: func(p *testProfile) string { return p.Name },
	})
}

type errTest string

func (e errTest) Error() string { return string(e) }

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"a", "prod-1", "mytool.test_2"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ". ", "..", ".hidden", "a b", "a/b", strings.Repeat("x", 65)} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if !strings.Contains(ValidateName("x y").Error(), "invalid profile name") {
		t.Fatal("error message diverges from the existing implementation")
	}
}

func TestNormalizeServer(t *testing.T) {
	got, err := NormalizeServer(" https://mytool.example.com/sub/ ", "MYTOOL")
	if err != nil || got != "https://mytool.example.com/sub" {
		t.Fatalf("unexpected normalization result: %q %v", got, err)
	}
	if _, err := NormalizeServer("mytool.example.com", "MYTOOL"); err == nil ||
		!strings.Contains(err.Error(), "the MYTOOL server address must be a full http or https URL") {
		t.Fatalf("missing scheme should raise a platform-name error: %v", err)
	}
}

func TestBaseValidate(t *testing.T) {
	// Default server backfill.
	b := &Base{Name: "p", Username: "u"}
	if err := b.Validate(ValidateOptions{Noun: "MYTOOL", DefaultServer: "https://d.example.com"}); err != nil {
		t.Fatal(err)
	}
	if b.Server != "https://d.example.com" {
		t.Fatalf("default server was not backfilled: %q", b.Server)
	}
	// Username is required.
	b2 := &Base{Name: "p", Server: "https://d.example.com"}
	if err := b2.Validate(ValidateOptions{Noun: "MYTOOL"}); err == nil ||
		err.Error() != "--username is required" {
		t.Fatalf("expected a username-required error: %v", err)
	}
	// The missing-password message includes the environment variable name.
	b3 := &Base{Name: "p", Server: "https://d.example.com", Username: "u"}
	err := b3.Validate(ValidateOptions{Noun: "MYTOOL", PasswordEnvVar: "MYTOOL_PASSWORD", RequirePassword: true})
	if err == nil || err.Error() != "no password provided; set MYTOOL_PASSWORD or enter it in the terminal" {
		t.Fatalf("unexpected missing-password message: %v", err)
	}
}

// TestBaseJSONFlatten locks down the compatibility invariant: a composed
// profile with an embedded Base serializes with flattened fields whose names
// match existing credential files field by field (including reading the
// interleaved cluster/namespace layout a legacy CLI uses).
func TestBaseJSONFlatten(t *testing.T) {
	// A legacy CLI layout: cluster/namespace interleaved between server and
	// username.
	legacy := `{
  "name": "k1",
  "server": "https://legacy.example.com",
  "cluster": "c1",
  "namespace": "ns1",
  "username": "u1",
  "password": "p1",
  "cookies": [{"name": "sid", "value": "v"}],
  "login_at": "2026-08-24T00:00:00Z",
  "cookie_origin": "https://legacy.example.com"
}`
	var p testProfile
	if err := json.Unmarshal([]byte(legacy), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "k1" || p.Server != "https://legacy.example.com" || p.Cluster != "c1" ||
		p.Namespace != "ns1" || p.Username != "u1" || p.Password != "p1" ||
		len(p.Cookies) != 1 || p.Cookies[0].Name != "sid" || p.CookieOrigin != "https://legacy.example.com" {
		t.Fatalf("legacy format did not read back correctly: %+v", p)
	}
	// Writing back keeps the flattened structure and field names.
	data, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"name"`, `"server"`, `"cluster"`, `"namespace"`, `"username"`, `"password"`, `"cookies"`, `"cookie_origin"`, `"login_at"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("flattened field %s missing after write-back: %s", field, string(data))
		}
	}
}

func TestMarkLoggedInAndPrepareSave(t *testing.T) {
	b := &Base{Name: "p", Server: "https://s.example.com/sub"}
	cookies := []Cookie{{Name: "sid", Value: "v"}}
	if err := b.MarkLoggedIn(cookies); err != nil {
		t.Fatal(err)
	}
	if b.CookieOrigin != "https://s.example.com" || b.LoginAt == "" || !b.LoggedIn() {
		t.Fatalf("unexpected MarkLoggedIn result: %+v", b)
	}
	// PrepareSave backfills a missing CookieOrigin.
	b2 := &Base{Name: "p", Server: "https://s.example.com", Cookies: cookies}
	if err := b2.PrepareSave(); err != nil {
		t.Fatal(err)
	}
	if b2.CookieOrigin != "https://s.example.com" {
		t.Fatalf("PrepareSave did not backfill CookieOrigin: %q", b2.CookieOrigin)
	}
}

func TestStoreSaveLoadRoundtrip(t *testing.T) {
	store := newTestStore(t, "TEST_PROFILE_HOME", "mytool", nil)
	in := &testProfile{Base: Base{Name: "default", Server: "https://mytool.example.com", Username: "u", Password: "p"}, Cluster: "c1"}
	if err := store.Save(in, true); err != nil {
		t.Fatal(err)
	}
	out, err := store.Load("default")
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "default" || out.Cluster != "c1" || out.Password != "p" {
		t.Fatalf("unexpected round-trip result: %+v", out)
	}
	// File permission 0600, inside the profiles/ subdirectory.
	path, _ := store.profilePath("default")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("profile file permission is %#o, want 0600", info.Mode().Perm())
	}
}

func TestStoreLoadErrors(t *testing.T) {
	store := newTestStore(t, "TEST_PROFILE_HOME", "mytool", nil)
	if _, err := store.Load("absent"); err == nil || err.Error() != `profile "absent" not found` {
		t.Fatalf("unexpected not-found message: %v", err)
	}
	// Write a file whose inner name does not match.
	dir, _ := store.profilesDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"name":"a"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("b"); err == nil || !strings.Contains(err.Error(), "profile file content mismatch") {
		t.Fatalf("unexpected name-mismatch error: %v", err)
	}
}

func TestStoreList(t *testing.T) {
	store := newTestStore(t, "TEST_PROFILE_HOME", "mytool", nil)
	names, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Fatalf("empty directory should return an empty slice: %v", names)
	}
	for _, name := range []string{"beta", "alpha"} {
		p := &testProfile{Base: Base{Name: name, Server: "https://s.example.com", Username: "u", Password: "p"}, Cluster: "c"}
		if err := store.Save(p, true); err != nil {
			t.Fatal(err)
		}
	}
	names, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("list should be sorted lexicographically: %v", names)
	}
	// Symbolic links inside the directory must be rejected.
	dir, _ := store.profilesDir()
	if err := os.Symlink(filepath.Join(dir, "alpha.json"), filepath.Join(dir, "link.json")); err != nil {
		t.Skip("symbolic links are not supported in this environment")
	}
	if _, err := store.List(); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symbolic link should be an error: %v", err)
	}
}

func TestStoreRemoveSemantics(t *testing.T) {
	// Non-strict: a missing file counts as success.
	store := newTestStore(t, "TEST_PROFILE_HOME", "mytool", nil)
	if err := store.Remove("absent"); err != nil {
		t.Fatalf("removing a missing file should succeed in non-strict mode: %v", err)
	}
	// Strict: a missing file is an error.
	strict := NewStore(StoreOptions[testProfile]{AppName: "mytool", EnvVar: "TEST_PROFILE_HOME", StrictRemove: true})
	if err := strict.Remove("absent"); err == nil || !strings.Contains(err.Error(), `profile "absent" not found`) {
		t.Fatalf("strict mode should report not found: %v", err)
	}
}

func TestRootRoundtrip(t *testing.T) {
	store := newTestStore(t, "TEST_PROFILE_HOME", "mytool", nil)
	root, err := store.LoadRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root.SchemaVersion != SchemaVersion {
		t.Fatalf("first load should carry the default version: %+v", root)
	}
	root.CurrentProfile = "default"
	if err := store.SaveRoot(root); err != nil {
		t.Fatal(err)
	}
	again, err := store.LoadRoot()
	if err != nil {
		t.Fatal(err)
	}
	if again.CurrentProfile != "default" || again.SchemaVersion != SchemaVersion {
		t.Fatalf("unexpected root config round-trip result: %+v", again)
	}
}

func TestDirLegacyFallback(t *testing.T) {
	// Test the legacy directory fallback via the XDG directory: when the new
	// directory does not exist but a legacy one does, the legacy one is kept.
	xdg := t.TempDir()
	t.Setenv("TEST_PROFILE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "legacy"), 0700); err != nil {
		t.Fatal(err)
	}
	store := NewStore(StoreOptions[testProfile]{AppName: "mytool", EnvVar: "TEST_PROFILE_HOME", LegacyDirs: []string{"legacy"}})
	dir, err := store.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(xdg, "legacy") {
		t.Fatalf("legacy directory should be kept: %s", dir)
	}
	// When the new directory exists it takes precedence.
	if err := os.MkdirAll(filepath.Join(xdg, "mytool"), 0700); err != nil {
		t.Fatal(err)
	}
	dir, err = store.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(xdg, "mytool") {
		t.Fatalf("new directory should take precedence: %s", dir)
	}
}

func TestCheckDirSecurity(t *testing.T) {
	store := newTestStore(t, "TEST_PROFILE_HOME", "mytool", nil)
	warnings, err := store.CheckDirSecurity()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("a missing directory should produce no warnings: %v", warnings)
	}
	dir, _ := store.Dir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	warnings, err = store.CheckDirSecurity()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0]["code"] != "insecure_config_dir" {
		t.Fatalf("unexpected directory permission warning: %v", warnings)
	}
}
