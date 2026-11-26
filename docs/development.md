# Development

This document covers local setup and the checks to run before opening a pull request. For the contribution workflow, see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Prerequisites

- Go **1.26** or later. The module declares `go 1.26.0`; CI also tests 1.27.
- Git.
- Optional, for the race detector: a C compiler (see [testing.md](testing.md)).

The SDK has no third-party dependencies. Do not add one without a clear reason, and check its license, maintenance status, and known vulnerabilities (for example with `govulncheck`).

## Setup

```bash
git clone https://github.com/nforge101/stalwart-go.git
cd stalwart-go
go test ./...
```

## Local checks

Run these before opening a pull request. They match the CI workflow in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml):

```bash
go vet ./...
go test ./...
go test -race ./...   # needs cgo and a C compiler locally; CI always runs it
go build ./...
go build ./examples/...
```

Check formatting with `gofmt -l .`. It should print nothing.

### Windows line endings

If Git has `core.autocrlf=true`, the working tree uses CRLF line endings while the repository stores LF. `gofmt -l` then reports every file even when the formatting is correct. To check formatting, run `gofmt` on an LF copy of the files, or rely on CI.

## Environment variables

The SDK itself never reads environment variables. Only the example and future integration tests do:

| Variable | Used by | Purpose |
|----------|---------|---------|
| `STALWART_URL` | `examples/basic` | Base URL of the Stalwart server (`https://...`) |
| `STALWART_API_KEY` | `examples/basic` | Stalwart API key (begins with `API_`) |

Keep local values in your shell or a secrets manager. Never commit them. `.env` files are git-ignored, but no loader reads them.

## Project layout

| Path | Contents |
|------|----------|
| `*.go` (module root) | The public `stalwart` package: client, options, auth, errors, transport |
| `examples/basic` | Example that builds a client from environment variables |
| `docs/` | Architecture, progress, testing, development, and compatibility documents |
| `.github/` | CI workflow and issue templates |

## Branches and commits

Work on a feature branch named for its slice (for example `feat/example-ci-docs`), then open a pull request into `main`. Follow the commit style in [CONTRIBUTING.md](../CONTRIBUTING.md).
