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

	// invalidTestKey is a well-formed but unissued API key.
	invalidTestKey = "API_integration-test-invalid-key"
)

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

// domainGetArgs asks for every domain, returning only the name property. Per
// RFC 8620, a null "ids" means all records. Stalwart's documentation shows no
// accountId argument for management calls, so none is sent.
func domainGetArgs() map[string]any {
	return map[string]any{"ids": nil, "properties": []string{"name"}}
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
	if u.Path != jmapPath {
		t.Errorf("session apiUrl path = %q, want %q", u.Path, jmapPath)
	}

	t.Logf("observed: session fields: %s", strings.Join(jsonKeys(mustMarshal(t, session)), ", "))
	t.Logf("observed: session capabilities: %s", strings.Join(names, ", "))
	t.Logf("observed: session apiUrl path: %q", u.Path)
}

// TestIntegration_DomainGet is the M2 acceptance check: a read-only management
// call, x:Domain/get, succeeds with a key that holds sysDomainGet.
func TestIntegration_DomainGet(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	args, err := c.callJMAP(testContext(t), domainGetMethod, domainGetArgs())
	if err != nil {
		failRequest(t, "POST "+jmapPath+" "+domainGetMethod, err)
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
// response. callJMAP must report it as a *MethodError matching ErrForbidden.
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

	_, err := c.callJMAP(testContext(t), domainGetMethod, domainGetArgs())
	if err == nil {
		t.Fatalf("a key without sysDomainGet was allowed to call %s; "+
			"check that it holds only the sign-in permission", domainGetMethod)
	}

	var methodErr *MethodError
	if !errors.As(err, &methodErr) {
		failRequest(t, "POST "+jmapPath+" "+domainGetMethod+" (a method-level denial is expected, not an HTTP error)", err)
	}
	if methodErr.Type != "forbidden" {
		t.Errorf("method error type = %q, want %q", methodErr.Type, "forbidden")
	}
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("method error does not match ErrForbidden")
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Errorf("method error unexpectedly matches ErrUnauthorized")
	}

	// Log whether a description was returned, not its text, so output stays
	// free of server-supplied values.
	t.Logf("observed: denied with HTTP 200 and a method-level error of type %q; description present: %t",
		methodErr.Type, methodErr.Description != "")
}

// TestIntegration_DomainsList checks Domains().List against the server. The
// test server must hold at least one domain.
func TestIntegration_DomainsList(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	domains, err := c.Domains().List(testContext(t))
	if err != nil {
		failRequest(t, "Domains().List", err)
	}
	if len(domains) == 0 {
		t.Fatal("the test server has no domains; add one before running the integration tests")
	}
	for i, d := range domains {
		if d.ID == "" || d.Name == "" {
			t.Errorf("domain %d lacks an ID or name", i)
		}
	}
	t.Logf("observed: Domains().List returned %d domain(s)", len(domains))
}

// TestIntegration_DomainsGet checks that Domains().Get returns the same domain
// that List reports for its ID.
func TestIntegration_DomainsGet(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	domains, err := c.Domains().List(testContext(t))
	if err != nil {
		failRequest(t, "Domains().List", err)
	}
	if len(domains) == 0 {
		t.Fatal("the test server has no domains; add one before running the integration tests")
	}

	want := domains[0]
	got, err := c.Domains().Get(testContext(t), want.ID)
	if err != nil {
		failRequest(t, "Domains().Get", err)
	}
	if got != want {
		t.Errorf("Get returned a different domain from List for the same ID")
	}
	t.Logf("observed: Domains().Get returned the domain that List reported")
}

// TestIntegration_DomainsGetUnknown checks that an ID the server does not have
// is reported as ErrNotFound, not as a transport or decoding error.
func TestIntegration_DomainsGetUnknown(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	_, err := c.Domains().Get(testContext(t), "integration-test-missing-domain")
	if !errors.Is(err, ErrNotFound) {
		failRequest(t, "Domains().Get with an unknown ID (want ErrNotFound)", err)
	}
	t.Logf("observed: unknown domain ID reported as ErrNotFound")
}

// TestIntegration_DomainsListNoPermission checks that a key without
// sysDomainGet cannot list domains, and that the denial is a *MethodError
// matching ErrForbidden.
func TestIntegration_DomainsListNoPermission(t *testing.T) {
	c := newIntegrationClient(t, envTestNoPermKey)

	_, err := c.Domains().List(testContext(t))
	if err == nil {
		t.Fatal("a key without sysDomainGet listed domains")
	}
	var methodErr *MethodError
	if !errors.As(err, &methodErr) || !errors.Is(err, ErrForbidden) {
		failRequest(t, "Domains().List with a no-permission key (want a forbidden *MethodError)", err)
	}
	t.Logf("observed: Domains().List denied with method-level error of type %q", methodErr.Type)
}

// TestIntegration_DomainFieldProbe records which documented Domain properties
// the server returns for x:Domain/get. It reads only and never fails for a
// missing property, because its purpose is to inform which fields to expose.
// Only property names are logged, never values.
func TestIntegration_DomainFieldProbe(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	candidates := []string{
		"name", "enable", "description", "aliases", "createdAt",
		"catchAllAddress", "subAddressing", "allowRelaying", "reportAddressUri",
		"certificateManagement", "dkimManagement", "dnsManagement",
	}
	args, err := c.callJMAP(testContext(t), domainGetMethod, map[string]any{
		"ids":        nil,
		"properties": candidates,
	})
	if err != nil {
		var methodErr *MethodError
		if errors.As(err, &methodErr) {
			t.Logf("observed: probe refused with method-level error of type %q; no fields recorded", methodErr.Type)
			return
		}
		failRequest(t, "probe x:Domain/get", err)
	}

	var got struct {
		List []json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(args, &got); err != nil {
		t.Fatalf("probe response is not an object: %v", err)
	}
	if len(got.List) == 0 {
		t.Logf("observed: probe returned no domains; no fields recorded")
		return
	}
	present := jsonKeys(got.List[0])
	t.Logf("observed: domain properties returned by the server: %s", strings.Join(present, ", "))
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
