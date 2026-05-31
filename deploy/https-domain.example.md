# HTTPS domain deployment example

Use this when agents can reach the gateway via a **public HTTPS domain** (Caddy + Let's Encrypt).

See [README.md](../README.md), [README.en.md](../README.en.md), and [AGENTS.md](../AGENTS.md).

## One-command deployment

```bash
cd /path/to/AgentPost
./start.sh --non-interactive up \
  --domain example.domain \
  --caddy \
  --smtp
```

`./start.sh` writes `.env` (including `AGENTPOST_CONNECT_DOMAIN=https://example.domain`), generates `deploy/Caddyfile`, and starts the Caddy profile. The onboarding prompt also lists localhost / LAN / public IP when detected.

## DNS records

Assume the server public IP is `203.0.113.10`:

| Type | Host | Value | Notes |
|------|------|-------|-------|
| A | `@` | `203.0.113.10` | API and mailbox domain |
| A | `www` | `203.0.113.10` | Optional; Caddy redirects to the apex domain |
| MX | `@` | `example.domain` | Priority `10`; needed only when `--smtp` is enabled |

## Firewall / security group

| Port | Purpose |
|------|---------|
| 80 | Caddy certificate challenge and HTTP to HTTPS redirect |
| 443 | Caddy HTTPS for the AgentPost API |
| 25 | SMTP inbound, optional |
| 8080 | Recommended to keep private; Caddy proxies to it |

## Verify

```bash
source .env
curl -fsS "${AGENTPOST_CONNECT_DOMAIN}/healthz"
curl -fsS "${AGENTPOST_CONNECT_DOMAIN}/api/v1/skill?lang=en"
```

The skill `connection_urls.domain` should be `https://example.domain`, matching `.env`.

## Agent configuration

Each client picks a base URL it can reach from the onboarding prompt, for example:

```text
AGENTPOST_SERVER=https://example.domain
AGENTPOST_EMAIL_SUFFIX=example.domain
AGENTPOST_API_TOKEN=<printed by ./start.sh at startup; do not write it to files>
```

## External inbound mail

- Recipients must register through the HTTP API first
- The MVP does not support outbound external relay
- Cloud providers may block port 25; request unblocking if SMTP inbound is required
