# AgentPost（智能体邮局）

专为 **AI Agent** 设计的开源、超轻量邮件网关 MVP。Agent 通过 **HTTP API** 注册临时邮箱、用 **Ed25519** 签名鉴权、在网关内投递消息，并通过轮询拉取收件箱——无需传统邮件服务器的复杂反垃圾与持久化方案。

## 特性

| 能力 | 说明 |
|------|------|
| 自由注册 | `POST /api/v1/register`，上传 Ed25519 公钥 |
| 签名发信 | `POST /api/v1/send`，请求体 + 时间戳 Ed25519 签名 |
| 轮询收件 | `GET /api/v1/messages`，适合无公网 IP 的 Agent |
| 内部投递 | 同网关下 `@domain` 用户互发，默认沙盒 |
| TTL 邮箱 | 账号最长 24 小时，后台自动清理 |
| 限速 | 每账号每分钟最多 2 封 |
| SMTP 入站（可选） | 解析 MIME，HTML 转纯文本后投递 |
| 网关 Token（可选） | 公网暴露时保护 `/api/v1/*` |
| 一键部署 | `./start.sh`（Docker 或本机 Go） |

**当前未实现：** 外网 SMTP relay、跨网关 Federation、WebHook 推送、多 Token 分发。

## 架构

```mermaid
flowchart TB
  subgraph agents [不同机器上的 Agent]
    A[Agent A]
    B[Agent B]
  end

  subgraph gateway [AgentPost 网关]
    HTTP[HTTP API :8080]
    Caddy[Caddy HTTPS 可选]
    SMTP[SMTP 入站 可选]
    Store[(内存 用户表 + 收件箱)]
    Caddy --> HTTP
    HTTP --> Store
    SMTP --> Store
  end

  A -->|注册 / 发信 / 轮询| Caddy
  B -->|注册 / 发信 / 轮询| Caddy
  Ext[外部邮箱] -.->|可选| SMTP
```

**协作模式：** 所有 Agent 连接**同一个** AgentPost 实例。Agent 只需能 **出站访问 HTTP**（主动轮询），不要求自身有公网 IP 或入站 WebHook。

---

## 选择部署场景

| 场景 | 典型环境 | 需要域名？ | 需要公网 IP？ | 推荐配置 |
|------|----------|------------|---------------|----------|
| **A. 本机 / 单机** | 开发调试，Agent 与网关在同一台机器 | 否 | 否 | `agent.local`，无 Token |
| **B. 局域网** | 办公室 / 实验室，多台内网机器协作 | 否 | 否（用内网 IP） | `agent.local` 或 `agent.lan`，可选 Token |
| **C. 公网生产** | 云服务器，Agent 分散在不同网络 | 建议有 | 是 | 真实域名 + HTTPS + Token，可选 SMTP |

> **重要概念：** `domain` 配置项只决定邮箱地址后缀（如 `bot@agent.local`），**不必**是已备案或已解析的真实 DNS 域名。Agent 实际连接的 **HTTP 地址**（`AGENTPOST_SERVER`）与邮箱后缀可以完全不同。

---

## 快速开始（场景 A：本机开发）

### 前置

- **推荐：** Docker + Docker Compose
- **或：** Go 1.25+

### 启动

```bash
git clone https://github.com/TBodyAltra/AgentPost.git
cd AgentPost
chmod +x start.sh
cp .env.example .env
./start.sh
```

验证：

```bash
curl -fsS http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

Agent 环境变量：

```text
AGENTPOST_SERVER=http://127.0.0.1:8080
AGENTPOST_EMAIL_SUFFIX=agent.local
```

Agent 或编排器应先拉取 skill 文档：

```bash
curl -fsS http://127.0.0.1:8080/api/v1/skill
```

---

## 场景 B：局域网部署（无公网 IP、无域名）

适合：同一 WiFi / 交换机 / VPN 下的多台机器；Agent 没有公网 IP，只要能访问网关的内网地址即可。

### 1. 在网关机器上启动

```bash
cp .env.example .env
# .env 保持默认即可，或显式设置：
# AGENTPOST_DOMAIN=agent.local
# AGENTPOST_ENABLE_SMTP=0

./start.sh --docker --domain agent.local
```

`./start.sh` 会自动生成网关 Token 并**打印到终端**（不写入任何文件）。内网可信环境可在启动前设空以禁用 Token：

```bash
AGENTPOST_API_TOKEN= ./start.sh --docker --domain agent.local
```

查看网关机器的内网 IP（示例 `192.168.1.100`）：

```bash
hostname -I
```

### 2. 其他机器上的 Agent 配置

```text
AGENTPOST_SERVER=http://192.168.1.100:8080
AGENTPOST_EMAIL_SUFFIX=agent.local
```

### 3. 注意事项

| 项目 | 说明 |
|------|------|
| 防火墙 | 网关机器需放行 **8080**（局域网入站） |
| 邮箱后缀 | `agent.local` 只是逻辑后缀，不需要 DNS 解析 |
| SMTP | 局域网场景通常**不需要**开启；Agent 之间通过 HTTP API 互发即可 |
| Token | 默认自动生成并打印到终端；内网可信环境可用 `AGENTPOST_API_TOKEN=` 禁用 |

### 4. 获取 skill 并验证

```bash
curl http://192.168.1.100:8080/api/v1/skill
curl http://192.168.1.100:8080/healthz
```

---

## 场景 C：公网 + 域名部署

适合：Agent 分布在不同网络（家庭、云、公司），需要通过互联网访问网关；可选接收 Gmail 等外部邮件。

详细 DNS / 防火墙清单见 [`deploy/agentpost.cn.md`](deploy/agentpost.cn.md)（以 `agentpost.cn` 为例，可替换为你的域名）。

### 1. DNS 记录

假设公网 IP 为 `203.0.113.10`，域名为 `example.com`：

| 类型 | 主机 | 值 | 说明 |
|------|------|-----|------|
| A | `@` | `203.0.113.10` | API + 邮箱根域 |
| A | `www` | `203.0.113.10` | 可选 |
| MX | `@` | `example.com` | 优先级 `10`，仅外部收信时需要 |

### 2. 配置 `.env`

```bash
cp .env.example .env
```

```bash
AGENTPOST_DOMAIN=example.com
AGENTPOST_PUBLIC_URL=https://example.com
AGENTPOST_ENABLE_SMTP=1
AGENTPOST_SMTP_PUBLISH_PORT=25
MODE=docker
```

启动时 `./start.sh` 会生成 Token 并打印到终端（不写入文件）。若要复用固定 Token：

```bash
AGENTPOST_API_TOKEN=$(openssl rand -hex 32) ./start.sh --docker --domain example.com --smtp
```

### 3. 启动（含 Caddy HTTPS 反代）

`docker-compose.yml` 已包含 **Caddy** 服务，与 AgentPost 一并启动：

```bash
./start.sh --docker --domain example.com --smtp
# 或
docker compose up -d --build
```

Caddy 会：
- 监听 **80 / 443**
- 自动申请 Let's Encrypt 证书
- 将 `https://example.com` 反代到 AgentPost `:8080`

> Ubuntu 默认 apt 源**没有** Caddy 包，推荐使用 Docker 方式，无需单独安装。

### 4. 防火墙 / 安全组

| 端口 | 用途 |
|------|------|
| 80 | HTTP（证书申请 + 跳转 HTTPS） |
| 443 | HTTPS API |
| 25 | SMTP 入站（可选，云厂商可能需申请解封） |
| 8080 | 建议不对公网开放（由 Caddy 反代） |

### 5. Agent 配置

```text
AGENTPOST_SERVER=https://example.com
AGENTPOST_EMAIL_SUFFIX=example.com
AGENTPOST_API_TOKEN=<部署时终端打印的值>
```

验证：

```bash
curl https://example.com/healthz
curl https://example.com/api/v1/skill
# {"status":"ok"}
```

---

## 什么是 Caddy 反代？

AgentPost 本身监听 `:8080`（HTTP，无 TLS）。公网场景下由 **Caddy** 作为「前台」：

```text
Agent  →  https://example.com:443  →  Caddy  →  http://agentpost:8080  →  AgentPost
```

好处：自动 HTTPS 证书、标准 443 端口、AgentPost 无需直接暴露到公网。

局域网场景（场景 A/B）**不需要** Caddy，直接用 `http://内网IP:8080` 即可。若 `docker-compose.yml` 中的 Caddy 服务不需要，可只启动 AgentPost：

```bash
docker compose up -d agentpost
```

---

## 常用命令

```bash
./start.sh                  # 有 Docker 则用 Compose，否则 go run
./start.sh --docker         # 强制 Docker 后台部署
./start.sh --native         # 本机 Go 前台（开发）
./start.sh --domain agent.local --http-port 8080
./start.sh --smtp           # 开启 SMTP 入站
./start.sh status           # 健康检查
./start.sh stop             # 停止 Docker 部署
./start.sh logs             # 查看 Docker 日志
```

---

## 配置说明

### 配置文件

[`config.example.yaml`](config.example.yaml)（也可由 `start.sh` 自动生成 `config.yaml`）：

```yaml
domain: agent.local          # 邮箱 @ 后缀，不必是真实 DNS 域名
http_addr: ":8080"
smtp_addr: ""                # 留空关闭 SMTP；":2525" 开启容器内入站
allow_external_relay: false  # MVP 禁止外发 relay
max_message_bytes: 1048576
```

网关 Token **不写入配置文件**，仅通过环境变量 `AGENTPOST_API_TOKEN` 在运行时注入。

### 环境变量（`.env` + 启动时 shell）

| 变量 | 说明 | 场景 |
|------|------|------|
| `AGENTPOST_DOMAIN` | 邮箱后缀，如 `agent.local` | 全部 |
| `AGENTPOST_HTTP_PORT` | 宿主机映射端口，默认 `8080` | 全部 |
| `AGENTPOST_PUBLIC_URL` | Agent 应使用的公网 URL（可选） | 公网 / 反代 |
| `AGENTPOST_API_TOKEN` | 网关 Token；**不要写入 `.env`**，由 `./start.sh` 生成并打印，或在启动命令前 export | 公网建议开启 |
| `AGENTPOST_ENABLE_SMTP` | `1` 开启 SMTP 入站 | 仅外部收信 |
| `AGENTPOST_SMTP_PUBLISH_PORT` | 宿主机 SMTP 端口，公网用 `25` | 仅外部收信 |
| `AGENTPOST_SMTP_ADDR` | 容器内监听，通常 `:2525` | 由 start.sh 设置 |
| `MODE` | `auto` / `docker` / `native` | 全部 |

Token 用法：

```bash
# 自动生成并打印（不存盘）
./start.sh --docker

# 复用固定 Token（仅当前 shell 会话）
AGENTPOST_API_TOKEN=$(openssl rand -hex 32) ./start.sh --docker

# 内网禁用 Token
AGENTPOST_API_TOKEN= ./start.sh --docker
```

---

## 鉴权说明（两层）

| 层级 | 作用 | 何时需要 |
|------|------|----------|
| **网关 Token** | 保护 `/api/v1/*` 不被随意调用 | 设置了 `AGENTPOST_API_TOKEN` 环境变量时 |
| **Ed25519 签名** | 标识具体 Agent，用于发信 / 收信 | `send`、`messages` 始终需要 |

网关 Token 请求头（二选一）：

```http
Authorization: Bearer <token>
X-AgentPost-Token: <token>
```

`/healthz` 与 `/api/v1/skill` 不需要任何鉴权。SMTP 入站不走 HTTP Token（邮件协议限制）。

---

## Agent Skill API

部署后，Agent 或编排器通过 HTTP 获取**本实例**的使用说明（Markdown 或 JSON）：

```bash
# Markdown（默认）
curl -fsS http://127.0.0.1:8080/api/v1/skill

# JSON（含结构化 meta）
curl -fsS -H 'Accept: application/json' http://127.0.0.1:8080/api/v1/skill
```

Skill 文档会尽量填好当前部署信息：

| 字段 | 说明 |
|------|------|
| `server_url` | 建议 Agent 连接的 HTTP(S) 地址 |
| `domain` | 邮箱 `@` 后缀 |
| `gateway_token_required` | 是否需要网关 Token |
| `smtp_inbound_enabled` | 是否开启 SMTP 入站 |
| 使用流程 / 建议 | 注册、签名、轮询、TTL、重启注意事项 |

**Skill 不包含 Token 值。** Token 仅在 `./start.sh` 首次启动时打印到终端，需由运维人员安全分发给 Agent。

公网 + Caddy 场景下，skill 会根据请求 `Host` / `X-Forwarded-Proto` 或 `AGENTPOST_PUBLIC_URL` 生成 `https://your-domain` 形式的地址。

---

## API 概览

| 方法 | 路径 | 网关 Token | Ed25519 | 说明 |
|------|------|------------|---------|------|
| `GET` | `/healthz` | 否 | 否 | 健康检查 |
| `GET` | `/api/v1/skill` | 否 | 否 | 本部署的使用说明（Markdown / JSON） |
| `POST` | `/api/v1/register` | 若已配置 | 否 | 注册临时邮箱 |
| `POST` | `/api/v1/send` | 若已配置 | 是 | 同域内部投递 |
| `GET` | `/api/v1/messages` | 若已配置 | 是 | 拉取收件箱（**会清空已返回消息**） |

所有 `POST` 请求需 `Content-Type: application/json`。

### 1. 注册

```http
POST /api/v1/register
Authorization: Bearer <token>    # 若启用了网关 Token
```

```json
{
  "username": "crypto-agent-007",
  "public_key": "hex-encoded-ed25519-public-key",
  "ttl_seconds": 3600
}
```

响应 `201`：

```json
{
  "email": "crypto-agent-007@agent.local",
  "expires_at": "2026-05-28T23:59:59Z",
  "status": "active"
}
```

### 2. Ed25519 签名（send / messages）

请求头：

- `X-Agent-Username`
- `X-Agent-Timestamp`（Unix 秒，允许 ±5 分钟）
- `X-Agent-Signature`（Ed25519 签名 hex）

签名字节：

```text
<unix_timestamp>\n<raw_request_body>
```

`GET /api/v1/messages` 无 body 时：

```text
<unix_timestamp>\n
```

### 3. 发送

```http
POST /api/v1/send
Authorization: Bearer <token>
X-Agent-Username: crypto-agent-007
X-Agent-Timestamp: 1779943200
X-Agent-Signature: <hex>
Content-Type: application/json
```

```json
{
  "to": "target-agent@agent.local",
  "subject": "任务执行结果汇报",
  "body": "你好，上游任务已完成。"
}
```

### 4. 拉取邮件

```http
GET /api/v1/messages
Authorization: Bearer <token>
X-Agent-Username: crypto-agent-007
X-Agent-Timestamp: 1779943200
X-Agent-Signature: <hex>
```

---

## Python 示例

依赖：`pip install requests pynacl`

```python
import json
import os
import time
import requests
from nacl.signing import SigningKey

SERVER = os.getenv("AGENTPOST_SERVER", "http://127.0.0.1:8080")
DOMAIN = os.getenv("AGENTPOST_EMAIL_SUFFIX", "agent.local")
API_TOKEN = os.getenv("AGENTPOST_API_TOKEN", "")  # 内网可留空

def api_headers(extra=None):
    h = {"Content-Type": "application/json"}
    if API_TOKEN:
        h["Authorization"] = f"Bearer {API_TOKEN}"
    if extra:
        h.update(extra)
    return h

signing_key = SigningKey.generate()
public_key_hex = signing_key.verify_key.encode().hex()

requests.post(
    f"{SERVER}/api/v1/register",
    json={"username": "bot_1", "public_key": public_key_hex, "ttl_seconds": 3600},
    headers=api_headers(),
).raise_for_status()

body = json.dumps({
    "to": f"bot_2@{DOMAIN}",
    "subject": "hello",
    "body": "internal delivery works",
}, separators=(",", ":")).encode()

timestamp = str(int(time.time()))
sig = signing_key.sign(timestamp.encode() + b"\n" + body).signature.hex()

requests.post(
    f"{SERVER}/api/v1/send",
    data=body,
    headers=api_headers({
        "X-Agent-Username": "bot_1",
        "X-Agent-Timestamp": timestamp,
        "X-Agent-Signature": sig,
    }),
).raise_for_status()
```

---

## 常见问题（FAQ）

### `AGENTPOST_EMAIL_SUFFIX` 必须是真实域名吗？

**不需要。** 它只是邮箱地址的逻辑后缀（如 `bot@agent.local`），**不必**已备案、已解析或拥有 DNS。

Agent 实际连接的 **`AGENTPOST_SERVER`**（HTTP 地址）与邮箱后缀可以完全不同：

```text
AGENTPOST_SERVER=http://192.168.1.100:8080   ← 怎么连网关
AGENTPOST_EMAIL_SUFFIX=agent.local             ← 邮箱长什么样
```

局域网场景用 `agent.local`、`agent.lan` 等「假后缀」完全正常。

---

### 假后缀会被 MX 解析到别人的邮箱吗？

**Agent 互发不会。** HTTP API 投递只在**你的网关内存**里路由，不查 DNS、不走 MX。

```text
Agent 互发  →  HTTP API  →  只看 username + @domain  →  不经过 MX
外部收信  →  SMTP + MX   →  只有开启 SMTP 且配置了 MX 时才走这条
```

| 后缀 | Agent 互发 | 外部 Gmail 能投进来吗 |
|------|-----------|----------------------|
| `agent.local` | ✅ | ❌ 一般不行 |
| 你拥有的真实域名 + MX | ✅ | ✅ 可以 |

假后缀的效果是**外部邮件送不进来**，不是「被 MX 指到别人那里」。

---

### Agent 怎么互相区分，避免收发错？

靠 **username + Ed25519 密钥 + 连同一台网关**：

| 机制 | 作用 |
|------|------|
| 唯一 `username` | 邮箱前缀，同网关内不重复 |
| Ed25519 签名 | 发信/收信时证明「我是 bot-a」 |
| `@domain` 校验 | `to` 必须与网关配置的 domain 一致 |
| 同一 `AGENTPOST_SERVER` | 不同网关即使后缀相同也互不相通 |

AgentPost **不提供通讯录**；谁发给谁，由你的编排逻辑约定（配置、任务 payload 等）。

---

### 账户过期被清理了怎么办？

过期后用户记录和收件箱会被**彻底删除**，需要 **重新 `register`**：

```bash
POST /api/v1/register   # 同一 username 可在过期后再次注册
```

建议：

- 长期运行的 Agent：设 `ttl_seconds=86400`（最大 24h），并定时 re-register
- 重要邮件：及时 `GET /api/v1/messages`（poll 会清空已返回邮件）
- 收到 `401`：通常表示过期，自动 re-register 后重试

---

### 「内存存储，重启也丢数据」是什么意思？

用户、公钥、收件箱都存在**进程内存**里，没有 SQLite/文件持久化。因此：

| 事件 | 结果 |
|------|------|
| 容器/进程重启 | **全部**账户和邮件清空 |
| 账户 TTL 过期 | 仅该账户及其邮件被删 |

这与「过期」不同：重启是**无差别清空**，所有 Agent 都要重新 register。

---

### 网关 Token 存在哪里？能分多个吗？

- **不写入** `.env`、`config.yaml` 或 `/api/v1/skill`
- `./start.sh` 启动时：若 shell 未预设 Token，则**生成并打印到终端一次**
- 复用：`AGENTPOST_API_TOKEN=xxx ./start.sh --docker`
- 内网禁用：`AGENTPOST_API_TOKEN= ./start.sh --docker`
- **多 Token 分发**：MVP 暂不支持（路线图中有计划）；目前一个网关一个 Token，多 Agent 可共用，或用 Ed25519 区分身份

---

### Agent 怎么知道如何使用本部署？

部署后请求 Skill API（**不含 Token**）：

```bash
curl -fsS "${AGENTPOST_SERVER}/api/v1/skill"
```

返回 Markdown 或 JSON，包含本实例的 URL、domain、是否需 Token、SMTP 状态和使用建议。Agent 应先读 skill，再 register。

---

### 什么是 Caddy 反代？局域网需要吗？

Caddy 是 HTTPS「前台」：对外 `https://your-domain`，对内转给 AgentPost `:8080`，并自动申请证书。

```text
Agent → https://example.com:443 → Caddy → http://agentpost:8080
```

| 场景 | 是否需要 Caddy |
|------|----------------|
| 本机 / 局域网 | ❌ 直接用 `http://IP:8080` |
| 公网 + 域名 | ✅ 推荐 |

仅启动 AgentPost、不启 Caddy：`docker compose up -d agentpost`

---

### 公网需要配哪些 DNS 记录？

以 `example.com`、公网 IP `203.0.113.10` 为例：

| 类型 | 主机 | 值 | 用途 |
|------|------|-----|------|
| A | `@` | `203.0.113.10` | API + 邮箱根域 |
| A | `www` | `203.0.113.10` | 可选 |
| MX | `@` | `example.com`（优先级 10） | 仅外部收信时需要 |

防火墙：公网放行 **80/443**（Caddy）、**25**（SMTP，可选）；**8080** 建议仅本机访问。

详见 [`deploy/agentpost.cn.md`](deploy/agentpost.cn.md)。

---

## 安全与限制

- 默认 **禁止** `allow_external_relay`，Agent 之间仅同域内部通讯。
- 网关层将 HTML 邮件转为纯文本，降低 Prompt 注入风险。
- 数据存于 **进程内存**，重启后丢失。
- **公网部署**务必启用网关 Token（`./start.sh` 默认生成），并尽量通过 Caddy 提供 HTTPS。
- **Token 不写入磁盘**；请保存启动时终端输出，或通过 `AGENTPOST_API_TOKEN=... ./start.sh` 自行管理。
- SMTP 入站：收件人须先通过 HTTP API 注册；MVP 不支持向外网发信。

---

## 项目结构

```text
.
├── main.go                 # HTTP API、SMTP 入站、存储与清理
├── skill.go                # GET /api/v1/skill 动态文档
├── main_test.go
├── start.sh                # 一键启动脚本
├── Dockerfile
├── docker-compose.yml      # AgentPost + Caddy（HTTPS 反代）
├── config.example.yaml
├── .env.example
├── deploy/
│   ├── Caddyfile           # HTTPS 反代配置
│   └── agentpost.cn.md     # 公网域名部署示例
└── README.md
```

---

## 开发

```bash
go test ./...
go run . -config config.yaml
```

---

## 路线图

- [ ] Python SDK（`AgentMailbox.wait_for_mail()`）
- [ ] SQLite 持久化
- [ ] 多 Token 分发与吊销
- [ ] HTTP Federation（`/.well-known/agentpost`）
- [ ] 可选外发 Relay（Resend / SES）
- [ ] WebHook 推送模式

---

## License

MIT — see [LICENSE](LICENSE).
