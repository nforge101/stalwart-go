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

**Use a dedicated test server. Never run them against production.** The current tests are read-only, but future tests will create and delete resources.

### Environment variables

Only the integration tests read these variables. The SDK itself never reads the environment.

| Variable | Purpose |
|----------|---------|
| `STALWART_TEST_URL` | Base URL of the test server, for example `https://mail.example.com` |
| `STALWART_TEST_API_KEY` | API key that can sign in and read domains |
| `STALWART_TEST_NOPERM_API_KEY` | API key that can sign in but cannot read domains |

If any variable is missing, the tests fail with a message naming it. They never skip silently.

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
| `STALWART_TEST_API_KEY` | "Sign in to the server"; "Domains Management: Get domains" | `authenticate`; `sysDomainGet` |
| `STALWART_TEST_NOPERM_API_KEY` | "Sign in to the server" | `authenticate` |

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

The output lists observed field names, counts, and statuses, never hostnames, identifiers, domain names, or keys. It is the basis for [stalwart-api-validation.md](stalwart-api-validation.md).

## Test hygiene

- Use placeholder credentials (for example `API_test-token-not-real`), never real keys.
- Do not disable TLS verification in tests.
- Keep tests independent of the real environment; inject values instead.
