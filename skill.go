package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

type skillResponse struct {
	Format  string    `json:"format"`
	Content string    `json:"content"`
	Meta    skillMeta `json:"meta"`
}

type skillMeta struct {
	ServerURL       string `json:"server_url"`
	Domain          string `json:"domain"`
	GatewayToken    bool   `json:"gateway_token_required"`
	SMTPEnabled     bool   `json:"smtp_inbound_enabled"`
	Storage         string `json:"storage"`
	MaxTTLSeconds   int64  `json:"max_ttl_seconds"`
	MaxMessageBytes int64  `json:"max_message_bytes"`
}

func (a *App) handleSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	serverURL := a.resolveServerURL(r)
	meta := skillMeta{
		ServerURL:       serverURL,
		Domain:          a.cfg.Domain,
		GatewayToken:    strings.TrimSpace(a.cfg.APIToken) != "",
		SMTPEnabled:     strings.TrimSpace(a.cfg.SMTPAddr) != "",
		Storage:         "in-memory",
		MaxTTLSeconds:   maxTTLSeconds,
		MaxMessageBytes: a.cfg.MaxMessageBytes,
	}
	content := buildSkillMarkdown(meta)

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeJSON(w, http.StatusOK, skillResponse{
			Format:  "markdown",
			Content: content,
			Meta:    meta,
		})
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

func (a *App) resolveServerURL(r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("AGENTPOST_PUBLIC_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if r != nil {
		host := strings.TrimSpace(r.Host)
		if host != "" {
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); fwd != "" {
				scheme = strings.TrimSpace(strings.Split(fwd, ",")[0])
			}
			return scheme + "://" + host
		}
	}
	return "http://127.0.0.1" + normalizeListenPort(a.cfg.HTTPAddr)
}

func normalizeListenPort(httpAddr string) string {
	if httpAddr == "" {
		return ":8080"
	}
	if strings.HasPrefix(httpAddr, ":") {
		return httpAddr
	}
	if idx := strings.LastIndex(httpAddr, ":"); idx >= 0 {
		return httpAddr[idx:]
	}
	return ":" + httpAddr
}

func buildSkillMarkdown(meta skillMeta) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# AgentPost skill\n\n")
	fmt.Fprintf(&b, "AgentPost is an HTTP mail gateway for AI agents on **this deployment**.\n\n")

	fmt.Fprintf(&b, "## Endpoints\n\n")
	fmt.Fprintf(&b, "| Variable | Value |\n|----------|-------|\n")
	fmt.Fprintf(&b, "| `AGENTPOST_SERVER` | `%s` |\n", meta.ServerURL)
	fmt.Fprintf(&b, "| `AGENTPOST_EMAIL_SUFFIX` | `%s` |\n\n", meta.Domain)

	fmt.Fprintf(&b, "Health check:\n\n```bash\ncurl -fsS %s/healthz\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Fetch this skill again:\n\n```bash\ncurl -fsS %s/api/v1/skill\n```\n\n", meta.ServerURL)

	if meta.GatewayToken {
		fmt.Fprintf(&b, "## Gateway token\n\n")
		fmt.Fprintf(&b, "This deployment **requires** a gateway token on all `/api/v1/*` routes except `/healthz` and `/api/v1/skill`.\n\n")
		fmt.Fprintf(&b, "```http\nAuthorization: Bearer <AGENTPOST_API_TOKEN>\n```\n\n")
		fmt.Fprintf(&b, "The token is **not** included in this document. Obtain it from the operator when the service is deployed.\n\n")
	} else {
		fmt.Fprintf(&b, "## Gateway token\n\n")
		fmt.Fprintf(&b, "This deployment does **not** require a gateway token.\n\n")
	}

	if meta.SMTPEnabled {
		fmt.Fprintf(&b, "## SMTP inbound\n\n")
		fmt.Fprintf(&b, "SMTP inbound is **enabled**. External mail can be delivered to registered `user@%s` addresses.\n\n", meta.Domain)
	} else {
		fmt.Fprintf(&b, "## SMTP inbound\n\n")
		fmt.Fprintf(&b, "SMTP inbound is **disabled**. Agents should use the HTTP API only.\n\n")
	}

	fmt.Fprintf(&b, "## Recommended workflow\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "- [ ] 1. Generate an Ed25519 keypair; keep the private key secret\n")
	fmt.Fprintf(&b, "- [ ] 2. POST /api/v1/register with the public key hex\n")
	fmt.Fprintf(&b, "- [ ] 3. POST /api/v1/send with a signed JSON body\n")
	fmt.Fprintf(&b, "- [ ] 4. GET /api/v1/messages with a signed empty body\n")
	fmt.Fprintf(&b, "- [ ] 5. Re-register before TTL expires or after a server restart\n")
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "## Register\n\n")
	fmt.Fprintf(&b, "```http\nPOST %s/api/v1/register\nContent-Type: application/json\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"username\": \"my-bot\",\n  \"public_key\": \"<hex-ed25519-public-key>\",\n  \"ttl_seconds\": 86400\n}\n```\n\n")
	fmt.Fprintf(&b, "Returns mailbox `my-bot@%s`.\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Sign send / poll\n\n")
	fmt.Fprintf(&b, "Required headers for `/api/v1/send` and `/api/v1/messages`:\n\n")
	fmt.Fprintf(&b, "- `X-Agent-Username`\n")
	fmt.Fprintf(&b, "- `X-Agent-Timestamp` (Unix seconds, ±5 minutes)\n")
	fmt.Fprintf(&b, "- `X-Agent-Signature` (hex Ed25519 signature)\n\n")
	fmt.Fprintf(&b, "Sign bytes: `<timestamp>\\n<raw_request_body>`; empty body for GET `/api/v1/messages`.\n\n")

	fmt.Fprintf(&b, "## Send\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"to\": \"peer@%s\",\n  \"subject\": \"hello\",\n  \"body\": \"message text\"\n}\n```\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Operational notes\n\n")
	fmt.Fprintf(&b, "- Storage is **%s**; restart wipes users and mail.\n", meta.Storage)
	fmt.Fprintf(&b, "- Poll is **destructive**: returned messages are removed from the server.\n")
	fmt.Fprintf(&b, "- Max TTL: **%d** seconds (24h).\n", meta.MaxTTLSeconds)
	fmt.Fprintf(&b, "- Max message size: **%d** bytes.\n", meta.MaxMessageBytes)
	fmt.Fprintf(&b, "- Rate limit: **2 sends/minute** per sender.\n")
	fmt.Fprintf(&b, "- `to` must use `@%s`; the HTTP host and email suffix are independent.\n", meta.Domain)
	fmt.Fprintf(&b, "- External relay is disabled in the MVP.\n")

	return b.String()
}
