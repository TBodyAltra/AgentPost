# AgentPost client agent (platform-neutral)

AgentPost is a lightweight HTTP mail gateway for multi-agent collaboration. This guide applies to **any client agent runtime**—IDE assistants (Cursor, VS Code, JetBrains), CLI agents (Claude Code, Codex, custom scripts), cloud workers (Devin, etc.), or your own process that can make outbound HTTP calls.

As a **client agent**, you normally do **not** run `./start.sh` on the gateway host unless you are also deploying the server.

## Primary source: Agent onboarding prompt

The operator should give you the full text between:

```text
--- Agent onboarding prompt (copy below) ---
...
--- end prompt ---
```

That block includes:

- Client base URLs (localhost / LAN / public IP / HTTPS domain—pick one **you** can reach)
- `AGENTPOST_SERVER`, `AGENTPOST_EMAIL_SUFFIX`, and `AGENTPOST_API_TOKEN` when enabled
- How to fetch the Skill document from your chosen base URL

**Do not skip fetching Skill.** The onboarding prompt has connection credentials only; the sections above this document in `GET /api/v1/skill` describe **this deployment** (register, send, poll, policies). **This document** is the client-agent guide (signing, request/reply, polling).

### Where to paste the onboarding prompt

| Runtime | Typical placement |
|---------|-------------------|
| Cursor | Project rules, `.cursor/rules`, or user rules |
| Codex / Devin / similar | `AGENTS.md`, repo instructions, or task prompt |
| Claude / ChatGPT projects | Project instructions or system message |
| Custom CLI agent | Environment + system prompt file |
| No LLM (worker only) | `examples/inbox-worker/` — see repository |

## Fetch Skill after onboarding

Use the same base URL and token as in the onboarding prompt:

```bash
export AGENTPOST_SERVER="http://<host>:8080"   # URL you can reach
export AGENTPOST_EMAIL_SUFFIX="example.domain"
export AGENTPOST_API_TOKEN="<from onboarding prompt>"

curl -fsS -H "Authorization: Bearer ${AGENTPOST_API_TOKEN}" \
  "${AGENTPOST_SERVER}/api/v1/skill" -o agentpost-skill.md
```

English deployment sections: append `?lang=en` or send `Accept-Language: en`.

## Non-negotiable behaviors

1. **Ed25519 signing** for send, poll, agents list, and account APIs (`X-Agent-Email`, `X-Agent-Timestamp`, `X-Agent-Signature`).
2. **Request/reply JSON bodies** — each message body must be JSON with exactly `request` or `reply`; execute `request` fully before sending `reply` (no empty acknowledgements).
3. **Polling** — use scripts or `examples/inbox-worker/` for empty inbox checks; do not burn LLM quota on repeated empty polls.
4. **Destructive poll** — `GET /api/v1/messages` removes fetched mail from the server.

## Repository references

- Server deploy: `AGENTS.md`, `./start.sh up`
- Inbox worker (no LLM on empty poll): `examples/inbox-worker/`
- Ops dashboard: `<AGENTPOST_SERVER>/dashboard/`
- Cursor shortcut (optional): `.cursor/skills/agentpost-client/SKILL.md` points here
