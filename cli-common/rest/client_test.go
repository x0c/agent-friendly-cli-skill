package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// envelopeDec simulates a project-side envelope decoder ({m,e,d} structure,
// m==1 means success).
func envelopeDec(status int, body []byte) (json.RawMessage, error) {
	if status >= 400 {
		return nil, StatusError(status, body)
	}
	var env struct {
		M int             `json:"m"`
		E string          `json:"e"`
		D json.RawMessage `json:"d"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("unexpected_response: failed to parse platform response: %w", err)
	}
	if env.M != 1 {
		return nil, &APIError{Kind: KindUnknown, Errcode: env.M, Msg: env.E}
	}
	return env.D, nil
}

func TestDoJSON_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type not set to application/json, got %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept not set to application/json")
		}
		if r.Header.Get("User-Agent") != "test-cli" {
			t.Errorf("User-Agent not injected")
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"m":1,"e":"","d":{"ok":true}}`))
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL, UserAgent: "test-cli"})
	raw, err := client.DoJSON(context.Background(), http.MethodPost, "/api/mytool/echo", map[string]any{"k": "v"}, envelopeDec)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || !payload.OK {
		t.Fatalf("unexpected envelope decode result: %v / %+v", err, payload)
	}
}

func TestDoJSON_BusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"m":409,"e":"code already exists","d":null}`))
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL})
	_, err := client.DoJSON(context.Background(), http.MethodGet, "/x", nil, envelopeDec)
	if err == nil {
		t.Fatal("business error not returned")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not an APIError: %T", err)
	}
	if apiErr.Errcode != 409 || apiErr.Msg != "code already exists" {
		t.Fatalf("unexpected business code/message: %+v", apiErr)
	}
}

func TestDoJSON_MarshalFailure(t *testing.T) {
	client := New(Options{BaseURL: "http://127.0.0.1:1"})
	_, err := client.DoJSON(context.Background(), http.MethodPost, "/x", func() {}, envelopeDec)
	if err == nil || !strings.Contains(err.Error(), "failed to marshal request body") {
		t.Fatalf("marshal failure not wrapped correctly: %v", err)
	}
}

func TestDoJSON_HTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL})
	_, err := client.DoJSON(context.Background(), http.MethodGet, "/x", nil, envelopeDec)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not an APIError: %T", err)
	}
	if apiErr.Code != "http_500" || apiErr.Kind != KindUnknown || apiErr.HTTPStatus != 500 {
		t.Fatalf("status error misclassified: %+v", apiErr)
	}
	if !strings.Contains(apiErr.Msg, "server boom") {
		t.Fatalf("error message should contain a response body snippet: %q", apiErr.Msg)
	}
	// Sentinel string-matching compatibility: Error() must render the Code
	// (Msg is rendered when non-empty; StatusError's Msg embeds the HTTP
	// status in the "http_500" prefix format, but to keep existing
	// strings.Contains matching working, we assert the HTTPStatusError's
	// Error text contains the sentinel).
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error text should contain the HTTP status: %q", err.Error())
	}
}

func TestTransportError_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL, Timeout: 30 * time.Millisecond})
	_, err := client.DoJSON(context.Background(), http.MethodGet, "/slow", nil, envelopeDec)
	if KindOf(err) != KindTimeout {
		t.Fatalf("timeout not classified as KindTimeout: %v", err)
	}
}

func TestTransportError_Network(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close() // close immediately to force a connection failure
	client := New(Options{BaseURL: url})
	_, err := client.DoJSON(context.Background(), http.MethodGet, "/x", nil, envelopeDec)
	if KindOf(err) != KindNetwork {
		t.Fatalf("connection failure not classified as KindNetwork: %v", err)
	}
}

func TestReadBody_Limit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 64*1024))
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL, MaxBody: 1024})
	req, err := client.NewRequest(context.Background(), http.MethodGet, "/big", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ReadBody(resp)
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("exceeding the limit did not error: %v", err)
	}
}

func TestURL_Join(t *testing.T) {
	client := New(Options{BaseURL: "https://example.com/sub/"})
	cases := map[string]string{
		"/api/x":         "https://example.com/sub/api/x",
		"api/x":          "https://example.com/sub/api/x",
		"api/x?q=1":      "https://example.com/sub/api/x?q=1",
		"http://o.com/y": "http://o.com/y",
	}
	for path, want := range cases {
		if got := client.URL(path); got != want {
			t.Errorf("URL(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestDo_RawResponseFor2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("stream-data"))
	}))
	defer server.Close()
	client := New(Options{BaseURL: server.URL})
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "/log", nil, "")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := client.ReadBody(resp)
	if err != nil || string(data) != "stream-data" {
		t.Fatalf("unexpected streamed read: %v / %q", err, data)
	}
}

func TestAPIError_ErrorRendering(t *testing.T) {
	cases := []struct {
		err  *APIError
		want string
	}{
		{&APIError{Kind: KindAuth, Code: "authentication_failed"}, "authentication_failed"},
		{&APIError{Kind: KindAuth, Msg: "request timed out"}, "request timed out"},
		{&APIError{Kind: KindNetwork, Msg: "network request failed: x"}, "network request failed: x"},
		{&APIError{Kind: KindUnknown, Errcode: 409}, "errcode=409"},
		{&APIError{Kind: KindUnknown, HTTPStatus: 404}, "http_404"},
		{&APIError{}, "request_failed"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("Error() = %q, want %q", got, c.want)
		}
	}
}

func TestCodeOf_KindOf(t *testing.T) {
	apiErr := &APIError{Kind: KindNotFound, Code: "http_404", HTTPStatus: 404}
	wrapped := fmt.Errorf("outer: %w", apiErr)
	if CodeOf(wrapped) != "http_404" {
		t.Fatalf("CodeOf failed to extract from wrapped error: %q", CodeOf(wrapped))
	}
	if KindOf(wrapped) != KindNotFound {
		t.Fatalf("KindOf failed to extract from wrapped error: %v", KindOf(wrapped))
	}
	if CodeOf(errors.New("x")) != "" || KindOf(errors.New("x")) != KindUnknown {
		t.Fatal("non-APIError should return zero values")
	}
}

func TestDefaultOptions(t *testing.T) {
	client := New(Options{BaseURL: "http://x", Timeout: -1, MaxBody: -1})
	if client.HTTP().Timeout != DefaultTimeout {
		t.Fatalf("default timeout not applied: %v", client.HTTP().Timeout)
	}
	if client.MaxBody() != DefaultMaxBody {
		t.Fatalf("default body limit not applied: %d", client.MaxBody())
	}
}

func TestCheckRedirectPassthrough(t *testing.T) {
	// Legacy web-app scenario: block 302 redirects.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	}))
	defer server.Close()
	client := New(Options{
		BaseURL:       server.URL,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	})
	req, _ := client.NewRequest(context.Background(), http.MethodGet, "/protected", nil, "")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected the raw 302 response, got %d", resp.StatusCode)
	}
}
