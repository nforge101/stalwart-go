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

### Example tests

`examples/basic` tests inject the environment through a function, so they never read the real environment. They check that the API key never appears in output or error text.

## Race detector

```bash
go test -race ./...
```

The race detector needs cgo and a C compiler:

- **Linux and CI:** the GitHub Actions Ubuntu runners include a C compiler. The CI workflow runs this command on every pull request and push to `main`.
- **Windows:** `-race` fails with `requires cgo` unless a C toolchain (for example MinGW-w64 or MSYS2) is installed and `CGO_ENABLED=1` is set. Without that toolchain, rely on CI for race results.

## Integration tests (planned, Milestone 2)

Integration tests against a real, version-pinned Stalwart server are not implemented yet. When they are added:

- They will be opt-in, behind a build tag (planned: `-tags=integration`).
- They will read the server URL and credentials from environment variables, never from committed files.
- They must never target a production server.

Until then, no test requires a Stalwart instance.

## Test hygiene

- Use placeholder credentials (for example `API_test-token-not-real`), never real keys.
- Do not disable TLS verification in tests.
- Keep tests independent of the real environment; inject values instead.
