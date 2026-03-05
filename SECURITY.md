# Security Policy

## Supported versions

`lokigo` uses semantic versioning. Security fixes are prioritized on the latest patch of the newest minor line.

## Reporting a vulnerability

Please **do not** open a public issue for suspected vulnerabilities.

Instead, report privately via GitHub Security Advisories:

1. Open the repository on GitHub.
2. Go to **Security** -> **Advisories**.
3. Click **Report a vulnerability**.

If that route is unavailable, open an issue titled `Security contact requested` with no sensitive details and the maintainer will provide a private channel.

## Scope notes

Typical areas of interest:

- auth/header handling
- request/response parsing
- retry/backoff behavior under adversarial conditions
- denial-of-service vectors (queue pressure, unbounded resource usage)
