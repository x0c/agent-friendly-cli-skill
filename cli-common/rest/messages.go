// All runtime messages of this package are declared here: change wording only
// in this file; for localized distributions, only this file's contents change.
package rest

var (
	msgCreateRequestFailed    = "failed to create request: %w"
	msgReadResponseBodyFailed = "failed to read response body: %w"
	msgBodyExceedsLimit       = "response body exceeds the %d-byte limit"
	msgMarshalBodyFailed      = "failed to marshal request body: %w"
	msgRequestTimeout         = "request timed out"
	msgNetworkRequestFailed   = "network request failed: %v"
	msgHTTPStatusBody         = "HTTP %d: %s"
	msgSnippetEllipsis        = "..."
)
