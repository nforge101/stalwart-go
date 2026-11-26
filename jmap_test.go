package stalwart

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// testMethod is the method name used by the JMAP helper tests.
const testMethod = "x:Test/get"

// writeJSONBody writes body as a JSON response with HTTP 200.
func writeJSONBody(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

// callTestJMAP sends one call to a server that always responds with body, and
// returns the call's result and error.
func callTestJMAP(t *testing.T, body string) (json.RawMessage, error) {
	t.Helper()
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, body)
	})
	c := newTestClient(t, srv, nil, "")
	return c.callJMAP(context.Background(), testMethod, nil)
}

func TestCallJMAP_Success(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != jmapPath {
			t.Errorf("path = %q, want %q", r.URL.Path, jmapPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization header is not the bearer token")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var req struct {
			Using       []string            `json:"using"`
			MethodCalls [][]json.RawMessage `json:"methodCalls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		wantUsing := []string{jmapCoreCapability, stalwartCapability}
		if !slices.Equal(req.Using, wantUsing) {
			t.Errorf("using = %v, want %v", req.Using, wantUsing)
		}
		if len(req.MethodCalls) != 1 || len(req.MethodCalls[0]) != 3 {
			t.Errorf("methodCalls = %d calls, want one call of three elements", len(req.MethodCalls))
			return
		}
		var name, callID string
		_ = json.Unmarshal(req.MethodCalls[0][0], &name)
		_ = json.Unmarshal(req.MethodCalls[0][2], &callID)
		if name != testMethod {
			t.Errorf("call name = %q, want %q", name, testMethod)
		}
		if callID != jmapCallID {
			t.Errorf("call ID = %q, want %q", callID, jmapCallID)
		}

		writeJSONBody(w, `{"methodResponses":[["x:Test/get",{"list":[{"id":"a"}]},"c1"]],"sessionState":"s1"}`)
	})
	c := newTestClient(t, srv, nil, "")

	raw, err := c.callJMAP(context.Background(), testMethod, map[string]any{"ids": nil})
	if err != nil {
		t.Fatalf("callJMAP returned error: %v", err)
	}
	var got struct {
		List []struct {
			ID string `json:"id"`
		} `json:"list"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(got.List) != 1 || got.List[0].ID != "a" {
		t.Errorf("result list = %+v, want one item with id a", got.List)
	}
}

func TestCallJMAP_MethodErrorForbidden(t *testing.T) {
	const desc = "You do not have enough permissions to access this resource."
	_, err := callTestJMAP(t, `{"methodResponses":[["error",{"type":"forbidden","description":"`+desc+`"},"c1"]]}`)

	var methodErr *MethodError
	if !errors.As(err, &methodErr) {
		t.Fatalf("error = %v, want *MethodError", err)
	}
	if methodErr.Type != "forbidden" {
		t.Errorf("Type = %q, want forbidden", methodErr.Type)
	}
	if methodErr.Description != desc {
		t.Errorf("Description = %q, want %q", methodErr.Description, desc)
	}
	if !errors.Is(err, ErrForbidden) {
		t.Error("error does not match ErrForbidden")
	}
	for _, s := range []error{ErrUnauthorized, ErrTransport, ErrDecode} {
		if errors.Is(err, s) {
			t.Errorf("error unexpectedly matches %v", s)
		}
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Error("method error was reported as *APIError")
	}
}

func TestCallJMAP_OtherMethodErrorsMatchNoSentinel(t *testing.T) {
	for _, typ := range []string{"invalidArguments", "unknownMethod", "serverFail"} {
		t.Run(typ, func(t *testing.T) {
			_, err := callTestJMAP(t, `{"methodResponses":[["error",{"type":"`+typ+`"},"c1"]]}`)

			var methodErr *MethodError
			if !errors.As(err, &methodErr) {
				t.Fatalf("error = %v, want *MethodError", err)
			}
			if methodErr.Type != typ {
				t.Errorf("Type = %q, want %q", methodErr.Type, typ)
			}
			for _, s := range []error{ErrForbidden, ErrUnauthorized, ErrTransport, ErrDecode} {
				if errors.Is(err, s) {
					t.Errorf("error unexpectedly matches %v", s)
				}
			}
		})
	}
}

func TestCallJMAP_MethodErrorDescriptionIsSanitized(t *testing.T) {
	desc := "token " + testToken + " rejected\x01" + strings.Repeat("a", 2000)
	body, err := json.Marshal(map[string]any{
		"methodResponses": [][]any{{
			"error",
			map[string]any{"type": "forbidden", "description": desc},
			"c1",
		}},
	})
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}

	_, err = callTestJMAP(t, string(body))

	var methodErr *MethodError
	if !errors.As(err, &methodErr) {
		t.Fatalf("error = %v, want *MethodError", err)
	}
	if strings.Contains(methodErr.Description, testToken) || strings.Contains(err.Error(), testToken) {
		t.Errorf("error contains the token")
	}
	if !strings.Contains(methodErr.Description, redactedPlaceholder) {
		t.Errorf("Description %q does not show the redaction placeholder", methodErr.Description)
	}
	if len(methodErr.Description) > maxErrorSnippet {
		t.Errorf("Description has %d bytes, want at most %d", len(methodErr.Description), maxErrorSnippet)
	}
	for i := 0; i < len(methodErr.Description); i++ {
		if b := methodErr.Description[i]; b < 0x20 || b == 0x7f {
			t.Fatalf("Description contains control character 0x%02x", b)
		}
	}
}

func TestCallJMAP_MethodErrorWithoutDescription(t *testing.T) {
	_, err := callTestJMAP(t, `{"methodResponses":[["error",{"type":"forbidden"},"c1"]]}`)

	var methodErr *MethodError
	if !errors.As(err, &methodErr) {
		t.Fatalf("error = %v, want *MethodError", err)
	}
	if methodErr.Description != "" {
		t.Errorf("Description = %q, want empty", methodErr.Description)
	}
	if !errors.Is(err, ErrForbidden) {
		t.Error("error does not match ErrForbidden")
	}
}

func TestCallJMAP_HTTPErrorsPassThrough(t *testing.T) {
	tests := []struct {
		status   int
		sentinel error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, "denied")
			})
			c := newTestClient(t, srv, nil, "")

			_, err := c.callJMAP(context.Background(), testMethod, nil)

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
			}
			if !errors.Is(err, tt.sentinel) {
				t.Errorf("error does not match %v", tt.sentinel)
			}
			var methodErr *MethodError
			if errors.As(err, &methodErr) {
				t.Error("HTTP error was reported as *MethodError")
			}
		})
	}
}

func TestCallJMAP_MalformedResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"not an object", `[]`},
		{"no method responses", `{"methodResponses":[]}`},
		{"two method responses", `{"methodResponses":[["x:Test/get",{},"c1"],["x:Test/get",{},"c1"]]}`},
		{"entry with two elements", `{"methodResponses":[["x:Test/get",{}]]}`},
		{"name is not a string", `{"methodResponses":[[1,{},"c1"]]}`},
		{"call ID is not a string", `{"methodResponses":[["x:Test/get",{},1]]}`},
		{"wrong call ID", `{"methodResponses":[["x:Test/get",{},"c2"]]}`},
		{"name does not match the request", `{"methodResponses":[["x:Other/get",{},"c1"]]}`},
		{"error without a type", `{"methodResponses":[["error",{"description":"x"},"c1"]]}`},
		{"error arguments not an object", `{"methodResponses":[["error","oops","c1"]]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := callTestJMAP(t, tt.body)
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("error = %v, want one matching ErrDecode", err)
			}
			var methodErr *MethodError
			if errors.As(err, &methodErr) {
				t.Error("malformed response was reported as *MethodError")
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				t.Error("malformed response was reported as *APIError")
			}
		})
	}
}
