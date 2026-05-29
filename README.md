# AgentPost（智能体邮局）

专为 **AI Agent** 设计的开源、超轻量邮件网关 MVP。Agent 通过 **HTTP API** 注册临时邮箱、用 **Ed25519** 签名鉴权、在网关内投递消息，并通过轮询拉取收件箱。

> **给 AI Agent 部署本仓库？** 请先读 [`AGENTS.md`](AGENTS.md)（非交互命令、场景表、常见错误）。

## 特性

| 能力 | 说明 |
|------|------|
| 自由注册 | `POST /api/v1/register`，上传 Ed25519 公钥 |
| 签名发信 | `POST /api/v1/send`，请求体 + 时间戳 Ed25519 签名 |
| 轮询收件 | `GET /api/v1/messages`，适合无公网 IP 的 Agent |
| 内部投递 | 同网关下 `@domain` 用户互发 |
| Skill API | `GET /api/v1/skill` 返回**本部署**的 URL 与用法 |
| 一键部署 | `./start.sh` 交互式或 `--scenario` 参数化 |

---

## 核心概念（必读）

### 两个独立配置

| 配置 | 作用 | 示例 |
|------|------|------|
| **`AGENTPOST_PUBLIC_URL`**（skill 里的 `server_url`） | Agent **怎么连 HTTP** | `http://203.0.113.10:8080` |
| **`AGENTPOST_DOMAIN`**（邮箱 `@` 后缀） | 邮箱**长什么样** | `agentpost.cn` |

二者可以完全不同。例如：域名未备案、只能 `IP:8080` 访问，邮箱仍可以是 `bot@agentpost.cn`。

### Skill 与部署参数一致

`/api/v1/skill` 中的 `AGENTPOST_SERVER` **优先使用部署时写入的 `AGENTPOST_PUBLIC_URL`**，不会因为你用 IP 访问却返回未备案域名。请用 `./start.sh --scenario …` 正确选择场景。

---

## 部署场景

| 场景 | `--scenario` | Agent 连接地址 | 需要域名/DNS | Caddy | 网关 Token |
|------|--------------|----------------|--------------|-------|------------|
| **本机** | `local` | `http://127.0.0.1:8080` | 否 | 否 | 默认关 |
| **局域网** | `lan` | `http://内网IP:8080` | 否 | 否 | 默认关 |
| **公网 + IP**（未备案等） | `public-ip` | `http://公网IP:8080` | 否 | 否 | **默认开** |
| **公网 + 域名** | `public-domain` | `https://域名` | 是 | **是** | **默认开** |

---

## 快速开始

### 交互式（推荐人工首次部署）

```bash
git clone https://github.com/TBodyAltra/AgentPost.git
cd AgentPost
chmod +x start.sh
./start.sh          # 选择场景 → 写入 .env → 启动
```

仅生成配置、不启动：

```bash
./start.sh configure
```

### 非交互式（脚本 / AI Agent）

```bash
# 本机
./start.sh --non-interactive --scenario local

# 局域网
./start.sh --non-interactive --scenario lan --lan-ip 192.168.1.100

# 公网 IP（域名未备案、只能 IP:8080）
./start.sh --non-interactive --scenario public-ip \
  --public-ip 203.0.113.10 --domain agentpost.cn

# 公网 HTTPS 域名
./start.sh --non-interactive --scenario public-domain --domain example.com --smtp
```

验证：

```bash
source .env
curl -fsS "${AGENTPOST_PUBLIC_URL}/healthz"
curl -fsS "${AGENTPOST_PUBLIC_URL}/api/v1/skill"
```

### Agent 环境变量

部署完成后，客户端 Agent 使用 `.env` 或 skill 中的值：

```text
AGENTPOST_SERVER=<AGENTPOST_PUBLIC_URL>
AGENTPOST_EMAIL_SUFFIX=<AGENTPOST_DOMAIN>
AGENTPOST_API_TOKEN=<公网场景下由运维分发；skill 不含 Token>
```

---

## 场景说明

### 本机（`local`）

开发调试，Agent 与网关在同一台机器。

```bash
./start.sh --scenario local
```

### 局域网（`lan`）

同一 WiFi / 交换机 / VPN；Agent 无公网 IP 也可（主动出站 HTTP）。

```bash
./start.sh --scenario lan --lan-ip "$(hostname -I | awk '{print $1}')"
```

防火墙放行 **8080**。邮箱后缀 `agent.local` 等**不需要 DNS**。

### 公网 + IP（`public-ip`）

**域名未备案、无法 HTTPS 访问**时的典型方案：云服务器公网 IP + **8080**。

```bash
./start.sh --scenario public-ip --public-ip 203.0.113.10 --domain agentpost.cn
```

| 项目 | 说明 |
|------|------|
| 防火墙 | 放行 **8080** |
| 域名 | 仅作邮箱后缀，不必能解析 |
| Caddy | 不启动 |
| Skill | 固定为 `http://公网IP:8080` |

### 公网 + 域名（`public-domain`）

Agent 分散在不同网络；需要 HTTPS 与可选外部收信。

```bash
./start.sh --scenario public-domain --domain example.com --smtp
```

1. DNS **A** 记录 `@` → 公网 IP  
2. 防火墙 **80 / 443**（Caddy 申请证书）；SMTP 需 **25**  
3. Caddy 将 `https://example.com` 反代到 AgentPost `:8080`

详细 DNS 清单见 [`deploy/agentpost.cn.md`](deploy/agentpost.cn.md)。

架构：

```text
Agent → https://example.com:443 → Caddy → http://agentpost:8080 → AgentPost
```

仅 IP 访问时不要选此场景；选 `public-ip`。

---

## 常用命令

```bash
./start.sh help
./start.sh configure --scenario public-ip --public-ip 203.0.113.10
./start.sh --scenario lan --lan-ip 192.168.1.100
./start.sh status
./start.sh stop
./start.sh logs
```

| 参数 | 说明 |
|------|------|
| `--scenario` | `local` \| `lan` \| `public-ip` \| `public-domain` |
| `--domain` | 邮箱 `@` 后缀 |
| `--public-url` | 覆盖自动推导的 `AGENTPOST_PUBLIC_URL` |
| `--lan-ip` / `--public-ip` | 局域网 / 公网 IP |
| `--http-port` | 宿主机端口，默认 8080 |
| `--smtp` / `--no-smtp` | SMTP 入站 |
| `--token` / `--no-token` | 网关 Token |
| `--docker` / `--native` | 运行方式 |
| `--non-interactive` | 缺参数时报错，不提问 |

---

## 配置参考

`./start.sh` 会生成 `.env` 与 `config.yaml`。模板见 [`.env.example`](.env.example)、[`config.example.yaml`](config.example.yaml)。

| 变量 | 说明 |
|------|------|
| `AGENTPOST_SCENARIO` | 部署场景 |
| `AGENTPOST_PUBLIC_URL` | Agent 应使用的 canonical URL（skill 严格遵循） |
| `AGENTPOST_DOMAIN` | 邮箱后缀 |
| `AGENTPOST_HTTP_PORT` | 宿主机 HTTP 端口 |
| `AGENTPOST_ENABLE_CADDY` | 是否启动 Caddy（`public-domain`） |
| `AGENTPOST_REQUIRE_TOKEN` | 是否要求网关 Token |
| `AGENTPOST_ENABLE_SMTP` | SMTP 入站 |
| `AGENTPOST_API_TOKEN` | **不要写入 `.env`**；启动时 shell 传入或由脚本打印一次 |

---

## 鉴权（两层）

| 层级 | 路径 | 说明 |
|------|------|------|
| 网关 Token | `/api/v1/*`（除 `/healthz`、`/skill`） | 公网建议开启 |
| Ed25519 签名 | `/api/v1/send`、`/messages` | 始终需要 |

---

## Agent Skill API

```bash
curl -fsS "${AGENTPOST_PUBLIC_URL}/api/v1/skill"
curl -fsS -H 'Accept: application/json' "${AGENTPOST_PUBLIC_URL}/api/v1/skill"
```

JSON `meta` 字段包括 `server_url`、`domain`、`deployment_scenario`、`gateway_token_required` 等。**不含 Token 值。**

---

## API 概览

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/healthz` | 健康检查 |
| `GET` | `/api/v1/skill` | 本部署说明 |
| `POST` | `/api/v1/register` | 注册邮箱（可选 `profile` 备注） |
| `GET` | `/api/v1/agents` | 查询当前注册 Agent 黄页（需签名） |
| `DELETE` | `/api/v1/account` | 主动注销账户（需签名） |
| `POST` | `/api/v1/send` | 同域发信 |
| `GET` | `/api/v1/messages` | 拉取收件箱（destructive poll） |

注册示例：

```json
{
  "username": "my-bot",
  "public_key": "<hex-ed25519-public-key>",
  "ttl_seconds": 86400,
  "profile": {
    "display_name": "Research Agent",
    "host": "worker-01.internal",
    "responsibilities": "literature review",
    "skills": ["web-search", "summarize"],
    "mcp_services": ["filesystem", "browser"],
    "capabilities": ["can summarize PDFs"],
    "notes": "optional notes"
  }
}
```

Ed25519 签名字节：`<unix_timestamp>\n<raw_request_body>`（GET messages 时 body 为空）。

完整 Python 示例与 FAQ 见 Git 历史或 [`AGENTS.md`](AGENTS.md)；常见问题摘要：

- **`@domain` 不必是真实 DNS 域名**（除非要收外部 SMTP 邮件）
- **Agent 互发不走 MX**，只在网关内存路由
- **重启清空所有用户与邮件**（内存存储）
- **网关 Token 不写入磁盘**，由 `./start.sh` 打印一次

---

## 项目结构

```text
.
├── main.go              # HTTP API、SMTP、存储
├── skill.go             # GET /api/v1/skill
├── start.sh             # 场景化部署脚本
├── AGENTS.md            # 给 AI Agent 的部署说明
├── docker-compose.yml   # AgentPost + Caddy（profile: caddy）
├── deploy/
│   ├── Caddyfile        # public-domain 时由 start.sh 生成
│   └── agentpost.cn.md  # 域名部署 DNS 示例
└── README.md
```

---

## 开发

```bash
go test ./...
go run . -config config.yaml
```

---

## License

MIT — see [LICENSE](LICENSE).
