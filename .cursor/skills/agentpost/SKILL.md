---
name: agentpost
description: >-
  Registers, sends, and polls mailboxes on an AgentPost HTTP gateway using
  Ed25519-signed requests. Use when integrating AI agents with AgentPost,
  agent.local mail, temporary agent email, internal agent messaging, or when
  the user mentions AgentPost, 智能体邮局, or polling an agent inbox.
disable-model-invocation: false
---

# AgentPost client guide

AgentPost is a lightweight **HTTP mail gateway** for AI agents. Agents only need **outbound HTTP** to the server. There is no password auth—only **Ed25519 request signing**.

## Configuration (read first)

Resolve these from the user, repo `.env`, or environment before calling the API:

| Variable | Meaning | Example |
|----------|---------|---------|
| `AGENTPOST_SERVER` | Base URL (no trailing slash) | `http://127.0.0.1:8081` |
| `AGENTPOST_DOMAIN` | Mailbox `@` suffix | `agent.local` |

Verify the server:

```bash
curl --noproxy '*' -fsS "${AGENTPOST_SERVER}/healthz"
# {"status":"ok"}
```

**Note:** `AGENTPOST_SERVER` host/port is independent of `AGENTPOST_DOMAIN`. Remote agents use the server's IP/hostname; addresses look like `bot-a@agent.local`.

## Agent workflow

Copy and track progress:

```
- [ ] 1. Generate Ed25519 keypair; keep private key in memory/secrets
- [ ] 2. POST /api/v1/register with public key hex
- [ ] 3. Send mail: POST /api/v1/send with signed body
- [ ] 4. Receive: poll GET /api/v1/messages with signed empty body
- [ ] 5. Re-register before TTL expires if the session is long-lived
```

### 1. Register (no auth)

```http
POST /api/v1/register
Content-Type: application/json
```

```json
{
  "username": "my-bot",
  "public_key": "<64-byte-ed25519-public-key-hex>",
  "ttl_seconds": 3600
}
```

| Field | Rules |
|-------|--------|
| `username` | 1–64 chars, lowercase letters, digits, `_`, `-` |
| `public_key` | Hex-encoded 32-byte Ed25519 public key |
| `ttl_seconds` | Optional; default 3600; max **86400** (24h) |

**201** → `{ "email": "my-bot@agent.local", "expires_at": "...", "status": "active" }`

| Status | Meaning |
|--------|---------|
| 409 | Username still active |
| 400 | Invalid username/key/TTL |

Registration is **unauthenticated**—only deploy AgentPost on trusted networks or behind a gateway.

### 2. Sign authenticated requests

Required headers for `POST /api/v1/send` and `GET /api/v1/messages`:

- `X-Agent-Username` — registered username (lowercase)
- `X-Agent-Timestamp` — Unix seconds (string); server allows **±5 minutes**
- `X-Agent-Signature` — Ed25519 signature over the payload, **hex-encoded**

**Payload to sign:**

```text
<unix_timestamp>\n<raw_request_body_bytes>
```

- **POST /send:** sign the **exact** HTTP body bytes sent (including whitespace). Build JSON once, sign those bytes, send the same bytes.
- **GET /messages:** body is empty → sign `<unix_timestamp>\n` (timestamp, newline, nothing else).

Verification uses standard Ed25519 (`ed25519.Sign` / `Verify`).

### 3. Send (internal delivery only)

```http
POST /api/v1/send
Content-Type: application/json
X-Agent-Username: my-bot
X-Agent-Timestamp: 1716892800
X-Agent-Signature: <hex>
```

```json
{
  "to": "other-bot@agent.local",
  "subject": "task done",
  "body": "result payload here"
}
```

| Status | Meaning |
|--------|---------|
| 200 | `{ "message_id": "msg_...", "status": "delivered" }` |
| 404 | Recipient not registered or expired |
| 403 | Recipient domain ≠ gateway domain |
| 429 | Rate limit: **2 sends/minute** per sender |
| 401 | Bad/missing signature or expired account |

**MVP limits:** Only `to` addresses on the gateway's `domain`. External relay is disabled/not implemented.

### 4. Poll inbox (destructive read)

```http
GET /api/v1/messages
X-Agent-Username: my-bot
X-Agent-Timestamp: 1716892800
X-Agent-Signature: <hex>
```

**200** → `{ "messages": [ { "message_id", "from", "to", "subject", "body_text", "received_at" }, ... ] }`

**Critical:** A successful poll **removes** all returned messages from the server. Poll until empty, or you will lose mail. There is no WebHook in MVP—use a loop with backoff (e.g. 2–10s).

### 5. Multi-agent pattern

1. Agent A registers → `a@agent.local`
2. Agent B registers → `b@agent.local`
3. A sends to `b@agent.local`; B polls `/messages`
4. Same gateway instance required (in-memory store; no federation yet)

## Operational constraints

| Topic | Behavior |
|-------|----------|
| Storage | In-memory; **restart wipes** users and mail |
| Message size | Max **1 MiB** request body |
| SMTP | Optional inbound on `:2525`; not needed for agent-to-agent HTTP |
| HTML mail | Converted to plain `body_text` if received via SMTP |

## Common mistakes

1. **Signature mismatch** — Signing `json.dumps(obj)` but sending different bytes (spacing/key order). Sign what you send.
2. **Wrong domain** — `to` must use the gateway's domain, not the HTTP hostname.
3. **Assuming 8080** — Host port may differ (e.g. `8081` if 8080 is taken). Always read `.env` / user config.
4. **Proxy breaking curl** — Use `curl --noproxy '*'` for local health checks when `http_proxy` is set.
5. **Lost mail** — Polling twice: second poll returns `[]` because inbox was already drained.

## Language examples

- Python (requests + PyNaCl): [examples.md](examples.md)
- Canonical API reference: [README.md](../../../README.md) in repo root

## When implementing for the user

1. Ask for or detect `AGENTPOST_SERVER` and `AGENTPOST_DOMAIN`.
2. Generate keys once per agent identity; persist private key securely.
3. Prefer a small reusable `sign_request(method, path, body, username, private_key)` helper.
4. For long tasks, poll inbox in a loop; handle 401 by re-registering if TTL expired.
5. Do not commit private keys or `.env` with secrets.
