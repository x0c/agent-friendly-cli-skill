package durfmt

import (
	"testing"
	"time"
)

// TestParse locks in the unified parsing syntax: all Go duration shapes +
// the day suffix (including fractional, signed and scientific notation) +
// overflow and invalid inputs. Expected values were checked one by one
// against the pre-adoption behavior of the three CLIs.
func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		// Go duration shapes (accepted by all three CLIs, same semantics)
		{"plain minutes", "30m", 30 * time.Minute},
		{"plain hours", "24h", 24 * time.Hour},
		{"compound", "1h30m", 90 * time.Minute},
		{"fractional unit", "1.5h", 90 * time.Minute},
		{"milliseconds", "300ms", 300 * time.Millisecond},
		{"zero", "0", 0},
		{"negative duration", "-30m", -30 * time.Minute},
		{"with plus sign", "+5m", 5 * time.Minute},
		// Day suffix (current behavior of the Kubernetes and tracing CLIs;
		// the log-query CLI previously accepted only positive integer days —
		// adopting this package upgrades its capability)
		{"integer days", "7d", 7 * 24 * time.Hour},
		{"fractional days", "1.5d", 36 * time.Hour},
		{"half day", "0.5d", 12 * time.Hour},
		{"zero days", "0d", 0},
		{"negative days", "-2d", -48 * time.Hour},
		{"days with plus sign", "+7d", 7 * 24 * time.Hour},
		{"scientific-notation days", "1e2d", 100 * 24 * time.Hour},
		{"surrounding whitespace", "  7d  ", 7 * 24 * time.Hour},
		// Overflow boundary: the int64 nanosecond cap is about 106751.99 days
		{"max whole days within overflow boundary", "106751d", 106751 * 24 * time.Hour},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpectedly failed: %v", tc.input, err)
			}
			if actual != tc.expected {
				t.Fatalf("Parse(%q) = %s, want %s", tc.input, actual, tc.expected)
			}
		})
	}
}

// TestParseErrors locks in rejection behavior: invalid input and overflow
// (NaN/Inf/beyond the int64 range). The log-query CLI's empty string does
// not take this path (an empty default flag value is not parsed); the caller
// keeps handling it as before.
func TestParseErrors(t *testing.T) {
	invalid := []string{
		"",         // empty string: rejected by all three CLIs as before
		"d",        // bare suffix
		"7D",       // uppercase D is neither a day suffix nor a Go duration unit
		"7xd",      // digits followed by junk characters (the old tracing CLI's Sscanf leniently accepted it; the unified behavior tightens it — see the consolidation report)
		"0x1d",     // hex float missing the p exponent, rejected by ParseFloat
		"one week", // natural language
		"abc",      // plain garbage input
		"7d5h",     // d cannot mix with Go duration units
		"106752d",  // whole days beyond the int64 nanosecond cap
		"1e300d",   // day value overflow
		"-1e300d",  // day value underflow
		"NaNd",     // NaN
		"Infd",     // infinity
	}
	for _, input := range invalid {
		t.Run(input, func(t *testing.T) {
			if d, err := Parse(input); err == nil {
				t.Fatalf("Parse(%q) should be rejected, got %s", input, d)
			}
		})
	}
}

// TestValue locks in the pflag duck-typing behavior: Set writes on success,
// failure leaves the target unchanged, String falls back when the target is
// unbound, and the Type name.
func TestValue(t *testing.T) {
	var target time.Duration
	value := NewValue(&target)
	if value.String() != "0s" {
		t.Fatalf("zero-value String should be 0s, got %s", value.String())
	}
	if value.Type() != "duration" {
		t.Fatalf("Type should be duration, got %s", value.Type())
	}
	if err := value.Set("1.5d"); err != nil {
		t.Fatalf("Set(1.5d) unexpectedly failed: %v", err)
	}
	if target != 36*time.Hour {
		t.Fatalf("after Set the target should be 36h, got %s", target)
	}
	if value.String() != "36h0m0s" {
		t.Fatalf("String should follow the target value, got %s", value.String())
	}
	if err := value.Set("garbage"); err == nil {
		t.Fatal("Set with invalid input must fail")
	}
	if target != 36*time.Hour {
		t.Fatalf("a failed Set must not change the target, got %s", target)
	}
	unbound := &Value{}
	if unbound.String() != "0s" {
		t.Fatalf("String of an unbound target should fall back to 0s, got %s", unbound.String())
	}
}
