// Package profilecmd provides the shared assembly for the profile
// add/list/show/use/remove command layer (A4 extraction, design contract
// 2026-11-24). All storage and presentation differences are absorbed through
// injection: Store absorbs persistence (paths/permissions/JSON fields remain
// the responsibility of each project's config package and cli-common/profile),
// View absorbs presentation (column widths/column sets/login-state markers),
// and AddConfig absorbs the add branch skeleton in its credential-backed /
// no-auth dual shape. The envelopes this package produces (error codes/
// messages/hints/exit codes) are byte-identical to the projects' current
// implementations, locked by each project's testdata/golden baselines.
//
// Fidelity notes (historical behavior that must not be tweaked in passing):
//   - remove's TTY confirmation interaction (prompt to stdout, Fscanln on
//     stdin, non-yes → cancelled/ExitGeneral) and the non-TTY
//     delete-without-asking semantics are shared as-is;
//   - the dry-run human message keeps reusing the status verb
//     (create/replace) by stripping the would_ prefix, as the historical
//     implementations did;
//   - the "profile not found" detection stays a string match
//     (strings.Contains), not converted to a semantic sentinel.
package profilecmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/profile"
)

// Store is the profile storage capability the command layer needs; each
// project's cmd package provides a thin adapter (package-level functions →
// methods, roughly 20 lines per project). P is usually *config.Profile.
type Store[P any] interface {
	// LoadRoot reads the root config (current_profile); first use returns
	// the default value.
	LoadRoot() (*profile.Root, error)
	// SaveRoot atomically saves the root config.
	SaveRoot(root *profile.Root) error
	// Load reads a single profile; it returns a profile-not-found error
	// when the name does not exist.
	Load(name string) (P, error)
	// Save atomically saves the profile after validation (the Strict/Tighten
	// write policy is decided by the adapting side).
	Save(p P) error
	// List returns the saved profile names, sorted lexicographically.
	List() ([]string, error)
	// Remove deletes the named profile.
	Remove(name string) error
}

// View projects P into its presentation form; presentation differences such
// as column widths, column sets and login-state markers are concentrated
// here and kept out of the shared package.
type View[P any] interface {
	// Public returns the summary without sensitive fields (the elements of
	// list's data.profiles and add's data.profile).
	Public(p P, current bool) map[string]any
	// ShowLine is show's human-readable one-line message (credential-backed
	// includes username, no-auth does not).
	ShowLine(p P) string
	// ListLine is list's table row (with the * current marker and the
	// login-state column).
	ListLine(p P, current bool) string
}

// CredentialConfig holds the password-reading parameters for
// credential-backed add; nil means the no-auth shape (e.g. a no-auth CLI).
type CredentialConfig struct {
	// EnvVars are the password environment variables; the order is the
	// priority (primary name first, compatibility alias after — never
	// inverted).
	EnvVars []string
	// Prompt is the hidden-input prompt text (e.g. "Enter mytool password: ").
	Prompt string
	// ValidateAfterPasswordHint is the hint used when validation with the
	// password fails (blocked by Validate(true)) (e.g. "set MYTOOL_PASSWORD,
	// or enter the password interactively in the terminal").
	ValidateAfterPasswordHint string
	// PasswordSource is the function producing data.password_source during a
	// dry run ("environment"/"interactive"); only needed by DryRun projects.
	PasswordSource func() string
}

// AddConfig holds the differential parameters of the add subcommand. All
// differences live in the assembly points (which profile to build, how to
// validate, how to compare, how to clear the session, how to speak); no
// branch-topology differences are included. If an adopter needs an 8th hook,
// wants to change the shared branches or add project-name checks, per design
// contract §0.5 that project's add falls back to staying local.
type AddConfig[P any] struct {
	// DefaultServer is the --server fallback value (each project's
	// config.DefaultServer).
	DefaultServer string
	// Short is the add subcommand's Short text.
	Short string
	// ServerFlagHelp / UsernameFlagHelp / ReplaceFlagHelp / EnvFlagHelp /
	// DryRunFlagHelp are the flag help texts (including the platform name
	// and environment notes).
	ServerFlagHelp   string
	UsernameFlagHelp string // credential-backed only registers --username
	ReplaceFlagHelp  string
	EnvFlagHelp      string // registered when ResolveServer is non-nil
	DryRunFlagHelp   string // registered when DryRun is true
	// InvalidProfileHint is the hint of the invalid_profile envelope
	// (wording differs per project).
	InvalidProfileHint string
	// InvalidEnvHint is the hint of the invalid_env envelope when
	// ResolveServer fails (only used by projects carrying --env, e.g.
	// "use --env test or --env prod").
	InvalidEnvHint string
	// ConflictMessage is the message of the profile_conflict envelope
	// ("a profile with the same name already exists with different
	// settings" / "…with a different server address").
	ConflictMessage string
	// DryRun enables the --dry-run preview path (would_create/would_replace).
	DryRun bool
	// ResolveServer resolves the final server address (e.g. --env taking
	// precedence over --server); nil means only the DefaultServer fallback
	// applies. Returned errors map to invalid_env/ExitUsage.
	ResolveServer func(serverFlag, envFlag string) (string, error)
	// BuildProfile constructs the new profile (credential-backed fills
	// username; no-auth ignores it).
	BuildProfile func(name, server, username string) P
	// Validate validates the profile; the credential-backed implementation
	// must, when requirePassword is true, write the password into the
	// profile before validating (the single gap through which the password
	// is injected, see the package doc), and must not touch the password
	// field when requirePassword is false. The no-auth shape ignores the
	// last two parameters.
	Validate func(p P, password string, requirePassword bool) error
	// EqualStored is the unchanged predicate (credential-backed compares
	// name/server/username/password, no-auth compares server only).
	EqualStored func(a, b P) bool
	// ResetSession clears session fields (Cookies/LoginAt etc.) when
	// --replace overwrites; no-auth passes nil.
	ResetSession func(p P)
	// SavedMessage is the human message on successful save (punctuation
	// differs verbatim across the projects).
	SavedMessage func(p P, replaced bool) string
}

// Config is the overall assembly of the profile parent command.
type Config[P any] struct {
	// CommandName is the command name (e.g. "mytool"), used in the hint of
	// the profile_not_found envelope.
	CommandName string
	// Short is the parent command's Short.
	Short string
	// ShowShort is the show subcommand's Short text (wording differs per
	// project).
	ShowShort string
	Store     Store[P]
	View      View[P]
	Add       AddConfig[P]
	// Credential being nil means the no-auth shape (no --username, no
	// password reading).
	Credential *CredentialConfig
	// ExtraSubcommands appends subcommand constructors (e.g. a
	// project-specific profile init); implementations stay in the calling
	// repository and out of the shared package.
	ExtraSubcommands []func() *cobra.Command
}

// NewCommand assembles the profile parent command
// (add/list/show/use/remove [+extensions]); the registration order matches
// the projects' current implementations.
func NewCommand[P any](cfg Config[P]) *cobra.Command {
	parent := &cobra.Command{Use: "profile", Short: cfg.Short}
	parent.AddCommand(
		NewAddCommand(cfg.Store, cfg.View, cfg.Add, cfg.Credential),
		NewListCommand(cfg.Store, cfg.View),
		NewShowCommand(cfg.Store, cfg.View, cfg.CommandName, cfg.ShowShort),
		NewUseCommand(cfg.Store, cfg.CommandName),
		NewRemoveCommand(cfg.Store),
	)
	for _, extra := range cfg.ExtraSubcommands {
		parent.AddCommand(extra())
	}
	return parent
}

// MapProfileNotFound maps a "profile not found" error to the
// profile_not_found/ExitNotFound envelope (the hint points at
// <command> profile list); every other error passes through untouched.
// Each project's system.go profileNotFound can become a one-line forward
// (kept local and untouched in this round).
// The not-found detection stays a plain string match on the store's error
// message (strings.Contains, matching the profile package's
// "profile %q not found"), deliberately not a semantic sentinel.
func MapProfileNotFound(err error, commandName string) error {
	if strings.Contains(err.Error(), "not found") {
		return appframe.CommandError("profile_not_found", err.Error(),
			fmt.Sprintf("run %s profile list to see the saved profiles", commandName), output.ExitNotFound)
	}
	return err
}
