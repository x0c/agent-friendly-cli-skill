// Package versioncmd provides the standard version command: it reports the
// local version, build information, the stable contract version, and the
// capability list. Several projects used to implement this independently
// with an identical output field set — the only differences were the command
// name, the capabilities list, and the platform notice — so it was
// consolidated into this package.
package versioncmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// Options is injected by each project: the version triple comes from
// ldflags-injected variables; Platform/Notice/Capabilities are the
// project-specific parts; ReadOnly marks read-only tools (such as mytool).
type Options struct {
	CommandName  string
	Version      string
	BuildCommit  string
	BuildTime    string
	Platform     string   // platform description text (goes into data.platform)
	Notice       string   // platform notice text (goes into data.notice and human output)
	Capabilities []string // capability list (command names)
	ReadOnly     bool
}

// New builds the standard version command. The output envelope field set is
// identical to the pre-existing implementations:
// version/build_commit/build_time/schema_version/read_only/platform/
// notice/capabilities.
func New(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: msgVersionShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			data := map[string]any{
				"version":        opts.Version,
				"build_commit":   opts.BuildCommit,
				"build_time":     opts.BuildTime,
				"schema_version": output.SchemaVersion,
				"read_only":      opts.ReadOnly,
				"platform":       opts.Platform,
				"notice":         opts.Notice,
				"capabilities":   opts.Capabilities,
			}
			human := fmt.Sprintf(msgHumanVersionLine,
				opts.CommandName, opts.Version, opts.BuildCommit, output.SchemaVersion, opts.Notice)
			return appframe.Respond(cmd, data, human, output.Meta{})
		},
	}
}
