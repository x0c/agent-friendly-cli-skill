package profilecmd

// The four isomorphic subcommands list / show / use / remove: use/remove are
// byte-identical across the projects, and list/show differences are purely
// presentational (column widths/column sets/one-line messages), absorbed
// through the View injection.

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// NewListCommand assembles the list subcommand: data is the profiles summary
// list + current_profile, and human mode prints the table rows joined from
// View.ListLine.
func NewListCommand[P any](store Store[P], view View[P]) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List local profiles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := store.LoadRoot()
		if err != nil {
			return err
		}
		names, err := store.List()
		if err != nil {
			return err
		}
		items := make([]map[string]any, 0, len(names))
		lines := make([]string, 0, len(names))
		for _, name := range names {
			profile, err := store.Load(name)
			if err != nil {
				return err
			}
			current := root.CurrentProfile == name
			items = append(items, view.Public(profile, current))
			lines = append(lines, view.ListLine(profile, current))
		}
		return appframe.Respond(cmd, map[string]any{"profiles": items, "current_profile": root.CurrentProfile}, strings.Join(lines, "\n"), output.Meta{})
	}}
}

// NewShowCommand assembles the show subcommand: data is a single profile
// summary (with the current marker), and human mode prints View.ShowLine.
func NewShowCommand[P any](store Store[P], view View[P], commandName, short string) *cobra.Command {
	return &cobra.Command{Use: "show <name>", Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := store.LoadRoot()
		if err != nil {
			return err
		}
		profile, err := store.Load(args[0])
		if err != nil {
			return MapProfileNotFound(err, commandName)
		}
		return appframe.Respond(cmd, view.Public(profile, root.CurrentProfile == args[0]), view.ShowLine(profile), output.Meta{})
	}}
}

// NewUseCommand assembles the use subcommand: it sets the root config's
// current_profile.
func NewUseCommand[P any](store Store[P], commandName string) *cobra.Command {
	return &cobra.Command{Use: "use <name>", Short: "Set the current profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := store.Load(args[0]); err != nil {
			return MapProfileNotFound(err, commandName)
		}
		root, err := store.LoadRoot()
		if err != nil {
			return err
		}
		root.CurrentProfile = args[0]
		if err := store.SaveRoot(root); err != nil {
			return err
		}
		return appframe.Respond(cmd, map[string]any{"current_profile": args[0]}, fmt.Sprintf("Current profile: %s", args[0]), output.Meta{})
	}}
}

// NewRemoveCommand assembles the remove subcommand: interactive confirmation
// under a TTY (non-yes → cancelled/ExitGeneral), --yes skips it; non-TTY
// deletes directly (historical behavior, kept). After removing the current
// profile it best-effort clears current_profile (failures are silent,
// historical behavior).
func NewRemoveCommand[P any](store Store[P]) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "remove <name>", Short: "Remove a profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes && output.IsTTY(os.Stdin) {
			fmt.Fprintf(cmd.OutOrStdout(), "Delete profile %q? Type yes to continue: ", args[0])
			var answer string
			fmt.Fscanln(os.Stdin, &answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "yes" {
				return appframe.CommandError("cancelled", "Deletion cancelled", "pass --yes explicitly to delete", output.ExitGeneral)
			}
		}
		if err := store.Remove(args[0]); err != nil {
			return err
		}
		root, err := store.LoadRoot()
		if err == nil && root.CurrentProfile == args[0] {
			root.CurrentProfile = ""
			_ = store.SaveRoot(root)
		}
		return appframe.Respond(cmd, map[string]any{"status": "removed", "profile": args[0]}, fmt.Sprintf("Deleted profile %q", args[0]), output.Meta{})
	}}
	command.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	return command
}
