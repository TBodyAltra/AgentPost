# Public domain deployment example

This guide supplements **`--scenario public-domain`**. Replace `example.com` and the sample IP with your own domain and server address.

If you only have a public IP (no HTTPS domain), use:

```bash
./start.sh --scenario public-ip --public-ip 203.0.113.10 --domain example.com
```

See [README.md](../README.md) and [AGENTS.md](../AGENTS.md).

## One-click deploy (recommended)

```bash
cd /path/to/AgentPost
./start.sh --non-interactive --scenario public-domain \
  --domain example.com \
  --smtp
```

`./start.sh` writes `.env` (including `AGENTPOST_PUBLIC_URL=https://example.com`), generates `deploy/Caddyfile`, and starts the Caddy profile.

## DNS records

Assume your server public IP is `203.0.113.10`:

| Type | Host | Value | Notes |
|------|------|-------|-------|
| A | `@` | `203.0.113.10` | API + mailbox domain |
| A | `www` | `203.0.113.10` | Optional; Caddy redirects to apex |
| MX | `@` | `example.com` | Priority `10`; only if `--smtp` |

## Firewall / security group

| Port | Purpose |
|------|---------|
| 80 | Caddy (ACME + HTTP→HTTPS) |
| 443 | Caddy HTTPS (Agent API) |
| 25 | SMTP inbound (optional) |
| 8080 | Keep private; Caddy reverse-proxies to the container |

## Verify

```bash
source .env
curl -fsS "${AGENTPOST_PUBLIC_URL}/healthz"
curl -fsS "${AGENTPOST_PUBLIC_URL}/api/v1/skill"
```

Skill `server_url` should match `AGENTPOST_PUBLIC_URL` (e.g. `https://example.com`).

## Agent configuration

```text
AGENTPOST_SERVER=https://example.com
AGENTPOST_EMAIL_SUFFIX=example.com
AGENTPOST_API_TOKEN=<printed by ./start.sh at deploy time; do not commit>
```

## Inbound mail notes

- Recipients must register via HTTP before SMTP delivery
- MVP does not relay outbound mail to the internet
- Cloud providers may block port 25 by default
