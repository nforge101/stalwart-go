//go:build integration

package stalwart

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestIntegration_DomainsWriteOperations exercises the exported Create, Update,
// and Delete on a disposable domain, behind the same write guard as the other
// write tests (see docs/testing.md). Create uses Stalwart's default settings, so
// the domain has an automatic DKIM signature, and Delete must remove it.
func TestIntegration_DomainsWriteOperations(t *testing.T) {
	requireWrites(t)
	w := newIntegrationClient(t, envTestWriteKey)
	parent := requireParentDomain(t, w)
	name := disposableName(t, parent)

	// Create.
	d, err := w.Domains().Create(testContext(t), name)
	if err != nil {
		failWriteOp(t, "Domains().Create", err)
	}
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		if err := w.Domains().Delete(testContext(t), d.ID); err != nil && !errors.Is(err, ErrNotFound) {
			t.Errorf("cleanup: could not delete the disposable domain %q: %s; remove it by hand",
				labelDomain(name), scrub(err.Error()))
		}
	})
	if d.Name != name || d.ID == "" {
		t.Errorf("Create returned a domain with an unexpected ID or name")
	}
	t.Logf("observed: Create returned a domain; ID form: %s", describeIDForm(d.ID))

	// A second create with the same name is refused as already existing.
	if _, err := w.Domains().Create(testContext(t), name); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate Create error = %v, want ErrAlreadyExists", err)
	} else {
		t.Logf("observed: duplicate Create returned ErrAlreadyExists")
	}

	// A name with spaces is refused before any request is sent.
	if _, err := w.Domains().Create(testContext(t), "not a domain."+parent); err == nil {
		t.Errorf("Create accepted a name with spaces")
	} else if strings.Contains(err.Error(), parent) {
		t.Errorf("the validation error names the parent domain")
	} else {
		t.Logf("observed: a name with spaces is refused locally")
	}

	// Update both properties and read them back.
	desc := "sdk integration test"
	enable := false
	if err := w.Domains().Update(testContext(t), d.ID, DomainPatch{Description: &desc, Enable: &enable}); err != nil {
		failWriteOp(t, "Domains().Update", err)
	}
	item := readDomain(t, w, d.ID, "name", "description", "enable")
	if stringField(item, "description") != desc {
		t.Errorf("description did not read back after Update")
	}
	var flags struct {
		Enable *bool `json:"enable"`
	}
	if err := json.Unmarshal(item, &flags); err != nil || flags.Enable == nil || *flags.Enable {
		t.Errorf("enable did not read back as false after Update")
	}
	t.Logf("observed: Update set description and enable, and both read back")

	// An empty patch is refused locally.
	if err := w.Domains().Update(testContext(t), d.ID, DomainPatch{}); err == nil {
		t.Errorf("Update accepted an empty patch")
	}

	// Delete removes the automatic DKIM signature and then the domain.
	if err := w.Domains().Delete(testContext(t), d.ID); err != nil {
		var linkedErr *LinkedObjectsError
		switch {
		case errors.Is(err, ErrForbidden):
			t.Fatalf("Delete was denied. Removing the domain's DKIM signature needs permission to destroy " +
				"DkimSignature objects, which is separate from the domain permissions. Add " +
				"\"Signatures Management: Destroy signatures\" to the write key (see docs/testing.md)")
		case errors.As(err, &linkedErr):
			t.Fatalf("Delete was refused by %d linked object(s) of types other than DkimSignature", len(linkedErr.Objects))
		default:
			failWriteOp(t, "Domains().Delete", err)
		}
	}
	deleted = true
	t.Logf("observed: Delete removed the domain and its DKIM signature")

	// After delete the domain is gone, and deleting again reports not found.
	if _, err := w.Domains().Get(testContext(t), d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete error = %v, want ErrNotFound", err)
	}
	if err := w.Domains().Delete(testContext(t), d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v, want ErrNotFound", err)
	}
	t.Logf("observed: Get and a second Delete both report ErrNotFound")
}

// failWriteOp fails the test for an exported write operation that returned err.
// Denials name the permissions that the write key needs.
func failWriteOp(t *testing.T, what string, err error) {
	t.Helper()
	if errors.Is(err, ErrForbidden) {
		t.Fatalf("%s was denied. The write key needs authenticate, sysDomainGet, sysDomainCreate, "+
			"sysDomainUpdate, and sysDomainDestroy (see docs/testing.md)", what)
	}
	failRequest(t, what, err)
}
