package stalwart

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// domainSetResponse wraps args in a successful x:Domain/set method response.
func domainSetResponse(args string) string {
	return `{"methodResponses":[["x:Domain/set",` + args + `,"c1"]]}`
}

// methodErrorResponse wraps a JMAP method-level error in an HTTP 200 body.
func methodErrorResponse(typ string) string {
	return `{"methodResponses":[["error",{"type":"` + typ + `","description":"denied"},"c1"]]}`
}

// TestDomainsCreate_RequestAndResult checks the request that Create sends and
// the Domain it returns. The request must not set a DKIM mode, so that the
// server's default applies.
func TestDomainsCreate_RequestAndResult(t *testing.T) {
	const name = "mail.example.com"
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		method, args := readCall(t, r)
		if method != domainSetMethod {
			t.Errorf("method = %q, want %q", method, domainSetMethod)
		}
		create, _ := args["create"].(map[string]any)
		obj, _ := create[createRef].(map[string]any)
		if obj["name"] != name {
			t.Errorf("create name = %v, want %q", obj["name"], name)
		}
		if len(obj) != 1 {
			t.Errorf("create object has %d properties, want only name", len(obj))
		}
		writeJSONBody(w, domainSetResponse(`{"created":{"new":{"id":"d9"}}}`))
	})
	c := newTestClient(t, srv, nil, "")

	d, err := c.Domains().Create(context.Background(), name)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if d != (Domain{ID: "d9", Name: name}) {
		t.Errorf("Create = %+v, want {d9 %s}", d, name)
	}
}

// TestDomainsCreate_InvalidNameNoRequest checks that names the SDK rejects are
// refused before any request is sent.
func TestDomainsCreate_InvalidNameNoRequest(t *testing.T) {
	tests := []struct {
		label string
		name  string
	}{
		{"empty", ""},
		{"one label", "example"},
		{"spaces", "not a domain.example.com"},
		{"leading hyphen", "-example.com"},
		{"trailing hyphen", "example-.com"},
		{"empty label", "example..com"},
		{"trailing dot", "example.com."},
		{"underscore", "my_domain.example.com"},
		{"non-ASCII", "exämple.com"},
		{"label too long", strings.Repeat("a", maxLabelLen+1) + ".com"},
		{"name too long", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63)},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.label, " ", "_"), func(t *testing.T) {
			var requests atomic.Int32
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				writeJSONBody(w, domainSetResponse(`{"created":{}}`))
			})
			c := newTestClient(t, srv, nil, "")

			if _, err := c.Domains().Create(context.Background(), tt.name); err == nil {
				t.Errorf("Create(%q) returned no error", tt.name)
			}
			if n := requests.Load(); n != 0 {
				t.Errorf("server received %d requests for an invalid name, want none", n)
			}
		})
	}
}

// TestValidateDomainName_Accepts checks names that the SDK sends.
func TestValidateDomainName_Accepts(t *testing.T) {
	for _, name := range []string{"a.b", "example.com", "Sub.Example.COM", "a-b.c1.io", strings.Repeat("a", 63) + ".com"} {
		if err := validateDomainName(name); err != nil {
			t.Errorf("validateDomainName(%q) = %v, want nil", name, err)
		}
	}
}

// TestDomainsCreate_AlreadyExists checks that a duplicate name maps to
// ErrAlreadyExists.
func TestDomainsCreate_AlreadyExists(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainSetResponse(`{"notCreated":{"new":{"type":"primaryKeyViolation","properties":["name"]}}}`))
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().Create(context.Background(), "example.com")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("error = %v, want ErrAlreadyExists", err)
	}
}

// TestDomainsCreate_Rejected checks that other rejections are returned as a
// *RejectedError with the server's type and description.
func TestDomainsCreate_Rejected(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainSetResponse(`{"notCreated":{"new":{"type":"validationFailed","description":"bad name"}}}`))
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().Create(context.Background(), "example.com")
	var rej *RejectedError
	if !errors.As(err, &rej) {
		t.Fatalf("error = %v, want *RejectedError", err)
	}
	if rej.Type != "validationFailed" || rej.Description != "bad name" {
		t.Errorf("RejectedError = %+v, want type validationFailed and description %q", rej, "bad name")
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAlreadyExists) {
		t.Error("a validation rejection matched a sentinel")
	}
}

// TestDomainsCreate_MalformedResponse checks that responses without a usable
// result wrap ErrDecode.
func TestDomainsCreate_MalformedResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"created without ID", `{"created":{"new":{}}}`},
		{"created ID with spaces", `{"created":{"new":{"id":"a b"}}}`},
		{"neither created nor notCreated", `{"created":{}}`},
		{"rejection without type", `{"notCreated":{"new":{"description":"x"}}}`},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.name, " ", "_"), func(t *testing.T) {
			srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSONBody(w, domainSetResponse(tt.body))
			})
			c := newTestClient(t, srv, nil, "")

			_, err := c.Domains().Create(context.Background(), "example.com")
			if !errors.Is(err, ErrDecode) {
				t.Errorf("error = %v, want ErrDecode", err)
			}
		})
	}
}

// TestDomainsCreate_Forbidden checks that a method-level denial is a
// *MethodError matching ErrForbidden.
func TestDomainsCreate_Forbidden(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, methodErrorResponse("forbidden"))
	})
	c := newTestClient(t, srv, nil, "")

	_, err := c.Domains().Create(context.Background(), "example.com")
	var methodErr *MethodError
	if !errors.As(err, &methodErr) || !errors.Is(err, ErrForbidden) {
		t.Errorf("error = %v, want a forbidden *MethodError", err)
	}
}

// TestDomainsUpdate_RequestAndResult checks the request for a patch that sets
// both properties.
func TestDomainsUpdate_RequestAndResult(t *testing.T) {
	desc := "office domain"
	enable := false
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		method, args := readCall(t, r)
		if method != domainSetMethod {
			t.Errorf("method = %q, want %q", method, domainSetMethod)
		}
		update, _ := args["update"].(map[string]any)
		props, _ := update["d1"].(map[string]any)
		if props["description"] != desc || props["enable"] != enable || len(props) != 2 {
			t.Errorf("update properties = %v, want description and enable only", props)
		}
		writeJSONBody(w, domainSetResponse(`{"updated":{"d1":null}}`))
	})
	c := newTestClient(t, srv, nil, "")

	err := c.Domains().Update(context.Background(), "d1", DomainPatch{Description: &desc, Enable: &enable})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}

// TestDomainsUpdate_EmptyPatchNoRequest checks that a patch with no properties
// is refused without a request.
func TestDomainsUpdate_EmptyPatchNoRequest(t *testing.T) {
	var requests atomic.Int32
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	})
	c := newTestClient(t, srv, nil, "")

	if err := c.Domains().Update(context.Background(), "d1", DomainPatch{}); err == nil {
		t.Error("Update with an empty patch returned no error")
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("server received %d requests, want none", n)
	}
}

// TestDomainsUpdate_InvalidIDNoRequest checks that an invalid ID is refused
// before any request.
func TestDomainsUpdate_InvalidIDNoRequest(t *testing.T) {
	var requests atomic.Int32
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	})
	c := newTestClient(t, srv, nil, "")
	desc := "x"

	for _, id := range []string{"", "a b"} {
		if err := c.Domains().Update(context.Background(), id, DomainPatch{Description: &desc}); err == nil {
			t.Errorf("Update(%q) returned no error", id)
		}
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("server received %d requests, want none", n)
	}
}

// TestDomainsUpdate_NotFound checks that an unknown ID maps to ErrNotFound.
func TestDomainsUpdate_NotFound(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainSetResponse(`{"notUpdated":{"d1":{"type":"notFound"}}}`))
	})
	c := newTestClient(t, srv, nil, "")
	desc := "x"

	err := c.Domains().Update(context.Background(), "d1", DomainPatch{Description: &desc})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// TestDomainsDelete_Simple checks a domain with no linked objects.
func TestDomainsDelete_Simple(t *testing.T) {
	var requests atomic.Int32
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		method, args := readCall(t, r)
		if method != domainSetMethod {
			t.Errorf("method = %q, want %q", method, domainSetMethod)
		}
		if ids, _ := args["destroy"].([]any); len(ids) != 1 || ids[0] != "d1" {
			t.Errorf("destroy = %v, want [d1]", args["destroy"])
		}
		writeJSONBody(w, domainSetResponse(`{"destroyed":["d1"]}`))
	})
	c := newTestClient(t, srv, nil, "")

	if err := c.Domains().Delete(context.Background(), "d1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("server received %d requests, want 1", n)
	}
}

// TestDomainsDelete_NotFound checks that an unknown ID maps to ErrNotFound.
func TestDomainsDelete_NotFound(t *testing.T) {
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONBody(w, domainSetResponse(`{"notDestroyed":{"d1":{"type":"notFound"}}}`))
	})
	c := newTestClient(t, srv, nil, "")

	if err := c.Domains().Delete(context.Background(), "d1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// deleteSequence is a fake server for a domain with linked objects. The first
// destroy of the domain is blocked by linked; later calls succeed unless
// stillLinked keeps the domain blocked. dkimDenied makes the DKIM removal
// forbidden. calls records the method of every call, in order.
func deleteSequence(t *testing.T, linked string, dkimDenied, stillLinked bool, calls *[]string) http.HandlerFunc {
	t.Helper()
	var domainDestroys atomic.Int32
	return func(w http.ResponseWriter, r *http.Request) {
		method, args := readCall(t, r)
		*calls = append(*calls, method)
		ids, _ := args["destroy"].([]any)
		id, _ := ids[0].(string)

		switch method {
		case domainSetMethod:
			n := domainDestroys.Add(1)
			if n == 1 || (stillLinked && n == 2) {
				writeJSONBody(w, domainSetResponse(`{"notDestroyed":{"`+id+`":{"type":"objectIsLinked","objectId":"`+id+`","linkedObjects":`+linked+`}}}`))
				return
			}
			writeJSONBody(w, domainSetResponse(`{"destroyed":["`+id+`"]}`))
		case dkimSignatureSetMethod:
			if dkimDenied {
				writeJSONBody(w, methodErrorResponse("forbidden"))
				return
			}
			// The response must name the same method as the request.
			writeJSONBody(w, `{"methodResponses":[["`+dkimSignatureSetMethod+`",{"destroyed":["`+id+`"]},"c1"]]}`)
		default:
			t.Errorf("unexpected method %q", method)
		}
	}
}

// TestDomainsDelete_RemovesDKIMSignature checks the full sequence: the domain
// destroy is blocked by a DkimSignature, the signature is removed, and the
// domain is destroyed.
func TestDomainsDelete_RemovesDKIMSignature(t *testing.T) {
	var calls []string
	srv := startTestServer(t, deleteSequence(t,
		`[{"object":"DkimSignature","id":"k1"}]`, false, false, &calls))
	c := newTestClient(t, srv, nil, "")

	if err := c.Domains().Delete(context.Background(), "d1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	want := []string{domainSetMethod, dkimSignatureSetMethod, domainSetMethod}
	if strings.Join(calls, " ") != strings.Join(want, " ") {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

// TestDomainsDelete_OtherLinkedTypeRemovesNothing checks that a linked type
// other than DkimSignature stops Delete before any removal.
func TestDomainsDelete_OtherLinkedTypeRemovesNothing(t *testing.T) {
	var calls []string
	srv := startTestServer(t, deleteSequence(t,
		`[{"object":"DkimSignature","id":"k1"},{"object":"Mailbox","id":"m1"}]`, false, false, &calls))
	c := newTestClient(t, srv, nil, "")

	err := c.Domains().Delete(context.Background(), "d1")
	var linkedErr *LinkedObjectsError
	if !errors.As(err, &linkedErr) {
		t.Fatalf("error = %v, want *LinkedObjectsError", err)
	}
	if len(linkedErr.Objects) != 2 {
		t.Errorf("Objects = %+v, want both linked objects", linkedErr.Objects)
	}
	if len(calls) != 1 {
		t.Errorf("calls = %v, want only the first destroy", calls)
	}
	if strings.Contains(err.Error(), "k1") || strings.Contains(err.Error(), "m1") {
		t.Errorf("error text contains an object ID: %q", err.Error())
	}
}

// TestDomainsDelete_DKIMRemovalForbidden checks that a denied DKIM removal stops
// Delete, matches ErrForbidden, and does not retry the domain destroy.
func TestDomainsDelete_DKIMRemovalForbidden(t *testing.T) {
	var calls []string
	srv := startTestServer(t, deleteSequence(t,
		`[{"object":"DkimSignature","id":"k1"}]`, true, false, &calls))
	c := newTestClient(t, srv, nil, "")

	err := c.Domains().Delete(context.Background(), "d1")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
	if len(calls) != 2 {
		t.Errorf("calls = %v, want the first destroy and the denied DKIM removal", calls)
	}
}

// TestDomainsDelete_StillLinked checks that a domain that is still blocked after
// the DKIM removal returns a *LinkedObjectsError.
func TestDomainsDelete_StillLinked(t *testing.T) {
	var calls []string
	srv := startTestServer(t, deleteSequence(t,
		`[{"object":"DkimSignature","id":"k1"}]`, false, true, &calls))
	c := newTestClient(t, srv, nil, "")

	var linkedErr *LinkedObjectsError
	if err := c.Domains().Delete(context.Background(), "d1"); !errors.As(err, &linkedErr) {
		t.Errorf("error = %v, want *LinkedObjectsError", err)
	}
}

// TestDomainsDelete_LinkedWithoutID checks that a linked object with no usable
// ID is a decoding error, and that nothing is removed.
func TestDomainsDelete_LinkedWithoutID(t *testing.T) {
	var calls []string
	srv := startTestServer(t, deleteSequence(t,
		`[{"object":"DkimSignature","id":""}]`, false, false, &calls))
	c := newTestClient(t, srv, nil, "")

	if err := c.Domains().Delete(context.Background(), "d1"); !errors.Is(err, ErrDecode) {
		t.Errorf("error = %v, want ErrDecode", err)
	}
	if len(calls) != 1 {
		t.Errorf("calls = %v, want only the first destroy", calls)
	}
}

// TestDomainsDelete_InvalidIDNoRequest checks that an invalid ID is refused
// before any request.
func TestDomainsDelete_InvalidIDNoRequest(t *testing.T) {
	var requests atomic.Int32
	srv := startTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	})
	c := newTestClient(t, srv, nil, "")

	if err := c.Domains().Delete(context.Background(), ""); err == nil {
		t.Error("Delete with an empty ID returned no error")
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("server received %d requests, want none", n)
	}
}

// TestRejectedError_Error checks the error text, including the empty
// description case.
func TestRejectedError_Error(t *testing.T) {
	if got := (&RejectedError{Type: "validationFailed"}).Error(); got != "stalwart: write rejected: validationFailed" {
		t.Errorf("Error() = %q", got)
	}
	if got := (&RejectedError{Type: "t", Description: "d"}).Error(); got != "stalwart: write rejected: t: d" {
		t.Errorf("Error() = %q", got)
	}
}
