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

| Stalwart version | Status | Evidence |
|------------------|--------|----------|
| None yet | Not validated | Real-server validation is Milestone 2 |

No Stalwart version is supported yet. The SDK does not claim compatibility with any management endpoint until it is validated against a pinned server. When validation is complete, this table will list each version, the image or digest used, and which operations were verified.

## Protocol assumptions

- Stalwart management operations use JMAP under the `urn:stalwart:jmap` capability. This is taken from Stalwart's documentation and is not yet verified against a server.
- Authentication uses bearer API keys only (`Authorization: Bearer API_...`). Basic authentication is deferred until validated.
