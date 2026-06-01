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
- **Search**: default substring match on mailbox, username, or domain (case-insensitive); use `/pattern/flags` (e.g. `/^bot-.*@lab/`, `/\\.internal$/i`) for regular expressions (escape `/` in the pattern as `\\/`). An unclosed `/pattern` shows an error instead of falling back to substring search. Invalid patterns show an error. The matrix still includes delivery peers of matched mailboxes.
- **Center matrix**: rows and columns are sorted by **domain** with **merged domain headers** on the top and left; thicker borders separate domain blocks. **Row = sender, column = recipient**; green dot = allowed. Click a cell to highlight that mailbox’s row/column only (domain headers stay unhighlighted); click a mailbox header for details. Scroll inside the matrix panel (wider scrollbars).
- **Message log** (below the matrix): **Sent** entries only (delivered to a queue). When the recipient polls with `GET /api/v1/messages`, that row shows a **double check** (Telegram-style read); a single check means delivered but not yet polled. **Hover a row** to preview the body (`request`/`reply` JSON is parsed; content is rendered as **Markdown**; `\uXXXX` and literal `\n` are decoded). The tooltip is scrollable. Selecting a mailbox filters to that address; click a row to jump to the peer mailbox. Kept in memory (last ~1000 entries), **cleared on restart**.
- **Inbox tab**: each queued message shows a Markdown-rendered body preview, not only the subject.
- **Right detail panel** (after selecting a mailbox): tabs **Overview / Routes / Inbox / Profile**.
- **Resizable panels**: drag the separators to resize the mailbox list (left), message log (bottom), and detail panel (right, when open). Sizes are stored in the browser and persist across reloads. Drag handles are hidden on narrow layouts (about ≤1100px).

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

| `AGENTPOST_REQUIRE_TOKEN` | Dashboard |
|-------------------------|-----------|
| `1` (`./start.sh up` default) | Paste `AGENTPOST_API_TOKEN` from `./start.sh`; header shows token required |
| `0` (`./start.sh up --no-token`) | Loads without login; header shows no token |

“Token required” in the header means the **gateway** enforces a token, not that the HTML page is locked. When auth is off, the UI probes the API without a token and proceeds on success.

Paste the token **as printed** (no extra spaces or newlines). The connect button shows “Connecting…” while working; failures clear stale browser storage. Some proxies strip `Authorization`; the UI also sends `X-AgentPost-Token`.

## Troubleshooting

### Local / LAN still asks for a token

1. Check `.env` has `AGENTPOST_REQUIRE_TOKEN=0`.
2. Clear a stale `AGENTPOST_API_TOKEN` in your shell or container, then reconfigure:
   ```bash
   unset AGENTPOST_API_TOKEN
   ./start.sh configure --non-interactive --no-token
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
