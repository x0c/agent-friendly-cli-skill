// Package rest provides a shared HTTP request skeleton for CLIs: enforced
// timeouts, response body limits, request headers, and typed transport
// errors. Response envelope parsing and business error-code systems differ
// widely across platforms, so they are injected via Decoder and implemented
// in each project.
package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Defaults: every caller gets a forced timeout and a response body limit,
// preventing CLIs from hanging or reading unbounded data into memory.
const (
	DefaultTimeout time.Duration = 30 * time.Second
	DefaultMaxBody int64         = 8 << 20
)

// ErrKind is the coarse error category, which the command layer maps to exit
// codes and structured error.code values.
type ErrKind int

const (
	KindUnknown  ErrKind = iota // generic failure
	KindAuth                    // auth/permission failure
	KindNotFound                // resource not found
	KindConflict                // conflict/already exists
	KindTimeout                 // timed out
	KindNetwork                 // network unreachable
)

// APIError is the unified typed error, covering three scenarios:
//   - transport errors (timeout / network unreachable): only Kind and Msg;
//   - HTTP status errors: Code is the "http_%d" sentinel, HTTPStatus is set;
//   - platform business errors: Errcode is the platform business code and
//     Msg is the platform message.
//
// Code is the externally stable error sentinel (e.g. "authentication_failed",
// "http_404"); the command layer can match on it via CodeOf(err). Error()
// renders Code when Msg is empty, so existing command-layer matching based
// on strings.Contains(err.Error(), sentinel) keeps working.
type APIError struct {
	Kind       ErrKind // coarse error category
	Code       string  // stable error sentinel (may be empty; not rendered when Msg is non-empty)
	Errcode    int     // platform business error code (0 for transport/HTTP-level errors)
	HTTPStatus int     // HTTP status code (0 when not applicable)
	Msg        string  // error description (platform message or human-readable note)
}

// Error implements the error interface. Msg is rendered first when non-empty;
// otherwise Code is rendered; when neither is present, errcode/HTTP status is
// used as a fallback so that every APIError has a readable error text.
func (e *APIError) Error() string {
	switch {
	case e.Msg != "":
		return e.Msg
	case e.Code != "":
		return e.Code
	case e.Errcode != 0:
		return fmt.Sprintf("errcode=%d", e.Errcode)
	case e.HTTPStatus != 0:
		return fmt.Sprintf("http_%d", e.HTTPStatus)
	default:
		return "request_failed"
	}
}

// KindOf extracts the ErrKind of an error; non-APIError errors yield
// KindUnknown.
func KindOf(err error) ErrKind {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Kind
	}
	return KindUnknown
}

// CodeOf extracts the stable sentinel Code of an APIError; non-APIError
// errors yield an empty string. It gives the command layer typed sentinel
// matching equivalent to string matching.
func CodeOf(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}

// StatusError builds the unified sentinel for HTTP status errors: Code is
// "http_%d", Kind is derived from the status code, and Msg carries a body
// snippet (truncated to 200 bytes) for troubleshooting. When the body is
// empty or yields no usable content, Msg contains only the status note.
func StatusError(status int, body []byte) *APIError {
	apiErr := &APIError{
		Kind:       classifyStatus(status),
		Code:       fmt.Sprintf("http_%d", status),
		HTTPStatus: status,
	}
	if snippet := bodySnippet(body); snippet != "" {
		apiErr.Msg = fmt.Sprintf(msgHTTPStatusBody, status, snippet)
	}
	return apiErr
}

// classifyStatus maps an HTTP status code to an ErrKind.
func classifyStatus(status int) ErrKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return KindAuth
	case status == http.StatusNotFound:
		return KindNotFound
	case status == http.StatusConflict:
		return KindConflict
	default:
		return KindUnknown
	}
}

// bodySnippet extracts a readable snippet from the response body for error
// messages.
func bodySnippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	const maxSnippet = 200
	runes := []rune(text)
	if len(runes) > maxSnippet {
		return string(runes[:maxSnippet]) + msgSnippetEllipsis
	}
	return text
}

// transportError classifies net/http transport-level errors into typed
// APIError values.
func transportError(err error) *APIError {
	if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		return &APIError{Kind: KindTimeout, Msg: msgRequestTimeout}
	}
	return &APIError{Kind: KindNetwork, Msg: fmt.Sprintf(msgNetworkRequestFailed, err)}
}

// isTimeout detects timeouts across net error variants.
func isTimeout(err error) bool {
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}
