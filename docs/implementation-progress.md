# Implementation progress

Concise tracker for milestones and slices. Update at the end of each completed slice, before committing.

## Current milestone

**M3 — Domain operations**

## Active slice

**M3-S2 — Read operations** (`feat/domain-read-operation`, implemented and verified against Stalwart 1.0.1, awaiting commit)

Scope: the first exported domain operations, both read-only: `Domains().List` and `Domains().Get`, using the verified `x:Domain/get`. `Domain` exposes only `ID` and `Name`. No write operations and no name lookup.

M1 (SDK foundation), M2 (API validation), and M3-S1 (JMAP helper and method errors) are complete.

## Completed slices

| Slice | Merged | Summary |
|-------|--------|---------|
| M1-S1 — OSS bootstrap | Squash-merged into `main` via PR #1 (`3830c5a`) | Pre-release status, `.gitignore` hardening, `SECURITY.md`, `CONTRIBUTING.md`, CI on Go 1.26.x and 1.27.x, issue templates, package doc |
| M1-S2 — SDK client foundation | Squash-merged into `main` via PR #2 (`6e41db7`) | `NewClient` with functional options, base URL validation, default HTTP client with timeout and no redirect following, unit tests |
| M1-S3 — Transport, errors, and auth | Squash-merged into `main` via PR #3 (`05e580e`) | Internal request helper, typed errors (`APIError`, sentinels), Bearer API-key auth with redaction, response limits, concurrency and TLS tests |
| M1-S4 — Example, CI, and docs | Squash-merged into `main` via PR #4 (`d9fd004`) | `examples/basic`, race detector and example build in CI, README quick start, testing/development/compatibility docs |
| M2 — Stalwart API validation | Squash-merged into `main` via PR #5 (`4f51a80`) | Opt-in read-only integration tests against Stalwart v1.0.1, sanitized evidence, first compatibility row |
| M3-S1 — JMAP helper and method errors | Squash-merged into `main` via PR #6 (`2980ea0`) | Internal `callJMAP` helper, exported `*MethodError` (`forbidden` matches `ErrForbidden`), unit tests for the envelope and both error types |
| M3-S2 — Read operations | Implemented on `feat/domain-read-operation`; verified against Stalwart 1.0.1 (all 9 integration tests pass); awaiting commit | `Domains().List` and `Domains().Get`, `Domain` with `ID` and `Name` only, `ErrNotFound`, `examples/domains`, field probe result recorded |

## Architectural decisions

| Decision | Choice |
|----------|--------|
| Public package | Module root, `package stalwart`; import `github.com/nforge101/stalwart-go` |
| Client construction | `NewClient(baseURL, opts ...Option)`; options are applied in order |
| Base URL | Must be `https`, have a host, and contain no user information, query, or fragment. A trailing slash is removed; a path prefix is allowed. Plain `http` is rejected, so credentials are never sent unencrypted |
| Authentication | Required: `NewClient` returns `ErrInvalidConfig` if no auth option is given. M1 supports one mechanism, `WithBearerToken`, which sends `Authorization: Bearer <token>`. Stalwart API keys (`API_...`) use this form and are verified against v1.0.1. Basic auth is deferred |
| Credential handling | Token must be 1 to 512 printable ASCII characters with no spaces. It never appears in `String`/`GoString` output, error messages, or `APIError.Message`. Error bodies are redacted before they are truncated |
| Default HTTP client | Standard TLS verification, 30 second timeout, redirects not followed (the redirect response is returned to the caller) |
| Caller-supplied HTTP client | Used as provided and never modified; `WithTimeout` cannot be combined with `WithHTTPClient` |
| Request paths | Relative to the base URL; must start with `/`, be printable ASCII, and contain no `?`, `#`, `\`, or `.`/`..` segments (including percent-encoded forms). Dynamic values must be escaped with `url.PathEscape` so they cannot add segments |
| Errors | `*APIError` for non-2xx responses (status and sanitized excerpt). `ErrUnauthorized` (401) and `ErrForbidden` (403) match via `errors.Is`. `ErrTransport` wraps send/receive failures, including timeouts and cancellation. `ErrDecode` wraps malformed success bodies. A 404 does not match any sentinel. Method-level JMAP errors inside an HTTP 200 are returned as `*MethodError`; its `Type` `forbidden` matches `ErrForbidden` |
| Domain operations | Exposed as `client.Domains()`, returning a `DomainService` with `List(ctx)` and `Get(ctx, id)`. `Domain` has `ID` and `Name` only. `Get` returns `ErrNotFound` when the server reports the ID as missing; an HTTP 404 does not match it. Invalid IDs (empty, longer than 256 bytes, or containing non-ASCII or spaces) fail locally, without a request. Lists are not paginated, because Stalwart returns all domains at once |
| JMAP calls | One internal helper, `callJMAP`, sends one method call per request to `/jmap/` with call ID `c1` and both verified capabilities. Method-level `error` responses become `*MethodError`. Responses of unexpected shape wrap `ErrDecode`. Error text never includes server-supplied names or identifiers |
| Response limits | Error bodies: 4 KiB read, 512 bytes kept. Success bodies: 16 MiB decode limit. Response bodies are always closed |
| Concurrency | `Client` is immutable after construction and safe for concurrent use |
| Environment variables | The SDK never reads them. The example reads `STALWART_URL` and `STALWART_API_KEY`. Integration tests read `STALWART_TEST_URL`, `STALWART_TEST_API_KEY`, and `STALWART_TEST_NOPERM_API_KEY`; the separate prefix keeps tests from picking up a real server's settings |
| Integration tests | Root package, behind `//go:build integration`, read-only. They use the default HTTP client and the operating system trust store, never a disabled certificate check. Missing variables fail the test instead of skipping it |
| Dependencies | Standard library only |
| Go version | `go 1.26.0` minimum; CI tests 1.26 and 1.27 |
| REST wrapper | Later milestone (M7); self-hosted by adopters; mandatory caller auth when introduced |
| Product posture | Public production-quality OSS SDK, not a hosted service or internal POC |

## Verified Stalwart API contracts

Verified against `stalwartlabs/stalwart:v1.0.1` (digest in [compatibility.md](compatibility.md)) on 2026-11-26. Details and sanitized evidence are in [stalwart-api-validation.md](stalwart-api-validation.md).

- `GET /jmap/session` with a bearer API key returns the session. The API URL path is `/jmap/`.
- `x:Domain/get` with `ids: null` and no `accountId` returns every domain (`accountId`, `list`, `notFound`; items have `id` and `name`).
- The minimum permissions for that call are `authenticate` and `sysDomainGet`.
- An unissued key gets HTTP 401; a key without the sign-in permission gets HTTP 403; a signed-in key without the method permission gets HTTP 200 with a method-level `forbidden` error.
- The session does not advertise `urn:stalwart:jmap`, yet using it in a request works.

Not verified: write operations, `x:Domain/query`, accounts, aliases, Basic auth, and any other Stalwart version.

## Test results and limitations

Recorded on Windows with Go **1.27.4**:

| Check | Result | Notes |
|-------|--------|-------|
| `go vet ./...` | Pass | Also passes with `-tags=integration` |
| `go test -count=1 ./...` | Pass | Integration tests are excluded without the tag |
| `go build ./...` | Pass | |
| `gofmt` | Pass | Checked on LF-normalized copies. The working tree uses CRLF because `core.autocrlf=true` |
| `go test -tags=integration` | Pass against v1.0.1 (M2) | Four tests, run manually against a local test server. Not run in CI. M3-S1 changed the no-permission test to use `callJMAP`; re-run before merging |
| `go test -race ./...` | Not run locally | Requires CGO and a C toolchain on this Windows host. Runs in CI on Linux |

Test design notes:

- Transport tests use `httptest.NewTLSServer`. Clients are built with `NewClient` and trust the test certificate through the server's own HTTP client (`srv.Client()`). Certificate verification stays enabled everywhere.
- On the development machine, Norton Web/Mail Shield re-signed loopback TLS certificates, so the TLS tests initially failed verification. Verification was working correctly. After adding Norton exclusions for `https://127.0.0.1/*`, `http://127.0.0.1/*`, and `https://localhost.localdomain/*`, the loopback certificate is no longer replaced, and the TLS tests pass. Keep these exclusions in mind if the tests fail on another machine with similar security software.
- Concurrency is exercised with 20 goroutines × 5 requests. Race detection runs in CI on Linux.
- Windows occasionally fails to delete the temporary test binary while a scanner holds it. This affects cleanup only; the exit code of the test run itself is what matters.
- Integration test output records field names, counts, and statuses only, so it can be shared as evidence without revealing hosts, identifiers, or keys.

## Outstanding risks

- Only one Stalwart version and configuration are validated. Compatibility with other versions is unknown.
- Write operations are unvalidated. Domain create, update, and destroy need their own validation before they are exposed.
- Integration tests run manually and are not in CI. They need a dedicated test server and credentials that CI does not have.
- `-race` has not been run locally on Windows. It runs only in CI until a C toolchain is installed.
- Basic auth is intentionally absent until validated.
- Consider adding `.gitattributes` (`* text=auto eol=lf`) so Windows contributors do not see CRLF-related `gofmt` noise.

## Next planned slice

**M3-S3 — Write operations.** Validate `x:Domain/set` (create, update, destroy) on disposable domains, behind an explicit write guard in the integration tests. Document conflict behavior, then add the exported operations.
