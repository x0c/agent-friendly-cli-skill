// Package cmd is mcskel's command layer. This file shows the standard entry
// point wiring for a new CLI: all boilerplate (error→exit-code mapping,
// envelope output, the version command) comes from the shared appframe and
// versioncmd packages; the project only declares its command name, platform
// copy, and the subcommand list.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/versioncmd"
)

// The version triplet is injected via ldflags by the Makefile (automatic once
// you include ../build.mk).
var (
	version     = "v0.1.0"
	buildCommit = "dev"
	buildTime   = "unknown"
)

// platformNotice is the platform-adaptation notice shown to users; each CLI
// should rewrite it to reflect the real risks of its target platform.
const platformNotice = "(Template notice) This CLI adapts to its internal platform; a platform upgrade may change APIs or page structure, and the tool will need to be updated to match."

// NewRootCommand builds the root command (embeddable in tests).
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "mcskel",
		Short:         "Starter template for a new CLI",
		Long:          "mcskel is a starter template for new CLIs. It demonstrates the standard way to consume the shared packages and emits a stable JSON envelope.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", false, "Output a stable JSON envelope")
	root.AddCommand(
		newProfileCommand(),
		newLoginCommand(),
		newDoctorCommand(),
		versioncmd.New(versioncmd.Options{
			CommandName:  "mcskel",
			Version:      version,
			BuildCommit:  buildCommit,
			BuildTime:    buildTime,
			Platform:     "Example Platform",
			Notice:       platformNotice,
			Capabilities: []string{"profile_list", "profile_add", "profile_remove", "profile_update", "login", "doctor", "version"},
			ReadOnly:     false,
		}),
	)
	return root
}

// Execute runs the real terminal command and returns a stable exit code.
//
// The template does not enable self-update (it is not part of the published
// dist artifacts). Once this CLI is officially released and its artifacts are
// committed to <project>/dist/ in this repository, enable the self-update
// injection below per the comments, and add the cli-autoupdate require and
// replace directives to go.mod (copy from the reference CLI in this repository):
//
//	if autoupdate.MaybeWorker() {
//		return 0
//	}
//	autoupdate.Start(autoupdate.Options{
//		CommandName:     "mcskel",
//		RepoSubdir:      "cli-skeleton",
//		GitURL:          "ssh://git@your-gitlab.example.com/your-org/your-cli.git", // repository holding the prebuilt artifacts, required
//		LocalVersion:    version,
//		SkillName:       "mcskel",
//		SkillRepoMarker: "your-org/skills", // marker of the skills repo hosting the companion skill, optional
//	})
func Execute() int {
	return appframe.Execute(NewRootCommand(), os.Args[1:], os.Stdout, os.Stderr)
}
