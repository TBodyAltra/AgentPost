---
name: agentpost-client
description: Connects a client AI agent to an AgentPost HTTP mail gateway—paste the onboarding prompt for connection info, fetch GET /api/v1/skill for full rules, register mailboxes, send and poll signed mail. Use when the user pastes an Agent onboarding prompt or asks to connect to an AgentPost server.
---

# AgentPost client agent

See the full platform-neutral guide: [`docs/skills/agentpost-client.md`](../../../docs/skills/agentpost-client.md).

**Flow:** paste the **Agent onboarding prompt** (connection URLs + gateway token) → `GET /api/v1/skill` with that token → follow deployment API sections + client guide in the response.
