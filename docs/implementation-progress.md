# Implementation progress

Concise tracker for milestones and slices. Update at the end of each completed slice.

## Current milestone

**M1 — SDK foundation + developer experience**

## Active slice

**M1-S1 — OSS bootstrap** (implemented on `feat/oss-bootstrap`; awaiting review/commit)

## Completed slices

_None merged yet. M1-S1 implementation is ready for review on this branch._

## Architectural decisions (locked for M1)

| Decision | Choice |
|----------|--------|
| Public package | Module root, `package stalwart`; import `github.com/nforge101/stalwart-go` |
| Client config | Functional options + validated base URL (M1-S2+) |
| Auth | Stalwart-facing only; Auth interface with Bearer/Basic adapters in M1; unverified against Stalwart until M2 |
| Transport | Generic JSON HTTP in M1; no JMAP layer until API validation |
| Dependencies | Stdlib only for M1 |
| Go version | `go 1.26.0` minimum; CI tests 1.26 and 1.27 |
| REST wrapper | Later milestone (M7); self-hosted by adopters; mandatory caller auth when introduced |
| Product posture | Public production-quality OSS SDK — not a hosted SaaS or internal POC |

## Verified Stalwart API contracts

_None yet._ Validation is Milestone 2. Do not invent management endpoints.

Known open risks from prior POC (unverified for this SDK):

- Management surface may be JMAP (`urn:stalwart:jmap`, `x:*` methods) rather than classic `/api/*` REST.
- Auth scheme (Bearer API key vs Basic vs other) must be confirmed against a pinned server version.
- POC used mutable image tag `stalwartlabs/stalwart:v1`; integration tests must pin digest/version.

## Test results and limitations

Recorded locally on Windows with Go **1.27.4** after adding package doc (`doc.go`):

| Check | Result | Notes |
|-------|--------|-------|
| `go test ./...` | Pass | `? github.com/nforge101/stalwart-go [no test files]` |
| `go vet ./...` | Pass | |
| `go build ./...` | Pass | |
| `go test -race ./...` | Not executed | Requires CGO on this Windows host; CI runs on Ubuntu without `-race` in M1-S1 |
| Integration / Stalwart | Not run | Requires M2 environment |

Note: before `doc.go`, `go test ./...` and `go vet ./...` exited non-zero with “matched no packages”. The package doc file establishes `package stalwart` without exposing client APIs.

## Outstanding risks

- Stalwart management API contract unverified on a pinned instance.
- Empty-module CI is a weak signal until client packages exist (M1-S2+).
- Local TLS/mkcert trust must use a custom `*http.Client` / CA — never default `InsecureSkipVerify`.

## Next planned slice

**M1-S2 — SDK client foundation** (`feat/sdk-client-foundation`): `NewClient`, options, base URL validation, custom HTTP client / timeout / context readiness, unit tests.
