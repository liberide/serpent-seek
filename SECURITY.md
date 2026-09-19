# Security Policy

## Supported Versions

Security fixes are applied to the latest release line only. Make sure you run
the newest published Docker image / Git commit before reporting an issue that
is already fixed upstream.

| Version           | Supported |
| ----------------- | --------- |
| latest release    | ✅        |
| older releases    | ❌        |

## Reporting a Vulnerability

**Do not open a public issue for security reports.**

Please report vulnerabilities privately through **GitHub Security Advisories**
of this repository:

https://github.com/liberide/serpent-seek/security/advisories/new

or by email: **security@liberide.dev**

Include:

- a description of the vulnerability and its impact,
- the affected version / commit and deployment (SQLite or PostgreSQL,
  auth enabled or not),
- steps to reproduce or a proof of concept,
- any suggested mitigation, if you have one.

You will receive an acknowledgement within a few days. Once the issue is
confirmed, we will prepare a fix, credit you in the advisory (unless you
prefer to remain anonymous), and publish the advisory together with the fixed
release.

## Scope Notes

This project handles third-party credentials (search provider API keys) and
issues API keys/passkeys for its own admin UI. Reports about credential
storage, redaction in logs, authentication/authorization bypasses, SSRF
through provider base URLs, and XSS in the admin UI are explicitly in scope.

Vulnerabilities in dependencies should preferably be reported upstream; if an
upstream fix requires a timely dependency bump here, tell us via the same
advisory channel.
