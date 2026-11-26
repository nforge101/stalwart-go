package stalwart

import (
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultUserAgent = "stalwart-go"
	maxUserAgentLen  = 256
)

// Option configures a Client. Options are passed to NewClient and applied in
// order.
type Option func(*clientConfig) error

// clientConfig holds values set by options before NewClient builds the Client.
type clientConfig struct {
	httpClient *http.Client
	timeout    time.Duration
	timeoutSet bool
	userAgent  string
}

// WithHTTPClient makes the Client send requests with hc instead of a default
// HTTP client.
//
// The Client uses hc as provided and does not modify it, so its Timeout,
// Transport, TLS configuration, and redirect policy are the caller's
// responsibility. To trust a private certificate authority (for example a
// local mkcert root), configure the transport's TLS settings on hc. Do not
// disable certificate verification.
//
// WithHTTPClient cannot be combined with WithTimeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *clientConfig) error {
		if hc == nil {
			return fmt.Errorf("%w: HTTP client must not be nil", ErrInvalidConfig)
		}
		c.httpClient = hc
		return nil
	}
}

// WithTimeout sets the overall timeout for each request made by the default
// HTTP client. The timeout covers connection, sending, and reading the
// response. The default is 30 seconds.
//
// d must be positive. WithTimeout cannot be combined with WithHTTPClient; set
// the timeout on that client instead.
func WithTimeout(d time.Duration) Option {
	return func(c *clientConfig) error {
		if d <= 0 {
			return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfig)
		}
		c.timeout = d
		c.timeoutSet = true
		return nil
	}
}

// WithUserAgent sets the User-Agent header sent with each request. The default
// is "stalwart-go".
//
// ua must be 1 to 256 printable ASCII characters.
func WithUserAgent(ua string) Option {
	return func(c *clientConfig) error {
		if len(ua) == 0 || len(ua) > maxUserAgentLen {
			return fmt.Errorf("%w: user agent must be 1 to %d characters", ErrInvalidConfig, maxUserAgentLen)
		}
		for i := 0; i < len(ua); i++ {
			if ua[i] < 0x20 || ua[i] > 0x7e {
				return fmt.Errorf("%w: user agent must contain only printable ASCII characters", ErrInvalidConfig)
			}
		}
		c.userAgent = ua
		return nil
	}
}
