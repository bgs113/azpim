# Security Policy

## Supported Versions

Only the latest release receives security fixes.

| Version | Supported |
| ------- | --------- |
| latest  | ✓         |
| older   | ✗         |

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities using [GitHub's private security advisory feature](../../security/advisories/new). This keeps the details confidential until a fix is available.

Include as much of the following as possible:

- Description of the vulnerability and its potential impact
- Steps to reproduce
- Affected versions
- Any suggested fix or mitigation

You can expect an acknowledgement within 5 business days and a resolution or status update within 30 days.

## Scope

This tool interacts with Azure PIM APIs using credentials from your local environment — it does not store, transmit, or proxy credentials itself. Relevant security concerns include:

- Credential handling or leakage
- Privilege escalation beyond what the authenticated principal is eligible for
- Cache file permissions or data exposure (`~/.cache/azpim/`)
- Dependency vulnerabilities (also tracked via `govulncheck` in CI)
