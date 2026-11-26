# Architecture

## System context

```text
 Go application --------------------> Go SDK -------------------> Stalwart Management API
                                      ^
 Other application --> REST API ------+
                        (optional, later, self-hosted)
```

- **Primary deliverable:** importable Go SDK at `github.com/nforge101/stalwart-go`.
- **Optional REST wrapper (later):** a separate executable that imports the SDK. Non-Go systems call the REST service; Go apps normally use the SDK directly.
- **No hosted control plane:** adopters run their own Stalwart (and optional REST) with their own credentials.

## Dependency direction

| Component | May depend on |
|-----------|----------------|
| SDK | Stalwart management API (HTTP), Go stdlib |
| Optional REST service | Public SDK only (not the reverse) |
| Examples / integration tests | Public SDK module path |

The SDK must remain independent of REST frameworks, databases, frontends, and deployment-specific infrastructure.

## Authentication boundaries

| Layer | Direction | Purpose |
|-------|-----------|---------|
| Application auth | End users → consumer backend | Product login / RBAC (owned by the integrator) |
| SDK → Stalwart | Consumer backend via SDK → Stalwart | Management privilege (admin API key or equivalent) |
| REST caller auth (M7+) | Callers → deployed REST service | Separate from Stalwart credentials; required before privileged write routes |

## Management protocol and errors

Stalwart's management operations use JMAP at `/jmap/`. Each SDK call sends one method call per request, through the internal helper `callJMAP` (`jmap.go`). The helper's result is one of three things:

| Outcome | Returned as | Meaning |
|---------|-------------|---------|
| Success | The method's arguments | The method ran |
| HTTP error | `*APIError` | Stalwart refused the request before any method ran (for example 401 or 403) |
| Method error | `*MethodError` | The request arrived, but Stalwart rejected the method inside an HTTP 200 response |

Callers must handle both error types. A check on the HTTP status alone would miss method errors. Unexpected response shapes wrap `ErrDecode`.

Public operations are added only after they are verified against a real server (see [stalwart-api-validation.md](stalwart-api-validation.md)).

## M1 scope

Milestone 1 establishes module hygiene, documentation, CI, and (in later M1 slices) a public client with safe transport and structured errors. It does **not** implement domain, account, or alias operations, and does not assume unverified Stalwart endpoints.

Wire-protocol details (for example JMAP management methods) are deferred until Milestone 2 validation against a pinned Stalwart version.
