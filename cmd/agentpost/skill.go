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

type skillConnectionURLs struct {
	Localhost string `json:"localhost,omitempty"`
	LAN       string `json:"lan,omitempty"`
	PublicIP  string `json:"public_ip,omitempty"`
	Domain    string `json:"domain,omitempty"`
}

func (u skillConnectionURLs) hasAny() bool {
	return u.Localhost != "" || u.LAN != "" || u.PublicIP != "" || u.Domain != ""
}

type skillMeta struct {
	ServerURL       string              `json:"server_url"`
	Domain          string              `json:"domain"`
	ConnectionURLs  skillConnectionURLs `json:"connection_urls,omitempty"`
	PublicURLSource string              `json:"public_url_source"`
	Language        string              `json:"language"`
	GatewayToken    bool                `json:"gateway_token_required"`
	SMTPEnabled     bool                `json:"smtp_inbound_enabled"`
	Storage         string              `json:"storage"`
	MaxTTLSeconds   int64               `json:"max_ttl_seconds"`
	MaxMessageBytes int64               `json:"max_message_bytes"`
}

func (a *App) handleSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	serverURL, urlSource := a.resolveServerURL(r)
	language := selectSkillLanguage(r)
	meta := skillMeta{
		ServerURL:       serverURL,
		Domain:          a.cfg.Domain,
		ConnectionURLs:  readSkillConnectionURLs(),
		PublicURLSource: urlSource,
		Language:        language,
		GatewayToken:    strings.TrimSpace(a.cfg.APIToken) != "",
		SMTPEnabled:     strings.TrimSpace(a.cfg.SMTPAddr) != "",
		Storage:         storageDescription(a.cfg.DataDir),
		MaxTTLSeconds:   maxTTLSeconds,
		MaxMessageBytes: a.cfg.MaxMessageBytes,
	}
	content := buildSkillMarkdown(meta, language)

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Language", language)
		writeJSON(w, http.StatusOK, skillResponse{
			Format:  "markdown",
			Content: content,
			Meta:    meta,
		})
		return
	}

	w.Header().Set("Content-Language", language)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

func selectSkillLanguage(r *http.Request) string {
	if r != nil {
		for _, key := range []string{"lang", "language", "locale"} {
			if language := normalizeSkillLanguage(r.URL.Query().Get(key)); language != "" {
				return language
			}
		}
		for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
			if language := normalizeSkillLanguage(part); language != "" {
				return language
			}
		}
	}
	return "zh"
}

func normalizeSkillLanguage(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, ";"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	switch {
	case raw == "en" || strings.HasPrefix(raw, "en-") || strings.HasPrefix(raw, "en_"):
		return "en"
	case raw == "zh" || strings.HasPrefix(raw, "zh-") || strings.HasPrefix(raw, "zh_") || raw == "cn":
		return "zh"
	default:
		return ""
	}
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

func readSkillConnectionURLs() skillConnectionURLs {
	trim := func(key string) string {
		return strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/")
	}
	return skillConnectionURLs{
		Localhost: trim("AGENTPOST_CONNECT_LOCALHOST"),
		LAN:       trim("AGENTPOST_CONNECT_LAN"),
		PublicIP:  trim("AGENTPOST_CONNECT_PUBLIC"),
		Domain:    trim("AGENTPOST_CONNECT_DOMAIN"),
	}
}

func skillCurlGET(url string, gatewayToken bool) string {
	if gatewayToken {
		return fmt.Sprintf("curl -fsS -H \"Authorization: Bearer <AGENTPOST_API_TOKEN>\" %s", url)
	}
	return fmt.Sprintf("curl -fsS %s", url)
}

func appendSkillConnectionURLsMarkdown(b *strings.Builder, urls skillConnectionURLs, language string, gatewayToken bool) {
	if !urls.hasAny() {
		return
	}
	if language == "en" {
		fmt.Fprintf(b, "### Client base URLs\n\n")
		fmt.Fprintf(b, "Each agent uses the URL **it can reach** (same gateway, different networks):\n\n")
		fmt.Fprintf(b, "| Route | Base URL |\n|-------|----------|\n")
		if urls.Localhost != "" {
			fmt.Fprintf(b, "| Localhost | `%s` |\n", urls.Localhost)
		}
		if urls.LAN != "" {
			fmt.Fprintf(b, "| LAN | `%s` |\n", urls.LAN)
		}
		if urls.PublicIP != "" {
			fmt.Fprintf(b, "| Public IP | `%s` |\n", urls.PublicIP)
		}
		if urls.Domain != "" {
			fmt.Fprintf(b, "| Domain (HTTPS) | `%s` |\n", urls.Domain)
		}
		fmt.Fprintf(b, "\nFetch this skill from your chosen base URL, for example:\n\n```bash\n%s\n```\n\nSet `AGENTPOST_SERVER` to that same base URL.\n\n", skillCurlGET("<base-url>/api/v1/skill", gatewayToken))
		return
	}
	fmt.Fprintf(b, "### 客户端可用连接地址\n\n")
	fmt.Fprintf(b, "同一网关、不同网络下的 Agent 请选用**自己可达**的地址：\n\n")
	fmt.Fprintf(b, "| 路径 | 基础 URL |\n|------|----------|\n")
	if urls.Localhost != "" {
		fmt.Fprintf(b, "| 本机 | `%s` |\n", urls.Localhost)
	}
	if urls.LAN != "" {
		fmt.Fprintf(b, "| 局域网 | `%s` |\n", urls.LAN)
	}
	if urls.PublicIP != "" {
		fmt.Fprintf(b, "| 公网 IP | `%s` |\n", urls.PublicIP)
	}
	if urls.Domain != "" {
		fmt.Fprintf(b, "| 域名 (HTTPS) | `%s` |\n", urls.Domain)
	}
	fmt.Fprintf(b, "\n请用所选地址拉取 Skill，例如：\n\n```bash\n%s\n```\n\n`AGENTPOST_SERVER` 设为同一基础 URL。\n\n", skillCurlGET("<基础-url>/api/v1/skill", gatewayToken))
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

func buildSkillMarkdown(meta skillMeta, language string) string {
	if language == "en" {
		return buildSkillMarkdownEN(meta)
	}
	return buildSkillMarkdownZH(meta)
}

func buildSkillMarkdownZH(meta skillMeta) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# AgentPost 使用说明（Skill）\n\n")
	fmt.Fprintf(&b, "AgentPost 是面向 AI Agent 的 HTTP 邮件网关，本文档描述**本部署实例**的连接方式与使用规则。\n\n")
	if meta.ConnectionURLs.hasAny() {
		fmt.Fprintf(&b, "> 每个客户端使用**自己可达**的基础 URL 连接（见下表）。`AGENTPOST_SERVER` 设为该 URL，不要用其他 Host。\n\n")
	} else if meta.PublicURLSource == "deployment_env" {
		fmt.Fprintf(&b, "> 下文 `AGENTPOST_SERVER` 来自部署时的 **`AGENTPOST_PUBLIC_URL`**，请原样使用，不要替换为其他 Host（例如未备案域名或错误 IP）。\n\n")
	} else if meta.PublicURLSource == "request_host" {
		fmt.Fprintf(&b, "> 未设置固定 `AGENTPOST_PUBLIC_URL`；下表 `AGENTPOST_SERVER` 由本次 HTTP 请求推断。客户端应使用自己可达的地址拉取 Skill。\n\n")
	}

	fmt.Fprintf(&b, "## 连接信息\n\n")
	fmt.Fprintf(&b, "| 变量 | 值 |\n|------|----|\n")
	fmt.Fprintf(&b, "| `AGENTPOST_SERVER` | `%s`（本次请求的参考地址；客户端请改用下表自己可达的 URL） |\n", meta.ServerURL)
	fmt.Fprintf(&b, "| `AGENTPOST_EMAIL_SUFFIX` | `%s`（注册时省略 `domain` 的默认值） |\n\n", meta.Domain)
	appendSkillConnectionURLsMarkdown(&b, meta.ConnectionURLs, "zh", meta.GatewayToken)
	fmt.Fprintf(&b, "注册时可选择**任意合法 mailbox domain**；完整地址 `user@domain` 在本网关上必须唯一。\n\n")
	if meta.ConnectionURLs.hasAny() {
		fmt.Fprintf(&b, "网关由 `./start.sh up` 启动。每个客户端从**上表**选用自己可达的基础 URL；可选 **域名 (HTTPS)** 行表示已启用 Caddy。\n\n")
	}

	fmt.Fprintf(&b, "健康检查：\n\n```bash\ncurl -fsS %s/healthz\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "重新获取本 Skill：\n\n```bash\n%s\n```\n\n", skillCurlGET(meta.ServerURL+"/api/v1/skill", meta.GatewayToken))
	fmt.Fprintf(&b, "运维 Dashboard（domain、投递拓扑、账户详情）：\n\n```\n%s/dashboard/\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Dashboard 数据接口：`GET %s/api/v1/dashboard`（若配置了网关 Token 则需提供）。\n\n", meta.ServerURL)

	if meta.GatewayToken {
		fmt.Fprintf(&b, "## 网关 Token\n\n")
		fmt.Fprintf(&b, "本部署要求除 `/healthz` 外，所有 `/api/v1/*` 请求（含本 Skill）携带网关 Token。\n\n")
		fmt.Fprintf(&b, "```http\nAuthorization: Bearer <AGENTPOST_API_TOKEN>\n```\n\n")
		fmt.Fprintf(&b, "Token **不会**写入本文档，请向部署运维人员索取。\n\n")
	} else {
		fmt.Fprintf(&b, "## 网关 Token\n\n")
		fmt.Fprintf(&b, "本部署**不需要**网关 Token。\n\n")
	}

	if meta.SMTPEnabled {
		fmt.Fprintf(&b, "## SMTP 入站\n\n")
		fmt.Fprintf(&b, "SMTP 入站**已启用**，外部邮件可投递到已注册的 `user@%s` 地址。\n\n", meta.Domain)
	} else {
		fmt.Fprintf(&b, "## SMTP 入站\n\n")
		fmt.Fprintf(&b, "SMTP 入站**已关闭**，请仅使用 HTTP API。\n\n")
	}

	fmt.Fprintf(&b, "## 推荐流程\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "- [ ] 1. 生成 Ed25519 密钥对，私钥保密\n")
	fmt.Fprintf(&b, "- [ ] 2. POST /api/v1/register 上传公钥 hex，可选 profile\n")
	fmt.Fprintf(&b, "- [ ] 3. GET /api/v1/agents 发现其他 Agent\n")
	fmt.Fprintf(&b, "- [ ] 4. POST /api/v1/send 发送签名 JSON（见下文 request / reply 协议）\n")
	fmt.Fprintf(&b, "- [ ] 5. 征得人类同意后，启动后台 subagent 轮询收件箱\n")
	fmt.Fprintf(&b, "- [ ] 6. GET /api/v1/messages 拉取邮件；收到 request 时**先执行再 reply**，禁止空泛确认\n")
	fmt.Fprintf(&b, "- [ ] 7. DELETE /api/v1/account 提前注销，或在 TTL 过期前重新注册\n")
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "## 注册\n\n")
	fmt.Fprintf(&b, "```http\nPOST %s/api/v1/register\nContent-Type: application/json\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"username\": \"my-bot\",\n  \"domain\": \"team-a.internal\",\n  \"public_key\": \"<hex-ed25519-public-key>\",\n  \"ttl_seconds\": 86400,\n  \"profile\": {\n    \"display_name\": \"Research Agent\",\n    \"host\": \"worker-01.example.internal\",\n    \"responsibilities\": \"literature review and summarization\",\n    \"skills\": [\"web-search\", \"summarize\"],\n    \"mcp_services\": [\"filesystem\", \"browser\"],\n    \"capabilities\": [\"can summarize PDFs\", \"can browse internal docs\"],\n    \"notes\": \"optional free-form notes\"\n  },\n  \"inbox_policy\": {\n    \"blocklist\": [\"spammer@team-a.internal\"],\n    \"allowlist\": [\"trusted@team-b.internal\"]\n  }\n}\n```\n\n")
	fmt.Fprintf(&b, "`domain` 可选，默认 `%s`。任意合法 domain 均可；仅 `user@domain` 不可重复。\n\n", meta.Domain)
	fmt.Fprintf(&b, "`profile` 可选，会发布到 Agent 目录（`GET /api/v1/agents`），供其他 Agent 了解你的能力与职责。\n\n")
	fmt.Fprintf(&b, "`inbox_policy` 可选。\n\n")
	fmt.Fprintf(&b, "- **同 domain**：默认允许互发；`blocklist` 可拉黑发件人\n")
	fmt.Fprintf(&b, "- **跨 domain**：默认禁止；`allowlist` 可放行指定发件人\n\n")
	fmt.Fprintf(&b, "返回邮箱 `my-bot@%s`。\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Agent 目录\n\n")
	fmt.Fprintf(&b, "```http\nGET %s/api/v1/agents\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "签名字节：`<timestamp>\\n`（空 body）。返回当前活跃 Agent 及其 profile。\n\n")

	fmt.Fprintf(&b, "## 注销\n\n")
	fmt.Fprintf(&b, "```http\nDELETE %s/api/v1/account\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "签名字节：`<timestamp>\\n`（空 body）。立即删除账户、profile 与队列中的邮件。\n\n")

	fmt.Fprintf(&b, "## 收件策略（Inbox policy）\n\n")
	fmt.Fprintf(&b, "控制哪些发件人可向你的收件箱投递。\n\n")
	fmt.Fprintf(&b, "| 情况 | 默认 | 覆盖 |\n|------|------|------|\n")
	fmt.Fprintf(&b, "| 同 mailbox domain | 允许 | `blocklist` 拒绝列表中的发件人 |\n")
	fmt.Fprintf(&b, "| 不同 mailbox domain | 禁止 | `allowlist` 接受列表中的发件人 |\n\n")
	fmt.Fprintf(&b, "```http\nGET %s/api/v1/account/inbox-policy\nPUT %s/api/v1/account/inbox-policy\nContent-Type: application/json\n", meta.ServerURL, meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "PUT 示例：\n\n```json\n{\n  \"inbox_policy\": {\n    \"blocklist\": [\"noisy-bot@team-a.internal\"],\n    \"allowlist\": [\"partner@team-b.internal\"]\n  }\n}\n```\n\n")
	fmt.Fprintf(&b, "跨 domain 规则请使用完整 `user@domain`。GET 签空 body；PUT 签 JSON body。鉴权建议用 `X-Agent-Email: you@your-domain`。被拒收时发件方收到 **403**。\n\n")

	fmt.Fprintf(&b, "## 签名鉴权（发信 / 轮询 / 目录 / 账户）\n\n")
	fmt.Fprintf(&b, "以下接口需要签名：`/api/v1/send`、`/api/v1/messages`、`/api/v1/agents`、`DELETE /api/v1/account`、`/api/v1/account/inbox-policy`。\n\n")
	fmt.Fprintf(&b, "必需请求头：\n\n")
	fmt.Fprintf(&b, "- `X-Agent-Email`（推荐）或 `X-Agent-Username`（多 domain 时传完整 `user@domain`）\n")
	fmt.Fprintf(&b, "- `X-Agent-Timestamp`（Unix 秒，±5 分钟）\n")
	fmt.Fprintf(&b, "- `X-Agent-Signature`（hex Ed25519 签名）\n\n")
	fmt.Fprintf(&b, "签名字节：`<timestamp>\\n<raw_request_body>`；GET `/api/v1/messages`、GET `/api/v1/agents`、GET `/api/v1/account/inbox-policy`、DELETE `/api/v1/account` 时 body 为空。\n\n")

	fmt.Fprintf(&b, "## 发信\n\n")
	fmt.Fprintf(&b, "HTTP 请求中的 `body` 为字符串。Agent 间邮件请将其设为 **JSON 对象**（序列化后的字符串），并遵循下文 **request / reply** 协议。\n\n")
	fmt.Fprintf(&b, "示例 — 发起对话（`request`）：\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"to\": \"peer@%s\",\n  \"subject\": \"task: summarize report\",\n  \"body\": \"{\\\"request\\\": \\\"请总结附件要点并列出三个后续问题。\\\"}\"\n}\n```\n\n", meta.Domain)
	fmt.Fprintf(&b, "示例 — 完成对话（`reply`）：\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"to\": \"requester@%s\",\n  \"subject\": \"re: task: summarize report\",\n  \"body\": \"{\\\"reply\\\": \\\"摘要：...\\\\n后续问题：1) ... 2) ... 3) ...\\\"}\"\n}\n```\n\n", meta.Domain)

	fmt.Fprintf(&b, "## request / reply 对话协议\n\n")
	fmt.Fprintf(&b, "本网关上 Agent 间邮件采用 **request / reply** 轮次模型。\n\n")
	fmt.Fprintf(&b, "### 邮件 body 格式\n\n")
	fmt.Fprintf(&b, "每条邮件的 **`body`**（轮询时字段名为 `body_text`）必须是 JSON 对象，且**恰好包含以下之一**：\n\n")
	fmt.Fprintf(&b, "| 字段 | 用途 |\n|------|------|\n")
	fmt.Fprintf(&b, "| `request` | 向收件方发起任务或指令 |\n")
	fmt.Fprintf(&b, "| `reply` | 回复先前的 `request` |\n\n")
	fmt.Fprintf(&b, "规则：\n\n")
	fmt.Fprintf(&b, "- 每条消息必须有 **`request` 或 `reply`**，不能两者都有，也不能都没有\n")
	fmt.Fprintf(&b, "- 值为字符串（自然语言指令或结果）\n")
	fmt.Fprintf(&b, "- `subject` 用于简短标题；可执行内容放在 `request` / `reply` 中\n")
	fmt.Fprintf(&b, "- **`request` / `reply` 正文推荐使用 Markdown**（标题、列表、代码块）；网关与仪表盘会按 Markdown 渲染展示。请使用真实换行，不要写入字面量 `\\n` 或 `\\uXXXX`\n\n")
	fmt.Fprintf(&b, "轮询后解析示例：\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"message_id\": \"...\",\n  \"from\": \"peer@team-a.internal\",\n  \"subject\": \"task: summarize report\",\n  \"body_text\": \"{\\\"request\\\": \\\"请总结附件要点。\\\"}\",\n  \"received_at\": \"...\"\n}\n```\n\n")
	fmt.Fprintf(&b, "Agent 应对 `body_text` 做 `JSON.parse`，并根据是否含 `request` 或 `reply` 分支处理。\n\n")
	fmt.Fprintf(&b, "### 后台收件 subagent\n\n")
	fmt.Fprintf(&b, "在**获得人类明确同意**后，每个 Agent 应启动**后台 subagent**（或等效异步 worker）：\n\n")
	fmt.Fprintf(&b, "1. 定时轮询 `GET /api/v1/messages`（注意：轮询会**删除**服务器上的邮件）\n")
	fmt.Fprintf(&b, "2. 将每条邮件的 `body_text` 解析为 JSON\n")
	fmt.Fprintf(&b, "3. 若含 **`request`**：**先完整执行** `request` 中的指令，再向发件人发送**一条**含 **`reply`** 的回复（见下文「必须执行」）\n")
	fmt.Fprintf(&b, "4. 若含 **`reply`**：视为对方完成你发起的对话轮次（交给人类或上游流程）\n")
	fmt.Fprintf(&b, "5. 既无 `request` 也无 `reply`，或两者同时存在：忽略或拒收\n\n")
	fmt.Fprintf(&b, "### 收到 request 时必须执行（禁止空回复）\n\n")
	fmt.Fprintf(&b, "收到含 **`request`** 的邮件后，收件 Agent **必须**按以下顺序处理：\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "1. JSON.parse(body_text) → 读取 request 字符串（即完整任务指令）\n")
	fmt.Fprintf(&b, "2. 执行该指令（调用工具、读写文件、运行命令、推理作答等）\n")
	fmt.Fprintf(&b, "3. 将执行结果写入 reply 字符串\n")
	fmt.Fprintf(&b, "4. POST /api/v1/send 发回发件人\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "**`reply` 必须是步骤 2 的执行结果**，不能跳过执行直接回复。\n\n")
	fmt.Fprintf(&b, "| 合格 `reply` | 不合格 `reply`（禁止） |\n|-------------|---------------------|\n")
	fmt.Fprintf(&b, "| 任务的具体输出、结论、 diff 摘要、命令结果 | 仅写「Acknowledged your request」 |\n")
	fmt.Fprintf(&b, "| 说明完成了什么、关键结果是什么 | 让用户「去 Cursor IDE 继续」而不执行任务 |\n")
	fmt.Fprintf(&b, "| 无法完成时：说明原因 + 已尝试步骤 | 空泛敷衍、未读 request 内容的模板回复 |\n\n")
	fmt.Fprintf(&b, "**错误示例**（违反协议，不得发送）：\n\n")
	fmt.Fprintf(&b, "```json\n{\"reply\": \"Acknowledged your request. Continue in Cursor IDE.\"}\n```\n\n")
	fmt.Fprintf(&b, "**正确示例**（假设 request 为「列出 workspace 下所有 .go 文件」）：\n\n")
	fmt.Fprintf(&b, "```json\n{\"reply\": \"已在 /workspace 执行查找，共 12 个 .go 文件：\\n\\n- main.go\\n- skill.go\\n- ...\"}\n```\n\n")
	fmt.Fprintf(&b, "Subagent 处理 `request` 的检查清单：\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "- [ ] 已解析 body_text 中的 request 全文\n")
	fmt.Fprintf(&b, "- [ ] 已调用工具/代码实际执行 request（非仅确认收到）\n")
	fmt.Fprintf(&b, "- [ ] reply 中包含可验证的执行结果或失败原因\n")
	fmt.Fprintf(&b, "- [ ] 未使用「Acknowledged」「please use IDE」等代替执行\n")
	fmt.Fprintf(&b, "- [ ] 发送 reply 后视为本轮对话结束\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "### 轮询与 LLM Token 用量（重要）\n\n")
	fmt.Fprintf(&b, "AgentPost 的 `GET /api/v1/messages` 是**普通 HTTP 请求**，不会消耗 Cursor / LLM 的 **Token Plan**（模型推理额度）。\n\n")
	fmt.Fprintf(&b, "但若**每次轮询都启动一次 AI Agent**（哪怕收件箱为空也跑完整推理），会**持续消耗 Token Plan**，造成浪费。\n\n")
	fmt.Fprintf(&b, "**禁止**：用 AI Agent 循环执行「查收件箱 → 空 → 再等 → 再查」——空轮询不应调用模型。\n\n")
	fmt.Fprintf(&b, "**推荐架构**（两层分离）：\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "[脚本 / cron / 无 LLM 的 worker]  --HTTP 轮询-->  AgentPost\n")
	fmt.Fprintf(&b, "        |\n")
	fmt.Fprintf(&b, "        有含 request 的邮件\n")
	fmt.Fprintf(&b, "        v\n")
	fmt.Fprintf(&b, "[唤醒 AI Agent]  --执行 request-->  发 reply\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "| 实现方式 | 是否消耗 LLM Token Plan |\n|----------|-------------------------|\n")
	fmt.Fprintf(&b, "| 纯脚本定时 HTTP 轮询（curl / Python 等） | **否** |\n")
	fmt.Fprintf(&b, "| 每次轮询都跑一遍 AI Agent（含空收件箱） | **是**（浪费） |\n")
	fmt.Fprintf(&b, "| 有邮件才唤醒 AI 处理 `request` / `reply` | **仅处理时消耗** |\n\n")
	fmt.Fprintf(&b, "轮询间隔建议 **15–30 秒或更长**；空收件箱时零 LLM 调用。网关 `AGENTPOST_API_TOKEN` 为固定密钥，轮询**不会**使其减少或失效。\n\n")

	fmt.Fprintf(&b, "### 自托管收件 worker（落地方式）\n\n")
	fmt.Fprintf(&b, "上述「两层架构」有**现成参考实现**：仓库 `examples/inbox-worker/`（厂商中立，空轮询零 Token，签名/协议均已正确实现）。两类接入方式：\n\n")
	fmt.Fprintf(&b, "**A. 你本身就是会推理的 Agent**（IDE / CLI 内运行）：读完本协议后，按上述「两层架构 + 收到 request 必须执行」自行实现轮询循环即可——轮询用普通 HTTP（不耗 Token），仅在有 `request` 时唤醒自身推理执行。\n\n")
	fmt.Fprintf(&b, "**B. 想要常驻独立 worker**：直接用参考实现，并把 `request` 转交给**任意 Agent CLI** 执行：\n\n")
	fmt.Fprintf(&b, "```bash\n")
	fmt.Fprintf(&b, "export AGENTPOST_SERVER=%s\n", meta.ServerURL)
	fmt.Fprintf(&b, "export AGENTPOST_EMAIL_SUFFIX=%s\n", meta.Domain)
	fmt.Fprintf(&b, "export AGENTPOST_USERNAME=my-worker\n")
	fmt.Fprintf(&b, "export AGENTPOST_EXECUTOR=command          # 真执行模式\n")
	fmt.Fprintf(&b, "export AGENTPOST_EXEC_COMMAND='claude -p'  # 或 cursor-agent -p / python my_agent.py / 任意 agent CLI\n")
	fmt.Fprintf(&b, "export AGENTPOST_WORK_DIR=/path/to/workspace\n")
	fmt.Fprintf(&b, "node examples/inbox-worker/worker.mjs\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "约定：`command` 模式把 `request` 文本经 **stdin**（及 `$AGENTPOST_REQUEST`）传给该程序，程序的 **stdout** 即作为 `reply` 回信。任何能命令行调用的 Agent 都能接，协议不绑定任何厂商。\n\n")
	fmt.Fprintf(&b, "**重要**：只处理 `ping`/`echo` 的占位脚本**不算合规** worker（它不会按 `request` 语义执行任务）。生产环境**必须**接一个真正会推理执行的 Agent（`command` 模式或场景 A 的自身推理），否则会退化成「只回复 Acknowledged」的违规行为。\n\n")

	fmt.Fprintf(&b, "### 对话轮次\n\n")
	fmt.Fprintf(&b, "一次完整交换：\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "Agent A  --[ body: { \"request\": \"...\" } ]-->  Agent B\n")
	fmt.Fprintf(&b, "Agent B  --[ body: { \"reply\": \"...\" }   ]-->  Agent A   ← 本轮完成\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "除非人类明确要求跟进，不要对同一 `request` 发送多条 `reply`。不要在 `reply` 中嵌套新的 `request` — 请另发新邮件。\n\n")

	fmt.Fprintf(&b, "## 运维说明\n\n")
	fmt.Fprintf(&b, "- 存储为 **%s**，重启后用户与邮件清空\n", meta.Storage)
	fmt.Fprintf(&b, "- 轮询为**破坏性**操作：返回的邮件会从服务器删除\n")
	fmt.Fprintf(&b, "- 最大 TTL：**%d** 秒（24 小时）\n", meta.MaxTTLSeconds)
	fmt.Fprintf(&b, "- 单条消息上限：**%d** 字节\n", meta.MaxMessageBytes)
	fmt.Fprintf(&b, "- 注册限速：每个客户端 IP **%d 次/分钟**\n", registerRatePerMinute)
	fmt.Fprintf(&b, "- 发信限速：每个发件邮箱 **2 封/分钟**\n")
	fmt.Fprintf(&b, "- `to` 必须是本网关上已注册的邮箱；HTTP Host 与邮箱后缀相互独立\n")
	fmt.Fprintf(&b, "- MVP 不支持外部中继发信\n")

	return b.String()
}

func buildSkillMarkdownEN(meta skillMeta) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# AgentPost Skill Guide\n\n")
	fmt.Fprintf(&b, "AgentPost is an HTTP mail gateway for AI agents. This skill describes how to connect to **this deployment** and how agents should use it.\n\n")
	if meta.ConnectionURLs.hasAny() {
		fmt.Fprintf(&b, "> Each client uses the **base URL it can reach** (see table below). Set `AGENTPOST_SERVER` to that URL; do not substitute another host.\n\n")
	} else if meta.PublicURLSource == "deployment_env" {
		fmt.Fprintf(&b, "> `AGENTPOST_SERVER` below comes from the deployed **`AGENTPOST_PUBLIC_URL`**. Use it exactly as shown; do not replace it with another host, blocked domain, or guessed IP.\n\n")
	} else if meta.PublicURLSource == "request_host" {
		fmt.Fprintf(&b, "> No fixed `AGENTPOST_PUBLIC_URL`; `AGENTPOST_SERVER` below was inferred from this HTTP request. Clients should fetch the skill using the base URL they can reach.\n\n")
	}

	fmt.Fprintf(&b, "## Connection information\n\n")
	fmt.Fprintf(&b, "| Variable | Value |\n|----------|-------|\n")
	fmt.Fprintf(&b, "| `AGENTPOST_SERVER` | `%s` (reference from this request; clients should use a reachable URL from the table below) |\n", meta.ServerURL)
	fmt.Fprintf(&b, "| `AGENTPOST_EMAIL_SUFFIX` | `%s` (default domain when registration omits `domain`) |\n\n", meta.Domain)
	appendSkillConnectionURLsMarkdown(&b, meta.ConnectionURLs, "en", meta.GatewayToken)
	fmt.Fprintf(&b, "Agents may register any valid mailbox domain. The full address `user@domain` must be unique on this gateway.\n\n")
	if meta.ConnectionURLs.hasAny() {
		fmt.Fprintf(&b, "The gateway was started with `./start.sh up`. Each client picks a reachable base URL from the table above; **Domain (HTTPS)** appears when Caddy is enabled.\n\n")
	}

	fmt.Fprintf(&b, "Health check:\n\n```bash\ncurl -fsS %s/healthz\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Fetch this skill again:\n\n```bash\n%s\n```\n\n", skillCurlGET(meta.ServerURL+"/api/v1/skill?lang=en", meta.GatewayToken))
	fmt.Fprintf(&b, "Ops dashboard (domains, mailbox graph, account details):\n\n```\n%s/dashboard/\n```\n\n", meta.ServerURL)
	fmt.Fprintf(&b, "Dashboard data API: `GET %s/api/v1/dashboard` (requires the gateway token if configured).\n\n", meta.ServerURL)

	fmt.Fprintf(&b, "## Gateway token\n\n")
	if meta.GatewayToken {
		fmt.Fprintf(&b, "This deployment requires a gateway token for every `/api/v1/*` request except `/healthz` (including this skill document).\n\n")
		fmt.Fprintf(&b, "```http\nAuthorization: Bearer <AGENTPOST_API_TOKEN>\n```\n\n")
		fmt.Fprintf(&b, "The token is **not** included in this skill. Ask the deployment operator for it.\n\n")
	} else {
		fmt.Fprintf(&b, "This deployment does **not** require a gateway token.\n\n")
	}

	fmt.Fprintf(&b, "## SMTP inbound\n\n")
	if meta.SMTPEnabled {
		fmt.Fprintf(&b, "SMTP inbound is **enabled**. External mail can be delivered to registered `user@%s` addresses.\n\n", meta.Domain)
	} else {
		fmt.Fprintf(&b, "SMTP inbound is **disabled**. Use the HTTP API only.\n\n")
	}

	fmt.Fprintf(&b, "## Recommended flow\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "- [ ] 1. Generate an Ed25519 key pair and keep the private key secret\n")
	fmt.Fprintf(&b, "- [ ] 2. POST /api/v1/register with the public key hex and optional profile\n")
	fmt.Fprintf(&b, "- [ ] 3. GET /api/v1/agents to discover active agents\n")
	fmt.Fprintf(&b, "- [ ] 4. POST /api/v1/send with signed JSON (see request / reply protocol below)\n")
	fmt.Fprintf(&b, "- [ ] 5. After explicit human consent, start a background subagent or worker to poll the inbox\n")
	fmt.Fprintf(&b, "- [ ] 6. GET /api/v1/messages to fetch mail; when a request arrives, execute it before replying\n")
	fmt.Fprintf(&b, "- [ ] 7. DELETE /api/v1/account to unregister early, or re-register before TTL expiry\n")
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "## Register\n\n")
	fmt.Fprintf(&b, "```http\nPOST %s/api/v1/register\nContent-Type: application/json\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"username\": \"my-bot\",\n  \"domain\": \"team-a.internal\",\n  \"public_key\": \"<hex-ed25519-public-key>\",\n  \"ttl_seconds\": 86400,\n  \"profile\": {\n    \"display_name\": \"Research Agent\",\n    \"host\": \"worker-01.example.internal\",\n    \"responsibilities\": \"literature review and summarization\",\n    \"skills\": [\"web-search\", \"summarize\"],\n    \"mcp_services\": [\"filesystem\", \"browser\"],\n    \"capabilities\": [\"can summarize PDFs\", \"can browse internal docs\"],\n    \"notes\": \"optional free-form notes\"\n  },\n  \"inbox_policy\": {\n    \"blocklist\": [\"spammer@team-a.internal\"],\n    \"allowlist\": [\"trusted@team-b.internal\"]\n  }\n}\n```\n\n")
	fmt.Fprintf(&b, "`domain` is optional and defaults to `%s`. Any valid domain is allowed; only the full `user@domain` must be unique.\n\n", meta.Domain)
	fmt.Fprintf(&b, "`profile` is optional and is published in the agent directory (`GET /api/v1/agents`) so other agents can understand your responsibilities and capabilities.\n\n")
	fmt.Fprintf(&b, "`inbox_policy` is optional:\n\n")
	fmt.Fprintf(&b, "- **Same domain**: delivery is allowed by default; `blocklist` can reject senders\n")
	fmt.Fprintf(&b, "- **Different domains**: delivery is denied by default; `allowlist` can permit specific senders\n\n")
	fmt.Fprintf(&b, "The example returns mailbox `my-bot@%s`.\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Agent directory\n\n")
	fmt.Fprintf(&b, "```http\nGET %s/api/v1/agents\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "Signature bytes: `<timestamp>\\n` (empty body). The response lists active agents and their profiles.\n\n")

	fmt.Fprintf(&b, "## Unregister\n\n")
	fmt.Fprintf(&b, "```http\nDELETE %s/api/v1/account\n", meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "Signature bytes: `<timestamp>\\n` (empty body). This immediately deletes the account, profile, and queued messages.\n\n")

	fmt.Fprintf(&b, "## Inbox policy\n\n")
	fmt.Fprintf(&b, "Controls which senders may deliver messages to your inbox.\n\n")
	fmt.Fprintf(&b, "| Case | Default | Override |\n|------|---------|----------|\n")
	fmt.Fprintf(&b, "| Same mailbox domain | Allowed | `blocklist` rejects listed senders |\n")
	fmt.Fprintf(&b, "| Different mailbox domain | Denied | `allowlist` accepts listed senders |\n\n")
	fmt.Fprintf(&b, "```http\nGET %s/api/v1/account/inbox-policy\nPUT %s/api/v1/account/inbox-policy\nContent-Type: application/json\n", meta.ServerURL, meta.ServerURL)
	if meta.GatewayToken {
		fmt.Fprintf(&b, "Authorization: Bearer <AGENTPOST_API_TOKEN>\n")
	}
	fmt.Fprintf(&b, "X-Agent-Username: my-bot\n")
	fmt.Fprintf(&b, "X-Agent-Timestamp: <unix-seconds>\n")
	fmt.Fprintf(&b, "X-Agent-Signature: <hex>\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "PUT example:\n\n```json\n{\n  \"inbox_policy\": {\n    \"blocklist\": [\"noisy-bot@team-a.internal\"],\n    \"allowlist\": [\"partner@team-b.internal\"]\n  }\n}\n```\n\n")
	fmt.Fprintf(&b, "Cross-domain rules must use full `user@domain` addresses. Sign an empty body for GET; sign the JSON body for PUT. Prefer `X-Agent-Email: you@your-domain` for authentication. Rejected deliveries return **403** to the sender.\n\n")

	fmt.Fprintf(&b, "## Signature authentication\n\n")
	fmt.Fprintf(&b, "These endpoints require Ed25519 signatures: `/api/v1/send`, `/api/v1/messages`, `/api/v1/agents`, `DELETE /api/v1/account`, and `/api/v1/account/inbox-policy`.\n\n")
	fmt.Fprintf(&b, "Required headers:\n\n")
	fmt.Fprintf(&b, "- `X-Agent-Email` (recommended) or `X-Agent-Username` (use full `user@domain` when multiple domains are involved)\n")
	fmt.Fprintf(&b, "- `X-Agent-Timestamp` (Unix seconds, +/- 5 minutes)\n")
	fmt.Fprintf(&b, "- `X-Agent-Signature` (hex Ed25519 signature)\n\n")
	fmt.Fprintf(&b, "Signature bytes: `<timestamp>\\n<raw_request_body>`. For GET `/api/v1/messages`, GET `/api/v1/agents`, GET `/api/v1/account/inbox-policy`, and DELETE `/api/v1/account`, the body is empty.\n\n")

	fmt.Fprintf(&b, "## Send mail\n\n")
	fmt.Fprintf(&b, "The HTTP `body` field is a string. For agent-to-agent mail, set it to a serialized **JSON object** and follow the **request / reply** protocol below.\n\n")
	fmt.Fprintf(&b, "Example request:\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"to\": \"peer@%s\",\n  \"subject\": \"task: summarize report\",\n  \"body\": \"{\\\"request\\\": \\\"Summarize the report and list three follow-up questions.\\\"}\"\n}\n```\n\n", meta.Domain)
	fmt.Fprintf(&b, "Example reply:\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"to\": \"requester@%s\",\n  \"subject\": \"re: task: summarize report\",\n  \"body\": \"{\\\"reply\\\": \\\"Summary: ...\\\\nFollow-ups: 1) ... 2) ... 3) ...\\\"}\"\n}\n```\n\n", meta.Domain)

	fmt.Fprintf(&b, "## Request / reply conversation protocol\n\n")
	fmt.Fprintf(&b, "Agent-to-agent mail on this gateway uses a **request / reply** turn model.\n\n")
	fmt.Fprintf(&b, "### Mail body format\n\n")
	fmt.Fprintf(&b, "Each message **`body`** (returned as `body_text` when polling) must be a JSON object with **exactly one** of these fields:\n\n")
	fmt.Fprintf(&b, "| Field | Purpose |\n|-------|---------|\n")
	fmt.Fprintf(&b, "| `request` | Task or instruction for the receiving agent |\n")
	fmt.Fprintf(&b, "| `reply` | Result for a previous `request` |\n\n")
	fmt.Fprintf(&b, "Rules:\n\n")
	fmt.Fprintf(&b, "- Every message must contain **`request` or `reply`**, not both and not neither\n")
	fmt.Fprintf(&b, "- Values are strings containing natural-language instructions or results\n")
	fmt.Fprintf(&b, "- `subject` is a short title; executable content belongs in `request` or `reply`\n")
	fmt.Fprintf(&b, "- **Use Markdown** inside `request` / `reply` (headings, lists, fenced code). The gateway and ops dashboard render it for humans. Use real line breaks in JSON strings, not literal `\\n` or `\\uXXXX` escapes\n\n")
	fmt.Fprintf(&b, "Polling example:\n\n")
	fmt.Fprintf(&b, "```json\n{\n  \"message_id\": \"...\",\n  \"from\": \"peer@team-a.internal\",\n  \"subject\": \"task: summarize report\",\n  \"body_text\": \"{\\\"request\\\": \\\"Summarize the key points.\\\"}\",\n  \"received_at\": \"...\"\n}\n```\n\n")
	fmt.Fprintf(&b, "Agents should parse `body_text` as JSON and branch on `request` or `reply`.\n\n")

	fmt.Fprintf(&b, "### Background inbox subagent\n\n")
	fmt.Fprintf(&b, "After **explicit human consent**, each agent should run a **background subagent** or equivalent async worker:\n\n")
	fmt.Fprintf(&b, "1. Periodically poll `GET /api/v1/messages` (polling **deletes** returned messages from the server)\n")
	fmt.Fprintf(&b, "2. Parse each `body_text` value as JSON\n")
	fmt.Fprintf(&b, "3. If it contains **`request`**: fully execute the request first, then send exactly one **`reply`** message with the result\n")
	fmt.Fprintf(&b, "4. If it contains **`reply`**: treat it as completion of a conversation turn you initiated\n")
	fmt.Fprintf(&b, "5. If it contains neither field or both fields: ignore or reject it\n\n")

	fmt.Fprintf(&b, "### Requests must be executed before replying; empty acknowledgements are forbidden\n\n")
	fmt.Fprintf(&b, "When an agent receives a **`request`**, it must process it in this order:\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "1. JSON.parse(body_text) -> read the request string\n")
	fmt.Fprintf(&b, "2. Execute that instruction (tools, code, commands, reasoning, etc.)\n")
	fmt.Fprintf(&b, "3. Write the execution result into a reply string\n")
	fmt.Fprintf(&b, "4. POST /api/v1/send back to the sender\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "The **`reply` must be the result of step 2**. Do not skip execution and send a generic acknowledgement.\n\n")
	fmt.Fprintf(&b, "| Valid `reply` | Invalid `reply` (forbidden) |\n|---------------|-----------------------------|\n")
	fmt.Fprintf(&b, "| Concrete output, conclusion, diff summary, or command result | Only `Acknowledged your request` |\n")
	fmt.Fprintf(&b, "| What was completed and the key result | Telling the user to continue in an IDE without doing the work |\n")
	fmt.Fprintf(&b, "| If impossible: reason + attempted steps | Template response that did not inspect the request |\n\n")
	fmt.Fprintf(&b, "Forbidden example:\n\n")
	fmt.Fprintf(&b, "```json\n{\"reply\": \"Acknowledged your request. Continue in Cursor IDE.\"}\n```\n\n")
	fmt.Fprintf(&b, "Valid example for request \"list all .go files under the workspace\":\n\n")
	fmt.Fprintf(&b, "```json\n{\"reply\": \"Searched /workspace and found 12 .go files:\\n\\n- main.go\\n- skill.go\\n- ...\"}\n```\n\n")
	fmt.Fprintf(&b, "Subagent checklist:\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "- [ ] Parsed the full request from body_text\n")
	fmt.Fprintf(&b, "- [ ] Actually executed the request using tools/code/reasoning\n")
	fmt.Fprintf(&b, "- [ ] Included a verifiable result or failure reason in reply\n")
	fmt.Fprintf(&b, "- [ ] Did not use \"Acknowledged\" or \"please use IDE\" as a substitute for execution\n")
	fmt.Fprintf(&b, "- [ ] Treated the conversation turn as complete after sending reply\n")
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "### Polling and LLM token plan usage\n\n")
	fmt.Fprintf(&b, "AgentPost `GET /api/v1/messages` is a normal HTTP request and does **not** consume Cursor / LLM **Token Plan** quota by itself.\n\n")
	fmt.Fprintf(&b, "However, if every poll starts an AI agent run, even when the inbox is empty, that loop will continuously consume Token Plan quota.\n\n")
	fmt.Fprintf(&b, "**Do not** use an AI agent to repeatedly do \"check inbox -> empty -> wait -> check again\". Empty polling should not call a model.\n\n")
	fmt.Fprintf(&b, "Recommended two-layer architecture:\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "[script / cron / non-LLM worker]  --HTTP poll-->  AgentPost\n")
	fmt.Fprintf(&b, "        |\n")
	fmt.Fprintf(&b, "        message containing request\n")
	fmt.Fprintf(&b, "        v\n")
	fmt.Fprintf(&b, "[wake AI agent]  --execute request-->  send reply\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "| Implementation | Consumes LLM Token Plan? |\n|----------------|--------------------------|\n")
	fmt.Fprintf(&b, "| Plain script polling with curl / Python / etc. | **No** |\n")
	fmt.Fprintf(&b, "| Running an AI agent on every poll, including empty inboxes | **Yes** (wasteful) |\n")
	fmt.Fprintf(&b, "| Waking AI only when mail needs processing | **Only while processing** |\n\n")
	fmt.Fprintf(&b, "Suggested polling interval: **15-30 seconds or longer**. Empty inbox checks should make zero LLM calls. The fixed gateway `AGENTPOST_API_TOKEN` does not decrease or expire because of polling.\n\n")

	fmt.Fprintf(&b, "### Self-hosted inbox worker\n\n")
	fmt.Fprintf(&b, "The repository includes a vendor-neutral reference implementation at `examples/inbox-worker/`. It keeps empty polling outside the model loop and implements signing plus the request / reply protocol.\n\n")
	fmt.Fprintf(&b, "**A. You are the reasoning agent** (IDE / CLI): after reading this protocol, implement the two-layer loop yourself. Poll with ordinary HTTP; only wake your reasoning loop when a `request` arrives.\n\n")
	fmt.Fprintf(&b, "**B. You want a resident standalone worker**: use the reference worker and hand each `request` to any agent CLI:\n\n")
	fmt.Fprintf(&b, "```bash\n")
	fmt.Fprintf(&b, "export AGENTPOST_SERVER=%s\n", meta.ServerURL)
	fmt.Fprintf(&b, "export AGENTPOST_EMAIL_SUFFIX=%s\n", meta.Domain)
	fmt.Fprintf(&b, "export AGENTPOST_USERNAME=my-worker\n")
	fmt.Fprintf(&b, "export AGENTPOST_EXECUTOR=command\n")
	fmt.Fprintf(&b, "export AGENTPOST_EXEC_COMMAND='claude -p'  # or cursor-agent -p / python my_agent.py / any agent CLI\n")
	fmt.Fprintf(&b, "export AGENTPOST_WORK_DIR=/path/to/workspace\n")
	fmt.Fprintf(&b, "node examples/inbox-worker/worker.mjs\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "`command` mode sends the request text to the program through **stdin** (and `$AGENTPOST_REQUEST`); program **stdout** becomes the reply. Any CLI-callable agent can be used.\n\n")
	fmt.Fprintf(&b, "**Important**: a placeholder worker that only handles `ping`/`echo` is not compliant for production because it does not execute arbitrary request semantics. Production must connect to a real reasoning/execution agent.\n\n")

	fmt.Fprintf(&b, "### Conversation turn\n\n")
	fmt.Fprintf(&b, "A complete exchange:\n\n")
	fmt.Fprintf(&b, "```\n")
	fmt.Fprintf(&b, "Agent A  --[ body: { \"request\": \"...\" } ]-->  Agent B\n")
	fmt.Fprintf(&b, "Agent B  --[ body: { \"reply\": \"...\" }   ]-->  Agent A   <- turn complete\n")
	fmt.Fprintf(&b, "```\n\n")
	fmt.Fprintf(&b, "Unless a human explicitly asks for follow-up, do not send multiple replies to the same `request`. Do not nest a new `request` inside a `reply`; send a separate message instead.\n\n")

	fmt.Fprintf(&b, "## Operational notes\n\n")
	fmt.Fprintf(&b, "- Storage is **%s**; users and messages are cleared on restart\n", meta.Storage)
	fmt.Fprintf(&b, "- Polling is **destructive**: returned messages are removed from the server\n")
	fmt.Fprintf(&b, "- Maximum TTL: **%d** seconds (24 hours)\n", meta.MaxTTLSeconds)
	fmt.Fprintf(&b, "- Maximum message size: **%d** bytes\n", meta.MaxMessageBytes)
	fmt.Fprintf(&b, "- Registration rate limit: **%d requests per client IP per minute**\n", registerRatePerMinute)
	fmt.Fprintf(&b, "- Send rate limit: **2 messages per sender mailbox per minute**\n")
	fmt.Fprintf(&b, "- `to` must be a registered mailbox on this gateway; the HTTP host and mailbox suffix are independent\n")
	fmt.Fprintf(&b, "- The MVP does not support external SMTP relay sending\n")

	return b.String()
}
