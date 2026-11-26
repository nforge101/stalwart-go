package stalwart

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	testBaseURL = "https://mail.example.com"

	// testToken is a placeholder. It is not a real Stalwart credential.
	testToken = "API_test-token-not-real"
)

// testAuth is the authentication option used by tests that need a valid client.
var testAuth = WithBearerToken(testToken)

func TestNewClient_ValidBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantPath string
	}{
		{name: "host only", raw: "https://mail.example.com", wantHost: "mail.example.com", wantPath: ""},
		{name: "trailing slash", raw: "https://mail.example.com/", wantHost: "mail.example.com", wantPath: ""},
		{name: "explicit port", raw: "https://mail.example.com:8443", wantHost: "mail.example.com:8443", wantPath: ""},
		{name: "path prefix", raw: "https://mail.example.com/stalwart/", wantHost: "mail.example.com", wantPath: "/stalwart"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.raw, testAuth)
			if err != nil {
				t.Fatalf("NewClient(%q) returned error: %v", tt.raw, err)
			}
			if c.baseURL.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", c.baseURL.Host, tt.wantHost)
			}
			if c.baseURL.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", c.baseURL.Path, tt.wantPath)
			}
		})
	}
}

func TestNewClient_InvalidBaseURL(t *testing.T) {
	const secret = "s3cr3t-value"

	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "no scheme", raw: "mail.example.com"},
		{name: "plain http", raw: "http://mail.example.com"},
		{name: "other scheme", raw: "ftp://mail.example.com"},
		{name: "missing host", raw: "https://"},
		{name: "port without host", raw: "https://:8443"},
		{name: "opaque URL", raw: "https:mail.example.com"},
		{name: "user information", raw: "https://admin:" + secret + "@mail.example.com"},
		{name: "query", raw: "https://mail.example.com/?token=x"},
		{name: "fragment", raw: "https://mail.example.com/#frag"},
		{name: "unparseable with user information", raw: "https://admin:" + secret + "@mail example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.raw, testAuth)
			if err == nil {
				t.Fatalf("NewClient(%q) returned nil error", tt.raw)
			}
			if c != nil {
				t.Errorf("NewClient(%q) returned non-nil client with error", tt.raw)
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("error %v does not wrap ErrInvalidConfig", err)
			}
			msg := err.Error()
			if strings.Contains(msg, secret) || strings.Contains(msg, "admin") {
				t.Errorf("error message leaks user information: %q", msg)
			}
			if tt.raw != "" && strings.Contains(msg, tt.raw) {
				t.Errorf("error message echoes raw base URL: %q", msg)
			}
		})
	}
}

func TestNewClient_Defaults(t *testing.T) {
	c, err := NewClient(testBaseURL, testAuth)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if c.httpClient == nil {
		t.Fatal("default HTTP client is nil")
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
	if c.userAgent != defaultUserAgent {
		t.Errorf("user agent = %q, want %q", c.userAgent, defaultUserAgent)
	}
	if c.auth == nil {
		t.Error("authenticator was not set")
	}
	if c.httpClient.CheckRedirect == nil {
		t.Fatal("default HTTP client must not follow redirects automatically")
	}
	if err := c.httpClient.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("CheckRedirect returned %v, want http.ErrUseLastResponse", err)
	}
}

func TestNewClient_NilOption(t *testing.T) {
	_, err := NewClient(testBaseURL, testAuth, nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestWithTimeout(t *testing.T) {
	c, err := NewClient(testBaseURL, testAuth, WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.httpClient.Timeout)
	}

	for _, d := range []time.Duration{0, -time.Second} {
		if _, err := NewClient(testBaseURL, testAuth, WithTimeout(d)); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("WithTimeout(%v) error = %v, want ErrInvalidConfig", d, err)
		}
	}
}

func TestWithHTTPClient_UsesCallerClientUnmodified(t *testing.T) {
	hc := &http.Client{Timeout: 7 * time.Second}

	c, err := NewClient(testBaseURL, testAuth, WithHTTPClient(hc))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if c.httpClient != hc {
		t.Error("client does not use the supplied HTTP client")
	}
	if hc.Timeout != 7*time.Second {
		t.Errorf("caller client timeout changed to %v", hc.Timeout)
	}
	if hc.CheckRedirect != nil {
		t.Error("caller client redirect policy was modified")
	}
}

func TestWithHTTPClient_Nil(t *testing.T) {
	_, err := NewClient(testBaseURL, testAuth, WithHTTPClient(nil))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestWithTimeout_ConflictsWithHTTPClient(t *testing.T) {
	hc := &http.Client{}

	orders := [][]Option{
		{testAuth, WithHTTPClient(hc), WithTimeout(time.Second)},
		{testAuth, WithTimeout(time.Second), WithHTTPClient(hc)},
	}
	for i, opts := range orders {
		if _, err := NewClient(testBaseURL, opts...); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("order %d: error = %v, want ErrInvalidConfig", i, err)
		}
	}
}

func TestWithUserAgent(t *testing.T) {
	c, err := NewClient(testBaseURL, testAuth, WithUserAgent("my-app/1.0"))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if c.userAgent != "my-app/1.0" {
		t.Errorf("user agent = %q, want %q", c.userAgent, "my-app/1.0")
	}

	maxLen := strings.Repeat("a", maxUserAgentLen)
	if _, err := NewClient(testBaseURL, testAuth, WithUserAgent(maxLen)); err != nil {
		t.Errorf("maximum-length user agent rejected: %v", err)
	}
}

func TestWithUserAgent_Invalid(t *testing.T) {
	tests := []struct {
		name string
		ua   string
	}{
		{name: "empty", ua: ""},
		{name: "too long", ua: strings.Repeat("a", maxUserAgentLen+1)},
		{name: "newline", ua: "bad\nagent"},
		{name: "tab", ua: "bad\tagent"},
		{name: "non-ASCII", ua: "héllo"},
		{name: "DEL character", ua: "bad\x7fagent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(testBaseURL, testAuth, WithUserAgent(tt.ua))
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}
