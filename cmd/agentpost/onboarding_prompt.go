package main

import (
	"fmt"
	"os"
	"strings"
)

// buildAgentOnboardingPrompt returns the same text ./start.sh prints after deployment.
// It embeds the full deployment Skill document (same as GET /api/v1/skill).
func buildAgentOnboardingPrompt(cfg Config, urls skillConnectionURLs, skillExample string) string {
	skillExample = strings.TrimRight(strings.TrimSpace(skillExample), "/")
	if skillExample == "" {
		skillExample = "http://127.0.0.1:8080"
	}

	meta := skillMetaForOnboarding(cfg, urls, skillExample)
	lang := onboardingPromptLanguage()
	skillDoc := buildSkillMarkdown(meta, lang)

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

	b.WriteString("2. Skill document (authoritative API reference for this deployment).\n")
	b.WriteString("   The full Skill is included below. To refresh later from your chosen base URL:\n\n")
	if token != "" {
		fmt.Fprintf(&b, "   curl -fsS -H \"Authorization: Bearer %s\" %s/api/v1/skill\n", token, skillExample)
	} else {
		fmt.Fprintf(&b, "   curl -fsS %s/api/v1/skill\n", skillExample)
	}
	if lang == "en" {
		fmt.Fprintf(&b, "   (English skill: append `?lang=en` or send `Accept-Language: en`.)\n")
	} else {
		fmt.Fprintf(&b, "   (英文 Skill：加 `?lang=en` 或请求头 `Accept-Language: en`。)\n")
	}

	b.WriteString("\n--- AgentPost Skill (this deployment) ---\n\n")
	b.WriteString(skillDoc)
	if !strings.HasSuffix(skillDoc, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n--- end skill ---\n\n")

	b.WriteString("3. Quick checklist:\n")
	b.WriteString("   - Register with POST /api/v1/register; poll with GET /api/v1/messages (signed).\n")
	b.WriteString("   - Message bodies: JSON with exactly \"request\" or \"reply\"; execute requests before replying.\n")
	b.WriteString("   - Poll with scripts/worker (not LLM on empty inbox); see Skill above for details.\n")
	b.WriteString("   - Paste this entire prompt into your client agent (IDE rules, AGENTS.md, system prompt, etc.).\n")
	b.WriteString("   - Optional repo guide: docs/skills/agentpost-client.md\n")
	b.WriteString("   - Operator dashboard: <base-url>/dashboard/\n\n")
	b.WriteString("--- end prompt ---\n\n")
	return b.String()
}

func onboardingPromptLanguage() string {
	if lang := normalizeSkillLanguage(os.Getenv("AGENTPOST_ONBOARDING_LANG")); lang != "" {
		return lang
	}
	return "zh"
}

func skillMetaForOnboarding(cfg Config, urls skillConnectionURLs, serverURL string) skillMeta {
	urlSource := "deployment_env"
	if strings.TrimSpace(os.Getenv("AGENTPOST_PUBLIC_URL")) == "" && !urls.hasAny() {
		urlSource = "default"
	}
	return skillMeta{
		ServerURL:       serverURL,
		Domain:          cfg.Domain,
		ConnectionURLs:  urls,
		PublicURLSource: urlSource,
		Language:        onboardingPromptLanguage(),
		GatewayToken:    strings.TrimSpace(cfg.APIToken) != "",
		SMTPEnabled:     strings.TrimSpace(cfg.SMTPAddr) != "",
		Storage:         storageDescription(cfg.DataDir),
		MaxTTLSeconds:   maxTTLSeconds,
		MaxMessageBytes: cfg.MaxMessageBytes,
	}
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
