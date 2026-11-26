package stalwart

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a client for the Stalwart management API.
//
// A Client is safe for concurrent use by multiple goroutines. Create it with
// NewClient; the zero value is not usable.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
	auth       authenticator
}

// NewClient returns a Client for the Stalwart server at baseURL.
//
// baseURL must be an absolute https URL with a host, for example
// "https://mail.example.com". It may include a path prefix when Stalwart is
// served under a sub-path, but it must not contain user information, a query,
// or a fragment. Plain http URLs are rejected so that credentials are never
// sent unencrypted.
//
// NewClient requires an authentication option, such as WithBearerToken.
//
// Options are applied in order. Invalid configuration returns an error that
// wraps ErrInvalidConfig.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	u, err := parseBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	cfg := clientConfig{
		timeout:   defaultTimeout,
		userAgent: defaultUserAgent,
	}
	for _, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: option must not be nil", ErrInvalidConfig)
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if cfg.httpClient != nil && cfg.timeoutSet {
		return nil, fmt.Errorf("%w: WithTimeout cannot be combined with WithHTTPClient", ErrInvalidConfig)
	}
	if cfg.auth == nil {
		return nil, fmt.Errorf("%w: authentication is required; use WithBearerToken", ErrInvalidConfig)
	}

	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = newDefaultHTTPClient(cfg.timeout)
	}

	return &Client{
		baseURL:    u,
		httpClient: httpClient,
		userAgent:  cfg.userAgent,
		auth:       cfg.auth,
	}, nil
}

// String describes the client without revealing its credentials.
func (c Client) String() string {
	return fmt.Sprintf("stalwart.Client{baseURL: %s, auth: %s}", c.baseURL, redactedPlaceholder)
}

// GoString describes the client without revealing its credentials.
func (c Client) GoString() string {
	return c.String()
}

// newDefaultHTTPClient returns the HTTP client used when the caller does not
// supply one. It verifies TLS certificates (the standard library default) and
// does not follow redirects. A redirect response is returned to the caller as
// an ordinary HTTP response, so credentials are never re-sent to a location
// the caller did not configure.
func newDefaultHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// parseBaseURL validates raw and returns it as a URL with any trailing slash
// removed from the path.
//
// Errors deliberately do not include raw or the url package's parse error,
// because either may contain credentials supplied in the URL.
func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: base URL is required", ErrInvalidConfig)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: base URL is not a valid URL", ErrInvalidConfig)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: base URL must use the https scheme", ErrInvalidConfig)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: base URL must not contain user information; configure credentials separately", ErrInvalidConfig)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: base URL must include a host", ErrInvalidConfig)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: base URL must not contain a query or fragment", ErrInvalidConfig)
	}

	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u, nil
}

// isPrintableASCII reports whether s contains only printable ASCII characters
// from '!' (0x21) through '~' (0x7e), so no spaces or control characters.
func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
