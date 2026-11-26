# Compatibility

This document lists the versions the SDK is tested against. Untested configurations are not supported.

## Go toolchains

| Go version | Status | Evidence |
|------------|--------|----------|
| 1.26.x | Supported (minimum, per `go.mod`) | CI on `ubuntu-latest` |
| 1.27.x | Supported | CI on `ubuntu-latest`; also used for local runs on Windows |
| Older than 1.26 | Not supported | `go.mod` declares `go 1.26.0` |

CI runs `go vet`, `go test`, `go test -race`, and `go build` on each Go version listed above.

## Operating systems

| OS | Status | Notes |
|----|--------|-------|
| Linux (`ubuntu-latest`) | Tested in CI | Runs the full check set, including `-race` |
| Windows | Tested locally by the maintainer | `-race` requires a C toolchain; see [testing.md](testing.md) |
| macOS | Not tested | |

## Stalwart Mail Server versions

| Stalwart version | Image digest | Authentication | API behavior verified | Exported SDK operations | Integration tests | Date |
|------------------|--------------|----------------|-----------------------|-------------------------|-------------------|------|
| v1.0.1 | `sha256:b9da4df8b4efc06bc926dd6e990bdeb0ad7b9261a7c3893d74b0461a00bb389b` | Bearer API key | Session; `x:Domain/get` (read-only); 401 and 403 responses; method-level `forbidden` error | None yet | Pass (4 tests) | 2026-11-26 |

Only the operations in the "API behavior verified" column are validated, and only on this version. Everything else, including every write operation, is unvalidated and therefore unsupported. The SDK does not yet export any management operation; domain operations arrive in Milestone 3.

Versions not listed in the table are not supported. See [stalwart-api-validation.md](stalwart-api-validation.md) for the evidence and its limits.

## Protocol notes

- Management operations use JMAP at `/jmap/`, with the session at `/jmap/session`. Verified on v1.0.1 for the read-only operations above.
- Authentication uses bearer API keys only (`Authorization: Bearer API_...`). Verified on v1.0.1. Basic authentication is deferred until validated.
- Permission denials arrive in two forms: an HTTP 403, and a method-level `forbidden` error inside an HTTP 200 response. See [stalwart-api-validation.md](stalwart-api-validation.md).
