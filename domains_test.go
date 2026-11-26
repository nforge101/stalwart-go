package stalwart

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// domainResponse wraps args in a successful x:Domain/get method response.
func domainResponse(args string) string {
	return `{"methodResponses":[["x:Domain/get",` + args + `,"c1"]]}`
}

// readCall decodes the single method call in r's body and returns its name and
// arguments. It reports a test error if the body does not have that shape.
func readCall(t *testing.T, r *http.Request) (string, map[string]any) {
	t.Helper()
	var req struct {
		MethodCalls [][]json.RawMessage `json:"methodCalls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decode request: %v", err)
		return "", nil
	}
	if len(req.MethodCalls) != 1 || len(req.MethodCalls[0]) != 3 {
		t.Errorf("request has %d calls, want one call of three elements", len(req.MethodCalls))
		return "", nil
	}
	var name string
	var args map[string]any
	_ = json.Unmarshal(req.MethodCalls[0][0], &name)
	_ = json.Unmarshal(req.MethodCalls[0][1], &args)
	return name, args
}

func TestDomainsList_RequestAndResult(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		name, args := readCall(t, r)
		if name != domainGetMethod {
			t.Errorf("method = %q, want %q", name, domainGetMethod)
		}
		if args["ids"] != nil {
			t.Errorf("ids = %v, want null to list all domains", args["ids"])
		}
		if props, _ := args["properties"].([]any); len(props) != 1 || props[0] != "name" {
			t.Errorf("properties = %v, want [name]", args["properties"])
		}
		writeJSONBody(w, domainResponse(`{"list":[{"id":"d1","name":"example.com"},{"id":"d2","name":"example.org"}],"notFound":[]}`))
	})
	c := newTestClient(t, srv, nil, "")

	domains, err := c.Domains().List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	want := []Domain{{ID: "d1", Name: "example.com"}, {ID: "d2", Name: "example.org"}}
	if !slices.Equal(domains, want) {
		t.Errorf("List = %+v, want %+v", domains, want)
	}
}

func TestDomainsList_Empty(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainResponse(`{"list":[],"notFound":[]}`))
	})
	c := newTestClient(t, srv, nil, "")

	domains, err := c.Domains().List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("List returned %d domains, want none", len(domains))
	}
}

func TestDomainsList_MalformedDomain(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{"missing ID", `{"list":[{"name":"example.com"}],"notFound":[]}`},
		{"missing name", `{"list":[{"id":"d1"}],"notFound":[]}`},
		{"list is not an array", `{"list":"oops","notFound":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(w, domainResponse(tt.args))
			})
			c := newTestClient(t, srv, nil, "")

			_, err := c.Domains().List(context.Background())
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("error = %v, want one matching ErrDecode", err)
			}
		})
	}
}

func TestDomainsList_Forbidden(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, `{"methodResponses":[["error",{"type":"forbidden","description":"no access"},"c1"]]}`)
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().List(context.Background())
	var methodErr *MethodError
	if !errors.As(err, &methodErr) {
		t.Fatalf("error = %v, want *MethodError", err)
	}
	if !errors.Is(err, ErrForbidden) {
		t.Error("error does not match ErrForbidden")
	}
}

func TestDomainsGet_RequestAndResult(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		name, args := readCall(t, r)
		if name != domainGetMethod {
			t.Errorf("method = %q, want %q", name, domainGetMethod)
		}
		if ids, _ := args["ids"].([]any); len(ids) != 1 || ids[0] != "d1" {
			t.Errorf("ids = %v, want [d1]", args["ids"])
		}
		writeJSONBody(w, domainResponse(`{"list":[{"id":"d1","name":"example.com"}],"notFound":[]}`))
	})
	c := newTestClient(t, srv, nil, "")

	d, err := c.Domains().Get(context.Background(), "d1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if d != (Domain{ID: "d1", Name: "example.com"}) {
		t.Errorf("Get = %+v, want {d1 example.com}", d)
	}
}

func TestDomainsGet_NotFound(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainResponse(`{"list":[],"notFound":["missing"]}`))
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Error("not-found error also matches ErrDecode")
	}
}

func TestDomainsGet_HTTPNotFoundIsNotErrNotFound(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "no such resource")
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().Get(context.Background(), "d1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("error = %v, want *APIError with status 404", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("HTTP 404 matched ErrNotFound; only a server-reported missing domain should")
	}
}

func TestDomainsGet_MismatchedOrExtraDomains(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{"different ID returned", `{"list":[{"id":"other","name":"example.org"}],"notFound":[]}`},
		{"two domains returned", `{"list":[{"id":"d1","name":"a.test"},{"id":"d1","name":"b.test"}],"notFound":[]}`},
		{"domain missing name", `{"list":[{"id":"d1"}],"notFound":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(w, domainResponse(tt.args))
			})
			c := newTestClient(t, srv, nil, "")

			_, err := c.Domains().Get(context.Background(), "d1")
			if !errors.Is(err, ErrDecode) {
				t.Fatalf("error = %v, want one matching ErrDecode", err)
			}
		})
	}
}

func TestDomainsGet_Forbidden(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, `{"methodResponses":[["error",{"type":"forbidden"},"c1"]]}`)
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().Get(context.Background(), "d1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("forbidden error matched ErrNotFound")
	}
}

// TestDomainsGet_InvalidIDSendsNoRequest checks that an unacceptable ID is
// rejected locally, before any request reaches the server.
func TestDomainsGet_InvalidIDSendsNoRequest(t *testing.T) {
	var hits atomic.Int32
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		writeJSONBody(w, domainResponse(`{"list":[],"notFound":[]}`))
	})
	c := newTestClient(t, srv, nil, "")

	invalid := []string{
		"",
		"has space",
		"tab\there",
		"id\x01",
		"caf\u00e9",
		strings.Repeat("a", maxDomainIDLen+1),
	}
	for _, id := range invalid {
		_, err := c.Domains().Get(context.Background(), id)
		if err == nil {
			t.Errorf("Get(%q) returned no error", id)
		}
		if errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) reported not found; an invalid ID is not a missing domain", id)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server received %d requests for invalid IDs, want none", n)
	}
}

func TestDomainsGet_MaxLengthIDAccepted(t *testing.T) {
	id := strings.Repeat("a", maxDomainIDLen)
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainResponse(`{"list":[{"id":"`+id+`","name":"example.com"}],"notFound":[]}`))
	})
	c := newTestClient(t, srv, nil, "")

	d, err := c.Domains().Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if d.ID != id {
		t.Errorf("Get returned ID of length %d, want %d", len(d.ID), len(id))
	}
}
