package medenvelope

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/rest"
)

// The message-bus CLI's existing injected copy (byte-identical); tests make
// fidelity assertions against it.
const (
	testParsePrefix = "unexpected_response: failed to parse platform response: "
	testFallbackFmt = "platform returned error code m=%d"
)

func newTestDecoder() rest.Decoder {
	return NewRESTDecoder(DecoderOptions{
		ParseErrPrefix:  testParsePrefix,
		CodeFallbackFmt: testFallbackFmt,
	})
}

// TestNewRESTDecoderHTTPError: HTTP >= 400 must return the rest.StatusError sentinel.
func TestNewRESTDecoderHTTPError(t *testing.T) {
	dec := newTestDecoder()
	data, err := dec(500, []byte(`internal error`))
	if data != nil {
		t.Fatalf("want nil data, got %s", data)
	}
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *rest.APIError, got %T: %v", err, err)
	}
	if apiErr.HTTPStatus != 500 || apiErr.Code != "http_500" {
		t.Errorf("want the 500/http_500 sentinel, got status=%d code=%q", apiErr.HTTPStatus, apiErr.Code)
	}
}

// TestNewRESTDecoderParseErr: the parse-failure error = injected prefix +
// underlying json text (via the %w chain), compared byte for byte.
func TestNewRESTDecoderParseErr(t *testing.T) {
	dec := newTestDecoder()
	body := []byte(`not json at all`)
	_, err := dec(200, body)
	if err == nil {
		t.Fatal("want a parse error, got nil")
	}
	want := testParsePrefix + json.Unmarshal(body, &map[string]any{}).Error()
	if err.Error() != want {
		t.Errorf("parse-failure copy mismatch:\ngot:  %s\nwant: %s", err.Error(), want)
	}
}

// TestNewRESTDecoderNonIntegerM: non-integer m takes the parse-failure path
// (the declared divergence D2 text).
func TestNewRESTDecoderNonIntegerM(t *testing.T) {
	dec := newTestDecoder()
	_, err := dec(200, []byte(`{"m":2.5,"e":"x","d":null}`))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	want := testParsePrefix + "m field is not an integer: 2.5"
	if err.Error() != want {
		t.Errorf("non-integer m copy mismatch:\ngot:  %s\nwant: %s", err.Error(), want)
	}
}

// TestNewRESTDecoderAPIError: m != 1 with non-empty e → APIError passes
// through the platform code and message.
func TestNewRESTDecoderAPIError(t *testing.T) {
	dec := newTestDecoder()
	_, err := dec(200, []byte(`{"m":3,"e":"ownerName must not be empty","d":null}`))
	apiErr, ok := err.(*rest.APIError)
	if !ok {
		t.Fatalf("want *rest.APIError, got %T: %v", err, err)
	}
	if apiErr.Errcode != 3 || apiErr.Msg != "ownerName must not be empty" {
		t.Errorf("APIError content mismatch: Errcode=%d Msg=%q", apiErr.Errcode, apiErr.Msg)
	}
}

// TestNewRESTDecoderFallback: empty or whitespace-only e takes the fallback
// copy (aligned with the message-bus CLI's old TrimSpace semantics).
func TestNewRESTDecoderFallback(t *testing.T) {
	dec := newTestDecoder()
	for name, body := range map[string]string{
		"e empty string":    `{"m":3,"e":"","d":null}`,
		"e whitespace only": `{"m":3,"e":"   ","d":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := dec(200, []byte(body))
			apiErr, ok := err.(*rest.APIError)
			if !ok {
				t.Fatalf("want *rest.APIError, got %T: %v", err, err)
			}
			if apiErr.Msg != "platform returned error code m=3" {
				t.Errorf("fallback copy mismatch: %q", apiErr.Msg)
			}
		})
	}
}

// TestNewRESTDecoderMissingM: a missing m keeps the message-bus CLI's old
// semantics — treated as m=0 on the business-failure branch.
func TestNewRESTDecoderMissingM(t *testing.T) {
	dec := newTestDecoder()
	_, err := dec(200, []byte(`{"e":"x","d":null}`))
	apiErr, ok := err.(*rest.APIError)
	if !ok {
		t.Fatalf("want *rest.APIError, got %T: %v", err, err)
	}
	if apiErr.Errcode != 0 || apiErr.Msg != "x" {
		t.Errorf("missing m should be a business failure with m=0: Errcode=%d Msg=%q", apiErr.Errcode, apiErr.Msg)
	}
}

// TestNewRESTDecoderSuccess: m == 1 (including the string form) returns d
// verbatim.
func TestNewRESTDecoderSuccess(t *testing.T) {
	dec := newTestDecoder()
	data, err := dec(200, []byte(`{"m":1,"e":"","d":{"list":[]}}`))
	if err != nil {
		t.Fatalf("want success, got error: %v", err)
	}
	if string(data) != `{"list":[]}` {
		t.Errorf("should return d verbatim, got %s", data)
	}
	// Declared divergence D1: a string-form m is accepted, matching the
	// release CLI's current behavior
	data, err = dec(200, []byte(`{"m":"1","e":"","d":true}`))
	if err != nil {
		t.Fatalf("a string-form m should be accepted (D1), got error: %v", err)
	}
	if string(data) != "true" {
		t.Errorf("should return d verbatim, got %s", data)
	}
}

// TestNewRESTDecoderPrefixWiring: the injected prefix enters the error byte
// for byte (guards against %s/%w concatenation dropping characters).
func TestNewRESTDecoderPrefixWiring(t *testing.T) {
	dec := newTestDecoder()
	_, err := dec(200, []byte(`{`))
	if err == nil || !strings.HasPrefix(err.Error(), testParsePrefix) {
		t.Errorf("the error should start with the injected prefix: %v", err)
	}
}
