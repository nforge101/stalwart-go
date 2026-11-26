package main

import (
	"bytes"
	"strings"
	"testing"
)

const (
	testURL = "https://mail.example.com"
	testKey = "API_test-token-not-real"
)

// fakeEnv returns a getenv function backed by vars, so tests never read the
// real process environment.
func fakeEnv(vars map[string]string) func(string) string {
	return func(name string) string {
		return vars[name]
	}
}

func TestRun_Success(t *testing.T) {
	var out bytes.Buffer
	err := run(fakeEnv(map[string]string{envURL: testURL, envAPIKey: testKey}), &out)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, testURL) {
		t.Errorf("output %q does not contain base URL", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("output %q does not show redacted credentials", got)
	}
	if strings.Contains(got, testKey) {
		t.Errorf("output %q reveals the API key", got)
	}
}

func TestRun_MissingVariables(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "URL missing",
			env:     map[string]string{envAPIKey: testKey},
			wantErr: envURL,
		},
		{
			name:    "API key missing",
			env:     map[string]string{envURL: testURL},
			wantErr: envAPIKey,
		},
		{
			name:    "both missing",
			env:     map[string]string{},
			wantErr: envURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(fakeEnv(tt.env), &out)
			if err == nil {
				t.Fatal("run() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not name %s", err, tt.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
		})
	}
}

func TestRun_InvalidConfigDoesNotLeakKey(t *testing.T) {
	tests := []struct {
		name string
		url  string
		key  string
	}{
		{name: "plain http URL", url: "http://mail.example.com", key: testKey},
		{name: "key with space", url: testURL, key: "API_bad key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(fakeEnv(map[string]string{envURL: tt.url, envAPIKey: tt.key}), &out)
			if err == nil {
				t.Fatal("run() error = nil, want error")
			}
			if strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q reveals the API key", err)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
		})
	}
}
