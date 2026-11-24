package medenvelope

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/rest"
)

// DecoderOptions holds the copy-injection parameters for NewRESTDecoder;
// both fields are required. The injected copy must be byte-identical to the
// adopting project's existing output (including colon style and spacing).
type DecoderOptions struct {
	// ParseErrPrefix is the copy prefix for JSON parse failures (including m
	// that cannot be turned into an integer); it must carry its own trailing
	// colon; the error wraps the underlying cause with %w. The message-bus
	// CLI injects "unexpected_response: failed to parse platform response: ".
	ParseErrPrefix string
	// CodeFallbackFmt is the fallback business-error copy when e is empty (or
	// whitespace only); "%d" receives the int value of m. The message-bus CLI
	// injects "platform returned error code m=%d".
	CodeFallbackFmt string
}

// NewRESTDecoder returns a decoder ready to hand to rest.DoJSON:
//   - HTTP >= 400 → rest.StatusError(status, body) (the http_%d sentinel);
//   - parse failure / m that cannot be turned into an integer →
//     ParseErrPrefix + %w (the text for non-integer m is "m field is not an
//     integer: <raw>"; see declared divergence D2 in the package docs);
//   - m != 1 → *rest.APIError{Errcode: m, Msg: e}; when e is empty (or
//     whitespace only), CodeFallbackFmt is used as the fallback (adopters of
//     the release-CLI-style "no fallback" do not use this constructor);
//   - m == 1 → returns d verbatim.
func NewRESTDecoder(opts DecoderOptions) rest.Decoder {
	return func(status int, body []byte) (json.RawMessage, error) {
		if status >= http.StatusBadRequest {
			return nil, rest.StatusError(status, body)
		}
		env, err := Parse(body)
		if err != nil {
			return nil, fmt.Errorf("%s%w", opts.ParseErrPrefix, err)
		}
		code, ok := env.Code()
		if !ok && env.M != "" {
			// Declared divergence D2: an m that is numeric but non-integer
			// takes the parse-failure path, with the text replacing the json
			// library's cannot unmarshal error
			return nil, fmt.Errorf("%sm field is not an integer: %s", opts.ParseErrPrefix, env.M.String())
		}
		if !env.OK() {
			msg := env.E
			// Aligned with the message-bus CLI's old implementation:
			// whitespace-only e also takes the fallback copy
			if strings.TrimSpace(msg) == "" {
				msg = fmt.Sprintf(opts.CodeFallbackFmt, code)
			}
			return nil, &rest.APIError{Errcode: code, Msg: msg}
		}
		return env.Data(), nil
	}
}
