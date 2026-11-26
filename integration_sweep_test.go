//go:build integration

package stalwart

import (
	"errors"
	"os"
	"regexp"
	"testing"
)

// envTestSweep controls whether the sweep deletes anything. Unset, or any value
// other than sweepDeleteValue, the sweep only counts the matching domains.
const (
	envTestSweep     = "STALWART_TEST_SWEEP"
	sweepDeleteValue = "delete"
)

// TestIntegration_SweepDisposableDomains removes disposable domains that earlier
// write tests left behind when their cleanup failed. It only considers names of
// the exact form the write tests create, sdk-it-<12 hex>.<parent>, where the
// parent is STALWART_TEST_PARENT_DOMAIN. Every other domain on the server is
// ignored.
//
// It needs the write guard (see docs/testing.md). Without STALWART_TEST_SWEEP=delete
// it only reports how many domains match. Run it by name:
//
//	go test -tags=integration -count=1 -v -run TestIntegration_SweepDisposableDomains -o integration.test.exe .
func TestIntegration_SweepDisposableDomains(t *testing.T) {
	requireWrites(t)
	w := newIntegrationClient(t, envTestWriteKey)
	parent := requireParentDomain(t, w)

	domains, err := w.Domains().List(testContext(t))
	if err != nil {
		failRequest(t, "Domains().List", err)
	}

	pattern := regexp.MustCompile(
		`^` + regexp.QuoteMeta(disposablePrefix) + `[0-9a-f]{12}\.` + regexp.QuoteMeta(parent) + `$`)
	var matches []Domain
	for _, d := range domains {
		if pattern.MatchString(d.Name) {
			matches = append(matches, d)
		}
	}
	t.Logf("observed: %d of %d domains on the server match the disposable pattern", len(matches), len(domains))
	if len(matches) == 0 {
		return
	}

	if os.Getenv(envTestSweep) != sweepDeleteValue {
		t.Logf("dry run: nothing deleted; set %s=%s to delete the %d matching domain(s)",
			envTestSweep, sweepDeleteValue, len(matches))
		return
	}

	removed, failed := 0, 0
	for i, d := range matches {
		err := w.Domains().Delete(testContext(t), d.ID)
		if err == nil || errors.Is(err, ErrNotFound) {
			removed++
			continue
		}
		failed++
		t.Errorf("could not delete matching domain #%d (%s): %s; remove it by hand",
			i+1, labelDomain(d.Name), redactDomainNames(err.Error(), d.Name, parent))
	}
	t.Logf("observed: sweep removed %d of %d matching domain(s); failed %d", removed, len(matches), failed)
}
