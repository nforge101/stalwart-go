# Testing

This document describes how the SDK is tested and how to run the tests.

## Unit tests (default)

```bash
go test ./...
```

Unit tests never contact a real Stalwart server. They run on any machine with Go 1.26 or later.

### Transport tests

Request and response behavior is tested against a local `httptest.NewTLSServer`. This is a real HTTPS server with a self-signed certificate, running inside the test process. The test client trusts that certificate through the server's own HTTP client (`srv.Client()`). Certificate verification stays enabled; the tests never set `InsecureSkipVerify`.

Other transport tests cover:

- Response body closing and size limits.
- Redaction of credentials echoed in error bodies.
- Timeouts, cancelled contexts, and transport failures.
- Concurrent use of a single `Client` (20 goroutines, 5 requests each).

The `Connection closed` or `TLS handshake error` lines in verbose output are expected. They come from tests that deliberately abandon connections.

### JMAP tests

`jmap_test.go` covers the management call helper against the same local TLS server. It checks the request shape and credentials, a method error for `forbidden` (which matches `ErrForbidden`), other method error types (which match no sentinel), description sanitization, HTTP errors passing through unchanged, and malformed response shapes (which wrap `ErrDecode`). `errors_test.go` checks the error text and unwrapping directly.

### Example tests

`examples/basic` tests inject the environment through a function, so they never read the real environment. They check that the API key never appears in output or error text.

## Race detector

```bash
go test -race ./...
```

The race detector needs cgo and a C compiler:

- **Linux and CI:** the GitHub Actions Ubuntu runners include a C compiler. The CI workflow runs this command on every pull request and push to `main`.
- **Windows:** `-race` fails with `requires cgo` unless a C toolchain (for example MinGW-w64 or MSYS2) is installed and `CGO_ENABLED=1` is set. Without that toolchain, rely on CI for race results.

## Integration tests

Integration tests send real requests to a Stalwart server. They live in `integration_test.go`, are excluded from normal runs, and need the `integration` build tag:

```bash
go test -tags=integration -count=1 -v -run TestIntegration .
```

**Use a dedicated test server. Never run them against production.** Most tests are read-only. The write tests in `integration_write_test.go` create, change, and delete domains, so they run only when you opt in (see [Write tests](#write-tests)).

### Environment variables

Only the integration tests read these variables. The SDK itself never reads the environment.

| Variable | Purpose |
|----------|---------|
| `STALWART_TEST_URL` | Base URL of the test server, for example `https://mail.example.com` |
| `STALWART_TEST_API_KEY` | API key that can sign in and read domains |
| `STALWART_TEST_NOPERM_API_KEY` | API key that can sign in but cannot read domains |
| `STALWART_TEST_WRITE_API_KEY` | Write tests only: API key that can create, update, and destroy domains |
| `STALWART_TEST_ALLOW_WRITES` | Write tests only: must be `1` to enable them |
| `STALWART_TEST_PARENT_DOMAIN` | Write tests only: a domain that already exists on the test server. Disposable domains are created as subdomains of it |
| `STALWART_TEST_SWEEP` | Sweep test only: set to `delete` to remove leftover disposable domains. Any other value (or unset) is a dry run that deletes nothing |

If a variable the read-only tests need is missing, they fail with a message naming it. They never skip silently. The write tests are the one exception: without `STALWART_TEST_ALLOW_WRITES=1` they skip, and the skip message says why.

Set the keys without typing them into a command, so they stay out of shell history. In PowerShell:

```powershell
$env:STALWART_TEST_URL = "https://mail.example.com"
$env:STALWART_TEST_API_KEY = Read-Host "Domain-read key"
$env:STALWART_TEST_NOPERM_API_KEY = Read-Host "No-permission key"
```

In a POSIX shell:

```bash
export STALWART_TEST_URL=https://mail.example.com
read -rs STALWART_TEST_API_KEY && export STALWART_TEST_API_KEY
read -rs STALWART_TEST_NOPERM_API_KEY && export STALWART_TEST_NOPERM_API_KEY
```

### Creating the API keys

Create both keys in the Stalwart WebUI under Account › Credentials › API Keys. Set each key's permission mode to `Replace` and select only the permissions below. A key can narrow its owner's permissions but never extend them, so the owning account must hold them too.

| Key | Permissions (WebUI label) | Internal name |
|-----|---------------------------|---------------|
| `STALWART_TEST_API_KEY` | "Sign in to the server"; "Domains Management: Get domains"; "Accounts Management: Get accounts" | `authenticate`; `sysDomainGet`; `sysAccountGet` (inferred from the WebUI label, not verified) |
| `STALWART_TEST_NOPERM_API_KEY` | "Sign in to the server" | `authenticate` |
| `STALWART_TEST_WRITE_API_KEY` (write tests only) | "Sign in to the server"; "Domains Management: Get domains"; "Domains Management: Create domains"; "Domains Management: Update domains"; "Domains Management: Destroy domains" | `authenticate`; `sysDomainGet`; `sysDomainCreate`; `sysDomainUpdate`; `sysDomainDestroy` |
| `STALWART_TEST_WRITE_API_KEY`, additionally for domain delete | "Signatures Management: Destroy signatures" | Not shown by the WebUI. Probably `sysDkimSignatureDestroy`, but that name is inferred, not verified |

The second row is needed by `Domains().Delete` for a domain with Stalwart's default settings, because deleting it first removes its automatic DKIM signature. It is also needed by `TestIntegration_DomainsWriteOperations` and `TestIntegration_DomainDestroyLinked`. Without it, those fail with a `forbidden` error, and the disposable domain is left behind.

Keep the write key separate from the read key, so that the key used for everyday runs cannot change anything.

Stalwart shows a key once, when it is created. Store it in a secrets manager or set it straight into the environment, and revoke any key that was exposed.

### TLS trust

The tests use the SDK's default HTTP client, which verifies the server certificate against the operating system trust store. For a server with a locally issued certificate (for example from mkcert), install that CA into your operating system trust store. Never disable certificate verification, and never copy the CA's private key into the project.

If verification fails, the test output says so. Security software that inspects HTTPS can also replace certificates. If that happens, exclude the test server in that software.

### What the tests check

| Test | Check |
|------|-------|
| `TestIntegration_Session` | `GET /jmap/session` succeeds and advertises the JMAP core capability; the API URL path is `/jmap/` |
| `TestIntegration_DomainGet` | `x:Domain/get` returns a `list` with the read-only key |
| `TestIntegration_InvalidKey` | An unissued key gets HTTP 401, which maps to `ErrUnauthorized` |
| `TestIntegration_NoPermissionKey` | A signed-in key without `sysDomainGet` gets HTTP 200 with a method-level `forbidden` error and no domain data |
| `TestIntegration_DomainsList` | `Domains().List` returns at least one domain with an ID and name. The test server must hold a domain |
| `TestIntegration_DomainsGet` | `Domains().Get` returns the same domain that `List` reports for its ID |
| `TestIntegration_DomainsGetUnknown` | An unknown ID returns `ErrNotFound` |
| `TestIntegration_DomainsListNoPermission` | A key without `sysDomainGet` gets a `*MethodError` matching `ErrForbidden` from `Domains().List` |
| `TestIntegration_DomainFieldProbe` | Records which documented `Domain` properties the server returns. Logs names only and never fails for a missing property |
| `TestIntegration_DomainValueTypes` | Records the JSON type of each documented `Domain` property across every domain: null, boolean, string (and whether it is an RFC 3339 timestamp), array, or object (with its `@type` tag if it has one). Logs types and counts, never values |

The output lists observed field names, counts, and statuses, never hostnames, identifiers, domain names, or keys. It is the basis for [stalwart-api-validation.md](stalwart-api-validation.md).

### Write tests

The write tests validate `x:Domain/set` before the SDK exports any write operation. They skip unless `STALWART_TEST_ALLOW_WRITES` is `1`, and they need `STALWART_TEST_WRITE_API_KEY` as well as the read-only variables.

```powershell
$env:STALWART_TEST_WRITE_API_KEY = Read-Host "Write key"
$env:STALWART_TEST_PARENT_DOMAIN = "<a domain that exists on your test server>"
$env:STALWART_TEST_ALLOW_WRITES = "1"
go test -tags=integration -count=1 -v -run "TestIntegration_Domain(Write|CreateInvalid)" -o integration.test.exe .
Remove-Item Env:STALWART_TEST_ALLOW_WRITES
```

Every domain the tests create is a random subdomain of `STALWART_TEST_PARENT_DOMAIN`, named `sdk-it-<random>.<parent>`. The tests check first that the parent is on the server, and they fail before any create if it is missing. Each created domain is destroyed when its test ends. Rejection descriptions are printed with the disposable and parent names replaced by `<domain>`. If cleanup fails, the failure message names the disposable domain so that you can delete it in the WebUI. That is the only time a domain name appears in test output.

The `.invalid` suffix was tried first and rejected by the server, so the tests no longer use it.

| Test | Check |
|------|-------|
| `TestIntegration_DomainWriteLifecycle` | Creates a domain with `dkimManagement` set to `Manual` (so no DKIM signature is generated and the destroy is not blocked), rejects a duplicate create, updates `description` (string) and `enable` (boolean) and reads both back, checks an update and a destroy for an unknown ID (an ID shaped like a real one), destroys the domain, and confirms `Domains().Get` then returns `ErrNotFound`. Records the response fields and rejection types |
| `TestIntegration_DomainCreateInvalid` | A create with no name or an empty name must be rejected. A name with spaces is recorded without failing, because Stalwart 1.0.1 accepts it. Records the rejection types |
| `TestIntegration_DomainDestroyLinked` | Optional; run it by name. Creates a domain with default settings, confirms that the destroy is blocked by linked objects (`objectIsLinked`), removes them, and destroys the domain. It needs permission to destroy the linked objects, which is not one of the domain permissions: for the `DkimSignature` that Stalwart generates, add "Signatures Management: Destroy signatures" to the write key. Without it the test fails with `forbidden` and leaves a disposable domain to delete by hand |
| `TestIntegration_DomainsWriteOperations` | The exported `Create`, `Update`, and `Delete` on a disposable domain. Create uses Stalwart's default settings, so Delete must remove the automatic DKIM signature. A denial during Delete means the write key lacks the "Signatures Management: Destroy signatures" permission |
| `TestIntegration_DomainWriteNoPermission` | A key that can sign in but has no domain permissions cannot create a domain. Records whether the denial is a method-level error or a rejection inside the set result |
| `TestIntegration_SweepDisposableDomains` | Removes leftover disposable domains. Matches only `sdk-it-<12 hex>.<parent>` under `STALWART_TEST_PARENT_DOMAIN`, so other domains are never touched. Without `STALWART_TEST_SWEEP=delete` it only counts the matches (dry run). With it, each match is removed with `Domains().Delete`, so linked DKIM signatures are removed first. Logs counts only |

## Test hygiene

- Use placeholder credentials (for example `API_test-token-not-real`), never real keys.
- Do not disable TLS verification in tests.
- Keep tests independent of the real environment; inject values instead.
