// Package writeguard consolidates the error assembly for read-before-write
// (optimistic lock) validation: a missing version yields
// confirmation_required (exit code 2); a version that does not match the
// current fingerprint yields conflict (exit code 5); a match passes through.
//
// The fingerprint algorithm is deliberately kept out of this package: two
// legacy CLIs compute fingerprints in a way that is not isomorphic to
// cli-common/fingerprint, and switching the algorithm invalidates all
// historical version credentials — so each project computes its own
// fingerprint and passes it here for comparison.
//
// Error codes, messages and hints are injected via Guard: one legacy CLI
// keeps old-style uppercase codes (locked by compat tests), while newer
// projects use lowercase codes — do not unify them opportunistically. The
// standard context keys are target / current_version / version_you_had /
// version_latest; extra keys (e.g., a bilingual reason note injected by
// another legacy CLI) are assembled by the caller inside target.
package writeguard

import (
	"strings"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
)

// Guard declares the per-project variations of a write validation. Messages
// and hints are templates supporting the {action} placeholder (an action
// description such as "modify the producer" or "stop the job").
type Guard struct {
	CodeMissing  string // stable error code for a missing version (e.g., confirmation_required)
	CodeMismatch string // stable error code for a mismatched version (e.g., conflict)
	ExitMissing  int    // exit code for a missing version (convention: 2)
	ExitMismatch int    // exit code for a mismatched version (convention: 5)
	MsgMissing   string // message template for a missing version (contains {action})
	MsgMismatch  string // message template for a mismatched version (contains {action})
	HintMissing  string // hint template for a missing version (contains {action})
	HintMismatch string // hint template for a mismatched version (contains {action})
}

// Check runs the three-stage validation and assembles a self-explanatory
// error aimed at agents.
//
// userVersion is the value of the --version flag; latest is the current
// fingerprint just fetched and recomputed before the write (computed by the
// project itself); target restates the identity of the object being operated
// on for cross-checking (id/code/name/status, assembled by the project);
// action is the action description (fills {action} in message/hint); getCmd
// is a copy-pasteable read command (goes into next_commands).
func (g Guard) Check(userVersion, latest string, target map[string]any, action, getCmd string) error {
	if userVersion == "" {
		return appframe.CommandErrorWithCtx(
			g.CodeMissing,
			expand(g.MsgMissing, action),
			expand(g.HintMissing, action),
			g.ExitMissing,
			[]string{getCmd},
			map[string]any{"target": target, "current_version": latest},
		)
	}
	if userVersion != latest {
		return appframe.CommandErrorWithCtx(
			g.CodeMismatch,
			expand(g.MsgMismatch, action),
			expand(g.HintMismatch, action),
			g.ExitMismatch,
			[]string{getCmd},
			map[string]any{"target": target, "version_you_had": userVersion, "version_latest": latest},
		)
	}
	return nil
}

// expand expands the {action} placeholder in a template.
func expand(template, action string) string {
	return strings.ReplaceAll(template, "{action}", action)
}
