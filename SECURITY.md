# Security Policy

## Reporting a vulnerability

Please do not report security vulnerabilities in public issues.

Use GitHub Security Advisories for this repository to privately report suspected vulnerabilities. Include:

- Affected version or commit
- Deployment flags (for example `--caddy`, `--no-token`, connection URLs in `.env`)
- Steps to reproduce
- Expected and actual impact
- Any logs, requests, or configuration snippets needed to understand the issue

If private reporting is unavailable, open a public issue that only asks for a secure contact channel and does not include exploit details.

## Supported scope

AgentPost is an MVP HTTP mail gateway with in-memory storage. Security reports are most useful when they concern the gateway code, deployment scripts, authentication, request signing, SMTP parsing, or abuse-prevention behavior.

Public operators remain responsible for their own firewall rules, DNS, TLS, token distribution, spam/abuse controls, and legal or compliance obligations.
