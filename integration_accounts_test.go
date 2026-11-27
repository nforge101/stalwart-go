//go:build integration

package stalwart

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// accountGetMethod is the JMAP method that reads accounts. The M4-S1 probe
// validates it before the SDK exposes it.
const accountGetMethod = "x:Account/get"

// mailboxGetMethod is a candidate method name for mailboxes. The M4-S1 probe
// only records whether the server knows it; it does not assume the name is
// right. An unknown-method answer is a valid finding.
const mailboxGetMethod = "x:Mailbox/get"

// TestIntegration_AccountProbe reads every account on the server and records
// the response fields and the JSON type of each account field. It is read-only
// and logs field names, types, and counts, never values, because account data
// can contain personal information.
func TestIntegration_AccountProbe(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	raw, err := c.callJMAP(testContext(t), accountGetMethod, map[string]any{"ids": nil})
	if err != nil {
		failRequest(t, accountGetMethod, err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("response is not a JSON object: %v", err)
	}
	var got struct {
		List []map[string]json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("response is not an object with a list of objects: %v", err)
	}

	t.Logf("observed: %s returned %d account(s); response fields: %s",
		accountGetMethod, len(got.List), strings.Join(sortedKeys(top), ", "))
	logFieldKinds(t, got.List)
	logStringValues(t, "@type", got.List)
	logAccountRelations(t, c, got.List)
}

// TestIntegration_MailboxProbe checks whether the server answers the candidate
// mailbox method. It logs the method error type when there is one, and fails
// only for a transport, authentication, or permission problem.
func TestIntegration_MailboxProbe(t *testing.T) {
	c := newIntegrationClient(t, envTestAPIKey)

	_, err := c.callJMAP(testContext(t), mailboxGetMethod, map[string]any{"ids": nil})
	var me *MethodError
	switch {
	case err == nil:
		t.Logf("observed: %s answered", mailboxGetMethod)
	case errors.As(err, &me):
		t.Logf("observed: %s answered with method error type %q", mailboxGetMethod, me.Type)
		if me.Type == "forbidden" {
			t.Errorf("%s is forbidden for this key; add the mailbox read permission to the read key", mailboxGetMethod)
		}
	default:
		failRequest(t, mailboxGetMethod, err)
	}
}

// logFieldKinds logs, for each field that appears in any object, the JSON type
// of its value and how many objects have it. Objects missing the field are
// counted as absent.
func logFieldKinds(t *testing.T, objs []map[string]json.RawMessage) {
	t.Helper()
	seen := map[string]bool{}
	for _, o := range objs {
		for k := range o {
			seen[k] = true
		}
	}
	fields := make([]string, 0, len(seen))
	for k := range seen {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	for _, f := range fields {
		counts := map[string]int{}
		for _, o := range objs {
			v, ok := o[f]
			if !ok {
				counts["absent"]++
				continue
			}
			counts[jsonKind(v)]++
		}
		kinds := make([]string, 0, len(counts))
		for k := range counts {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for i, k := range kinds {
			kinds[i] = fmt.Sprintf("%s x%d", k, counts[k])
		}
		t.Logf("observed: field %s: %s", f, strings.Join(kinds, "; "))
	}
}

// maxTagLen bounds a value that logStringValues prints.
const maxTagLen = 32

// logStringValues logs the distinct string values of field and how many objects
// have each. Use it only for fields whose values are type labels, never for
// names, addresses, or identifiers. A longer value is replaced by a placeholder.
func logStringValues(t *testing.T, field string, objs []map[string]json.RawMessage) {
	t.Helper()
	counts := map[string]int{}
	for _, o := range objs {
		v, ok := o[field]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			continue
		}
		if len(s) > maxTagLen {
			s = "<long value>"
		}
		counts[s]++
	}
	values := make([]string, 0, len(counts))
	for v := range counts {
		values = append(values, v)
	}
	sort.Strings(values)
	for i, v := range values {
		values[i] = fmt.Sprintf("%q x%d", v, counts[v])
	}
	t.Logf("observed: field %s values: %s", field, strings.Join(values, "; "))
}

// logAccountRelations records how an account's name, emailAddress, and domainId
// relate, as counts only. It reads the domains to resolve each domainId and
// prints no names or addresses.
func logAccountRelations(t *testing.T, c *Client, accounts []map[string]json.RawMessage) {
	t.Helper()
	domains, err := c.Domains().List(testContext(t))
	if err != nil {
		failRequest(t, "Domains().List for the account probe", err)
	}
	byID := make(map[string]string, len(domains))
	for _, d := range domains {
		byID[d.ID] = d.Name
	}

	var nameHasAt, domainKnown, addressMatches int
	for _, a := range accounts {
		var name, address, domainID string
		_ = json.Unmarshal(a["name"], &name)
		_ = json.Unmarshal(a["emailAddress"], &address)
		_ = json.Unmarshal(a["domainId"], &domainID)
		if strings.Contains(name, "@") {
			nameHasAt++
		}
		domain, known := byID[domainID]
		if known {
			domainKnown++
		}
		if known && address == name+"@"+domain {
			addressMatches++
		}
	}
	t.Logf("observed: of %d account(s): name contains '@': %d; domainId matches a domain on the server: %d; emailAddress equals name@<that domain>: %d",
		len(accounts), nameHasAt, domainKnown, addressMatches)
}

// sortedKeys returns the keys of m in sorted order.
func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
