package stalwart

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testPayload struct {
	Name string `json:"name"`
}

type testResult struct {
	OK bool `json:"ok"`
}

// startTestServer starts an HTTPS test server running h.
//
// The server uses a self-signed certificate. Tests trust it through the
// server's own HTTP client (srv.Client()), so certificate verification stays
// enabled. Verification is never disabled.
func startTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// newTestClient returns a Client built with NewClient that sends requests to srv
// under basePath. hc is used when non-nil; otherwise the server's own HTTP
// client is used, which trusts the test certificate.
func newTestClient(t *testing.T, srv *httptest.Server, hc *http.Client, basePath string) *Client {
	t.Helper()
	if hc == nil {
		hc = srv.Client()
	}
	c, err := NewClient(srv.URL+basePath, testAuth, WithHTTPClient(hc))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return c
}

func TestEndpoint_Valid(t *testing.T) {
	tests := []struct {
		name        string
		base        string
		path        string
		wantEscaped string
	}{
		{name: "root", base: "https://mail.example.com", path: "/", wantEscaped: "/"},
		{name: "resource", base: "https://mail.example.com", path: "/domains", wantEscaped: "/domains"},
		{name: "path prefix", base: "https://mail.example.com/stalwart/", path: "/domains/x", wantEscaped: "/stalwart/domains/x"},
		{name: "escaped slash stays one segment", base: "https://mail.example.com", path: "/domains/" + url.PathEscape("a/b"), wantEscaped: "/domains/a%2Fb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.base, testAuth)
			if err != nil {
				t.Fatalf("NewClient returned error: %v", err)
			}
			u, err := c.endpoint(tt.path)
			if err != nil {
				t.Fatalf("endpoint(%q) returned error: %v", tt.path, err)
			}
			if got := u.EscapedPath(); got != tt.wantEscaped {
				t.Errorf("EscapedPath() = %q, want %q", got, tt.wantEscaped)
			}
		})
	}
}

func TestEndpoint_Rejected(t *testing.T) {
	c, err := NewClient(testBaseURL, testAuth)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	paths := []string{
		"domains",     // missing leading slash
		"/../secret",  // parent segment
		"/a/%2e%2e/b", // encoded parent segment
		"/a/%2E%2E/b", // encoded parent segment, uppercase
		"/a/./b",      // current-directory segment
		"/a?x=1",      // query
		"/a#frag",     // fragment
		"/a b",        // space
		"/a\\b",       // backslash
		"/bad%zz",     // invalid escape
		"/a\nb",       // control character
	}

	for _, p := range paths {
		if _, err := c.endpoint(p); !errors.Is(err, errInvalidPath) {
			t.Errorf("endpoint(%q) error = %v, want errInvalidPath", p, err)
		}
	}
}

func TestDoJSON_RequestConstruction(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.URL.EscapedPath(); got != "/stalwart/domains" {
			t.Errorf("path = %q, want /stalwart/domains", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization header not set correctly")
		}
		if got := r.Header.Get("User-Agent"); got != defaultUserAgent {
			t.Errorf("User-Agent = %q, want %q", got, defaultUserAgent)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var in testPayload
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		if in.Name != "example" {
			t.Errorf("body name = %q, want example", in.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})

	c := newTestClient(t, srv, nil, "/stalwart")

	var out testResult
	if err := c.doJSON(context.Background(), http.MethodPost, "/domains", testPayload{Name: "example"}, &out); err != nil {
		t.Fatalf("doJSON returned error: %v", err)
	}
	if !out.OK {
		t.Error("response was not decoded into out")
	}
}

func TestDoJSON_NoBodyOmitsContentType(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want empty for a request without a body", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, srv, nil, "")

	if err := c.doJSON(context.Background(), http.MethodGet, "/ping", nil, nil); err != nil {
		t.Errorf("doJSON returned error for 204 response: %v", err)
	}
}

func TestDoJSON_EscapedPathValueIsOneSegment(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/domains/a%2Fb" {
			t.Errorf("path = %q, want /domains/a%%2Fb", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, srv, nil, "")

	p := "/domains/" + url.PathEscape("a/b")
	if err := c.doJSON(context.Background(), http.MethodGet, p, nil, nil); err != nil {
		t.Errorf("doJSON returned error: %v", err)
	}
}

func TestDoJSON_APIErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		sentinel error   // expected errors.Is target, or nil for none
		notSent  []error // sentinels the error must not match
	}{
		{name: "bad request", status: 400, body: "bad input", sentinel: nil, notSent: []error{ErrUnauthorized, ErrForbidden}},
		{name: "unauthorized", status: 401, body: "no credentials", sentinel: ErrUnauthorized, notSent: []error{ErrForbidden}},
		{name: "forbidden", status: 403, body: "missing permission", sentinel: ErrForbidden, notSent: []error{ErrUnauthorized}},
		{name: "not found", status: 404, body: "no such resource", sentinel: nil, notSent: []error{ErrUnauthorized, ErrForbidden}},
		{name: "server error", status: 500, body: "internal failure", sentinel: nil, notSent: []error{ErrUnauthorized, ErrForbidden}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			c := newTestClient(t, srv, nil, "")

			err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, nil)

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
			}
			if !strings.Contains(apiErr.Message, tt.body) {
				t.Errorf("Message = %q, want it to contain %q", apiErr.Message, tt.body)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("error does not match %v", tt.sentinel)
			}
			for _, s := range tt.notSent {
				if errors.Is(err, s) {
					t.Errorf("error unexpectedly matches %v", s)
				}
			}
			if errors.Is(err, ErrTransport) || errors.Is(err, ErrDecode) {
				t.Error("HTTP status error matched a transport or decode sentinel")
			}
		})
	}
}

func TestDoJSON_SuccessWithNoContent(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c := newTestClient(t, srv, nil, "")

	if err := c.doJSON(context.Background(), http.MethodDelete, "/x", nil, nil); err != nil {
		t.Errorf("doJSON returned error: %v", err)
	}
}

func TestDoJSON_RedactsEchoedToken(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "token "+testToken+" was rejected")
	})
	c := newTestClient(t, srv, nil, "")

	err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, nil)
	if err == nil {
		t.Fatal("doJSON returned nil error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Errorf("error message contains the token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), redactedPlaceholder) {
		t.Errorf("error message %q does not show the redaction placeholder", err.Error())
	}
}

func TestDoJSON_BoundsErrorBody(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, strings.Repeat("x", 1<<20))
	})
	c := newTestClient(t, srv, nil, "")

	var apiErr *APIError
	err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, nil)
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if len(apiErr.Message) > maxErrorSnippet {
		t.Errorf("Message length = %d, want at most %d", len(apiErr.Message), maxErrorSnippet)
	}
}

func TestDoJSON_SanitizesControlCharacters(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "line one\r\nline two\x00\x1b[31m")
	})
	c := newTestClient(t, srv, nil, "")

	var apiErr *APIError
	err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, nil)
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	for _, r := range apiErr.Message {
		if r < 0x20 || r == 0x7f {
			t.Errorf("Message contains control character %U: %q", r, apiErr.Message)
		}
	}
}

func TestDoJSON_DecodeError(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "this is not json")
	})
	c := newTestClient(t, srv, nil, "")

	var out testResult
	err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, &out)
	if !errors.Is(err, ErrDecode) {
		t.Errorf("error = %v, want ErrDecode", err)
	}
	if errors.Is(err, ErrTransport) {
		t.Error("decode error matched ErrTransport")
	}
}

func TestDoJSON_TransportError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c := newTestClient(t, srv, nil, "")
	srv.Close()

	err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, nil)
	if !errors.Is(err, ErrTransport) {
		t.Errorf("error = %v, want ErrTransport", err)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Error("transport failure matched *APIError")
	}
}

func TestDoJSON_CancelledContext(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the server with a cancelled context")
	})
	c := newTestClient(t, srv, nil, "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.doJSON(ctx, http.MethodGet, "/x", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if !errors.Is(err, ErrTransport) {
		t.Errorf("error = %v, want ErrTransport", err)
	}
}

func TestDoJSON_Timeout(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	hc := &http.Client{
		Transport: srv.Client().Transport, // trusts the test certificate
		Timeout:   50 * time.Millisecond,
	}
	c := newTestClient(t, srv, hc, "")

	err := c.doJSON(context.Background(), http.MethodGet, "/x", nil, nil)
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("error = %v, want ErrTransport", err)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("error = %v, want a net.Error whose Timeout() is true", err)
	}
}

// trackingTransport records when response bodies are closed.
type trackingTransport struct {
	base   http.RoundTripper
	closed *atomic.Bool
}

func (t trackingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	resp.Body = closeTracker{ReadCloser: resp.Body, closed: t.closed}
	return resp, nil
}

type closeTracker struct {
	io.ReadCloser
	closed *atomic.Bool
}

func (c closeTracker) Close() error {
	c.closed.Store(true)
	return c.ReadCloser.Close()
}

func TestDoJSON_ClosesResponseBody(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "success", status: http.StatusOK},
		{name: "error", status: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"ok":true}`)
			})

			var closed atomic.Bool
			hc := &http.Client{Transport: trackingTransport{base: srv.Client().Transport, closed: &closed}}
			c := newTestClient(t, srv, hc, "")

			var out testResult
			_ = c.doJSON(context.Background(), http.MethodGet, "/x", nil, &out)
			if !closed.Load() {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestDoJSON_ConcurrentRequests(t *testing.T) {
	const goroutines = 20
	const requestsEach = 5

	var served atomic.Int64
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		served.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c := newTestClient(t, srv, nil, "")

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*requestsEach)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsEach; j++ {
				var out testResult
				if err := c.doJSON(context.Background(), http.MethodGet, "/ping", nil, &out); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent request failed: %v", err)
	}
	if got, want := served.Load(), int64(goroutines*requestsEach); got != want {
		t.Errorf("server handled %d authorized requests, want %d", got, want)
	}
}
