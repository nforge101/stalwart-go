package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	stalwart "github.com/nforge101/stalwart-go"
)

const testKey = "API_test-token-not-real"

// fakeEnv returns a getenv function backed by vars, so tests never read the
// real process environment.
func fakeEnv(vars map[string]string) func(string) string {
	return func(name string) string {
		return vars[name]
	}
}

// startServer starts an HTTPS test server that answers every request with
// body and checks that the request carries the test key.
func startServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
			t.Errorf("Authorization header is not the test key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_ListsDomainNames(t *testing.T) {
	srv := startServer(t, `{"methodResponses":[["x:Domain/get",{"list":[{"id":"d1","name":"example.com"},{"id":"d2","name":"example.org"}],"notFound":[]},"c1"]]}`)

	var out bytes.Buffer
	err := run(fakeEnv(map[string]string{envURL: srv.URL, envAPIKey: testKey}), &out,
		stalwart.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	want := "2 domain(s)\nexample.com\nexample.org\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	if strings.Contains(out.String(), testKey) {
		t.Error("output contains the API key")
	}
}

func TestRun_ForbiddenIsReportedWithoutKey(t *testing.T) {
	srv := startServer(t, `{"methodResponses":[["error",{"type":"forbidden","description":"no access"},"c1"]]}`)

	var out bytes.Buffer
	err := run(fakeEnv(map[string]string{envURL: srv.URL, envAPIKey: testKey}), &out,
		stalwart.WithHTTPClient(srv.Client()))
	if err == nil {
		t.Fatal("run() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error %q does not mention forbidden", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Error("error contains the API key")
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want none on error", out.String())
	}
}

func TestRun_MissingVariables(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"URL missing", map[string]string{envAPIKey: testKey}, envURL},
		{"API key missing", map[string]string{envURL: "https://mail.example.com"}, envAPIKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(fakeEnv(tt.env), &out)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want one naming %s", err, tt.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
		})
	}
}
