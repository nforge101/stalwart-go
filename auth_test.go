package stalwart

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNewClient_RequiresAuthentication(t *testing.T) {
	_, err := NewClient(testBaseURL)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestWithBearerToken_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "too long", token: strings.Repeat("a", maxBearerTokenLen+1)},
		{name: "space", token: "API_abc def"},
		{name: "newline", token: "API_abc\ndef"},
		{name: "tab", token: "API_abc\tdef"},
		{name: "non-ASCII", token: "API_héllo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(testBaseURL, WithBearerToken(tt.token))
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
			if tt.token != "" && strings.Contains(err.Error(), tt.token) {
				t.Errorf("error message contains the token: %q", err.Error())
			}
		})
	}
}

func TestWithBearerToken_MaxLengthAccepted(t *testing.T) {
	token := "API_" + strings.Repeat("a", maxBearerTokenLen-4)
	if _, err := NewClient(testBaseURL, WithBearerToken(token)); err != nil {
		t.Errorf("token of maximum length rejected: %v", err)
	}
}

func TestBearerAuth_Authorize(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://mail.example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	bearerAuth{token: "API_example"}.authorize(req)

	if got := req.Header.Get("Authorization"); got != "Bearer API_example" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer API_example")
	}
}

func TestBearerAuth_Redact(t *testing.T) {
	a := bearerAuth{token: "API_example"}
	in := "rejected token API_example for account"
	want := "rejected token [REDACTED] for account"
	if got := a.redact(in); got != want {
		t.Errorf("redact() = %q, want %q", got, want)
	}
}

func TestCredentialsNotRevealedByFormatting(t *testing.T) {
	const token = "API_secret-value-123"

	c, err := NewClient(testBaseURL, WithBearerToken(token))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	outputs := []string{
		fmt.Sprintf("%v", c),
		fmt.Sprintf("%+v", *c),
		fmt.Sprintf("%#v", c),
		fmt.Sprintf("%s", c),
		fmt.Sprintf("%v", bearerAuth{token: token}),
		fmt.Sprintf("%#v", bearerAuth{token: token}),
	}
	for i, out := range outputs {
		if strings.Contains(out, token) {
			t.Errorf("formatting %d revealed the token: %q", i, out)
		}
	}
	if !strings.Contains(outputs[0], redactedPlaceholder) {
		t.Errorf("String() = %q, want a redaction placeholder", outputs[0])
	}
}
