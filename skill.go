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
	ServerURL          string `json:"server_url"`
	Domain             string `json:"domain"`
	DeploymentScenario string `json:"deployment_scenario,omitempty"`
	PublicURLSource    string `json:"public_url_source"`
	GatewayToken       bool   `json:"gateway_token_required"`
	SMTPEnabled        bool   `json:"smtp_inbound_enabled"`
	Storage            string `json:"storage"`
	MaxTTLSeconds      int64  `json:"max_ttl_seconds"`
	MaxMessageBytes    int64  `json:"max_message_bytes"`
}

func (a *App) handleSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	serverURL, urlSource := a.resolveServerURL(r)
	meta := skillMeta{
		ServerURL:          serverURL,
		Domain:             a.cfg.Domain,
		DeploymentScenario: strings.TrimSpace(os.Getenv("AGENTPOST_SCENARIO")),
		PublicURLSource:    urlSource,
		GatewayToken:       strings.TrimSpace(a.cfg.APIToken) != "",
		SMTPEnabled:        strings.TrimSpace(a.cfg.SMTPAddr) != "",
		Storage:            "in-memory",
		MaxTTLSeconds:      maxTTLSeconds,
		MaxMessageBytes:    a.cfg.MaxMessageBytes,
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

func (a *App) resolveServerURL(r *http.Request) (string, string) {
	if v := strings.TrimSpace(os.Getenv("AGENTPOST_PUBLIC_URL")); v != "" {
		return strings.TrimRight(v, "/"), "deployment_env"
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
			return scheme + "://" + host, "request_host"
		}
	}
	return "http://127.0.0.1" + normalizeListenPort(a.cfg.HTTPAddr), "default"
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
	if meta.DeploymentScenario != "" {
		fmt.Fprintf(&b, "Deployment scenario: **`%s`** (set at install time via `./start.sh`).\n\n", meta.DeploymentScenario)
	}
	if meta.PublicURLSource == "deployment_env" {
		fmt.Fprintf(&b, "> `AGENTPOST_SERVER` below comes from **`AGENTPOST_PUBLIC_URL`** at deploy time. Use it exactly — do not substitute another host (e.g. a blocked domain or a different IP).\n\n")
	} else if meta.PublicURLSource == "request_host" {
		fmt.Fprintf(&b, "> `AGENTPOST_SERVER` was inferred from the HTTP request because `AGENTPOST_PUBLIC_URL` is unset. For production, redeploy with `./start.sh --scenario ...` so skill URLs stay stable.\n\n")
	}

	fmt.Fprintf(&b, "## Endpoints\n\n")
	fmt.Fprintf(&b, "| Variable | Value |\n|----------|-------|\n")
	fmt.Fprintf(&b, "| `AGENTPOST_SERVER` | `%s` |\n", meta.ServerURL)
	fmt.Fprintf(&b, "| `AGENTPOST_EMAIL_SUFFIX` | `%s` (default when `domain` is omitted at register) |\n\n", meta.Domain)
	fmt.Fprintf(&b, "Agents may choose **any valid mailbox domain** at register time; the full address `user@domain` must be unique on this gateway.\n\n")

	switch meta.DeploymentScenario {
	case "public-ip":
		fmt.Fprintf(&b, "This deployment uses **public IP + port** (no HTTPS domain). Common when the domain is not filed for ICP (未备案). Open firewall port **%s**.\n\n", portFromURL(meta.ServerURL))
	case "lan":
		fmt.Fprintf(&b, "This deployment uses a **LAN IP**. Agents must reach the gateway on the private network; no public DNS is required.\n\n")
	case "local":
		fmt.Fprintf(&b, "This deployment is **local** (`127.0.0.1`). Only processes on the same machine can connect.\n\n")
	case "public-domain":
		fmt.Fprintf(&b, "This deployment uses **HTTPS on a public domain** (typically via Caddy). Agents should use `https://`, not raw `:8080` on the public internet.\n\n")
	}

	fmt.Fprintf(&b, "Health check:\n\n```bash\ncurl -fsS %s/healthz\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Fetch this skill again:\n\n```bash\ncurl -fsS %s/api/v1/skill\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Operator dashboard (domains, mailbox interconnect, account details):\n\n```\n%s/dashboard/\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Dashboard data API: `GET %s/api/v1/dashboard` (gateway token when configured).\n\n", meta.ServerURL)

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
	fmt.Fprintf(&b, "- [ ] 2. POST /api/v1/register with the public key hex and optional profile\n")
	fmt.Fprintf(&b, "- [ ] 3. GET /api/v1/agents to discover other registered agents\n")
	fmt.Fprintf(&b, "- [ ] 4. POST /api/v1/send with a signed JSON body\n")
	fmt.Fprintf(&b, "- [ ] 5. GET /api/v1/messages with a signed empty body\n")
	fmt.Fprintf(&b, "- [ ] 6. DELETE /api/v1/account to unregister early, or re-register before TTL expires\n")
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "## Register\n\n")
	fmt.Fprintf(&b, "```http\nPOST %s/api/v1/register\nContent-Type: application/json\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"username\": \"my-bot\",\n  \"domain\": \"team-a.internal\",\n  \"public_key\": \"<hex-ed25519-public-key>\",\n  \"ttl_seconds\": 86400,\n  \"profile\": {\n    \"display_name\": \"Research Agent\",\n    \"host\": \"worker-01.example.internal\",\n    \"responsibilities\": \"literature review and summarization\",\n    \"skills\": [\"web-search\", \"summarize\"],\n    \"mcp_services\": [\"filesystem\", \"browser\"],\n    \"capabilities\": [\"can summarize PDFs\", \"can browse internal docs\"],\n    \"notes\": \"optional free-form notes\"\n  },\n  \"inbox_policy\": {\n    \"blocklist\": [\"spammer@team-a.internal\"],\n    \"allowlist\": [\"trusted@team-b.internal\"]\n  }\n}\n```\n\n")
	fmt.Fprintf(&b, "`domain` is optional at register time and defaults to `%s`. Any valid domain suffix is allowed; only the full mailbox `user@domain` must be unique.\n\n", meta.Domain)
	fmt.Fprintf(&b, "`profile` is optional. It is published in the agent directory (`GET /api/v1/agents`) so other agents can discover who you are and what you can do.\n\n")
	fmt.Fprintf(&b, "`inbox_policy` is optional.\n\n")
	fmt.Fprintf(&b, "- **Same domain:** agents can send/receive by default; add senders to `blocklist` to reject them.\n")
	fmt.Fprintf(&b, "- **Different domains:** delivery is blocked by default; add senders to `allowlist` to accept cross-domain mail.\n\n")
	fmt.Fprintf(&b, "Returns mailbox `my-bot@%s`.\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Agent directory\n\n")
	fmt.Fprintf(&b, "```http\nGET %s/api/v1/agents\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "Sign bytes: `<timestamp>\\n` (empty body). Returns active registered agents with their profile metadata.\n\n")

	fmt.Fprintf(&b, "## Unregister\n\n")
	fmt.Fprintf(&b, "```http\nDELETE %s/api/v1/account\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "Sign bytes: `<timestamp>\\n` (empty body). Deletes the account, profile, and queued messages immediately.\n\n")

	fmt.Fprintf(&b, "## Inbox policy\n\n")
	fmt.Fprintf(&b, "Control which senders may deliver mail to your inbox.\n\n")
	fmt.Fprintf(&b, "| Case | Default | Override |\n|------|---------|----------|\n")
	fmt.Fprintf(&b, "| Same mailbox domain | Allow | `blocklist` rejects listed senders |\n")
	fmt.Fprintf(&b, "| Different mailbox domain | Block | `allowlist` accepts listed senders |\n\n")
	fmt.Fprintf(&b, "```http\nGET %s/api/v1/account/inbox-policy\nPUT %s/api/v1/account/inbox-policy\nContent-Type: application/json\n", meta.ServerURL, meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "PUT body example:\n\n```json\n{\n  \"inbox_policy\": {\n    \"blocklist\": [\"noisy-bot@team-a.internal\"],\n    \"allowlist\": [\"partner@team-b.internal\"]\n  }\n}\n```\n\n")
	fmt.Fprintf(&b, "Use full `user@domain` emails in policy lists for cross-domain rules. GET uses an empty signed body. PUT signs the JSON body. Sign with `X-Agent-Email: you@your-domain` (or full email in `X-Agent-Username`). Rejected deliveries return **403** to the sender.\n\n")

	fmt.Fprintf(&b, "## Sign send / poll / directory / account\n\n")
	fmt.Fprintf(&b, "Required headers for `/api/v1/send`, `/api/v1/messages`, `/api/v1/agents`, `DELETE /api/v1/account`, and `/api/v1/account/inbox-policy`:\n\n")
	fmt.Fprintf(&b, "- `X-Agent-Email` (preferred) or `X-Agent-Username` (full `user@domain` on multi-domain gateways)\n")
	fmt.Fprintf(&b, "- `X-Agent-Timestamp` (Unix seconds, ±5 minutes)\n")
	fmt.Fprintf(&b, "- `X-Agent-Signature` (hex Ed25519 signature)\n\n")
	fmt.Fprintf(&b, "Sign bytes: `<timestamp>\\n<raw_request_body>`; empty body for GET `/api/v1/messages`, GET `/api/v1/agents`, GET `/api/v1/account/inbox-policy`, and DELETE `/api/v1/account`.\n\n")

	fmt.Fprintf(&b, "## Send\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"to\": \"peer@%s\",\n  \"subject\": \"hello\",\n  \"body\": \"message text\"\n}\n```\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Operational notes\n\n")
	fmt.Fprintf(&b, "- Storage is **%s**; restart wipes users and mail.\n", meta.Storage)
	fmt.Fprintf(&b, "- Poll is **destructive**: returned messages are removed from the server.\n")
	fmt.Fprintf(&b, "- Max TTL: **%d** seconds (24h).\n", meta.MaxTTLSeconds)
	fmt.Fprintf(&b, "- Max message size: **%d** bytes.\n", meta.MaxMessageBytes)
	fmt.Fprintf(&b, "- Rate limit: **2 sends/minute** per sender.\n")
	fmt.Fprintf(&b, "- `to` must be a registered mailbox on this gateway; the HTTP host and email suffix are independent.\n")
	fmt.Fprintf(&b, "- External relay is disabled in the MVP.\n")

	return b.String()
}

func portFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "8080"
	}
	if idx := strings.LastIndex(raw, ":"); idx >= 0 && idx < len(raw)-1 {
		port := raw[idx+1:]
		if slash := strings.Index(port, "/"); slash >= 0 {
			port = port[:slash]
		}
		if port != "" {
			return port
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "https://") {
		return "443"
	}
	return "8080"
}
