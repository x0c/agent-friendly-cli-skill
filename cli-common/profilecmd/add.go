package profilecmd

// The add subcommand: credential-backed / no-auth dual-shape assembly plus
// the dry-run preview path. The branch skeleton (ResolveServer →
// BuildProfile → Validate → Load existing → [dry-run] → read password →
// Validate(true) → unchanged/conflict/--replace → ResetSession → Save →
// Respond) matches the projects' current implementations branch for branch.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/prompt"
)

// NewAddCommand assembles the add subcommand. store absorbs persistence,
// view absorbs the data.profile summary, cfg absorbs the differential
// parameters, and a nil cred takes the no-auth path (no password, no session
// cleanup).
func NewAddCommand[P any](store Store[P], view View[P], cfg AddConfig[P], cred *CredentialConfig) *cobra.Command {
	var server, env, username string
	var replace, dryRun bool
	command := &cobra.Command{
		Use:   "add <name>",
		Short: cfg.Short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedServer := server
			if cfg.ResolveServer != nil {
				value, err := cfg.ResolveServer(server, env)
				if err != nil {
					return appframe.CommandError("invalid_env", err.Error(), cfg.InvalidEnvHint, output.ExitUsage)
				}
				resolvedServer = value
			}
			if strings.TrimSpace(resolvedServer) == "" {
				resolvedServer = cfg.DefaultServer
			}
			profile := cfg.BuildProfile(args[0], resolvedServer, username)
			if err := cfg.Validate(profile, "", false); err != nil {
				return appframe.CommandError("invalid_profile", err.Error(), cfg.InvalidProfileHint, output.ExitUsage)
			}
			current, loadErr := store.Load(args[0])
			if loadErr != nil && !strings.Contains(loadErr.Error(), "not found") {
				return loadErr
			}
			// Generic P cannot be compared against nil; a nil-error Load is
			// treated as existing, which behaves the same as the projects'
			// current "current != nil" check (Load returns the zero value P
			// when not found).
			exists := loadErr == nil
			if cfg.DryRun && dryRun {
				status := "would_create"
				if exists {
					status = "would_replace"
				}
				if exists && !replace && !cfg.EqualStored(current, profile) {
					return appframe.CommandError("profile_conflict", cfg.ConflictMessage, "pass --replace explicitly to overwrite once confirmed", output.ExitConflict)
				}
				data := map[string]any{"status": status, "profile": view.Public(profile, false)}
				if cred != nil && cred.PasswordSource != nil {
					data["password_source"] = cred.PasswordSource()
				}
				// Reusing the status verb (create/replace) by stripping the
				// would_ prefix is the historical behavior, kept verbatim.
				return appframe.Respond(cmd, data, fmt.Sprintf("Dry run: would %s profile %q", strings.TrimPrefix(status, "would_"), args[0]), output.Meta{DryRun: true})
			}
			if cred != nil {
				password, readErr := readPassword(cmd, cred)
				if readErr != nil {
					return readErr
				}
				if err := cfg.Validate(profile, password, true); err != nil {
					return appframe.CommandError("missing_password", err.Error(), cred.ValidateAfterPasswordHint, output.ExitUsage)
				}
			}
			if exists {
				if cfg.EqualStored(current, profile) {
					return appframe.Respond(cmd, map[string]any{"status": "unchanged", "profile": view.Public(current, false)}, fmt.Sprintf("Profile %q unchanged", args[0]), output.Meta{})
				}
				if !replace {
					return appframe.CommandError("profile_conflict", cfg.ConflictMessage, "pass --replace explicitly to overwrite once confirmed", output.ExitConflict)
				}
				if cfg.ResetSession != nil {
					cfg.ResetSession(profile)
				}
			}
			if err := store.Save(profile); err != nil {
				return err
			}
			status := "created"
			if exists {
				status = "replaced"
			}
			return appframe.Respond(cmd, map[string]any{"status": status, "profile": view.Public(profile, false)}, cfg.SavedMessage(profile, exists), output.Meta{})
		},
	}
	command.Flags().StringVar(&server, "server", "", cfg.ServerFlagHelp)
	if cfg.ResolveServer != nil {
		command.Flags().StringVar(&env, "env", "", cfg.EnvFlagHelp)
	}
	if cred != nil {
		command.Flags().StringVar(&username, "username", "", cfg.UsernameFlagHelp)
	}
	command.Flags().BoolVar(&replace, "replace", false, cfg.ReplaceFlagHelp)
	if cfg.DryRun {
		command.Flags().BoolVar(&dryRun, "dry-run", false, cfg.DryRunFlagHelp)
	}
	return command
}

// readPassword reads the password: environment variables first (EnvVars
// order is the priority), then hidden terminal input; when non-interactive
// with none provided, it maps to a structured missing_password error whose
// wording matches the projects' current implementations verbatim (the
// message is formatted from EnvVars[0]).
func readPassword(cmd *cobra.Command, cred *CredentialConfig) (string, error) {
	value, err := prompt.ReadPassword(prompt.PasswordOptions{
		EnvVars: cred.EnvVars,
		Prompt:  cred.Prompt,
		Stderr:  cmd.ErrOrStderr(),
		Stdout:  cmd.OutOrStdout(),
	})
	if err != nil {
		// In a non-interactive environment with no password provided, map to
		// a structured error with wording matching the historical behavior.
		if errors.Is(err, prompt.ErrNeedPassword) {
			return "", appframe.CommandError("missing_password", fmt.Sprintf("non-interactive mode requires %s to be set", cred.EnvVars[0]), "provide the password via an environment variable; it never appears on the command line", output.ExitUsage)
		}
		return "", err
	}
	return value, nil
}
