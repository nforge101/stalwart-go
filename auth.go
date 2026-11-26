package stalwart

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	// maxBearerTokenLen bounds token size. Stalwart API keys are far shorter;
	// keeping the limit small also guarantees that a whole token fits within
	// the error-body read limit, so it can always be redacted.
	maxBearerTokenLen = 512

	// redactedPlaceholder replaces credentials wherever they would otherwise
	// be printed.
	redactedPlaceholder = "[REDACTED]"
)

// authenticator attaches credentials to outgoing requests.
//
// Implementations must be safe for concurrent use. They must never reveal
// their secret through String, GoString, or error values.
type authenticator interface {
	// authorize sets the authentication headers on req.
	authorize(req *http.Request)

	// redact returns s with every occurrence of the secret replaced by a
	// placeholder.
	redact(s string) string
}

// bearerAuth authenticates with a bearer token, such as a Stalwart API key
// (which begins with "API_").
type bearerAuth struct {
	token string
}

// authorize sets the Authorization header to "Bearer <token>".
func (a bearerAuth) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.token)
}

// redact replaces the token in s.
func (a bearerAuth) redact(s string) string {
	return strings.ReplaceAll(s, a.token, redactedPlaceholder)
}

// String describes the credential without revealing it.
func (a bearerAuth) String() string {
	return "Bearer " + redactedPlaceholder
}

// GoString describes the credential without revealing it.
func (a bearerAuth) GoString() string {
	return a.String()
}

// newBearerAuth validates token and returns an authenticator for it.
//
// Validation errors never include the token.
func newBearerAuth(token string) (authenticator, error) {
	if len(token) == 0 || len(token) > maxBearerTokenLen {
		return nil, fmt.Errorf("%w: bearer token must be 1 to %d characters", ErrInvalidConfig, maxBearerTokenLen)
	}
	if !isPrintableASCII(token) {
		return nil, fmt.Errorf("%w: bearer token must contain only printable ASCII characters without spaces", ErrInvalidConfig)
	}
	return bearerAuth{token: token}, nil
}
