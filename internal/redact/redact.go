// Package redact strips credentials from text before it is logged, stored as
// an error message or returned to a client.
//
// Several platform APIs (Meta Graph, Threads, Instagram) take the access
// token as a query or form parameter. When such a request fails, net/http's
// *url.Error and many API error bodies include the full URL, token and all.
package redact

import (
	"log/slog"
	"regexp"
)

const mask = "[REDACTED]"

// Each pattern captures the part to keep in group 1 and the secret in group 2.
var patterns = []*regexp.Regexp{
	// key=value in query strings and form bodies
	regexp.MustCompile(`(?i)(\b(?:access_token|refresh_token|id_token|fb_exchange_token|input_token|client_secret|code_verifier|code|password|api_key|x-amz-signature|x-amz-credential)=)([^&\s"'<>]+)`),
	// "access_token": "..." in JSON
	regexp.MustCompile(`(?i)("(?:access_token|refresh_token|id_token|client_secret|password)"\s*:\s*")([^"]+)`),
	// Authorization: Bearer ...
	regexp.MustCompile(`(?i)(\bbearer\s+)([A-Za-z0-9._~+/=-]{8,})`),
}

// String returns s with credential values replaced by [REDACTED].
func String(s string) string {
	for _, re := range patterns {
		s = re.ReplaceAllString(s, "${1}"+mask)
	}
	return s
}

// ReplaceAttr is a slog.HandlerOptions.ReplaceAttr that redacts string and
// error attribute values, including the message.
func ReplaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch v := a.Value.Any().(type) {
	case string:
		if r := String(v); r != v {
			return slog.String(a.Key, r)
		}
	case error:
		if r := String(v.Error()); r != v.Error() {
			return slog.String(a.Key, r)
		}
	}
	return a
}
