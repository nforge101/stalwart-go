# Contributing to stalwart-go

Thank you for contributing. This project is a public Go SDK for Stalwart management and provisioning. Development proceeds in small vertical slices.

## Principles

- One approved slice → implementation → review → tests → commit → PR → merge → next slice.
- Do not invent Stalwart management endpoints. Validate against real server behavior and version-matched documentation.
- Keep the SDK independent of the future optional REST server.
- Prefer the Go standard library unless a dependency is clearly justified.
- Never commit secrets, credentials, certificates, or proprietary material.

## Development workflow

1. Open or discuss an issue for non-trivial changes when practical.
2. Branch from an up-to-date `main`, for example `feat/sdk-client-foundation`.
3. Implement only the agreed slice.
4. Run local checks (see below).
5. Open a pull request against `main` with a clear summary, test results, and limitations.
6. Do not merge without review.

## Local checks

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Integration tests against a real Stalwart instance (when present) are opt-in and must never target production. See upcoming `docs/testing.md`.

## Commit messages

Use concise, conventional messages focused on why the change exists, for example:

```text
docs: establish public OSS bootstrap and M1 progress tracking
```

## Public API

Treat exported identifiers carefully. Pre-1.0 breakage is allowed but should be deliberate and documented in the PR and progress notes.

## Code of conduct expectations

Be respectful in issues and pull requests. Assume good faith. Harassment or hostile behavior is not acceptable.
