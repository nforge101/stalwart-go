# Stalwart API validation

This document records what has been verified against a real Stalwart server, and what is only taken from Stalwart's documentation. If an operation is not listed as verified, the SDK does not claim it works.

Evidence is sanitized. It never contains hostnames, account or domain identifiers, domain names, or credentials.

## Target

| Item | Value |
|------|-------|
| Stalwart image | `stalwartlabs/stalwart:v1.0.1` |
| Image digest | `sha256:b9da4df8b4efc06bc926dd6e990bdeb0ad7b9261a7c3893d74b0461a00bb389b` |
| Validated on | 2026-11-26 |
| Method | `integration_test.go` (build tag `integration`), run against a dedicated local test server |
| Authentication | API key as a bearer token, permission mode `Replace` |
| TLS | Verified by the SDK's default HTTP client against the operating system trust store (locally issued CA) |

## Summary

| Operation | Status |
|-----------|--------|
| `GET /jmap/session` with a bearer API key | Verified |
| `x:Domain/get` (read-only) | Verified |
| Unissued API key is rejected with HTTP 401 | Verified |
| Signed-in key without `sysDomainGet` is denied with a method-level `forbidden` error | Verified; SDK reports `*MethodError` (M3-S1) |
| Key without the sign-in permission is rejected with HTTP 403 | Observed during setup; not covered by an automated test |
| Basic authentication | Not validated |
| `x:Domain/set`, `x:Domain/query`, accounts, aliases | Not validated (later milestones) |

## Verified details

### Authentication and permissions

A bearer API key is accepted on both `/jmap/session` and `/jmap/`. The minimum permissions for reading domains are:

| WebUI label | Internal name |
|-------------|---------------|
| Sign in to the server | `authenticate` |
| Domains Management: Get domains | `sysDomainGet` |

The key's permission mode is `Replace`. A key can narrow its owner's permissions but never extend them.

### Session

```text
GET /jmap/session
Authorization: Bearer API_<redacted>
Accept: application/json
```

The response is HTTP 200 with a JSON object. Its fields are `accounts`, `apiUrl`, `capabilities`, `downloadUrl`, `eventSourceUrl`, `primaryAccounts`, `state`, `uploadUrl`, and `username`. The path of `apiUrl` is `/jmap/`.

Capabilities advertised to the least-privilege key (`authenticate` and `sysDomainGet` only):

```text
urn:ietf:params:jmap:blob
urn:ietf:params:jmap:calendars
urn:ietf:params:jmap:calendars:parse
urn:ietf:params:jmap:contacts
urn:ietf:params:jmap:contacts:parse
urn:ietf:params:jmap:core
urn:ietf:params:jmap:emailpush
urn:ietf:params:jmap:filenode
urn:ietf:params:jmap:mail
urn:ietf:params:jmap:principals
urn:ietf:params:jmap:principals:availability
urn:ietf:params:jmap:quota
urn:ietf:params:jmap:sieve
urn:ietf:params:jmap:submission
urn:ietf:params:jmap:vacationresponse
urn:ietf:params:jmap:webpush-vapid
urn:ietf:params:jmap:websocket
```

Two observations follow from this list:

- `urn:stalwart:jmap` is **not** advertised, yet a request that lists it in `using` succeeds. The SDK must not gate management calls on finding that capability in the session.
- The capability set depends on the key's permissions. A key holding every permission also saw `urn:ietf:params:jmap:metadata`, which the restricted key did not.

### `x:Domain/get`

```text
POST /jmap/
Authorization: Bearer API_<redacted>
Content-Type: application/json
Accept: application/json

{
  "using": ["urn:ietf:params:jmap:core", "urn:stalwart:jmap"],
  "methodCalls": [
    ["x:Domain/get", {"ids": null, "properties": ["name"]}, "c1"]
  ]
}
```

The response is HTTP 200 with a `methodResponses` array containing one entry, `["x:Domain/get", <arguments>, "c1"]`.

| Observation | Result |
|-------------|--------|
| `ids: null` | Accepted; returns every domain on the server |
| `accountId` omitted from the request | Accepted |
| Argument fields in the response | `accountId`, `list`, `notFound` |
| `state` in the response arguments | Absent. Management `get` responses do not follow the standard JMAP shape exactly |
| `list` | An array. Each item has the string fields `id` and `name` |

Values of `accountId` and `notFound` were not recorded. Top-level fields of the response other than `methodResponses` were also not recorded.

### Error responses

Stalwart reports permission and credential problems in three distinct ways:

| Situation | HTTP status | Body | SDK result today |
|-----------|-------------|------|------------------|
| Unissued API key | 401 | JSON object with the fields `type`, `title`, `status`, `detail`; `type` is `about:blank` | `*APIError`, matches `ErrUnauthorized` |
| Key without the sign-in permission | 403 | Same object shape: `title` is `Forbidden`, `detail` is "You do not have enough permissions to access this resource." | `*APIError`, matches `ErrForbidden` |
| Signed-in key without the method permission | 200 | One method response named `error`; its arguments have the fields `type` (value `forbidden`) and `description` | `*MethodError` with `Type` `forbidden`, matching `ErrForbidden` (M3-S1) |

The third form matters most. An HTTP-level error check does not detect it, so a caller that only checks HTTP status would treat a denied request as success. From M3-S1, `callJMAP` reports it as a `*MethodError`, and the integration test `TestIntegration_NoPermissionKey` asserts this against the real server.

## Domain read operations (M3-S2)

The SDK's `Domains().List` and `Domains().Get` send these requests. The integration tests `TestIntegration_DomainsList`, `TestIntegration_DomainsGet`, `TestIntegration_DomainsGetUnknown`, and `TestIntegration_DomainsListNoPermission` passed against v1.0.1.

| Request | Expected result | Status |
|---------|-----------------|--------|
| `x:Domain/get` with `ids: null` and `properties: ["name"]` | `list` of every domain | Verified (M2) |
| `x:Domain/get` with `ids: ["<id>"]` for an existing domain | `list` with that domain | Verified (M3-S2) |
| `x:Domain/get` with `ids: ["<id>"]` for an unknown domain | Empty `list`; the ID in `notFound`. The SDK returns `ErrNotFound` | Verified (M3-S2) |
| `x:Domain/get` without `sysDomainGet` | Method-level `forbidden` error | Verified (M2) |

A field probe (`TestIntegration_DomainFieldProbe`) requests the other documented `Domain` properties and logs which the server returns. Only the field names are recorded. On the test server, the probe returned all 12 candidate properties for the first domain: `aliases`, `allowRelaying`, `catchAllAddress`, `certificateManagement`, `createdAt`, `description`, `dkimManagement`, `dnsManagement`, `enable`, `reportAddressUri`, `subAddressing`, and `name`. The probe checks key presence only, not values or value types, and it checked one domain. A field is added to the SDK only after its value type is checked across all domains.

## Documented but not verified

Stalwart's documentation (not yet checked against this server) describes:

- `x:Domain/set` for create, update, and destroy, requiring `sysDomainCreate`, `sysDomainUpdate`, and `sysDomainDestroy`.
- `x:Domain/query`, requiring `sysDomainQuery`.
- `/.well-known/jmap` redirecting to `/jmap/session`. The SDK does not follow redirects, so it uses `/jmap/session` directly.

None of these are used by the SDK until they are validated.

## Limits of this validation

- One Stalwart version and one configuration were tested. Other versions are unsupported until validated.
- Only read operations were exercised. No resource was created, changed, or deleted.
- Basic authentication was not tested.

## Reproducing

See [testing.md](testing.md) for the environment variables, key setup, and the command:

```bash
go test -tags=integration -count=1 -v -run TestIntegration .
```
