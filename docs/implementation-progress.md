# Implementation progress

Concise tracker for milestones and slices. Update at the end of each completed slice, before committing.

## Current milestone

**M1 — SDK foundation + developer experience**

## Active slice

**M1-S3 — Transport, errors, and auth** (`feat/transport-errors-auth`, implemented, awaiting review and commit)

Scope: an internal request helper, a structured error model, Bearer API-key authentication, redaction, and concurrency tests. No management operations and no exported request methods yet.

## Completed slices

| Slice | Merged | Summary |
|-------|--------|---------|
| M1-S1 — OSS bootstrap | Squash-merged into `main` via PR #1 (`3830c5a`) | Pre-release status, `.gitignore` hardening, `SECURITY.md`, `CONTRIBUTING.md`, CI on Go 1.26.x and 1.27.x, issue templates, package doc |
| M1-S2 — SDK client foundation | Squash-merged into `main` via PR #2 (`6e41db7`) | `NewClient` with functional options, base URL validation, default HTTP client with timeout and no redirect following, unit tests |

## Architectural decisions

| Decision | Choice |
|----------|--------|
| Public package | Module root, `package stalwart`; import `github.com/nforge101/stalwart-go` |
| Client construction | `NewClient(baseURL, opts ...Option)`; options are applied in order |
| Base URL | Must be `https`, have a host, and contain no user information, query, or fragment. A trailing slash is removed; a path prefix is allowed. Plain `http` is rejected, so credentials are never sent unencrypted |
| Authentication | Required: `NewClient` returns `ErrInvalidConfig` if no auth option is given. M1 supports one mechanism, `WithBearerToken`, which sends `Authorization: Bearer <token>`. Stalwart API keys (`API_...`) use this form. Basic auth is deferred until M2 validates it against a real server |
| Credential handling | Token must be 1 to 512 printable ASCII characters with no spaces. It never appears in `String`/`GoString` output, error messages, or `APIError.Message`. Error bodies are redacted before they are truncated |
| Default HTTP client | Standard TLS verification, 30 second timeout, redirects not followed (the redirect response is returned to the caller) |
| Caller-supplied HTTP client | Used as provided and never modified; `WithTimeout` cannot be combined with `WithHTTPClient` |
| Request paths | Relative to the base URL; must start with `/`, be printable ASCII, and contain no `?`, `#`, `\`, or `.`/`..` segments (including percent-encoded forms). Dynamic values must be escaped with `url.PathEscape` so they cannot add segments |
| Errors | `*APIError` for non-2xx responses (status and sanitized excerpt). `ErrUnauthorized` (401) and `ErrForbidden` (403) match via `errors.Is`. `ErrTransport` wraps send/receive failures, including timeouts and cancellation. `ErrDecode` wraps malformed success bodies. A 404 does not match any sentinel |
| Response limits | Error bodies: 4 KiB read, 512 bytes kept. Success bodies: 16 MiB decode limit. Response bodies are always closed |
| Concurrency | `Client` is immutable after construction and safe for concurrent use |
| Dependencies | Standard library only |
| Go version | `go 1.26.0` minimum; CI tests 1.26 and 1.27 |
| REST wrapper | Later milestone (M7); self-hosted by adopters; mandatory caller auth when introduced |
| Product posture | Public production-quality OSS SDK, not a hosted service or internal POC |

## Verified Stalwart API contracts

_None yet._ Validation is Milestone 2. Do not invent management endpoints.

Documented by Stalwart (not yet verified against our instance; see the M2 validation work):

- Management operations use JMAP under the `urn:stalwart:jmap` capability (`x:Domain/get|set|query`).
- API keys are bearer credentials (`Authorization: Bearer API_...`). They act as their owner, are long-lived unless `expiresAt` is set, are stored hashed (shown once), and can be restricted by permissions, `allowedIps`, and expiry.
- Management calls need the `authenticate` permission and the relevant `sys*` permissions (for example `sysDomainGet`).

Known open risks from the earlier POC (unverified for this SDK):

- The POC returned `404` for `/api` and `/api/principal`, and `unknownMethod` for JMAP `Domain/get`. These do not show that the operations are unsupported.
- The POC used the mutable image tag `stalwartlabs/stalwart:v1`. Integration tests must pin a version or digest.

## Test results and limitations

Recorded on Windows with Go **1.27.4**:

| Check | Result | Notes |
|-------|--------|-------|
| `go vet ./...` | Pass | |
| `go test -count=1 -v ./...` | Pass | 32 top-level tests, 38 subtests, 0 failures |
| `go build ./...` | Pass | |
| `gofmt` | Pass | Checked on LF-normalized copies. The working tree uses CRLF because `core.autocrlf=true` |
| `go test -race ./...` | Not executed | Requires CGO and a C toolchain on this Windows host. CI does not run `-race` yet |
| Integration / Stalwart | Not run | Requires the M2 environment |

Test design notes:

- Transport tests use `httptest.NewTLSServer`. Clients are built with `NewClient` and trust the test certificate through the server's own HTTP client (`srv.Client()`). Certificate verification stays enabled everywhere.
- On the development machine, Norton Web/Mail Shield re-signed loopback TLS certificates, so the TLS tests initially failed verification. Verification was working correctly. After adding Norton exclusions for `https://127.0.0.1/*`, `http://127.0.0.1/*`, and `https://localhost.localdomain/*`, the loopback certificate is no longer replaced, and the TLS tests pass. Keep these exclusions in mind if the tests fail on another machine with similar security software.
- Concurrency is exercised with 20 goroutines × 5 requests. Race detection is still pending (see above).
- Windows occasionally fails to delete the temporary test binary while a scanner holds it. This affects cleanup only; the exit code of the test run itself is what matters, and the run passed.

## Outstanding risks

- The Stalwart management API contract is unverified on a pinned instance, and the auth scheme (API key via Bearer) is documented but not confirmed for the management endpoints.
- `-race` has not been run locally or in CI yet.
- Basic auth is intentionally absent until validated.
- Consider adding `.gitattributes` (`* text=auto eol=lf`) so Windows contributors do not see CRLF-related `gofmt` noise.

## Next planned slice

**M1-S4 — Example, CI, and docs** (`feat/example-ci-docs`): `examples/basic` that constructs a client from environment variables and performs no management operation; README quick-start aligned with the implemented API; `docs/testing.md`, `docs/development.md`, and `docs/compatibility.md`; CI extended to compile examples.
