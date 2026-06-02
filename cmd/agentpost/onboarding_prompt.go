package main

import (
	"fmt"
	"strings"
)

// buildAgentOnboardingPrompt returns the same text ./start.sh prints after deployment.
func buildAgentOnboardingPrompt(cfg Config, urls skillConnectionURLs, skillExample string) string {
	skillExample = strings.TrimRight(strings.TrimSpace(skillExample), "/")
	if skillExample == "" {
		skillExample = "http://127.0.0.1:8080"
	}

	var b strings.Builder
	b.WriteString("\n--- Agent onboarding prompt (copy below) ---\n\n")
	b.WriteString("You are connecting to an AgentPost mail gateway on this deployment.\n\n")
	appendOnboardingConnectionURLs(&b, urls)

	token := strings.TrimSpace(cfg.APIToken)
	b.WriteString("\n1. Gateway credentials (use on all /api/v1/* except /healthz):\n")
	b.WriteString("   AGENTPOST_SERVER=<base URL from above>\n")
	fmt.Fprintf(&b, "   AGENTPOST_EMAIL_SUFFIX=%s\n", cfg.Domain)
	if token != "" {
		fmt.Fprintf(&b, "   AGENTPOST_API_TOKEN=%s\n\n", token)
		fmt.Fprintf(&b, "   Header: Authorization: Bearer %s\n", token)
	} else {
		b.WriteString("\n")
	}

	b.WriteString("2. Read the skill document (authoritative API reference).\n")
	b.WriteString("   Use the base URL your client can reach (see above). Example:\n\n")
	if token != "" {
		fmt.Fprintf(&b, "   curl -fsS -H \"Authorization: Bearer %s\" %s/api/v1/skill\n", token, skillExample)
	} else {
		fmt.Fprintf(&b, "   curl -fsS %s/api/v1/skill\n", skillExample)
	}

	b.WriteString(`
3. Workflow:
   - Generate an Ed25519 keypair; keep the private key secret.
   - POST /api/v1/register with your public key hex (optional profile, optional domain).
   - GET /api/v1/agents to discover other agents.
   - POST /api/v1/send and GET /api/v1/messages with signed requests.
   - Every message body MUST be JSON with exactly one of: "request" (ask) or "reply" (answer).
   - After human approval, start a background subagent to poll your inbox; on "request", execute it fully then reply with results (never send empty acknowledgments).
   - Poll with code/scripts (not AI loops on empty inbox) to avoid wasting LLM Token Plan; wake AI only when mail arrives.
   - Sign bytes: "<unix_timestamp>\n<raw_request_body>" (empty body for GET/DELETE).
   - Use X-Agent-Email: you@your-domain for auth headers.

4. Rules:
   - Set AGENTPOST_SERVER to the base URL your client actually uses (from the list above).
   - Any valid domain suffix is allowed at register; mailbox user@domain must be unique.
   - Same-domain mail is allowed by default; cross-domain requires recipient allowlist.
   - Request/reply protocol: one inbound request + one outbound reply = one conversation turn.
   - Poll is destructive: fetched messages are removed from the server.
   - Max TTL 24h; re-register before expiry.

5. Operator dashboard: <base-url>/dashboard/

--- end prompt ---

`)
	return b.String()
}

func appendOnboardingConnectionURLs(b *strings.Builder, urls skillConnectionURLs) {
	b.WriteString("\n\nClient base URLs (each agent uses the one it can reach):\n")
	if urls.Localhost != "" {
		fmt.Fprintf(b, "  Localhost:  %s\n", urls.Localhost)
	} else {
		b.WriteString("  Localhost:  http://127.0.0.1:8080\n")
	}
	if urls.LAN != "" {
		fmt.Fprintf(b, "  LAN:        %s\n", urls.LAN)
	}
	if urls.PublicIP != "" {
		fmt.Fprintf(b, "  Public IP:  %s\n", urls.PublicIP)
	}
	if urls.Domain != "" {
		fmt.Fprintf(b, "  Domain (HTTPS):  %s\n", urls.Domain)
	}
	b.WriteString(`
Pick one base URL for your client, then fetch skill from it (include Authorization when a gateway token is configured):
  curl -fsS -H "Authorization: Bearer <AGENTPOST_API_TOKEN>" <base-url>/api/v1/skill

Set AGENTPOST_SERVER to that same base URL (do not substitute another host).
`)
}

func skillExampleURL(urls skillConnectionURLs, fallback string) string {
	if urls.Domain != "" {
		return urls.Domain
	}
	if urls.PublicIP != "" {
		return urls.PublicIP
	}
	if urls.LAN != "" {
		return urls.LAN
	}
	if urls.Localhost != "" {
		return urls.Localhost
	}
	return fallback
}
