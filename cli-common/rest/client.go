package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Options controls the network boundaries of a Client. BaseURL is the request
// base address (e.g. https://api.example.com). When Timeout or MaxBody is
// less than or equal to zero, defaults (30s / 8MB) apply so that every call
// is subject to a forced timeout and a body limit.
type Options struct {
	BaseURL       string
	Timeout       time.Duration
	Jar           http.CookieJar
	UserAgent     string
	MaxBody       int64
	Transport     http.RoundTripper
	CheckRedirect func(req *http.Request, via []*http.Request) error
}

// Client is an HTTP client skeleton bound to a single service address.
type Client struct {
	baseURL   string
	http      *http.Client
	userAgent string
	maxBody   int64
}

// New creates the client. It does not validate BaseURL (construction errors
// are deferred to the first request; callers can combine this with their
// existing "NewClient returns error" semantics as needed).
func New(opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxBody := opts.MaxBody
	if maxBody <= 0 {
		maxBody = DefaultMaxBody
	}
	httpClient := &http.Client{
		Timeout:       timeout,
		Jar:           opts.Jar,
		Transport:     opts.Transport,
		CheckRedirect: opts.CheckRedirect,
	}
	return &Client{
		baseURL:   strings.TrimRight(opts.BaseURL, "/"),
		http:      httpClient,
		userAgent: opts.UserAgent,
		maxBody:   maxBody,
	}
}

// HTTP returns the underlying http.Client for scenarios that need the raw
// client, such as WebSocket dialing.
func (c *Client) HTTP() *http.Client { return c.http }

// URL joins the full request address: if path starts with http:// or
// https:// it is returned as-is; otherwise it is appended to BaseURL.
func (c *Client) URL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return c.baseURL + "/" + strings.TrimLeft(path, "/")
}

// MaxBody returns the response body limit.
func (c *Client) MaxBody() int64 { return c.maxBody }

// NewRequest builds a request and uniformly sets User-Agent and Content-Type
// (when the body is non-empty). Other headers (Accept, X-Requested-With and
// other platform-specific items) are left to the caller.
func (c *Client) NewRequest(ctx context.Context, method, path string, body []byte, contentType string) (*http.Request, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL(path), reader)
	if err != nil {
		return nil, fmt.Errorf(msgCreateRequestFailed, err)
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	return req, nil
}

// Do executes the request and classifies transport-level errors. Status-code
// semantics (including 3xx redirects and non-2xx errors) are left entirely to
// the caller: on success the raw response is returned and its Body must be
// closed by the caller; use ReadBody when the full response body is needed.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, transportError(err)
	}
	return resp, nil
}

// ReadBody reads the full response body up to the configured limit (erroring
// out when the limit is exceeded) and closes it.
func (c *Client) ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, fmt.Errorf(msgReadResponseBodyFailed, err)
	}
	if int64(len(data)) > c.maxBody {
		return nil, fmt.Errorf(msgBodyExceedsLimit, c.maxBody)
	}
	return data, nil
}

// Decoder is the per-platform response envelope decoder: it takes the HTTP
// status code and the (limit-read) response body, and returns either the
// business data or an error. Status-code semantics are fully decided by the
// decoder: a platform may carry a business error inside a 200, or parseable
// error details inside a 4xx.
type Decoder func(status int, body []byte) (json.RawMessage, error)

// DoJSON assembles and executes a JSON request: marshal the body → build the
// request (Content-Type and Accept both application/json) → execute →
// read with limit → hand off to dec for envelope decoding.
func (c *Client) DoJSON(ctx context.Context, method, path string, body any, dec Decoder) (json.RawMessage, error) {
	var payload []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf(msgMarshalBodyFailed, err)
		}
		payload = raw
	}
	req, err := c.NewRequest(ctx, method, path, payload, "application/json")
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	data, err := c.ReadBody(resp)
	if err != nil {
		return nil, err
	}
	return dec(resp.StatusCode, data)
}
