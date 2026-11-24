// Package medenvelope consolidates the {m,e,d} response envelope (shared by
// the release-system CICD CLI and the message-bus platform CLI): m=1 means
// success, e is the platform error message, and d is the raw business data.
//
// This package does only three transport-independent things: parsing, the
// success predicate, and turning m into a number. HTTP status handling and
// error copy assembly are out of scope — they are injected by the adopting
// project, so existing output stays byte-identical.
//
// Declared behavior boundaries (relative to the message-bus CLI's old int
// implementation; all involve input shapes never observed in practice):
//  1. A JSON string-form numeric m (e.g. "m":"1") is accepted, matching the
//     release CLI's current behavior (the old message-bus CLI reported a
//     parse failure); this input shape has never been observed on the
//     message-bus platform, and the change is a tolerance relaxation, not a
//     tightening.
//  2. When m cannot be turned into an integer, the "parse failure" path is
//     taken instead of the json library's cannot unmarshal text: for
//     non-integers (e.g. 2.5, 1e3) and m beyond int range, the error text is
//     uniformly the injected prefix + "m field is not an integer: <raw>";
//     non-numeric literals that cannot be numericized (e.g. true, "abc")
//     still take the parse-failure path and keep the underlying json text,
//     with only the Go destination type name in it changing from int to
//     json.Number as the implementation changed.
//  3. A missing m (including null) keeps the message-bus CLI's old
//     semantics: treated as m=0 on the business-failure branch, without a
//     parse error.
package medenvelope

import (
	"encoding/json"
	"strconv"
)

// Envelope is the unified representation of the {m,e,d} envelope.
// M uses json.Number instead of int: the release-system side may return a
// quoted numeric literal "1", and json.Number accepts both numbers and
// string-form numbers (aligned with the release CLI's existing behavior).
type Envelope struct {
	M json.Number     `json:"m"`
	E string          `json:"e"`
	D json.RawMessage `json:"d"`
}

// Parse parses the response body (a thin passthrough wrapper over
// json.Unmarshal; errors are returned as-is so the adopting project can wrap
// them into its own copy with %w).
func Parse(data []byte) (*Envelope, error) {
	env := &Envelope{}
	if err := json.Unmarshal(data, env); err != nil {
		return nil, err
	}
	return env, nil
}

// OK reports business success: the numeric literal of m equals "1"
// (byte-identical to the release CLI's existing check — both the number 1
// and the quoted "1" succeed; other shapes such as "1.0" fail).
func (e *Envelope) OK() bool {
	return e.M.String() == "1"
}

// Code returns the int representation of the business code (for
// rest.APIError.Errcode). Returns (0, false) when m is missing, non-integer,
// or out of int range.
func (e *Envelope) Code() (int, bool) {
	if e.M == "" {
		return 0, false
	}
	i, err := strconv.ParseInt(e.M.String(), 10, 0)
	if err != nil {
		return 0, false
	}
	return int(i), true
}

// Data returns the raw business data; nil when d is missing. Whether d being
// null counts as empty and how empty values fall back is decided by the
// caller (semantics like the release CLI's "treat a null d as an empty
// object" stay inside that project).
func (e *Envelope) Data() json.RawMessage {
	return e.D
}
