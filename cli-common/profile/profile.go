// Package profile provides the profile credential storage engine for CLI
// tools: config directory resolution, the root config, profile CRUD, and
// name/server validation. The on-disk layout (directory structure, JSON field
// names, 0700/0600 permission semantics) stays compatible with what existing
// CLIs already write; each CLI embeds Base to compose its own profile type,
// fields flatten during JSON serialization, so existing credential files
// round-trip losslessly.
package profile

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/secretfile"
)

// SchemaVersion is the current structure version of the root config.
const SchemaVersion = "v1"

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateName rejects profile names that could traverse paths or cannot be
// safely persisted to disk.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) || name == "." || name == ".." {
		return errors.New(msgInvalidProfileName)
	}
	return nil
}

// NormalizeServer canonicalizes an address to a stable origin so cookies are
// never silently reused across different addresses. noun is the platform name
// (e.g. "MYTOOL") used in error messages.
func NormalizeServer(raw, noun string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf(msgServerMustBeFullHTTP, noun)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// ServerOrigin returns the scheme://host portion of an address.
func ServerOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// Cookie is a persistable representation of an HTTP cookie.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Expires  string `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
}

// Root holds global configuration and never contains any secrets.
type Root struct {
	SchemaVersion  string `json:"schema_version"`
	CurrentProfile string `json:"current_profile,omitempty"`
}

// Base carries the common fields of a composed profile. A CLI embeds this
// struct into its own profile type; fields flatten during JSON serialization,
// keeping byte-level compatibility with the existing on-disk format.
type Base struct {
	Name         string   `json:"name"`
	Server       string   `json:"server"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	Cookies      []Cookie `json:"cookies,omitempty"`
	CookieOrigin string   `json:"cookie_origin,omitempty"`
	LoginAt      string   `json:"login_at,omitempty"`
}

// ValidateOptions controls how Base is validated, covering the differences
// between existing CLIs.
type ValidateOptions struct {
	Noun            string       // Platform name (e.g. "MYTOOL"), used in server address error messages
	DefaultServer   string       // Default server filled in when server is empty; empty means it must be provided explicitly
	PasswordEnvVar  string       // Environment variable name mentioned in the missing-password hint
	RequirePassword bool         // Whether a non-empty password is required
	SkipUsername    bool         // Username validation is deferred to project-specific checks (e.g. a legacy CLI validates it jointly with other fields)
	Extra           func() error // Project-specific validation, run after the base checks
}

// Validate validates and normalizes the base fields of a profile.
func (b *Base) Validate(opts ValidateOptions) error {
	if err := ValidateName(b.Name); err != nil {
		return err
	}
	if strings.TrimSpace(b.Server) == "" && opts.DefaultServer != "" {
		b.Server = opts.DefaultServer
	}
	server, err := NormalizeServer(b.Server, opts.Noun)
	if err != nil {
		return err
	}
	b.Server = server
	if !opts.SkipUsername && strings.TrimSpace(b.Username) == "" {
		return errors.New(msgUsernameRequired)
	}
	if opts.RequirePassword && strings.TrimSpace(b.Password) == "" {
		return fmt.Errorf(msgPasswordMissing, opts.PasswordEnvVar)
	}
	if opts.Extra != nil {
		return opts.Extra()
	}
	return nil
}

// PrepareSave fills in the cookie binding origin before saving: when cookies
// exist but CookieOrigin is empty, it is backfilled from the current server
// address so the cookies cannot be mistaken for coming from another address.
func (b *Base) PrepareSave() error {
	if b.CookieOrigin == "" && len(b.Cookies) > 0 {
		value, err := ServerOrigin(b.Server)
		if err != nil {
			return err
		}
		b.CookieOrigin = value
	}
	return nil
}

// MarkLoggedIn updates the cookies and the login timestamp.
func (b *Base) MarkLoggedIn(cookies []Cookie) error {
	value, err := ServerOrigin(b.Server)
	if err != nil {
		return err
	}
	b.Cookies = cookies
	b.CookieOrigin = value
	b.LoginAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

// LoggedIn reports whether a saved login state exists.
func (b *Base) LoggedIn() bool {
	return b.LoginAt != "" && len(b.Cookies) > 0
}

// PublicMap returns a generic summary without the password or cookie values
// (the shape the legacy CLIs use); CLIs whose summary needs extra fields
// (e.g. cluster information) assemble their own.
func (b *Base) PublicMap(current bool) map[string]any {
	return map[string]any{
		"name":         b.Name,
		"server":       b.Server,
		"username":     b.Username,
		"has_password": strings.TrimSpace(b.Password) != "",
		"logged_in":    b.LoggedIn(),
		"login_at":     b.LoginAt,
		"current":      current,
	}
}

// StoreOptions describes the profile storage layout and validation hooks of
// one CLI.
type StoreOptions[T any] struct {
	AppName      string                                 // Config directory name (e.g. "mytool")
	EnvVar       string                                 // Dedicated environment variable (e.g. "MYTOOL_CONFIG_HOME"), used for test isolation and custom locations
	LegacyDirs   []string                               // Legacy directory names (e.g. "legacy" left over from an older layout); reused when present so saved login states are not lost
	Validate     func(p *T, requirePassword bool) error // Validation before saving; nil skips
	PrepareSave  func(p *T) error                       // Pre-save hook (e.g. backfilling CookieOrigin); nil skips
	ProfileName  func(p *T) string                      // Used for post-read consistency checks; nil skips
	StrictRemove bool                                   // Whether Remove errors when the file does not exist (one legacy CLI's semantics)
}

// Store is the generic profile storage engine; each CLI gets its own instance.
type Store[T any] struct {
	opts StoreOptions[T]
}

// NewStore creates a profile storage engine.
func NewStore[T any](opts StoreOptions[T]) *Store[T] {
	return &Store[T]{opts: opts}
}

// Dir resolves the config directory by convention: dedicated environment
// variable → XDG_CONFIG_HOME/<AppName> → ~/.config/<AppName>. When LegacyDirs
// is configured and the new directory does not exist while a legacy one does,
// the legacy directory is kept.
func (s *Store[T]) Dir() (string, error) {
	if value := strings.TrimSpace(os.Getenv(s.opts.EnvVar)); value != "" {
		return filepath.Clean(value), nil
	}
	resolve := func(base string) string {
		newDir := filepath.Join(base, s.opts.AppName)
		if _, err := os.Stat(newDir); err == nil {
			return newDir
		}
		for _, legacy := range s.opts.LegacyDirs {
			if _, err := os.Stat(filepath.Join(base, legacy)); err == nil {
				return filepath.Join(base, legacy)
			}
		}
		return newDir
	}
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return resolve(filepath.Clean(value)), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf(msgHomeDirUnknown, err)
	}
	return resolve(filepath.Join(home, ".config")), nil
}

func (s *Store[T]) rootPath() (string, error) {
	dir, err := s.Dir()
	return filepath.Join(dir, "config.json"), err
}

func (s *Store[T]) profilesDir() (string, error) {
	dir, err := s.Dir()
	return filepath.Join(dir, "profiles"), err
}

func (s *Store[T]) profilePath(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := s.profilesDir()
	return filepath.Join(dir, name+".json"), err
}

// RootPath returns the full path of the root config file.
func (s *Store[T]) RootPath() (string, error) { return s.rootPath() }

// LoadRoot reads the root config; on first use it returns defaults without
// creating any file.
func (s *Store[T]) LoadRoot() (*Root, error) {
	path, err := s.rootPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Root{SchemaVersion: SchemaVersion}, nil
	} else if err != nil {
		return nil, err
	}
	var root Root
	if err := secretfile.Read(path, &root, secretfile.Strict); err != nil {
		return nil, err
	}
	if root.SchemaVersion == "" {
		root.SchemaVersion = SchemaVersion
	}
	return &root, nil
}

// SaveRoot atomically saves the root config.
func (s *Store[T]) SaveRoot(root *Root) error {
	if root.SchemaVersion == "" {
		root.SchemaVersion = SchemaVersion
	}
	path, err := s.rootPath()
	if err != nil {
		return err
	}
	return secretfile.Write(path, root, secretfile.Strict)
}

// Load reads a single profile: a missing file reports "profile not found";
// a name inside the file that does not match the requested name reports a
// content mismatch (guards against mixed-up files).
func (s *Store[T]) Load(name string) (*T, error) {
	path, err := s.profilePath(name)
	if err != nil {
		return nil, err
	}
	var profile T
	if err := secretfile.Read(path, &profile, secretfile.Strict); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(msgProfileNotFound, name)
		}
		return nil, err
	}
	if s.opts.ProfileName != nil && s.opts.ProfileName(&profile) != name {
		return nil, fmt.Errorf(msgProfileContentMismatch, name)
	}
	return &profile, nil
}

// Save validates (including project hooks) and then atomically saves a
// profile; requirePassword is passed through to the validation hook so the
// existing "saving metadata without a password" paths of the CLIs keep working.
func (s *Store[T]) Save(p *T, requirePassword bool) error {
	if s.opts.Validate != nil {
		if err := s.opts.Validate(p, requirePassword); err != nil {
			return err
		}
	}
	if s.opts.PrepareSave != nil {
		if err := s.opts.PrepareSave(p); err != nil {
			return err
		}
	}
	var name string
	if s.opts.ProfileName != nil {
		name = s.opts.ProfileName(p)
	}
	path, err := s.profilePath(name)
	if err != nil {
		return err
	}
	return secretfile.Write(path, p, secretfile.Strict)
}

// List returns the saved profile names sorted lexicographically. Symbolic
// links inside the directory cause a hard error (a security hardening
// inherited from a legacy CLI).
func (s *Store[T]) List() ([]string, error) {
	dir, err := s.profilesDir()
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(dir); os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf(msgProfileDirUnsafe, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(msgProfileDirHasSymlink, entry.Name())
		}
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			if ValidateName(name) == nil {
				result = append(result, name)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

// Remove deletes the named profile. With StrictRemove a missing file is an
// error (one legacy CLI's semantics); otherwise a missing file counts as
// success (another legacy CLI's semantics).
func (s *Store[T]) Remove(name string) error {
	path, err := s.profilePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			if s.opts.StrictRemove {
				return fmt.Errorf(msgProfileNotFound, name)
			}
			return nil
		}
		return err
	}
	return nil
}

// CheckDirSecurity inspects the permissions of the config directory and the
// profile files, for use by doctor-style commands.
func (s *Store[T]) CheckDirSecurity() ([]map[string]string, error) {
	dir, err := s.Dir()
	if err != nil {
		return nil, err
	}
	warnings := make([]map[string]string, 0)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return warnings, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm() != 0700 {
		warnings = append(warnings, map[string]string{
			"code":    "insecure_config_dir",
			"message": fmt.Sprintf(msgConfigDirPerm0700, dir),
		})
	}
	profilesDirPath, err := s.profilesDir()
	if err != nil {
		return warnings, err
	}
	entries, err := os.ReadDir(profilesDirPath)
	if os.IsNotExist(err) {
		return warnings, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(profilesDirPath, entry.Name())
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if fi.Mode().Perm() != 0600 {
			warnings = append(warnings, map[string]string{
				"code":    "insecure_profile_file",
				"message": fmt.Sprintf(msgProfileFilePerm0600, path),
			})
		}
	}
	return warnings, nil
}
