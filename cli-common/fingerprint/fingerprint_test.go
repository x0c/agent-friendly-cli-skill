package fingerprint

import (
	"encoding/json"
	"testing"
)

// The fingerprint must guarantee: same input yields same output, key order is
// irrelevant, excluded fields do not participate, and nested structures are
// deterministic.
func TestOf_StableAndKeyOrderIndependent(t *testing.T) {
	f := New(map[string]bool{"updateTime": true, "updaterName": true})
	a, err := f.Of(map[string]any{"name": "order group", "code": "order", "updateTime": "2026-01-01", "updaterName": "Alice"})
	if err != nil {
		t.Fatalf("failed to compute fingerprint: %v", err)
	}
	b, err := f.Of(map[string]any{"updaterName": "Bob", "code": "order", "updateTime": "2026-02-02", "name": "order group"})
	if err != nil {
		t.Fatalf("failed to compute fingerprint: %v", err)
	}
	if a != b {
		t.Fatalf("identical business fields with different volatile metadata should yield the same fingerprint: %s vs %s", a, b)
	}
}

func TestOf_BusinessFieldChangeAltersFingerprint(t *testing.T) {
	f := New(map[string]bool{"updateTime": true})
	a, _ := f.Of(map[string]any{"code": "order", "count": 1})
	b, _ := f.Of(map[string]any{"code": "order", "count": 2})
	if a == b {
		t.Fatalf("a change in business fields must change the fingerprint: %s", a)
	}
}

func TestOf_NestedStructuresAndArrays(t *testing.T) {
	f := New(nil)
	a, _ := f.Of(map[string]any{
		"spec": map[string]any{"z": 1, "a": []any{map[string]any{"y": 2, "x": 3}, 7}},
	})
	b, _ := f.Of(map[string]any{
		"spec": map[string]any{"a": []any{map[string]any{"x": 3, "y": 2}, 7}, "z": 1},
	})
	if a != b {
		t.Fatalf("nested structures with different key order but equal values should yield the same fingerprint: %s vs %s", a, b)
	}
}

func TestOfRaw_EquivalentToOf(t *testing.T) {
	f := New(map[string]bool{"version": true})
	obj := map[string]any{"code": "grp", "version": "abc123"}
	viaOf, _ := f.Of(obj)
	raw, _ := json.Marshal(obj)
	viaRaw, err := f.OfRaw(raw)
	if err != nil {
		t.Fatalf("OfRaw failed: %v", err)
	}
	if viaOf != viaRaw {
		t.Fatalf("Of and OfRaw should be equivalent for the same data: %s vs %s", viaOf, viaRaw)
	}
}

func TestOf_TwelveHexChars(t *testing.T) {
	f := New(nil)
	v, _ := f.Of(map[string]any{"k": "v"})
	if len(v) != 12 {
		t.Fatalf("fingerprint length should be 12, got %d", len(v))
	}
}
