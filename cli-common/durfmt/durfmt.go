// Package durfmt provides the unified "friendly duration" parsing for all
// CLIs in this repo: on top of Go duration syntax (30m, 24h, 1.5h, 1h30m) it
// supports day-suffix forms (7d, 1.5d, -2d).
//
// The syntax baseline is the Kubernetes CLI's historical implementation (the
// most complete of the three): the day count goes through
// strconv.ParseFloat, so fractional days (1.5d), signed days (+7d, -2d) and
// scientific-notation days (1e2d) all parse, with NaN/Inf/beyond-int64
// overflow checks. In 2026-11 it was consolidated from three duplicated
// implementations (the Kubernetes, log-query and tracing CLIs) for shared
// use by duration flags such as --since / --duration / --compare-baseline;
// no formatting API is provided (none of the three had a formatting use
// case).
package durfmt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Parse parses a friendly duration: TrimSpace first, then branch on "ends
// with d" — the day suffix is parsed as a float and converted to nanoseconds
// (with an overflow check); everything else goes to time.ParseDuration.
// Signs are unconstrained (whether a value must be positive is validated by
// each command per its own semantics); the parsing semantics are
// byte-identical to the Kubernetes CLI's historical implementation.
func Parse(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasSuffix(trimmed, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %q, day-suffix example: 7d", raw)
		}
		nanoseconds := days * float64(24*time.Hour)
		if math.IsNaN(nanoseconds) || math.IsInf(nanoseconds, 0) || nanoseconds > float64(time.Duration(1<<63-1)) || nanoseconds < float64(time.Duration(-1<<63)) {
			return 0, fmt.Errorf("duration out of supported range: %q", raw)
		}
		return time.Duration(nanoseconds), nil
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %q, supported: 30m, 24h or 7d", raw)
	}
	return parsed, nil
}

// Value is a duration flag value that works with pflag. This package does
// not import spf13/pflag (in cli-common it is only an indirect dependency,
// keeping go.mod untouched); it satisfies the pflag.Value interface
// structurally via the method set Set(string) error + String(); callers wire
// it with flags.Var(durfmt.NewValue(&v), "since", "...").
type Value struct {
	target *time.Duration
}

// NewValue builds a duration flag value bound to the target variable.
func NewValue(target *time.Duration) *Value {
	return &Value{target: target}
}

// String returns the current value (the zero duration when unbound), for
// pflag to print defaults.
func (value *Value) String() string {
	if value.target == nil {
		return "0s"
	}
	return value.target.String()
}

// Set parses and writes the target variable; on parse failure the target is
// left unchanged.
func (value *Value) Set(raw string) error {
	parsed, err := Parse(raw)
	if err != nil {
		return err
	}
	*value.target = parsed
	return nil
}

// Type returns the flag type name for --help display.
func (value *Value) Type() string { return "duration" }
