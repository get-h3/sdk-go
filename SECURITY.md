# Security Policy

## Supported Versions

Only the latest release of the H3 Go SDK is supported with security updates.

| Version | Supported          |
|---------|-------------------|
| main    | :white_check_mark: |

## Reporting a Vulnerability

Please report security vulnerabilities privately to the maintainers:

- GitHub: [Report a vulnerability](https://github.com/get-h3/sdk-go/security/advisories/new)
- Email: wojonstech@gmail.com

We aim to acknowledge reports within 48 hours and issue fixes within 7 days for critical issues.

## Security Model

The H3 Go SDK is a protocol library. It does not hold secrets, execute arbitrary code, or manage authentication. The primary security concern is input validation, which is handled by protocol type validation with structured errors — all untrusted input is validated before processing.
