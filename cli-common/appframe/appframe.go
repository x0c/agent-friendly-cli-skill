// Package appframe consolidates the entry-point wiring layer for the current
// generation of CLIs: error→exit-code mapping, JSON/human dual-mode output
// detection, and unified command-level error construction. These functions
// were previously duplicated verbatim (~90 lines) across four legacy CLIs,
// where pairwise diffs came down to command names and wording.
//
// This package deliberately does not depend on autoupdate: the self-update
// injection order (MaybeWorker → Start → Execute) stays in each project's
// entry point, keeping the shared package decoupled from the updater
// lifecycle.
package appframe

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

// cobra returns subcommands in alphabetical order by default (Commands()
// sorts in place on first call, controlled by a global switch with no local
// escape hatch). The unknown_command contract requires available_commands to
// enumerate the command surface in registration order, so any CLI importing
// this package disables sorting; the side effect is that --help also lists
// commands in registration order (each CLI's registration order is the
// author's intended command surface, e.g. query first and version last,
// consistent with the version command's capabilities list).
func init() {
	cobra.EnableCommandSorting = false
}

// Execute runs the root command and returns a stable exit code for tests and
// embedded callers; the caller can inject independent input/output streams.
func Execute(root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	if err == nil {
		return output.ExitOK
	}
	appErr := executeError(root, err)
	if MachineOutput(root, args, stdout) {
		_ = output.Failure(stdout, appErr)
	} else {
		output.HumanError(stderr, appErr)
	}
	return appErr.Exit
}

// executeError is the error classification entry point for Execute: on top of
// the generic classification done by AsAppError, unknown-command errors
// additionally carry available_commands context (enumerating it requires the
// root command).
func executeError(root *cobra.Command, err error) *output.AppError {
	var appErr *output.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if typed, path, suggestions, ok := parseUnknownCommand(err.Error()); ok {
		return newUnknownCommandError(root, typed, path, suggestions)
	}
	return AsAppError(err)
}

// AsAppError classifies an arbitrary error as an AppError: AppError values
// are returned as-is; cobra unknown-command errors map to unknown_command
// (exit code 2); cobra usage errors map to invalid_usage; everything else
// maps to operation_failed.
// Note: this function has no root command context, so unknown_command carries
// no available_commands (local funnels such as mytool's take this path);
// entry points going through appframe.Execute fill it in.
func AsAppError(err error) *output.AppError {
	var appErr *output.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if typed, path, suggestions, ok := parseUnknownCommand(err.Error()); ok {
		return newUnknownCommandError(nil, typed, path, suggestions)
	}
	if strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "requires") || strings.Contains(err.Error(), "argument") {
		return output.NewError("invalid_usage", msgInvalidUsageMessage, err.Error(), output.ExitUsage)
	}
	return output.NewError("operation_failed", err.Error(), msgOperationFailedHint, output.ExitGeneral)
}

// unknownCommandPrefix is the fixed prefix of cobra's unknown-command error
// (cobra's legacyArgs and NoArgs both produce the same shape: unknown
// command %q for %q[suggestions section]).
const unknownCommandPrefix = `unknown command "`

// parseUnknownCommand parses cobra's unknown-command error message and
// extracts the mistyped command name, the command path of the reporting
// level, and the suggestion list (suggestions reuse cobra's built-in
// Levenshtein/prefix matching, extracted directly from the "Did you mean
// this?" section it appends, rather than reinventing fuzzy matching).
// Malformed input (defensive fallback) returns ok=false and defers to the
// generic classification.
func parseUnknownCommand(msg string) (typed, path string, suggestions []string, ok bool) {
	if !strings.HasPrefix(msg, unknownCommandPrefix) {
		return "", "", nil, false
	}
	rest := msg[len(unknownCommandPrefix):]
	idx := strings.Index(rest, `" for "`)
	if idx < 0 {
		return "", "", nil, false
	}
	typed = unquoteCommandToken(rest[:idx])
	// The command path ends with a quote and may be directly followed by the
	// suggestions section (\n\nDid you mean this?…).
	after := rest[idx+len(`" for "`):]
	end := strings.IndexByte(after, '"')
	if end < 0 {
		return "", "", nil, false
	}
	path = unquoteCommandToken(after[:end])
	return typed, path, parseDidYouMean(after[end+1:]), true
}

// unquoteCommandToken restores a command name/path escaped by cobra's %q
// (including special characters such as quotes).
func unquoteCommandToken(token string) string {
	if unquoted, err := strconv.Unquote(`"` + token + `"`); err == nil {
		return unquoted
	}
	return token
}

// parseDidYouMean extracts the command names from cobra's suggestions
// section (one per line, indented with \t).
func parseDidYouMean(section string) []string {
	var names []string
	for _, line := range strings.Split(section, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "Did you mean") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// newUnknownCommandError builds the structured contract for an unknown
// command: error code unknown_command, exit code 2 (usage error), a message
// carrying the closest suggestion, a hint pointing at <reporting level>
// --help, and a context that echoes the mistyped command name and enumerates
// available_commands (in registration order). When root is nil (the
// AsAppError path without root context) available_commands is omitted.
func newUnknownCommandError(root *cobra.Command, typed, path string, suggestions []string) *output.AppError {
	message := msgUnknownCommandPrefix + typed + "\""
	if len(suggestions) > 0 {
		message += msgClosestSuggestionPrefix + suggestions[0] + msgClosestSuggestionSuffix
	}
	ctx := map[string]any{"unknown_command": typed}
	if root != nil {
		if cmds := availableCommands(resolveCommandPath(root, path)); len(cmds) > 0 {
			ctx["available_commands"] = cmds
		}
	}
	appErr := output.NewError(
		"unknown_command",
		message,
		msgRunHelpPrefix+path+msgRunHelpSuffix,
		output.ExitUsage,
	)
	appErr.Detail.NextCommands = []string{path + " --help"}
	appErr.Detail.Context = ctx
	return appErr
}

// resolveCommandPath splits the command path on spaces (e.g. "mytool logs")
// and walks from the root command down to the level that actually reported
// the error (when NoArgs validation fails at a subcommand level,
// available_commands should enumerate that level's subcommands rather than
// the root's); if any level fails to resolve, it falls back to the root.
func resolveCommandPath(root *cobra.Command, path string) *cobra.Command {
	cmd := root
	for _, part := range strings.Fields(path)[1:] { // first segment is the root command name, skip it
		next := findSubCommand(cmd, part)
		if next == nil {
			return root
		}
		cmd = next
	}
	return cmd
}

// findSubCommand looks up a direct subcommand by name.
func findSubCommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

// availableCommands lists every available subcommand of the reporting level
// in registration order so agents can discover the command surface directly.
// The help/completion commands auto-injected by cobra are framework
// internals rather than product surface and are filtered out; commands with
// declared cobra aliases are annotated with "(aliases: …)".
func availableCommands(cmd *cobra.Command) []string {
	var names []string
	for _, sub := range cmd.Commands() {
		name := sub.Name()
		if name == "help" || name == "completion" {
			continue
		}
		if len(sub.Aliases) > 0 {
			name += msgAliasPrefix + strings.Join(sub.Aliases, msgAliasSeparator) + msgAliasSuffix
		}
		names = append(names, name)
	}
	return names
}

// MachineOutput decides whether overall output goes to machine mode: an
// explicit --json argument, a persistent --json flag on the root, or stdout
// not being a terminal — any one of the three.
func MachineOutput(root *cobra.Command, args []string, stdout io.Writer) bool {
	if requestedMachineOutput(args) {
		return true
	}
	value, err := root.PersistentFlags().GetBool("json")
	return err == nil && value || !output.IsTTY(stdout)
}

// requestedMachineOutput scans the raw arguments for an explicit --json
// (machine-readable errors must be emitted even when command parsing fails,
// so we cannot rely solely on cobra's flag parsing result).
func requestedMachineOutput(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}

// Respond is the unified exit point of the command layer: machine mode
// prints the success envelope, human mode prints readable text. When human
// is empty, human mode prints no body.
func Respond(cmd *cobra.Command, data any, human string, meta output.Meta) error {
	if CommandMachineOutput(cmd) {
		return output.Success(cmd.OutOrStdout(), data, meta)
	}
	if human != "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), human)
		return err
	}
	return nil
}

// CommandMachineOutput decides the output mode of a single command (root
// --json flag or non-terminal).
func CommandMachineOutput(cmd *cobra.Command) bool {
	value, err := cmd.Root().PersistentFlags().GetBool("json")
	return err == nil && value || !output.IsTTY(cmd.OutOrStdout())
}

// CommandError builds a command-layer error with a stable error code and
// exit code.
func CommandError(code, message, hint string, exit int) error {
	return output.NewError(code, message, hint, exit)
}

// CommandErrorWithNext additionally attaches next_commands (copy-pasteable
// follow-up commands) to the error.
func CommandErrorWithNext(code, message, hint string, exit int, next ...string) error {
	appErr := output.NewError(code, message, hint, exit)
	appErr.Detail.NextCommands = next
	return appErr
}

// CommandErrorWithCtx additionally attaches error.context on top of
// next_commands, echoing the identity of the matched object (such as
// app_id/job_id/job_name/current_version) for agents to verify.
func CommandErrorWithCtx(code, message, hint string, exit int, next []string, ctx map[string]any) error {
	appErr := output.NewError(code, message, hint, exit)
	appErr.Detail.NextCommands = next
	appErr.Detail.Context = ctx
	return appErr
}
