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
| Client construction, transport, errors, bearer auth | Implemented (pre-release) |
| Example and CI | Implemented (pre-release) |
| Stalwart API validation | Read-only calls validated on one Stalwart version (Milestone 2) |
| Domain / account / alias operations | Planned (after validation) |
| Optional REST wrapper | Planned (later milestone) |
| Tagged release | Not yet |

No management operations are available yet. See [docs/implementation-progress.md](docs/implementation-progress.md) for the current milestone and slice.

## Requirements

- Go **1.26** or later (CI tests the Go 1.26 and 1.27 toolchains)
- A Stalwart instance for live operations and integration tests (not required to compile or unit-test the module)

Validated Stalwart versions are listed in [docs/compatibility.md](docs/compatibility.md). Untested configurations are not claimed as supported.

## Installation

There is no tagged release yet. To use the pre-release code:

```bash
go get github.com/nforge101/stalwart-go@main
```

For reproducible builds, pin a specific commit instead of `@main`.

## Quick start

The client is created with the Stalwart server's HTTPS base URL and an API key. Credentials are read from the environment or a secrets manager, never hard-coded:

```go
package main

import (
	"fmt"
	"log"
	"os"

	stalwart "github.com/nforge101/stalwart-go"
)

func main() {
	client, err := stalwart.NewClient(
		os.Getenv("STALWART_URL"),                    // for example https://mail.example.com
		stalwart.WithBearerToken(os.Getenv("STALWART_API_KEY")), // begins with API_
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(client) // prints the base URL with credentials redacted
}
```

A runnable version is in [`examples/basic`](examples/basic/main.go):

```bash
export STALWART_URL=https://mail.example.com
export STALWART_API_KEY=API_your_key_here
go run ./examples/basic
```

The example makes no network request and performs no management operation.

## Security

- Never commit Stalwart admin passwords, API tokens, certificates, or private keys.
- Configure credentials via environment variables or a secrets manager.
- The SDK verifies TLS certificates by default. Do not disable certificate verification for convenience.
- See [SECURITY.md](SECURITY.md) and (as it grows) [docs/security.md](docs/security.md).

## Documentation

| Document | Description |
|----------|-------------|
| [docs/architecture.md](docs/architecture.md) | System context and dependency direction |
| [docs/implementation-progress.md](docs/implementation-progress.md) | Milestone and slice progress |
| [docs/development.md](docs/development.md) | Local setup and checks |
| [docs/testing.md](docs/testing.md) | Test strategy and how to run tests |
| [docs/compatibility.md](docs/compatibility.md) | Supported Go and Stalwart versions |
| [docs/stalwart-api-validation.md](docs/stalwart-api-validation.md) | Verified Stalwart API behavior and evidence |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting |

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md). Development follows small, reviewable vertical slices on feature branches with PRs into `main`.

## License

[MIT](LICENSE) © NForge101
