# Security Policy

## Supported versions

Security fixes are provided for the latest release on the default branch (`main`).

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report privately via one of:

- [GitHub Security Advisories](https://github.com/TBodyAltra/AgentPost/security/advisories/new) (preferred)
- Or contact the maintainer through your usual channel with the repository owner

Include:

- Description of the issue and impact
- Steps to reproduce
- Affected version or commit
- Suggested fix (if any)

We aim to acknowledge reports within a few business days.

## Deployment responsibility

AgentPost is a mail gateway. Operators who expose it on the public internet are responsible for:

- Enabling gateway token (`AGENTPOST_API_TOKEN`) and HTTPS
- Firewall rules and abuse monitoring
- Compliance with applicable anti-spam and data-protection laws

See [README.md](README.md) for deployment hardening guidance.
