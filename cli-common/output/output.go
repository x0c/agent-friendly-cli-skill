// Package output provides the machine output envelope and exit codes shared
// by all CLIs built on this library.
//
// This package is the single source of truth for the output contract: new
// CLIs must import it; existing projects integrate through a thin
// internal/output wrapper (type aliases + function forwarding) instead of
// copying this file.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// Exit code tiers: repository-wide semantics, interpreted by callers
// (including agents and scripts) by code.
const (
	ExitOK        = 0 // success
	ExitGeneral   = 1 // general failure
	ExitUsage     = 2 // usage error / confirmation_required (missing read-before-write precondition such as --version)
	ExitNotFound  = 3 // target not found
	ExitAuth      = 4 // permission / authentication failure
	ExitConflict  = 5 // conflict (version mismatch, duplicate name, etc.)
	ExitTimeout   = 6 // timeout
	SchemaVersion = "v1"
)

// Error is a machine-readable error; Message is human-readable text, while
// Code is a stable lowercase snake_case identifier.
type Error struct {
	Code         string         `json:"code"`
	Message      string         `json:"message"`
	Hint         string         `json:"hint,omitempty"`
	NextCommands []string       `json:"next_commands,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
	Retryable    bool           `json:"retryable"`
}

// Meta holds common metadata that does not belong to the main result.
type Meta struct {
	SchemaVersion string `json:"schema_version"`
	DryRun        bool   `json:"dry_run,omitempty"`
	Partial       bool   `json:"partial,omitempty"`
}

// Envelope is the fixed envelope for all non-streaming machine output.
type Envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data"`
	Error *Error `json:"error"`
	Meta  Meta   `json:"meta"`
}

// AppError separates the program error from the exit code, so callers never
// have to parse prose to determine the outcome.
type AppError struct {
	Detail Error
	Exit   int
}

func (e *AppError) Error() string { return e.Detail.Message }

// NewError creates an application error with a stable error code.
func NewError(code, message, hint string, exit int) *AppError {
	return &AppError{Detail: Error{Code: code, Message: message, Hint: hint}, Exit: exit}
}

// IsTTY reports whether the given stream is attached to a terminal; custom
// writers are always treated as non-interactive.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// WriteJSON writes single-line JSON to stdout, easy for agents and pipes to
// consume.
func WriteJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(v)
}

// Success writes the success envelope.
func Success(w io.Writer, data any, meta Meta) error {
	meta.SchemaVersion = SchemaVersion
	return WriteJSON(w, Envelope{OK: true, Data: data, Error: nil, Meta: meta})
}

// Failure writes the failure envelope; failures must be machine-parseable
// too.
func Failure(w io.Writer, appErr *AppError) error {
	return WriteJSON(w, Envelope{OK: false, Data: nil, Error: &appErr.Detail, Meta: Meta{SchemaVersion: SchemaVersion}})
}

// HumanError prints a concise, human-readable error for people reading the
// terminal directly.
func HumanError(w io.Writer, err error) {
	fmt.Fprintf(w, msgHumanErrorLine, err)
}
