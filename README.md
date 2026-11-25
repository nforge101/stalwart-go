# stalwart-go

Go SDK for [Stalwart Mail Server](https://stalw.art/) management and provisioning.

> **Under Active Development — Pre-release**
>
> Stalwart Go is currently under active development. Features are being implemented and validated incrementally.
>
> Public APIs may change before the first stable release.
>
> Production use is not yet recommended.
>
> Contributions, feedback, and issue reports are welcome.

## What this project is

**Primary product:** a reusable Go library (`github.com/nforge101/stalwart-go`) that applications import to automate verified Stalwart administrative operations (domains, accounts, aliases—where supported).

**Secondary product (later):** an optional standalone REST API wrapper that reuses the same SDK. Adopters download and run it against **their own** Stalwart servers. NForge101 does not host a shared public control plane.

This repository is a public open-source project under the [NForge101](https://github.com/nforge101) initiative (MIT License).

## Status

| Area | Status |
|------|--------|
| Go module (`go.mod`) | Declared |
| Public client API | Planned (Milestone 1) |
| Stalwart API validation | Planned (Milestone 2) |
| Domain / account / alias operations | Planned (after validation) |
| Optional REST wrapper | Planned (later milestone) |
| Tagged release | Not yet |

See [docs/implementation-progress.md](docs/implementation-progress.md) for the current milestone and slice.

## Requirements

- Go **1.26** or later (developed against the Go 1.26 / 1.27 toolchains)
- A Stalwart instance for live operations and integration tests (not required to compile the module once source exists)

Supported Stalwart versions will be listed in `docs/compatibility.md` after real-server validation. Untested configurations are not claimed as supported.

## Installation

Once the first importable client code is published:

```bash
go get github.com/nforge101/stalwart-go
```

Runnable examples will live under `examples/` and will use:

```go
import stalwart "github.com/nforge101/stalwart-go"
```

## Security

- Never commit Stalwart admin passwords, API tokens, certificates, or private keys.
- Configure credentials via environment variables or a secrets manager.
- The SDK will verify TLS by default. Do not disable certificate verification for convenience.
- See [SECURITY.md](SECURITY.md) and (as it grows) [docs/security.md](docs/security.md).

## Documentation

| Document                                                           | Description                             |
| ------------------------------------------------------------------ | --------------------------------------- |
| [docs/architecture.md](docs/architecture.md)                       | System context and dependency direction |
| [docs/implementation-progress.md](docs/implementation-progress.md) | Milestone/slice progress                |                                      |
| [CONTRIBUTING.md](CONTRIBUTING.md)                                 | How to contribute                       |
| [SECURITY.md](SECURITY.md)                                         | Vulnerability reporting                 |

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md). Development follows small, reviewable vertical slices on feature branches with PRs into `main`.

## License

[MIT](LICENSE) © NForge101
