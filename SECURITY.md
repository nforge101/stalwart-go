# Security Policy

## Supported versions

This project is under active pre-release development. Security fixes will target the `main` branch and any tagged releases once they exist. Pre-1.0 APIs may change.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Prefer one of the following:

1. **GitHub Private Vulnerability Reporting** for [nforge101/stalwart-go](https://github.com/nforge101/stalwart-go/security/advisories/new) (enable Private vulnerability reporting in the repository settings if it is not already on).
2. Contact the NForge101 maintainers through the organization if private reporting is unavailable.

Include steps to reproduce, affected versions/commits, and impact. We will acknowledge reports as promptly as practical and coordinate disclosure.

## Security expectations for this project

- Never commit real credentials, API tokens, private keys, or certificates.
- Use environment variables or a secrets manager for Stalwart and (later) REST caller credentials.
- TLS certificate verification must remain enabled by default in the SDK.
- The optional REST wrapper (when added) must not expose unauthenticated administrative write routes.
- Integration tests must use disposable resources on dedicated non-production Stalwart instances.
