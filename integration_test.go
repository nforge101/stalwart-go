//go:build integration

package stalwart

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// Integration tests run against a real, dedicated Stalwart server. They are
// excluded from normal test runs and need the "integration" build tag:
//
//	go test -tags=integration -count=1 -v -run TestIntegration .
//
// They perform read-only requests only. Never point them at a production
// server. Configuration comes from the environment (see docs/testing.md):
//
//	STALWART_TEST_URL              base URL of the test server
//	STALWART_TEST_API_KEY          API key allowed to read domains
//	STALWART_TEST_NOPERM_API_KEY   API key that can sign in but not read domains
//
// Test output records field names, counts, and HTTP statuses only. It never
// prints hostnames, identifiers, domain names, or credentials, so it can be
// shared as validation evidence.
const (
	envTestURL       = "STALWART_TEST_URL"
	envTestAPIKey    = "STALWART_TEST_API_KEY"
	envTestNoPermKey = "STALWART_TEST_NOPERM_API_KEY"

	integrationTimeout = 30 * time.Second

	jmapSessionPath = "/jmap/session"
	jmapAPIPath     = "/jmap/"

	jmapCoreCapability = "urn:ietf:params:jmap:core"
	stalwartCapability = "urn:stalwart:jmap"

	domainGetMethod = "x:Domain/get"

	// invalidTestKey is a well-formed but unissued API key.
	invalidTestKey = "API_integration-test-invalid-key"
)

// jmapRequest is the body of a JMAP API request. Each method call is the
// three-element array [name, arguments, callId].
type jmapRequest struct {
	Using       []string `json:"using"`
	MethodCalls [][]any  `json:"methodCalls"`
}

// jmapResponse is the body of a JMAP API response. Each method response is the
// three-element array [name, arguments, callId].
type jmapResponse struct {
	MethodResponses [][]json.RawMessage `json:"methodResponses"`
}

// requireEnv returns the value of the named environment variable, or fails the
// test with a message that names the variable but never its value.
func requireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("integration environment is incomplete: %s is not set (see docs/testing.md)", name)
	}
	return v
}

// newIntegrationClient returns a Client for the test server that authenticates
// with the key stored in the environment variable keyEnv.
func newIntegrationClient(t *testing.T, keyEnv string) *Client {
	t.Helper()
	return newClientWithKey(t, requireEnv(t, keyEnv))
}

// newClientWithKey returns a Client for the test server that authenticates
// with key. It uses the default HTTP client, so TLS verification relies on the
// operating system trust store.
func newClientWithKey(t *testing.T, key string) *Client {
	t.Helper()
	c, err := NewClient(requireEnv(t, envTestURL), WithBearerToken(key), WithTimeout(integrationTimeout))
	if err != nil {
		t.Fatalf("NewClient: %s", scrub(err.Error()))
	}
	return c
}

// testContext returns a context that expires after integrationTimeout.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	t.Cleanup(cancel)
	return ctx
}

// scrub removes the test server's URL and host from s, so that error text can
// be shared without revealing where the server runs.
func scrub(s string) string {
	base := os.Getenv(envTestURL)
	if base == "" {
		return s
	}
	s = strings.ReplaceAll(s, base, "<"+envTestURL+">")
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		s = strings.ReplaceAll(s, u.Host, "<host>")
	}
	return s
}

// failRequest fails the test for a request that returned err, adding guidance
// for the most common setup problem, an untrusted server certificate.
func failRequest(t *testing.T, what string, err error) {
	t.Helper()
	msg := scrub(err.Error())
	if errors.Is(err, ErrTransport) && strings.Contains(msg, "x509") {
		t.Fatalf("%s: TLS verification failed: %s\n"+
			"Trust the server's CA in your operating system trust store. "+
			"Do not disable certificate verification (see docs/testing.md).", what, msg)
	}
	t.Fatalf("%s: %s", what, msg)
}

// jsonKeys returns the sorted keys of the JSON object in raw, or nil if raw is
// not an object.
func jsonKeys(raw []byte) []string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stringField returns the string value of the named field of the JSON object
// in raw, or "" if it is absent or not a string.
func stringField(raw []byte, name string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(m[name], &s); err != nil {
		return ""
	}
	return s
}

// describeBody summarizes an error body without revealing its values: the
// field names, and the "type" field when present.
func describeBody(body string) string {
	keys := jsonKeys([]byte(body))
	if keys == nil {
		return "body is not a JSON object"
	}
	desc := "JSON object with fields " + strings.Join(keys, ", ")
	if typ := stringField([]byte(body), "type"); typ != "" {
		desc += `; type "` + typ + `"`
	}
	return desc
}

// onlyMethodResponse returns the name, arguments, and call ID of the single
// method response in resp, failing the test if the response has another shape.
func onlyMethodResponse(t *testing.T, resp jmapResponse) (name string, args json.RawMessage, callID string) {
	t.Helper()
	if len(resp.MethodResponses) != 1 {
		t.Fatalf("got %d method responses, want 1", len(resp.MethodResponses))
	}
	mr := resp.MethodResponses[0]
	if len(mr) != 3 {
		t.Fatalf("method response has %d elements, want 3 (name, arguments, call ID)", len(mr))
	}
	if err := json.Unmarshal(mr[0], &name); err != nil {
		t.Fatalf("method response name is not a string: %v", err)
	}
	if err := json.Unmarshal(mr[2], &callID); err != nil {
		t.Fatalf("method response call ID is not a string: %v", err)
	}
	return name, mr[1], callID
}

// domainGetRequest asks for every domain, returning only the name property.
// Per RFC 8620, a null "ids" means all records. Stalwart's documentation shows
// no accountId argument for management calls, so none is sent.
func domainGetRequest() jmapRequest {
	return jmapRequest{
		Using: []string{jmapCoreCapability, stalwartCapability},
		MethodCalls: [][]any{{
			domainGetMethod,
			map[string]any{"ids": nil, "properties": []string{"name"}},
			"c1",
		}},
	}
}

// TestIntegration_Session checks that the session resource is served and
// advertises the JMAP core capability, and that the API URL matches the path
// the SDK uses for management calls.
func TestIntegration_Session(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	var session map[string]json.RawMessage
	if err := c.doJSON(testContext(t), http.MethodGet, jmapSessionPath, nil, &session); err != nil {
		failRequest(t, "GET "+jmapSessionPath, err)
	}

	var capabilities map[string]json.RawMessage
	if err := json.Unmarshal(session["capabilities"], &capabilities); err != nil {
		t.Fatalf("session has no capabilities object: %v", err)
	}
	if _, ok := capabilities[jmapCoreCapability]; !ok {
		t.Errorf("session does not advertise %s", jmapCoreCapability)
	}

	names := make([]string, 0, len(capabilities))
	for name := range capabilities {
		names = append(names, name)
	}
	sort.Strings(names)

	var apiURL string
	if err := json.Unmarshal(session["apiUrl"], &apiURL); err != nil || apiURL == "" {
		t.Fatalf("session has no apiUrl string")
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		t.Fatalf("session apiUrl is not a valid URL")
	}
	if u.Path != jmapAPIPath {
		t.Errorf("session apiUrl path = %q, want %q", u.Path, jmapAPIPath)
	}

	t.Logf("observed: session fields: %s", strings.Join(jsonKeys(mustMarshal(t, session)), ", "))
	t.Logf("observed: session capabilities: %s", strings.Join(names, ", "))
	t.Logf("observed: session apiUrl path: %q", u.Path)
}

// TestIntegration_DomainGet is the M2 acceptance check: a read-only management
// call, x:Domain/get, succeeds with a key that holds sysDomainGet.
func TestIntegration_DomainGet(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	var resp jmapResponse
	if err := c.doJSON(testContext(t), http.MethodPost, jmapAPIPath, domainGetRequest(), &resp); err != nil {
		failRequest(t, "POST "+jmapAPIPath+" "+domainGetMethod, err)
	}

	name, args, callID := onlyMethodResponse(t, resp)
	if name == "error" {
		t.Fatalf("%s returned a method-level error of type %q", domainGetMethod, stringField(args, "type"))
	}
	if name != domainGetMethod {
		t.Fatalf("method response name = %q, want %q", name, domainGetMethod)
	}
	if callID != "c1" {
		t.Errorf("method response call ID = %q, want %q", callID, "c1")
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(args, &got); err != nil {
		t.Fatalf("method response arguments are not an object: %v", err)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(got["list"], &list); err != nil {
		t.Fatalf("method response has no list array: %v", err)
	}
	for i, item := range list {
		if stringField(item, "id") == "" || stringField(item, "name") == "" {
			t.Errorf("domain %d lacks a string id or name", i)
		}
	}

	t.Logf("observed: %s returned %d domain(s)", domainGetMethod, len(list))
	t.Logf("observed: response argument fields: %s", strings.Join(jsonKeys(args), ", "))
	if len(list) > 0 {
		t.Logf("observed: domain object fields: %s", strings.Join(jsonKeys(list[0]), ", "))
	}
}

// TestIntegration_InvalidKey checks that the server rejects an unissued key
// with 401, which the SDK reports as ErrUnauthorized.
func TestIntegration_InvalidKey(t *testing.T) {
	c := newClientWithKey(t, invalidTestKey)

	err := c.doJSON(testContext(t), http.MethodGet, jmapSessionPath, nil, nil)
	if err == nil {
		t.Fatal("server accepted an unissued API key")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		failRequest(t, "GET "+jmapSessionPath, err)
	}
	t.Logf("observed: HTTP %d; %s", apiErr.StatusCode, describeBody(apiErr.Message))
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("error does not match ErrUnauthorized")
	}
}

// TestIntegration_NoPermissionKey checks how Stalwart 1.0.1 denies a key that
// can sign in but lacks sysDomainGet. The HTTP request succeeds, and the denial
// arrives as a method-level JMAP error of type "forbidden" inside a 200
// response. It is not an HTTP 403, so ErrForbidden does not apply.
//
// A key that lacks the sign-in permission is refused earlier, with an HTTP 403
// before any method runs. That case was observed while setting up the test
// keys and is not covered here, because it needs a third key.
func TestIntegration_NoPermissionKey(t *testing.T) {
	c := newIntegrationClient(t, envTestNoPermKey)

	var session map[string]json.RawMessage
	if err := c.doJSON(testContext(t), http.MethodGet, jmapSessionPath, nil, &session); err != nil {
		failRequest(t, "GET "+jmapSessionPath+" (the key must hold the sign-in permission)", err)
	}

	var resp jmapResponse
	if err := c.doJSON(testContext(t), http.MethodPost, jmapAPIPath, domainGetRequest(), &resp); err != nil {
		failRequest(t, "POST "+jmapAPIPath+" "+domainGetMethod+" (a method-level denial is expected, not an HTTP error)", err)
	}

	name, args, callID := onlyMethodResponse(t, resp)
	if name != "error" {
		t.Fatalf("a key without sysDomainGet was allowed to call %s (response %q); "+
			"check that it holds only the sign-in permission", domainGetMethod, name)
	}
	if typ := stringField(args, "type"); typ != "forbidden" {
		t.Errorf("method-level error type = %q, want %q", typ, "forbidden")
	}
	if callID != "c1" {
		t.Errorf("method response call ID = %q, want %q", callID, "c1")
	}

	t.Logf("observed: denied with HTTP 200 and a method-level error of type %q; error fields: %s",
		stringField(args, "type"), strings.Join(jsonKeys(args), ", "))
}

// mustMarshal encodes v as JSON, failing the test on error.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}
