package medenvelope

import "testing"

// TestParseOKCode is table-driven coverage of every input shape of m
// (numeric / quoted / other integers / missing / non-integer), locking in
// the OK and Code decisions (declared boundaries 1 and 2 in the package
// docs).
func TestParseOKCode(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantOK   bool
		wantCode int
		codeOK   bool
	}{
		{"numeric 1 succeeds", `{"m":1,"e":"","d":{}}`, true, 1, true},
		{"string 1 succeeds", `{"m":"1","e":"","d":{}}`, true, 1, true},
		{"numeric 2 business failure", `{"m":2,"e":"x","d":null}`, false, 2, true},
		{"string 2 business failure", `{"m":"2","e":"x","d":null}`, false, 2, true},
		{"m missing", `{"e":"x","d":null}`, false, 0, false},
		{"non-integer 2.5", `{"m":2.5,"e":"x","d":null}`, false, 0, false},
		{"non-integer 1.0", `{"m":1.0,"e":"x","d":null}`, false, 0, false},
		{"scientific notation", `{"m":1e3,"e":"x","d":null}`, false, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := Parse([]byte(tc.body))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if env.OK() != tc.wantOK {
				t.Errorf("OK() = %v, want %v (body=%s)", env.OK(), tc.wantOK, tc.body)
			}
			code, ok := env.Code()
			if ok != tc.codeOK || code != tc.wantCode {
				t.Errorf("Code() = (%d, %v), want (%d, %v)", code, ok, tc.wantCode, tc.codeOK)
			}
		})
	}
}

// TestData locks in d's pass-through semantics: missing yields nil, null and
// normal objects are both returned verbatim; empty-value fallback semantics
// are left to the adopting project.
func TestData(t *testing.T) {
	env, err := Parse([]byte(`{"m":1,"e":""}`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if env.Data() != nil {
		t.Errorf("Data() should be nil when d is missing, got %q", string(env.Data()))
	}

	env, err = Parse([]byte(`{"m":1,"d":null}`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if string(env.Data()) != "null" {
		t.Errorf("a null d should be returned verbatim, got %q", string(env.Data()))
	}

	env, err = Parse([]byte(`{"m":1,"d":{"list":[]}}`))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if string(env.Data()) != `{"list":[]}` {
		t.Errorf("a normal-object d should be returned verbatim, got %q", string(env.Data()))
	}
}
