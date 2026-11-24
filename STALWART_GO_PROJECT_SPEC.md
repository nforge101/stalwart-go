# NForge101 / stalwart-go — Project Specification and Cursor Implementation Guide

**Status:** Planning / implementation handoff  
**Repository:** `github.com/nforge101/stalwart-go`  
**Owner:** NForge101 (GitHub organization)  
**License:** MIT  
**Primary deliverable:** Reusable Go SDK  
**Secondary deliverable:** Optional standalone REST API built on the SDK  
**Working principle:** Validate Stalwart's real API contract before implementing its wrappers.

> **Instructions for Cursor Agent:** Read this document before writing code. Treat it as the project brief, not proof that any proposed Stalwart endpoint exists. Inspect the repository and local environment first. Implement incrementally, run checks, and request clarification where a decision affects the public API, security, or compatibility. Do not generate all milestones in one pass.

## 1. Problem, motivation, and intended users

We previously explored Stalwart Community Edition as a potential alternative to Exchange for centrally managing email across multiple client/project domains. The desired capability is **API-driven, repeatable provisioning**: when a project is created in an external application, the application can create or configure the corresponding mail domain, mailboxes, aliases, and applicable settings without manual administration in the Stalwart web UI.

The initial implementation will be a **public, idiomatic Go library** for interacting with supported Stalwart management APIs. A separate **optional HTTP REST service**, in the same repository, will later expose selected SDK operations to systems written in Java, Python, JavaScript, etc. The REST service is not required for Go consumers.

Target users:
- Go developers embedding Stalwart administration/provisioning into their applications.
- Platform teams automating domain, account, and alias lifecycle operations.
- Non-Go systems integrating through the optional REST wrapper.
- Maintainers and contributors seeking a well-tested, documented open-source SDK.

### Goals

1. Offer a clear, stable, context-aware Go client for **verified** Stalwart administrative operations.
2. Support authentication, configurable transport, safe request handling, structured errors, and testability.
3. Provide domain, account/mailbox, and alias management **where supported by the targeted Stalwart version and configuration**.
4. Supply realistic runnable examples, integration tests against Stalwart, and complete documentation.
5. Later expose a secure, versioned REST API reusing the same SDK; publish OpenAPI/Swagger documentation.
6. Keep deployment simple and avoid unnecessary dependencies.

### Non-goals for initial releases

- Building a new mail server, SMTP/IMAP implementation, or email client.
- Replacing Stalwart's existing admin console.
- Building a frontend UI.
- Introducing a database, durable job queue, or multi-tenant identity platform.
- Promising every Stalwart version or every management operation is supported.
- Storing Stalwart admin credentials in source code, logs, or example configuration.

## 2. Known Stalwart POC context — preserve these facts

The following describes the **previous local proof of concept**, not a guaranteed working API contract:

| Item | Previously observed/configured |
|---|---|
| Host OS | Windows 11, Docker Desktop |
| Docker Desktop | Reported as 29.8.1; Docker Compose v5.5.1 |
| Image | `stalwartlabs/stalwart:v1` (mutable major-version tag) |
| Reported server version | Stalwart Community Edition v1.0.1 in the POC; re-check running image/version |
| Container name | `stalwart` |
| Local hostname | `mail.poc.test` mapped to `127.0.0.1` in the Windows hosts file |
| Local management UI | `/admin` on the configured host/port |
| HTTP/TLS | Initially loopback `127.0.0.1:8080`; later HTTPS on port 443 |
| TLS | Local `mkcert` certificate/key for `mail.poc.test`, copied to a certificate directory inside the container |
| Volumes | `stalwart-config` mounted to `/etc/stalwart`; `stalwart-data` mounted to `/var/lib/stalwart` |
| Bootstrap | Temporary administrator credentials; initial admin login loop was resolved after re-setup |
| Configuration | Minimal `/etc/stalwart/config.json` observed; console invoked with `stalwart --config /etc/stalwart/config.json --console` |

### API discovery observations

- `GET /jmap/session` was observed to respond successfully.
- `GET /api` returned `404` in an earlier test.
- `GET /api/principal` returned `404` in an earlier test.
- A JMAP `Domain/get` attempt returned `unknownMethod`.
- An OAuth-related endpoint returned `404` in a test without the expected Host header; do **not** infer from that result alone that OAuth is unsupported.
- Earlier Windows `curl` tests encountered local certificate trust/revocation warnings with the `mkcert` setup. Do not work around TLS errors by disabling verification in production.

**Critical conclusion:** These observations do not establish the correct administrative endpoints or authentication scheme. In particular, a working JMAP session endpoint does **not** prove that JMAP provides domain provisioning. The SDK must be based on the actual management API exposed by the running version and authoritative version-matched documentation/source. A 404 may reflect path, method, authorization, reverse proxy, host header, feature configuration, or version differences.

### Before implementing real Stalwart operations

1. Inspect the running image's **immutable image digest** and actual Stalwart version (`docker inspect`, image metadata, admin UI/console as appropriate).
2. Inspect effective Stalwart configuration, exposed ports, host-header routing, and TLS configuration; never paste secrets into documentation.
3. Locate authoritative API documentation/source corresponding to that version; record endpoints, methods, request/response schemas, authentication, pagination, and error formats.
4. Manually validate a **read-only** administrative request first; then test a safe create/read/delete sequence on disposable resources.
5. Record sanitized request/response examples and findings in `docs/stalwart-api-validation.md`, with version, date, and evidence. If an operation is unsupported or unclear, mark it as such; do not invent it.
6. Prefer a pinned Stalwart image version/digest for repeatable integration tests; avoid relying on mutable `:v1` for CI.

### Previous broader platform design context

The original business need included multiple project/client domains, mailbox provisioning, aliases, tenant isolation, security, spam/abuse controls, backups, and monitoring. These remain **platform-level considerations**. The SDK exposes verified management capabilities; it must not claim to enforce tenant isolation, email security, or mail-server backup policies on its own. Such guarantees depend on Stalwart configuration and the calling platform.

## 3. Architectural decisions

### System view

```text
 Go application --------------------> Go SDK -------------------> Stalwart Management API
                                      ^
 Other application --> REST API ------+
                        (optional)
```

- **One repository, one SDK module initially:** `github.com/nforge101/stalwart-go`.
- **SDK first:** importable Go library at the module root or another deliberately documented public package; choose one and keep imports simple.
- **REST API later:** executable under `cmd/stalwart-api/`, handlers in `internal/rest/`; it imports the SDK, never the reverse.
- **No database initially:** the REST service is stateless. Do not claim durable async jobs, persistent audit history, or project-state reconciliation without additional infrastructure.
- **Minimal dependencies:** prefer `net/http`, `context`, `encoding/json`, `errors`, `testing`, `httptest` for SDK foundation. Choose REST router/OpenAPI tooling only when justified.
- **Public API stability:** use semantic versioning; pre-1.0 changes may be breaking, but should still be deliberate and documented.

### Proposed repository structure (evolve as needed)

```text
stalwart-go/
├── client.go                 # Public client and constructor
├── options.go                # Configuration / options
├── errors.go                 # Structured SDK errors
├── transport.go              # Shared request handling (if needed)
├── domains.go                # After API validation
├── accounts.go               # After API validation
├── aliases.go                # After API validation
├── *_test.go                 # Unit tests near code
├── cmd/
│   └── stalwart-api/
│       └── main.go           # Added in REST milestone
├── internal/
│   └── rest/                 # REST routes, middleware, handlers (later)
├── examples/
│   ├── basic/main.go
│   ├── domains/main.go       # Added with domain support
│   ├── accounts/main.go      # Added with account support
│   └── provisioning/main.go  # Added with full workflow
├── tests/
│   ├── integration/          # Real-server tests, gated/tagged
│   └── consumer/             # Optional independent go.mod fixture
├── openapi/
│   └── openapi.yaml          # REST milestone; validated spec
├── docs/
│   ├── architecture.md
│   ├── development.md
│   ├── testing.md
│   ├── compatibility.md
│   ├── stalwart-api-validation.md
│   └── security.md
├── .github/workflows/ci.yml
├── .gitignore
├── go.mod
├── README.md
├── LICENSE
└── STALWART_GO_PROJECT_SPEC.md
```

Do not create empty placeholder packages or misleading examples merely to match the tree. Introduce files as capabilities become real.

## 4. SDK functional requirements

### 4.1 Client foundation — first milestone

- Public constructor with validated base URL and explicit authentication configuration.
- Support custom `*http.Client`, request timeout, `context.Context`, and cancellation.
- Safe URL/path joining; no accidental credentials in URL or logs.
- Correct HTTP methods, headers, JSON encoding/decoding, response body closure, bounded error-body reads, and handling of non-2xx responses.
- Typed/inspectable errors that preserve status and relevant sanitized Stalwart error detail; use `errors.Is`/`errors.As` where appropriate.
- Distinguish HTTP errors, authentication/authorization failures, transport/timeouts, and decoding errors. Do not claim a server capability is unsupported solely from a generic 404.
- Explicit TLS verification by default; allow injection of a custom trusted CA via caller-supplied HTTP client for local `mkcert` testing. Never set `InsecureSkipVerify` as a default.
- No secrets in error messages, debug logs, examples, or tests.
- Design for safe concurrent use where practical; do not mutate shared request state.

**Authentication note:** Implement only the mechanism(s) verified for the target Stalwart management API. Do not assume API-key, Basic, OAuth, or JMAP auth is interchangeable. A generic credential abstraction may be designed before the concrete mechanism is validated.

### 4.2 Domain management — after validation

Target capabilities, conditional on real API support:
- Create domain; get/list domains; update relevant domain properties; delete domain.
- Validate inputs and surface meaningful errors.
- Account for pagination/filtering if supported.
- Document idempotency semantics and conflict behavior; do not invent server-side idempotency guarantees.

### 4.3 Accounts / mailboxes — after validation

Target capabilities, conditional on real API support:
- Create, retrieve/list, update, disable (if supported), and delete accounts.
- Manage mailbox-related properties supported by the management API.
- Handle password/credential material carefully: never log it or return it unnecessarily.
- Explain distinction between creating a principal/account and an actual usable mailbox if Stalwart models them separately.

### 4.4 Aliases — after validation

Target capabilities, conditional on real API support:
- Create, list/get, modify, and remove aliases/forwarding mappings.
- Validate alias targets and distinguish internal aliases from forwarding behavior if the server differentiates them.
- Do not assume aliases are a distinct resource; map to Stalwart's actual model.

### 4.5 Future optional operations

Consider only after the core SDK works: group/principal management, quota/policy administration, capability discovery, and more management features. Prioritize based on documented API support and user demand.

## 5. REST API wrapper — later milestone

The optional REST service must call **public SDK methods**, not duplicate Stalwart HTTP logic. It is a separate runnable application in the same repository, **not a second SDK**.

Requirements:
- Versioned routes, e.g. `/v1/...`; choose resource names only after SDK models are finalized.
- Health/readiness endpoints with careful semantics: process health vs Stalwart connectivity are different.
- Authentication/authorization for callers **independent of** the service's privileged Stalwart credentials. Define a secure default before enabling write endpoints; no unauthenticated administrative API.
- Bind address/port configuration; production TLS handled by the service or a trusted reverse proxy, documented explicitly.
- Input validation, request-size limits, timeouts, structured error responses, correlation/request IDs, and safe logging.
- Secret configuration via environment variables or an appropriate secrets manager; never embed secrets in images.
- Appropriate status codes, error mapping, and safe retries; avoid blindly retrying non-idempotent writes.
- Optional CORS only when needed and tightly configured.
- Graceful shutdown and clean dependency injection.
- **OpenAPI 3.x specification** at `openapi/openapi.yaml`, matching the implemented endpoints, with schemas, examples, authentication scheme, status codes, and error responses.
- Swagger UI or another documentation viewer may be bundled or hosted separately; distinguish the **OpenAPI spec** from the **Swagger UI**.
- Publish Docker image and release binaries when the service is ready; SDK users should not need the REST server or its runtime dependencies.

**Multi-tenancy warning:** One server-wide admin credential behind a REST API creates a powerful cross-domain control plane. Do not expose it publicly without strong authentication, authorization, tenant scoping, audit/monitoring strategy, and deployment guidance. Statelessness does not provide tenant isolation automatically.

## 6. Testing strategy — mandatory

| Layer | What it proves | Stalwart required? | Automation |
|---|---|---|---|
| Unit tests | Client config, transport, auth headers, parsing, error behavior | No | Every PR |
| Public consumer examples | SDK is usable through its documented public import/API | No for compile; yes for live operations | Compile every PR |
| Real-server integration | Requests actually work with a specific Stalwart version | Yes | Opt-in locally; controlled CI later |
| End-to-end provisioning | Domain → account → alias → verification → cleanup | Yes | After features exist |
| REST API tests | Routing, validation, auth, errors, SDK delegation, OpenAPI conformance | Mock SDK for unit; real server for E2E | REST milestone |
| Independent consumer fixture | Released module can be imported from a separate Go module | No for compile | Release validation |

### Unit tests

Use `httptest` to verify URL construction, headers/auth redaction, context cancellation, timeouts, custom HTTP client, non-2xx handling, malformed JSON, network failures, and concurrent behavior as appropriate. Avoid tests that merely duplicate implementation assumptions.

### Runnable examples

- Start with `examples/basic/main.go` using the actual public import path `github.com/nforge101/stalwart-go`.
- Examples must compile, avoid hardcoded credentials, and use environment variables for live connection settings.
- Do not show nonexistent methods in README or executable examples. A conceptual example must be clearly marked **pseudocode** and not presented as runnable.
- Add domain/account/alias/provisioning examples only when corresponding SDK operations are implemented and validated.
- Document the side effects of write examples; require an explicit opt-in or use disposable resource names.
- `go run ./examples/basic` is a convenience demonstration, **not** a substitute for real integration tests.

### Integration tests

- Keep them out of default fast test runs using an `integration` build tag or equivalent explicit gating.
- Example command once suite exists: `go test -tags=integration ./tests/integration/...`.
- Read test URL/credentials from environment, e.g. `STALWART_TEST_URL`, `STALWART_TEST_USERNAME`, `STALWART_TEST_PASSWORD` or a verified API-token variable. Choose the real scheme after validation.
- Use disposable, uniquely named test resources, verify state, and register cleanup with `t.Cleanup`.
- Never run destructive tests against production. Require a dedicated test server and an explicit safety guard for writes.
- Record tested Stalwart version and configuration. For CI, pin image versions and provide secure test secrets.

### Independent consumer compatibility

At a later milestone, use a fixture with its own `go.mod` that imports a tagged version of `github.com/nforge101/stalwart-go`. A `replace` directive may help local development, but **release validation must also test without `replace`** so real module resolution is exercised.

## 7. Documentation requirements

### README.md — user-facing landing page

Include:
1. What the project is, who should use it, and current status.
2. Supported Stalwart versions and tested configurations (link to compatibility matrix).
3. Installation command (`go get github.com/nforge101/stalwart-go`) once importable.
4. **Compiling** quick-start using actual SDK APIs; environment-variable configuration.
5. Supported operations table: implemented / tested / planned.
6. Links to runnable examples, integration test guide, architecture, and REST API docs when available.
7. Security guidance: privileged credentials, TLS, least privilege, no secrets in Git.
8. Contribution guidance, MIT license, and issue-reporting instructions.
9. Clear distinction between the SDK and the optional REST service.

### docs/architecture.md

Document system context, dependency direction, SDK components, request lifecycle, auth/transport/error strategy, deployment boundaries, design tradeoffs, and future REST layering. Include Mermaid diagrams where helpful and keep diagrams synchronized with actual code.

### docs/development.md

Explain prerequisites, Go module setup, formatting/linting, local commands, branch/PR workflow, commit conventions, API design rules, and how to add an SDK operation. Record the supported Go version policy.

### docs/testing.md

Explain fast tests, integration test setup, safety guards, examples, CI, and how to test against multiple Stalwart versions. Include precise commands and expected environment variables.

### docs/compatibility.md

Maintain a matrix of Stalwart version, API behavior, authentication method, supported SDK operations, integration test result, and date. **Untested is not supported**; do not infer compatibility from semver alone.

### docs/stalwart-api-validation.md

Maintain version-specific, sanitized evidence of the actual endpoints, methods, schemas, status codes, auth, and observations. Record contradictions or unknowns explicitly. This is the primary bridge from our earlier POC to reliable SDK code.

### docs/security.md

Describe credential handling, HTTPS/certificates, logging redaction, REST caller authentication, privilege boundaries, rate limiting, secret rotation, responsible disclosure, and deployment risks.

### REST documentation

When REST exists, publish `openapi/openapi.yaml`, usage examples (`curl`), configuration reference, and Docker/binary deployment instructions. Validate the OpenAPI spec in CI and ensure it matches implemented handlers.

## 8. Go version, dependencies, and release policy

- Developer's current local Go installation: **Go 1.27.4** (as reported).
- Initial declared minimum in `go.mod`: **Go 1.26.0**, as previously chosen. Reassess before release based on dependency requirements, actual test results, and supported Go toolchains.
- CI should test at least the **declared minimum** and **current stable** Go version. A `go.mod` declaration alone does not prove compatibility.
- Use `go fmt`/`gofmt`, `go vet`, `go test`, `go build`; optionally `go test -race` where supported.
- Avoid forcing a newer Go minimum through unnecessary dependencies.
- Use semantic tags such as `v0.1.0` when a real usable release exists; include changelog/release notes.
- For the REST executable, publish platform-specific binaries and a Docker image later. A Go SDK is consumed as source through Go modules, not distributed as a required binary.

## 9. Security and open-source hygiene

- Never commit admin passwords, tokens, `.env` files, certificates/private keys, personal data, or client/employer proprietary materials.
- Use `.env.example` with placeholders if useful, and ensure `.env` is ignored.
- Never disable TLS verification in the SDK by default; document how to trust a local `mkcert` CA for the POC.
- Do not use production domains/accounts for integration testing.
- Keep SDK package dependencies independent from REST server dependencies where practical.
- Prefer explicit, narrowly scoped behavior over magic defaults.
- Include MIT license, contributing guidelines when contributors arrive, and a security reporting policy before broad adoption.
- Commit authorship should reflect the actual author; organization owns the repository. Personal and work Git identities are configured separately outside this specification.

## 10. Delivery milestones and acceptance criteria

### M1 — SDK foundation + developer experience

**Deliver:** Go module, public client/configuration API, safe transport and errors, unit tests, one runnable/compiling basic consumer example, initial README, architecture/development/testing docs, CI workflow.

**Acceptance:** `go test ./...`, `go vet ./...`, `go build ./...` pass; examples compile; no fabricated management endpoints or hardcoded credentials; public API is understandable. The basic example must not pretend an unverified management operation exists.

### M2 — Stalwart API validation + compatibility baseline

**Deliver:** Identify running Stalwart version, authenticate with a verified management endpoint, document actual API contract, establish dedicated integration environment and tests, record compatibility matrix.

**Acceptance:** At least one read-only management call validated against a real Stalwart server with sanitized evidence; tests fail clearly if the environment is missing or incompatible.

### M3 — Domain operations

**Deliver:** Verified domain operations, unit and real-server tests, runnable example, README/docs updates.

**Acceptance:** Create/read/list/delete lifecycle where supported, documented conflict/error behavior, cleanup verified.

### M4 — Account/mailbox operations

**Deliver:** Verified account/mailbox operations, secure credential handling, tests, examples, docs.

**Acceptance:** Disposable account lifecycle verified; no secret leakage.

### M5 — Alias operations and complete provisioning example

**Deliver:** Verified alias operations, end-to-end sample domain → account → alias, tests, cleanup, docs.

**Acceptance:** Provisioning workflow runs against a dedicated test instance and leaves no test artifacts.

### M6 — SDK hardening and first public release

**Deliver:** Public API review, compatibility tests, independent consumer fixture, README polish, CI matrix, changelog, tagged release.

**Acceptance:** A separate module can install and compile against the released version without a local `replace` directive.

### M7 — Optional REST API + OpenAPI/Swagger

**Deliver:** Secure REST server calling SDK, OpenAPI spec, documentation UI/guide, mock and integration tests, configuration/deployment docs.

**Acceptance:** No unauthenticated privileged write routes; documented endpoints match actual behavior; SDK remains usable independently.

### M8 — Distribution and operations

**Deliver:** Docker image, binary releases, deployment examples, observability and security hardening.

**Acceptance:** Reproducible build, documented runtime config, clean startup/shutdown, release artifacts verified.

## 11. Cursor AI implementation workflow

For each milestone:

1. **Inspect:** Read this spec, existing README/docs, `go.mod`, and relevant source/tests. Do not overwrite unrelated files.
2. **Plan:** State scope, proposed files, public API changes, external dependencies, and unverified assumptions.
3. **Validate:** For Stalwart operations, check the actual server version and authoritative API contract **before** coding. If access to the POC is unavailable, ask for sanitized outputs or implement only contract-independent foundation work.
4. **Implement:** Make small, idiomatic changes; keep SDK and REST boundaries clean.
5. **Test:** Run formatting, tests, vet, build, and example compilation; run integration tests only against an approved disposable instance.
6. **Document:** Update README, relevant docs, compatibility notes, and examples in the same change as functionality.
7. **Review:** Summarize changes, evidence, commands/results, limitations, and next milestone. Do not claim tests passed unless they actually ran.
8. **Commit:** Prefer focused commits and feature branches/PRs; do not auto-push or release without explicit approval.

### First prompt to give Cursor Agent

> Read `STALWART_GO_PROJECT_SPEC.md` fully and inspect the existing repository. We are implementing **M1 only**: SDK foundation and developer experience. First propose the package structure, minimal public client API, error model, unit-test plan, and example structure. Flag all assumptions about Stalwart authentication/endpoints as **unverified**; do not implement domain/account/alias operations or the REST server yet. After presenting the plan, implement M1, run `go test ./...`, `go vet ./...`, and `go build ./...`, ensure the public consumer example compiles, update the README and architecture/development/testing docs, and report actual results. Do not commit or push without my approval.

## 12. Open decisions / questions to resolve with evidence

- Which exact Stalwart version(s) will be supported first, and how will they be pinned in tests?
- What are the verified management API endpoints, authentication methods, and resource schemas for that version?
- Are domains, accounts, and aliases separate management resources or different principal/configuration representations?
- What minimum Go version should be retained at first release, based on tested compatibility?
- Which SDK public API shape provides the simplest consumer experience without overcommitting to unsupported operations?
- Which REST caller authentication and authorization strategy is appropriate before exposing administrative operations?
- Will the REST API need tenant-scoped authorization, durable provisioning workflows, or audit persistence in a later release?

**Definition of done:** The SDK is not complete merely because it compiles. It must be importable, documented, tested in isolation, validated against real supported Stalwart versions, and usable by an external developer. The REST wrapper is not complete merely because it exposes endpoints; it must enforce a secure privilege boundary and accurately document its behavior.
