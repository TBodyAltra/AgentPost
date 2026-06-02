package main

import (
	"fmt"
	"strings"
)

// buildAgentOnboardingPrompt returns the same text ./start.sh prints after deployment.
// It lists connection URLs and credentials only; the full Skill is fetched via GET /api/v1/skill.
func buildAgentOnboardingPrompt(cfg Config, urls skillConnectionURLs, skillExample string) string {
	skillExample = strings.TrimRight(strings.TrimSpace(skillExample), "/")
	if skillExample == "" {
		skillExample = "http://127.0.0.1:8080"
	}

	var b strings.Builder
	b.WriteString("\n--- Agent onboarding prompt (copy below) ---\n\n")
	b.WriteString("You are connecting to an AgentPost mail gateway on this deployment.\n")
	b.WriteString("This prompt gives connection info only. Fetch the Skill document for API rules and client behavior.\n\n")
	appendOnboardingConnectionURLs(&b, urls)

	token := strings.TrimSpace(cfg.APIToken)
	b.WriteString("\n1. Gateway credentials (every /api/v1/* call; only /healthz is public):\n")
	b.WriteString("   AGENTPOST_SERVER=<base URL from above>\n")
	fmt.Fprintf(&b, "   AGENTPOST_EMAIL_SUFFIX=%s\n", cfg.Domain)
	if token != "" {
		fmt.Fprintf(&b, "   AGENTPOST_API_TOKEN=%s\n\n", token)
		fmt.Fprintf(&b, "   Header: Authorization: Bearer %s\n", token)
	} else {
		b.WriteString("\n")
	}

	b.WriteString("\n2. Fetch the Skill document (deployment API + client agent guide):\n")
	b.WriteString("   Use the base URL your client can reach (see above).\n\n")
	if token != "" {
		fmt.Fprintf(&b, "   curl -fsS -H \"Authorization: Bearer %s\" %s/api/v1/skill\n", token, skillExample)
	} else {
		fmt.Fprintf(&b, "   curl -fsS %s/api/v1/skill\n", skillExample)
	}
	b.WriteString("   English deployment sections: append `?lang=en` or send `Accept-Language: en`.\n\n")
	b.WriteString("   Follow the Skill for register, send, poll, inbox policy, request/reply protocol, and polling patterns.\n")
	b.WriteString("   Paste this onboarding prompt into your client agent (IDE rules, AGENTS.md, system prompt, etc.).\n\n")
	b.WriteString("--- end prompt ---\n\n")
	return b.String()
}

func appendOnboardingConnectionURLs(b *strings.Builder, urls skillConnectionURLs) {
	b.WriteString("Client base URLs (each agent uses the one it can reach):\n")
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
	b.WriteString("\nSet AGENTPOST_SERVER to the base URL your client can actually reach (do not substitute another host).\n")
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
