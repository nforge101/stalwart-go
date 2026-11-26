//go:build integration

package stalwart

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

// Write validation tests create, change, and delete domains on a real,
// dedicated Stalwart server. They need the same "integration" build tag as the
// read-only tests, plus an explicit opt-in, a separate API key, and a parent
// domain:
//
//	STALWART_TEST_ALLOW_WRITES=1       enables the write tests (any other value skips them)
//	STALWART_TEST_WRITE_API_KEY        API key allowed to create, update, and destroy domains
//	STALWART_TEST_PARENT_DOMAIN        a domain already on the test server; disposable domains are created under it
//
// Every domain the tests create is a random subdomain of the parent domain,
// named sdk-it-<random>.<parent>. Each created domain is destroyed when its test
// ends. Never point these tests at a production server.
//
// Output follows the read-only convention: field names, types, and statuses.
// Rejection descriptions are printed with the disposable and parent domain
// names replaced by placeholders. A failed cleanup prints the disposable name
// so that it can be removed by hand.
const (
	envTestWriteKey     = "STALWART_TEST_WRITE_API_KEY"
	envTestAllowWrites  = "STALWART_TEST_ALLOW_WRITES"
	envTestParentDomain = "STALWART_TEST_PARENT_DOMAIN"

	// testDomainSetMethod is the JMAP method that creates, updates, and destroys
	// domains. It is not exported by the SDK yet; these tests validate it first.
	testDomainSetMethod = "x:Domain/set"

	disposablePrefix = "sdk-it-"

	// maxRejectionText bounds how much of a rejection description is printed.
	maxRejectionText = 300
)

// domainSetResult holds the arguments of an x:Domain/set response, which follow
// the standard JMAP /set shape (RFC 8620, section 5.3).
type domainSetResult struct {
	Created      map[string]json.RawMessage `json:"created"`
	NotCreated   map[string]json.RawMessage `json:"notCreated"`
	Updated      map[string]json.RawMessage `json:"updated"`
	NotUpdated   map[string]json.RawMessage `json:"notUpdated"`
	Destroyed    []string                   `json:"destroyed"`
	NotDestroyed map[string]json.RawMessage `json:"notDestroyed"`
}

// createOutcome is the result of one create request.
type createOutcome struct {
	id        string          // ID of the created domain, empty if none
	created   json.RawMessage // the created object, nil if the create was rejected
	rejection json.RawMessage // the rejection, nil if the create was accepted
	topFields []string        // field names of the whole response
}

// requireWrites skips the test unless the write guard is set. Skipping is
// deliberate and visible with -v; the read-only tests never skip.
func requireWrites(t *testing.T) {
	t.Helper()
	if os.Getenv(envTestAllowWrites) != "1" {
		t.Skipf("write tests are disabled; set %s=1 to create and delete disposable domains (see docs/testing.md)", envTestAllowWrites)
	}
}

// requireParentDomain returns the parent domain from the environment and checks
// that the server has it. Disposable domains are created under it, so the test
// fails before any create if it is missing. Neither the value nor the list of
// domains is printed.
func requireParentDomain(t *testing.T, w *Client) string {
	t.Helper()
	parent := requireEnv(t, envTestParentDomain)
	if strings.ContainsAny(parent, " /\\") || !strings.Contains(parent, ".") {
		t.Fatalf("%s must be a domain name such as example.test (see docs/testing.md)", envTestParentDomain)
	}
	domains, err := w.Domains().List(testContext(t))
	if err != nil {
		failRequest(t, "Domains().List for the parent domain", err)
	}
	for _, d := range domains {
		if d.Name == parent {
			return parent
		}
	}
	t.Fatalf("the parent domain named by %s is not on the test server; add it before running the write tests", envTestParentDomain)
	return ""
}

// disposableName returns a random name for a domain that a test creates. The
// name is a subdomain of parent, so it lives under a domain the server already
// has.
func disposableName(t *testing.T, parent string) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random domain name: %v", err)
	}
	return disposablePrefix + hex.EncodeToString(b) + "." + parent
}

// minRedactedLen is the shortest name that redactDomainNames replaces. Domain
// IDs can be a single character, and replacing one would mangle the text.
const minRedactedLen = 4

// redactDomainNames replaces each named domain in s with a placeholder, then
// removes the test server's URL and host. Names shorter than minRedactedLen are
// left alone. The names are checked in the order given, so list the longest name
// first.
func redactDomainNames(s string, names ...string) string {
	for _, n := range names {
		if len(n) >= minRedactedLen {
			s = strings.ReplaceAll(s, n, "<domain>")
		}
	}
	return scrub(s)
}

// describeRejection summarizes a /set rejection: its field names, its type, the
// names of the properties it blames, and its description. Each name in names is
// replaced by a placeholder in the description.
func describeRejection(raw json.RawMessage, names ...string) string {
	keys := jsonKeys(raw)
	if keys == nil {
		return "the rejection is not a JSON object"
	}
	desc := "fields " + strings.Join(keys, ", ")
	if typ := stringField(raw, "type"); typ != "" {
		desc += `; type "` + typ + `"`
	}
	var blamed struct {
		Properties []string `json:"properties"`
	}
	if json.Unmarshal(raw, &blamed) == nil && len(blamed.Properties) > 0 {
		desc += "; properties " + strings.Join(blamed.Properties, ", ")
	}
	if text := stringField(raw, "description"); text != "" {
		desc += fmt.Sprintf("; description %q", truncateText(redactDomainNames(text, names...)))
	}
	var linked struct {
		Objects json.RawMessage `json:"linkedObjects"`
	}
	if json.Unmarshal(raw, &linked) == nil && len(linked.Objects) > 0 {
		desc += "; linkedObjects " + truncateText(redactDomainNames(string(linked.Objects), names...))
	}
	return desc
}

// truncateText shortens s to maxRejectionText bytes.
func truncateText(s string) string {
	if len(s) > maxRejectionText {
		return s[:maxRejectionText] + "..."
	}
	return s
}

// labelDomain returns name with the parent domain replaced by a placeholder, so
// that a message can identify a disposable domain without revealing the parent.
func labelDomain(name string) string {
	if parent := os.Getenv(envTestParentDomain); parent != "" {
		return strings.ReplaceAll(name, parent, "<parent>")
	}
	return name
}

// describeSetCounts summarizes the shape of a set response: its field names and
// how many entries each result map or list holds. It prints no values.
func describeSetCounts(res domainSetResult, top []string) string {
	return fmt.Sprintf("fields %s; updated %d, notUpdated %d, destroyed %d, notDestroyed %d",
		strings.Join(top, ", "), len(res.Updated), len(res.NotUpdated), len(res.Destroyed), len(res.NotDestroyed))
}

// describeIDForm describes the shape of an ID without revealing it: its length
// and which kinds of characters it uses.
func describeIDForm(id string) string {
	var lower, upper, digit, other bool
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			other = true
		}
	}
	var kinds []string
	if lower {
		kinds = append(kinds, "lowercase letters")
	}
	if upper {
		kinds = append(kinds, "uppercase letters")
	}
	if digit {
		kinds = append(kinds, "digits")
	}
	if other {
		kinds = append(kinds, "other characters")
	}
	return fmt.Sprintf("length %d; %s", len(id), strings.Join(kinds, ", "))
}

// unknownDomainID returns an ID that looks like real but names no domain. It
// changes the last character of real until Domains().Get reports the result as
// not found, so the ID has the same form as an ID the server issues.
func unknownDomainID(t *testing.T, c *Client, real string) string {
	t.Helper()
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i := 0; i < len(alphabet); i++ {
		candidate := real[:len(real)-1] + string(alphabet[i])
		if candidate == real {
			continue
		}
		_, err := c.Domains().Get(testContext(t), candidate)
		if errors.Is(err, ErrNotFound) {
			return candidate
		}
		if err != nil {
			failRequest(t, "Domains().Get while choosing an unknown ID", err)
		}
	}
	t.Fatalf("could not find an unused ID to use as an unknown ID")
	return ""
}

// describeUpdated summarizes an entry of the "updated" map. JMAP allows null
// or an object with the properties the server changed on its own.
func describeUpdated(raw json.RawMessage) string {
	if string(raw) == "null" {
		return "null"
	}
	if keys := jsonKeys(raw); keys != nil {
		return "object with fields " + strings.Join(keys, ", ")
	}
	return "neither null nor an object"
}

// failSet fails the test for an x:Domain/set call that returned err, naming
// the likely cause when the key lacks permission.
func failSet(t *testing.T, err error) {
	t.Helper()
	var methodErr *MethodError
	if errors.As(err, &methodErr) && errors.Is(err, ErrForbidden) {
		t.Fatalf("%s was denied (method error %q). The write key needs authenticate, sysDomainGet, "+
			"sysDomainCreate, sysDomainUpdate, and sysDomainDestroy (see docs/testing.md)",
			testDomainSetMethod, methodErr.Type)
	}
	failRequest(t, testDomainSetMethod, err)
}

// domainSetRaw sends one x:Domain/set call and decodes the standard result. It
// returns the error from callJMAP unchanged, so callers can inspect it.
func domainSetRaw(t *testing.T, c *Client, args map[string]any) (domainSetResult, []string, error) {
	t.Helper()
	raw, err := c.callJMAP(testContext(t), testDomainSetMethod, args)
	if err != nil {
		return domainSetResult{}, nil, err
	}
	var res domainSetResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return domainSetResult{}, nil, fmt.Errorf("%w: response is not a JMAP set result", ErrDecode)
	}
	return res, jsonKeys(raw), nil
}

// mustDomainSet is domainSetRaw that fails the test on error.
func mustDomainSet(t *testing.T, c *Client, args map[string]any) (domainSetResult, []string) {
	t.Helper()
	res, top, err := domainSetRaw(t, c, args)
	if err != nil {
		failSet(t, err)
	}
	return res, top
}

// linkedObject identifies an object that blocks a destroy. Stalwart lists these
// in the linkedObjects field of an objectIsLinked rejection.
type linkedObject struct {
	Object string `json:"object"` // the object type, for example DkimSignature
	ID     string `json:"id"`
}

// parseLinkedObjects decodes the linkedObjects field of a rejection.
func parseLinkedObjects(rejection json.RawMessage) []linkedObject {
	var r struct {
		Linked []linkedObject `json:"linkedObjects"`
	}
	if json.Unmarshal(rejection, &r) != nil {
		return nil
	}
	return r.Linked
}

// isObjectName reports whether s is safe to use as the type in a method name:
// a letter followed by letters and digits.
func isObjectName(s string) bool {
	if s == "" || len(s) > 64 || !(s[0] >= 'A' && s[0] <= 'Z' || s[0] >= 'a' && s[0] <= 'z') {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// destroyOne destroys the object id with the given set method, for example
// x:Domain/set. It reports whether the server confirmed the destroy; if not, it
// returns the server's rejection. A response that lists the ID in neither map is
// an error.
func destroyOne(ctx context.Context, c *Client, method, id string) (bool, json.RawMessage, error) {
	raw, err := c.callJMAP(ctx, method, map[string]any{"destroy": []string{id}})
	if err != nil {
		return false, nil, err
	}
	var res domainSetResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return false, nil, fmt.Errorf("%w: response is not a JMAP set result", ErrDecode)
	}
	if slices.Contains(res.Destroyed, id) {
		return true, nil, nil
	}
	rejection, rejected := res.NotDestroyed[id]
	if !rejected {
		return false, nil, fmt.Errorf("%w: the ID is in neither destroyed nor notDestroyed", ErrDecode)
	}
	return false, rejection, nil
}

// destroyDomainAndLinked destroys the domain id. If the server reports that
// other objects are linked to it, it destroys those first and tries once more.
// It returns the types of the linked objects it removed. A non-nil rejection
// means the domain still exists, and says why.
func destroyDomainAndLinked(ctx context.Context, c *Client, id string) (removed []string, rejection json.RawMessage, err error) {
	destroyed, rejection, err := destroyOne(ctx, c, testDomainSetMethod, id)
	if err != nil || destroyed {
		return nil, nil, err
	}
	if stringField(rejection, "type") != "objectIsLinked" {
		return nil, rejection, nil
	}

	for _, l := range parseLinkedObjects(rejection) {
		if !isObjectName(l.Object) || l.ID == "" {
			return removed, rejection, nil
		}
		ok, rej, err := destroyOne(ctx, c, "x:"+l.Object+"/set", l.ID)
		if err != nil {
			return removed, rejection, fmt.Errorf("destroying a linked %s object: %w", l.Object, err)
		}
		if !ok {
			return removed, rej, nil
		}
		removed = append(removed, l.Object)
	}

	destroyed, rejection, err = destroyOne(ctx, c, testDomainSetMethod, id)
	if err != nil || destroyed {
		return removed, nil, err
	}
	return removed, rejection, nil
}

// destroyOnCleanup destroys the domain id when the test ends, first removing any
// objects linked to it. cleaner must be allowed to destroy domains and, for a
// domain that has linked objects, those objects. A domain that is already gone
// is not an error. If the destroy fails, the message names the disposable domain
// so that it can be removed by hand.
func destroyOnCleanup(t *testing.T, cleaner *Client, id, name string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
		defer cancel()

		label := labelDomain(name)
		removed, rejection, err := destroyDomainAndLinked(ctx, cleaner, id)
		if len(removed) > 0 {
			t.Logf("cleanup: removed linked object(s) of type %s before destroying the domain", strings.Join(removed, ", "))
		}
		if err != nil {
			t.Errorf("cleanup: could not destroy the disposable domain %q: %s; remove it by hand", label, scrub(err.Error()))
			return
		}
		if rejection == nil || stringField(rejection, "type") == "notFound" {
			return
		}
		t.Errorf("cleanup: the server did not destroy the disposable domain %q (%s); remove it by hand",
			label, describeRejection(rejection, name, id, os.Getenv(envTestParentDomain)))
	})
}

// createDomain sends one create request with the given properties. If the
// server accepts it, the new domain is registered for cleanup through cleaner.
// name is used only to label that cleanup. The error, if any, is the one
// callJMAP returned.
func createDomain(t *testing.T, c, cleaner *Client, name string, props map[string]any) (createOutcome, error) {
	t.Helper()
	res, top, err := domainSetRaw(t, c, map[string]any{"create": map[string]any{"n1": props}})
	if err != nil {
		return createOutcome{}, err
	}

	out := createOutcome{topFields: top}
	switch created, ok := res.Created["n1"]; {
	case ok:
		out.created = created
		out.id = stringField(created, "id")
		if out.id == "" {
			t.Errorf("the server accepted a domain but returned no ID, so it cannot be cleaned up; "+
				"remove the domain %q by hand", labelDomain(name))
		} else {
			destroyOnCleanup(t, cleaner, out.id, name)
		}
	default:
		rejection, rejected := res.NotCreated["n1"]
		if !rejected {
			return createOutcome{}, fmt.Errorf("%w: create is in neither created nor notCreated", ErrDecode)
		}
		out.rejection = rejection
	}
	return out, nil
}

// updateDomain sends one update. label names the patched properties in test
// output. It returns whether the server accepted the update.
func updateDomain(t *testing.T, c *Client, id, label string, patch map[string]any) bool {
	t.Helper()
	res, top := mustDomainSet(t, c, map[string]any{"update": map[string]any{id: patch}})
	if rejection, ok := res.NotUpdated[id]; ok {
		t.Errorf("update of %s was rejected: %s", label, describeRejection(rejection, id))
		return false
	}
	entry, ok := res.Updated[id]
	if !ok {
		t.Errorf("update of %s is in neither updated nor notUpdated", label)
		return false
	}
	t.Logf("observed: update of %s accepted; response fields: %s; updated entry: %s",
		label, strings.Join(top, ", "), describeUpdated(entry))
	return true
}

// mustField returns the named field of the JSON object raw, failing the test if
// raw is not an object or has no such field.
func mustField(t *testing.T, raw json.RawMessage, name string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("the response is not a JSON object: %v", err)
	}
	v, ok := m[name]
	if !ok {
		t.Fatalf("the response has no %s field", name)
	}
	return v
}

// readDomain returns the named properties of the domain id, read with
// x:Domain/get.
func readDomain(t *testing.T, c *Client, id string, props ...string) json.RawMessage {
	t.Helper()
	raw, err := c.callJMAP(testContext(t), domainGetMethod, map[string]any{
		"ids":        []string{id},
		"properties": props,
	})
	if err != nil {
		failRequest(t, domainGetMethod, err)
	}
	var res struct {
		List []json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || len(res.List) != 1 {
		t.Fatalf("%s did not return exactly one domain for the ID", domainGetMethod)
	}
	return res.List[0]
}

// TestIntegration_DomainWriteLifecycle creates a disposable domain, rejects a
// duplicate create, updates two properties, rejects changes to an unknown ID,
// and destroys the domain. It records the response shapes and rejection types
// that the exported write operations will rely on.
func TestIntegration_DomainWriteLifecycle(t *testing.T) {
	requireWrites(t)
	w := newIntegrationClient(t, envTestWriteKey)
	parent := requireParentDomain(t, w)
	name := disposableName(t, parent)

	// Create. By default Stalwart generates a DKIM signature for a new domain,
	// and that signature blocks a destroy (see TestIntegration_DomainDestroyLinked).
	// This test sets dkimManagement to Manual so that the domain has no linked
	// objects and the key needs no permission beyond the domain ones.
	out, err := createDomain(t, w, w, name, map[string]any{
		"name":           name,
		"dkimManagement": map[string]any{"@type": "Manual"},
	})
	if err != nil {
		failSet(t, err)
	}
	if out.rejection != nil {
		t.Fatalf("create was rejected: %s", describeRejection(out.rejection, name, parent))
	}
	if out.id == "" {
		t.FailNow() // createDomain has already reported the missing ID
	}
	t.Logf("observed: create response fields: %s", strings.Join(out.topFields, ", "))
	t.Logf("observed: created object fields: %s", strings.Join(jsonKeys(out.created), ", "))
	t.Logf("observed: domain ID form: %s", describeIDForm(out.id))

	got, err := w.Domains().Get(testContext(t), out.id)
	if err != nil {
		failRequest(t, "Domains().Get after create", err)
	}
	if got.Name != name {
		t.Errorf("the created domain reads back with a different name")
	}

	dkim := readDomain(t, w, out.id, "name", "dkimManagement")
	if kind := jsonKind(mustField(t, dkim, "dkimManagement")); kind != `object with @type "Manual"` {
		t.Errorf("dkimManagement did not read back as Manual: %s", kind)
	} else {
		t.Logf("observed: a domain created with dkimManagement Manual reads back as Manual")
	}

	// Duplicate create.
	dup, err := createDomain(t, w, w, name, map[string]any{"name": name})
	if err != nil {
		failSet(t, err)
	}
	if dup.created != nil {
		t.Errorf("a second create with the same name was accepted")
	} else {
		t.Logf("observed: duplicate create rejected: %s", describeRejection(dup.rejection, name, parent))
	}

	// Update a string property.
	const description = "sdk integration test"
	if updateDomain(t, w, out.id, "description", map[string]any{"description": description}) {
		item := readDomain(t, w, out.id, "name", "description")
		if stringField(item, "description") != description {
			t.Errorf("description did not read back after the update")
		}
	}

	// Update a boolean property.
	if updateDomain(t, w, out.id, "enable", map[string]any{"enable": false}) {
		item := readDomain(t, w, out.id, "name", "enable")
		var enable struct {
			Enable *bool `json:"enable"`
		}
		switch err := json.Unmarshal(item, &enable); {
		case err != nil || enable.Enable == nil:
			t.Errorf("enable is not a boolean after the update")
		case *enable.Enable:
			t.Errorf("enable is still true after updating it to false")
		default:
			t.Logf("observed: enable is a boolean and reads back false")
		}
	}

	// Unknown ID. The ID has the form of a real ID, so the server treats it as
	// an ID that names nothing, not as malformed input. A literal that is not
	// shaped like a server ID was silently dropped by the server.
	unknown := unknownDomainID(t, w, out.id)

	res, top := mustDomainSet(t, w, map[string]any{
		"update": map[string]any{unknown: map[string]any{"description": description}},
	})
	switch rejection, rejected := res.NotUpdated[unknown]; {
	case rejected:
		t.Logf("observed: update of an unknown ID rejected: %s", describeRejection(rejection, unknown))
	case len(res.Updated) > 0:
		t.Errorf("update of an unknown ID was reported as updated; %s", describeSetCounts(res, top))
	default:
		t.Errorf("update of an unknown ID is in neither updated nor notUpdated; %s", describeSetCounts(res, top))
	}

	res, top = mustDomainSet(t, w, map[string]any{"destroy": []string{unknown}})
	switch rejection, rejected := res.NotDestroyed[unknown]; {
	case slices.Contains(res.Destroyed, unknown):
		t.Errorf("destroy of an unknown ID was reported as destroyed")
	case rejected:
		t.Logf("observed: destroy of an unknown ID rejected: %s", describeRejection(rejection, unknown))
	default:
		t.Errorf("destroy of an unknown ID is in neither destroyed nor notDestroyed; %s", describeSetCounts(res, top))
	}

	// Destroy.
	res, top = mustDomainSet(t, w, map[string]any{"destroy": []string{out.id}})
	if !slices.Contains(res.Destroyed, out.id) {
		t.Fatalf("destroy was not confirmed: %s", describeRejection(res.NotDestroyed[out.id], name, parent, out.id))
	}
	t.Logf("observed: destroy confirmed; response fields: %s", strings.Join(top, ", "))

	_, err = w.Domains().Get(testContext(t), out.id)
	switch {
	case err == nil:
		t.Errorf("the domain is still readable after destroy")
	case !errors.Is(err, ErrNotFound):
		failRequest(t, "Domains().Get after destroy (want ErrNotFound)", err)
	}
}

// TestIntegration_DomainCreateInvalid checks that the server refuses a create
// that lacks a usable name. If it accepts one, the test fails and the domain is
// cleaned up.
func TestIntegration_DomainCreateInvalid(t *testing.T) {
	requireWrites(t)
	w := newIntegrationClient(t, envTestWriteKey)
	parent := requireParentDomain(t, w)

	tests := []struct {
		label      string // describes the case in test output
		name       string // the name sent; used to label cleanup and redact output
		props      map[string]any
		mustReject bool // false records the outcome without failing the test
	}{
		{"missing name", "<no name>", map[string]any{}, true},
		{"empty name", "<empty name>", map[string]any{"name": ""}, true},
		// Stalwart 1.0.1 accepts a name with spaces, so this case records the
		// outcome only. The SDK must validate names itself.
		{"name with spaces", "not a domain." + parent, map[string]any{"name": "not a domain." + parent}, false},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.label, " ", "_"), func(t *testing.T) {
			out, err := createDomain(t, w, w, tt.name, tt.props)
			if err != nil {
				failSet(t, err)
			}
			if out.created != nil {
				if tt.mustReject {
					t.Fatalf("the server accepted a create for %s", tt.label)
				}
				t.Logf("observed: create accepted for %s; the domain will be destroyed at the end of the test", tt.label)
				return
			}
			t.Logf("observed: create rejected for %s: %s", tt.label, describeRejection(out.rejection, tt.name, parent))
		})
	}
}

// TestIntegration_DomainWriteNoPermission checks that a key that can sign in
// but has no domain permissions cannot create a domain. The write key is used
// only to remove a domain if the denial unexpectedly fails.
func TestIntegration_DomainWriteNoPermission(t *testing.T) {
	requireWrites(t)
	w := newIntegrationClient(t, envTestWriteKey)
	parent := requireParentDomain(t, w)
	c := newIntegrationClient(t, envTestNoPermKey)
	name := disposableName(t, parent)

	out, err := createDomain(t, c, w, name, map[string]any{"name": name})
	if err != nil {
		var methodErr *MethodError
		if errors.As(err, &methodErr) && errors.Is(err, ErrForbidden) {
			t.Logf("observed: create denied with a method-level error of type %q", methodErr.Type)
			return
		}
		failRequest(t, "create with a no-permission key", err)
	}
	if out.created != nil {
		t.Fatalf("a key without sysDomainCreate created a domain")
	}
	t.Logf("observed: create denied inside the set result: %s", describeRejection(out.rejection, name, parent))
}

// TestIntegration_DomainDestroyLinked creates a domain with Stalwart's default
// settings, which generate objects linked to the domain, and checks how the
// server reports a destroy that those objects block and how the block is
// resolved. It is not part of the default write run, because removing the
// linked objects needs a permission beyond the domain ones (see
// docs/testing.md). Run it by name:
//
//	go test -tags=integration -count=1 -v -run TestIntegration_DomainDestroyLinked .
func TestIntegration_DomainDestroyLinked(t *testing.T) {
	requireWrites(t)
	w := newIntegrationClient(t, envTestWriteKey)
	parent := requireParentDomain(t, w)
	name := disposableName(t, parent)

	out, err := createDomain(t, w, w, name, map[string]any{"name": name})
	if err != nil {
		failSet(t, err)
	}
	if out.rejection != nil {
		t.Fatalf("create was rejected: %s", describeRejection(out.rejection, name, parent))
	}
	if out.id == "" {
		t.FailNow() // createDomain has already reported the missing ID
	}

	destroyed, rejection, err := destroyOne(testContext(t), w, testDomainSetMethod, out.id)
	if err != nil {
		failSet(t, err)
	}
	if destroyed {
		t.Logf("observed: a domain created with default settings was destroyed without linked objects")
		return
	}
	if typ := stringField(rejection, "type"); typ != "objectIsLinked" {
		t.Fatalf("destroy was rejected with an unexpected type: %s", describeRejection(rejection, name, parent, out.id))
	}
	linked := parseLinkedObjects(rejection)
	counts := map[string]int{}
	for _, l := range linked {
		counts[l.Object]++
	}
	types := make([]string, 0, len(counts))
	for typ, n := range counts {
		types = append(types, fmt.Sprintf("%s x%d", typ, n))
	}
	slices.Sort(types)
	t.Logf("observed: destroy blocked by %d linked object(s): %s", len(linked), strings.Join(types, ", "))

	removed, rejection, err := destroyDomainAndLinked(testContext(t), w, out.id)
	if err != nil {
		var methodErr *MethodError
		if errors.As(err, &methodErr) && errors.Is(err, ErrForbidden) {
			t.Fatalf("removing the linked objects was denied (method error %q). The write key needs "+
				"permission to destroy them (see docs/testing.md)", methodErr.Type)
		}
		failRequest(t, "destroying the domain and its linked objects", err)
	}
	if rejection != nil {
		t.Fatalf("the domain is still blocked after removing its linked objects: %s",
			describeRejection(rejection, name, parent, out.id))
	}
	t.Logf("observed: destroyed after removing %d linked object(s) of type %s", len(removed), strings.Join(removed, ", "))

	_, err = w.Domains().Get(testContext(t), out.id)
	switch {
	case err == nil:
		t.Errorf("the domain is still readable after destroy")
	case !errors.Is(err, ErrNotFound):
		failRequest(t, "Domains().Get after destroy (want ErrNotFound)", err)
	}
}
