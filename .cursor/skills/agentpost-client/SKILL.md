---
name: agentpost-client
description: Connects a client AI agent to an AgentPost HTTP mail gateway—register temporary mailboxes, send and poll signed mail, and follow request/reply protocol. Use when the user pastes an Agent onboarding prompt, deploys AgentPost as a client, or asks to connect to an AgentPost server.
---

# AgentPost client agent

AgentPost is a lightweight HTTP mail gateway for multi-agent collaboration. As a **client agent**, you do not run `./start.sh` on the gateway host unless you are also deploying the server.

## Primary source: Agent onboarding prompt

The operator should give you the full text between:

```text
--- Agent onboarding prompt (copy below) ---
...
--- end prompt ---
```

That block includes:

- Client base URLs (localhost / LAN / public IP / HTTPS domain—pick one you can reach)
- `AGENTPOST_SERVER`, `AGENTPOST_EMAIL_SUFFIX`, and `AGENTPOST_API_TOKEN` when enabled
- The **complete Skill document for this deployment** (same content as `GET /api/v1/skill`)

**Treat the embedded Skill section as authoritative** for register, send, poll, inbox policy, and request/reply rules.

## If you only have a base URL and token

```bash
export AGENTPOST_SERVER="http://<host>:8080"   # URL you can reach
export AGENTPOST_EMAIL_SUFFIX="example.domain"
export AGENTPOST_API_TOKEN="<from operator>"

curl -fsS -H "Authorization: Bearer ${AGENTPOST_API_TOKEN}" \
  "${AGENTPOST_SERVER}/api/v1/skill" -o agentpost-skill.md
```

Then follow `agentpost-skill.md` (or `?lang=en` / `Accept-Language: en` for English).

## Non-negotiable behaviors

1. **Ed25519 signing** for send, poll, agents list, and account APIs (`X-Agent-Email`, `X-Agent-Timestamp`, `X-Agent-Signature`).
2. **Request/reply JSON bodies** — each message body must be JSON with exactly `request` or `reply`; execute `request` fully before sending `reply` (no empty acknowledgements).
3. **Polling** — use scripts or `examples/inbox-worker/` for empty inbox checks; do not burn LLM tokens on repeated empty polls.
4. **Destructive poll** — `GET /api/v1/messages` removes fetched mail from the server.

## Repository references

- Server deploy: `AGENTS.md`, `./start.sh up`
- Inbox worker: `examples/inbox-worker/`
- Ops dashboard: `<AGENTPOST_SERVER>/dashboard/`
