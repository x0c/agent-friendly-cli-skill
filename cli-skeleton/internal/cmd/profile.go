// The profile command demonstrates the standard wiring of the shared
// profile.Store (multi-profile credential management: 0700 directory, 0600
// files, atomic writes, and full CRUD all provided by the shared package),
// plus the canonical "read-then-write" write-command pattern using
// writeguard + fingerprint.
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/fingerprint"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/profile"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/prompt"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/writeguard"
)

// Profile is this CLI's credential model: embed profile.Base and its fields
// are flattened on disk. Add platform-specific fields directly to this struct.
type Profile struct {
	profile.Base
}

// store is this CLI's profile storage engine (one instance per CLI).
var store = profile.NewStore[Profile](profile.StoreOptions[Profile]{
	AppName: "mcskel",
	EnvVar:  "MCSKEL_CONFIG_HOME",
	Validate: func(p *Profile, requirePassword bool) error {
		return p.Base.Validate(profile.ValidateOptions{
			Noun:            "template platform",
			PasswordEnvVar:  "MCSKEL_PASSWORD",
			RequirePassword: requirePassword,
		})
	},
	PrepareSave: func(p *Profile) error { return p.Base.PrepareSave() },
	ProfileName: func(p *Profile) string { return p.Base.Name },
})

// fp is the profile fingerprint calculator: it excludes volatile metadata
// (login-state fields) and feeds the read-then-write guard for the pre-write
// comparison.
var fp = fingerprint.New(map[string]bool{
	"cookies": true, "cookie_origin": true, "login_at": true, "password": true,
})

// guard is this CLI's read-then-write guard: error codes and copy are
// injected once and reused everywhere.
var guard = writeguard.Guard{
	CodeMissing:  "confirmation_required",
	CodeMismatch: "conflict",
	ExitMissing:  output.ExitUsage,
	ExitMismatch: output.ExitConflict,
	MsgMissing:   "Blocked '{action}': --version was not provided; you have not read the profile's current state first",
	MsgMismatch:  "Blocked '{action}': --version does not match the profile's current state",
	HintMissing:  "Read-then-write protection: you must read before writing. Run profile show to get the version, then retry with --version",
	HintMismatch: "The profile may have changed since you read it. Run profile show again and retry with the latest version",
}

func newProfileCommand() *cobra.Command {
	root := &cobra.Command{Use: "profile", Short: "Manage multiple profile credentials"}
	root.AddCommand(newProfileListCommand(), newProfileAddCommand(), newProfileRemoveCommand(), newProfileUpdateCommand(), newProfileShowCommand())
	return root
}

func newProfileListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List saved profiles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		names, err := store.List()
		if err != nil {
			return err
		}
		current, _ := store.LoadRoot()
		currentName := ""
		if current != nil {
			currentName = current.CurrentProfile
		}
		data := map[string]any{"profiles": names, "current": currentName}
		return appframe.Respond(cmd, data, fmt.Sprintf("%d profile(s) (current: %s)", len(names), currentName), output.Meta{})
	}}
}

func newProfileAddCommand() *cobra.Command {
	var name, server, username string
	command := &cobra.Command{Use: "add", Short: "Add a profile (password via environment variable or hidden input)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		password, err := prompt.ReadPassword(prompt.PasswordOptions{
			EnvVars: []string{"MCSKEL_PASSWORD"}, Prompt: "Password for the template platform:",
		})
		if err != nil {
			return appframe.CommandError("auth_input_unavailable", "Unable to read the password", "Set the MCSKEL_PASSWORD environment variable and run in an interactive terminal", output.ExitAuth)
		}
		p := &Profile{}
		p.Name, p.Server, p.Username, p.Password = name, server, username, password
		if err := store.Save(p, true); err != nil {
			return err
		}
		data := map[string]any{"name": name, "server": p.Server, "saved": true}
		return appframe.Respond(cmd, data, "Profile saved (password stored with 0600 permissions; re-login happens automatically once the session expires)", output.Meta{})
	}}
	command.Flags().StringVar(&name, "name", "", "Profile name")
	command.Flags().StringVar(&server, "server", "", "Platform server address")
	command.Flags().StringVar(&username, "username", "", "Login username")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("server")
	return command
}

// newProfileShowCommand is the canonical "read" pattern: output the object
// plus its version fingerprint.
func newProfileShowCommand() *cobra.Command {
	var name string
	command := &cobra.Command{Use: "show", Short: "Show a profile (includes the read-then-write version)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadProfile(name)
		if err != nil {
			return err
		}
		version, err := fp.Of(p)
		if err != nil {
			return err
		}
		data := map[string]any{"name": p.Name, "server": p.Server, "username": p.Username, "version": version}
		return appframe.Respond(cmd, data, fmt.Sprintf("Profile %s (version %s)", p.Name, version), output.Meta{})
	}}
	command.Flags().StringVar(&name, "name", "", "Profile name")
	_ = command.MarkFlagRequired("name")
	return command
}

// newProfileUpdateCommand is the canonical "write" pattern: the three-stage
// read-then-write guard check.
func newProfileUpdateCommand() *cobra.Command {
	var name, server, username, userVersion string
	command := &cobra.Command{Use: "update", Short: "Modify a profile (read-then-write, requires --version)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadProfile(name)
		if err != nil {
			return err
		}
		latest, err := fp.Of(p)
		if err != nil {
			return err
		}
		// Object identity echo + action description + a copy-paste read
		// command — all assembled by the shared guard.
		target := map[string]any{"name": p.Name, "server": p.Server, "username": p.Username}
		if err := guard.Check(userVersion, latest, target, "update profile", "mcskel profile show --name "+p.Name); err != nil {
			return err
		}
		if server != "" {
			p.Server = server
		}
		if username != "" {
			p.Username = username
		}
		if err := store.Save(p, false); err != nil {
			return err
		}
		newVersion, _ := fp.Of(p)
		data := map[string]any{"name": p.Name, "server": p.Server, "username": p.Username, "version": newVersion}
		return appframe.Respond(cmd, data, fmt.Sprintf("Profile updated (new version %s)", newVersion), output.Meta{})
	}}
	command.Flags().StringVar(&name, "name", "", "Profile name")
	command.Flags().StringVar(&server, "server", "", "New server address")
	command.Flags().StringVar(&username, "username", "", "New username")
	command.Flags().StringVar(&userVersion, "version", "", "Read-then-write token: the version field from profile show output (required; missing values are blocked by the guard)")
	_ = command.MarkFlagRequired("name")
	return command
}

func newProfileRemoveCommand() *cobra.Command {
	var name string
	command := &cobra.Command{Use: "remove", Short: "Delete a profile", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := store.Remove(name); err != nil {
			return err
		}
		return appframe.Respond(cmd, map[string]any{"removed": name}, "Deleted "+name, output.Meta{})
	}}
	command.Flags().StringVar(&name, "name", "", "Profile name")
	_ = command.MarkFlagRequired("name")
	return command
}

// loadProfile resolves the target profile: explicit --name → the only
// profile → the current profile.
func loadProfile(name string) (*Profile, error) {
	if strings.TrimSpace(name) != "" {
		return store.Load(name)
	}
	names, err := store.List()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, appframe.CommandErrorWithNext("profile_not_found", "No profiles exist", "Run profile add first to save credentials", output.ExitNotFound, "mcskel profile add")
	}
	if len(names) > 1 {
		return nil, appframe.CommandErrorWithNext("profile_ambiguous", "Multiple profiles exist; --name is required", "Run profile list to see all profiles", output.ExitUsage, "mcskel profile list")
	}
	return store.Load(names[0])
}
