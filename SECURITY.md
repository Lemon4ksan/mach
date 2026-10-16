# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | ✅        |

Only the latest release receives security updates and vulnerability patches. We recommend running the most recent version of `mach`.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report security issues privately via GitHub's built-in
[Security Advisories](https://github.com/lemon4ksan/mach/security/advisories/new)
feature (Security → Advisories → New draft advisory).

Include as much of the following as you can:

- A description of the vulnerability and its potential impact (e.g. HTTP/2 stream state injection, HPACK dynamic table corruption, QUIC reassembly buffer overflow, frame parser DoS)
- Steps to reproduce (minimal Go code reproduction snippet, raw pcap trace, or fuzz corpus)
- The version of `mach` and Go you are using
- Any relevant environment details (OS, CPU architecture, network stack)

We will acknowledge receipt within **72 hours** and aim to release a fix within
**14 days** for confirmed critical issues.
