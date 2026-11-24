// Package fingerprint implements the fingerprint algorithm behind the
// read-before-write optimistic lock: an object is normalized into a
// deterministic JSON form (declared fields excluded, keys sorted), and the
// first N hex characters of its sha256 are taken as the fingerprint
// (12 by default).
//
// Each CLI only declares its own exclusion list (volatile metadata and
// self-injected fields); the algorithm and serialization live here, so
// individual projects must not roll their own canonicalize. See the
// writeguard package for how this is wired into write validation.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// defaultLen is the default fingerprint length: the first 12 hex chars of sha256.
const defaultLen = 12

// Fingerprinter is a fingerprint calculator bound to a set of excluded fields.
type Fingerprinter struct {
	exclude map[string]bool
	length  int
}

// New creates a fingerprint calculator. exclude holds the top-level field
// names that do not participate in the fingerprint (volatile metadata such as
// updateTime/updaterName, plus the version field the CLI injects itself).
func New(exclude map[string]bool) *Fingerprinter {
	m := make(map[string]bool, len(exclude))
	for k, v := range exclude {
		m[k] = v
	}
	return &Fingerprinter{exclude: m, length: defaultLen}
}

// Of computes the fingerprint of an arbitrary object (struct or map).
func (f *Fingerprinter) Of(obj any) (string, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return f.ofRaw(raw)
}

// OfRaw computes the fingerprint of raw JSON (e.g., the d field returned by a
// platform) for callers that bypass structs.
func (f *Fingerprinter) OfRaw(raw json.RawMessage) (string, error) {
	return f.ofRaw(raw)
}

func (f *Fingerprinter) ofRaw(raw []byte) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	stripped := f.strip(m)
	canonical := canonicalize(stripped)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])[:f.length], nil
}

func (f *Fingerprinter) strip(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if f.exclude[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// canonicalize recursively serializes a value into deterministic JSON with
// sorted keys.
func canonicalize(v any) []byte {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, _ := json.Marshal(k)
			buf = append(buf, kb...)
			buf = append(buf, ':')
			buf = append(buf, canonicalize(val[k])...)
		}
		buf = append(buf, '}')
		return buf
	case []any:
		buf := []byte{'['}
		for i, item := range val {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, canonicalize(item)...)
		}
		buf = append(buf, ']')
		return buf
	default:
		b, _ := json.Marshal(val)
		return b
	}
}
