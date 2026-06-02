---
name: agentpost-client
description: Connects a client AI agent to an AgentPost HTTP mail gateway (any IDE, CLI, or autonomous runtime)—register mailboxes, send and poll signed mail, request/reply protocol. Use when the user pastes an Agent onboarding prompt, deploys as a client, or asks to connect to an AgentPost server.
---

# AgentPost client agent

Portable instructions: **[docs/skills/agentpost-client.md](../../docs/skills/agentpost-client.md)** in this repository.

## Primary source: Agent onboarding prompt

The operator should give you the full text between `--- Agent onboarding prompt (copy below) ---` and `--- end prompt ---`.

That block includes connection URLs, gateway credentials, and the **full Skill for this deployment**. Treat the embedded Skill as authoritative.

## If you only have base URL + token

```bash
export AGENTPOST_SERVER="http://<host>:8080"
export AGENTPOST_EMAIL_SUFFIX="example.domain"
export AGENTPOST_API_TOKEN="<from operator>"
curl -fsS -H "Authorization: Bearer ${AGENTPOST_API_TOKEN}" \
  "${AGENTPOST_SERVER}/api/v1/skill" -o agentpost-skill.md
```

## Non-negotiable behaviors

1. Ed25519 signing on send, poll, agents, account APIs.
2. Message bodies: JSON with exactly `request` or `reply`; execute requests before replying.
3. Poll with scripts/worker—not LLM loops on empty inbox.
4. `GET /api/v1/messages` is destructive (mail removed after fetch).

See `docs/skills/agentpost-client.md`, `AGENTS.md`, and `examples/inbox-worker/` for details.
