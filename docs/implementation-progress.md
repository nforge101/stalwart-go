# Implementation progress

Concise tracker for milestones and slices. Update at the end of each completed slice, before committing.

## Current milestone

**M1 — SDK foundation + developer experience**

## Active slice

**M1-S2 — SDK client foundation** (`feat/sdk-client-foundation`, implemented, awaiting review and commit)

Scope: `NewClient`, functional options, base URL validation, default HTTP client with timeout and no redirect following, and unit tests. Authentication, request execution, and management operations are out of scope (M1-S3 and M2).

## Completed slices

| Slice | Merged | Summary |
|-------|--------|---------|
| M1-S1 — OSS bootstrap | Squash-merged into `main` (PR #1, commit `3830c5a`) | Pre-release status, `.gitignore` hardening, `SECURITY.md`, `CONTRIBUTING.md`, CI on Go 1.26.x and 1.27.x, issue templates, package doc |

## Architectural decisions

| Decision | Choice |
|----------|--------|
| Public package | Module root, `package stalwart`; import `github.com/nforge101/stalwart-go` |
| Client construction | `NewClient(baseURL, opts ...Option)`; options are applied in order |
| Base URL | Must be `https`, have a host, and contain no user information, query, or fragment. A trailing slash is removed; a path prefix is allowed. Plain `http` is rejected, so credentials are never sent unencrypted |
| Default HTTP client | Standard TLS verification, 30 second timeout, redirects not followed (the redirect response is returned to the caller) |
| Caller-supplied HTTP client | Used as provided and never modified; `WithTimeout` cannot be combined with `WithHTTPClient` |
| Errors | Invalid configuration wraps the exported sentinel `ErrInvalidConfig`. Messages never include the raw URL, user information, or parser output |
| Auth | Stalwart-facing only; Bearer and Basic adapters arrive in M1-S3 and remain unverified against Stalwart until M2 |
| Transport | Generic JSON HTTP helper in M1-S3; no JMAP layer until API validation |
| Dependencies | Standard library only |
| Go version | `go 1.26.0` minimum; CI tests 1.26 and 1.27 |
| REST wrapper | Later milestone (M7); self-hosted by adopters; mandatory caller auth when introduced |
| Product posture | Public production-quality OSS SDK, not a hosted service or internal POC |

## Verified Stalwart API contracts

_None yet._ Validation is Milestone 2. Do not invent management endpoints.

Known open risks from the earlier POC (unverified for this SDK):

- The management surface may be JMAP (`urn:stalwart:jmap`, `x:*` methods) rather than classic `/api/*` REST.
- The auth scheme (Bearer API key, Basic, or other) must be confirmed against a pinned server version.
- The POC used the mutable image tag `stalwartlabs/stalwart:v1`. Integration tests must pin a version or digest.

## Test results and limitations

Recorded on Windows with Go **1.27.4**:

| Check | Result | Notes |
|-------|--------|-------|
| `go vet ./...` | Pass | |
| `go test -count=1 ./...` | Pass | Client and option unit tests (`client_test.go`) |
| `go build ./...` | Pass | |
| `gofmt` | Pass | Checked on LF-normalized copies. The working tree uses CRLF because `core.autocrlf=true`, so `gofmt -l` on the raw Windows files lists them; the committed files are LF |
| `go test -race ./...` | Not executed | Requires CGO and a C toolchain on this Windows host. CI does not run `-race` yet |
| Integration / Stalwart | Not run | Requires the M2 environment |

Limitations:

- No request execution exists yet, so timeouts, cancellation, and redirect behavior are tested by configuration only.
- The Windows test binary occasionally fails to delete from the temp directory while a scanner holds it. This affects only cleanup; the tests pass.

## Outstanding risks

- The Stalwart management API contract is unverified on a pinned instance.
- Empty-module CI was a weak signal in M1-S1; M1-S2 adds real tests, but no request-path coverage yet.
- Local TLS (mkcert) must use a caller-supplied CA on an HTTP client, never `InsecureSkipVerify`.
- Consider adding `.gitattributes` (`* text=auto eol=lf`) so Windows contributors do not see CRLF-related `gofmt` noise.

## Next planned slice

**M1-S3 — Transport, errors, and auth** (`feat/transport-errors-auth`): shared request helper with path joining, structured `APIError`, bounded error-body reads, Bearer and Basic auth with credential redaction, and unit tests using `httptest`.
