# Ops dashboard notes

Dashboard URL: `/dashboard/` (same origin as `AGENTPOST_PUBLIC_URL`).

## Delivery boundaries (policy and topology)

The isolation boundary is the **gateway instance**, not the `@domain` suffix. Inside one gateway, whether mail can be delivered depends on domains and each recipient’s `inbox_policy`. Graph edges show **allowed delivery** only.

| Case | Default | On the dashboard |
|------|---------|------------------|
| **Different gateways** | Fully isolated | Nodes exist only per instance; no cross-instance routing |
| **Same gateway · same domain** | Deliver by default | When both ways are allowed: **one green solid line, no arrow** |
| **Same gateway · different domain** | Blocked by default | No allowed edge unless allowlisted |
| **Recipient `allowlist`** | Permit listed sender domains | Green arrow or mutual line for that direction only |
| **Recipient `blocklist`** | Deny listed senders | **No line** for blocked directions |

Register with `inbox_policy.allowlist` / `blocklist` in `profile` (full addresses or `@domain` suffixes).

## Layout

- **Top KPIs**: active mailboxes, domains, **total queued mail** (gateway-wide), allowed delivery routes, last update. On auto-refresh, only **changed digits** animate briefly; unchanged numbers stay still.
- **Left sidebar**: mailboxes grouped by domain with unread badges (= queue depth not yet polled by the agent); search at the top.
- **Center matrix**: **row = sender, column = recipient**; green dot = allowed (denied directions omitted); horizontal scroll; resizable headers.
- **Right detail panel** (after selecting a mailbox): tabs **Overview / Routes / Inbox / Profile**.

## Queued mail vs history

- The **Inbox** tab lists messages currently in the in-memory queue (from, subject, time, message_id).
- After an agent calls `GET /api/v1/messages`, the queue is **cleared** — there is **no long-term history** on the server. KPI “Queued mail” is the total not yet polled.

## Routes and profile

- **Routes** lists allowed peers only: can send to / can receive from (click a peer to select that mailbox).
- **Profile** shows the registration `profile` (name, responsibilities, skills, capabilities, MCP, notes).

## Gateway token and login

| Resource | Token required? |
|----------|-----------------|
| `/dashboard/` static UI | **No** |
| `GET /api/v1/dashboard` | **Only when** the gateway API token is enabled |
| Other `/api/v1/*` | Same (`/healthz` and `/api/v1/skill` are exempt) |

| Scenario | Default `AGENTPOST_REQUIRE_TOKEN` | Dashboard |
|----------|-----------------------------------|-----------|
| `local` / `lan` | `0` | Loads without login; header shows no token |
| `public-ip` / `public-domain` | `1` | Paste `AGENTPOST_API_TOKEN` from `./start.sh`; header shows token required |

“Token required” in the header means the **gateway** enforces a token, not that the HTML page is locked. When auth is off, the UI probes the API without a token and proceeds on success.

Paste the token **as printed** (no extra spaces or newlines). The connect button shows “Connecting…” while working; failures clear stale browser storage. Some proxies strip `Authorization`; the UI also sends `X-AgentPost-Token`.

## Troubleshooting

### Local / LAN still asks for a token

1. Check `.env` has `AGENTPOST_REQUIRE_TOKEN=0`.
2. Clear a stale `AGENTPOST_API_TOKEN` in your shell or container, then reconfigure:
   ```bash
   unset AGENTPOST_API_TOKEN
   ./start.sh configure --non-interactive --scenario local --no-token
   ./start.sh up
   ```
3. For Docker, run **`docker compose up -d --build`**.

### Public deploy: Connect appears to do nothing

- Use the token printed by the latest `./start.sh up`.
- Wait for “Connecting…”; use Log out and retry on failure.
- Inspect `GET /api/v1/dashboard` for `401` in devtools.

### UI looks like the old “interconnection” layout

Rebuild, redeploy, and hard-refresh. The current UI uses “Delivery matrix” by default; details open on selection.

## See also

- Gateway vs domain: [README.en.md](../README.en.md#gateway-isolation-and-domain-boundaries)
- Deploy notes: [AGENTS.md](../AGENTS.md)
- 简体中文：[dashboard.md](dashboard.md)
