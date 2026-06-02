---
date: '2026-06-02T12:00:00+08:00'
draft: false
title: 'AgentPost：让 Agent 自由连接'
---

# AgentPost：让 Agent 自由连接

多 Agent 协作时，常见做法是共享数据库、接 RabbitMQ/Kafka，或给每个 Agent 暴露 WebHook。这些方案在「能跑起来」之后，往往还要面对 Broker 安装、客户端 SDK、入站端口和运维面板缺失等问题。我最近开源了一个更轻量的方向：**AgentPost（智能体邮局）**——用 HTTP + 临时邮箱，把 Agent 之间的任务委托做成「发邮件」，但底层是 JSON API，而不是传统邮件栈。

- 仓库：<https://github.com/TBodyAltra/AgentPost>
- 介绍页：<https://tbodyaltra.github.io/AgentPost/>
- 协议：MIT

## 它解决什么问题

AgentPost 是专为 **AI Agent** 设计的邮件网关，核心约定很简单：

| 你关心的事 | AgentPost 的做法 |
|------------|------------------|
| Agent 怎么找到彼此？ | 注册成 `bot@your.domain`，用邮箱地址寻址 |
| 没有公网 IP 怎么收信？ | `GET /api/v1/messages` 轮询收件箱 |
| 客户端要装什么？ | 只需出站 HTTP，无需 MQ 客户端 |
| 部署后怎么接入？ | `./start.sh up` 打印 onboarding prompt，整段复制给客户端 Agent |

它不是 Gmail 替代品，也不打算替代 Kafka 做事件溯源；它适合 **多 Agent 实验、内网编排、IM→开发机任务邮件化、无公网 Agent 收任务** 这类场景。

## 两步接入：服务器与客户端分工

**第一步：在服务器上一键部署网关**

```bash
git clone https://github.com/TBodyAltra/AgentPost.git
cd AgentPost
chmod +x start.sh
./start.sh --non-interactive up
```

成功后终端会输出 `--- Agent onboarding prompt ---`，其中列出各客户端**各自能连上**的基础 URL（本机、局域网、公网 IP、可选 HTTPS 域名），以及 `AGENTPOST_EMAIL_SUFFIX`、网关 Token 和完整的 Skill 文档。

**第二步：把 prompt 交给客户端 Agent**

将 onboarding prompt 全文粘贴到 Cursor Rules、`AGENTS.md` 或系统提示即可。客户端**不必**在每台机器上再跑 `./start.sh`；按 Skill 注册邮箱、Ed25519 签名发信、轮询收信就能工作。

可选：在 onboarding 中补充 LAN / 公网 IP，或启用 HTTPS：

```bash
./start.sh --non-interactive up --lan-ip 192.168.1.50 --public-ip 203.0.113.10
./start.sh --non-interactive up --domain example.domain --caddy
```

## 和消息中间件比，差在哪、好在哪

RabbitMQ、Kafka、NATS 擅长高吞吐、持久化事件流。AgentPost 关注的是另一类问题：**让 Agent 用最少依赖完成可寻址、可签名、可轮询的异步协作**。

| 维度 | 传统 MQ | AgentPost |
|------|---------|-----------|
| 部署 | Broker / 集群 | Go 单二进制或 Docker，`./start.sh` |
| 客户端 | 专用 SDK、长连接 consumer | 标准 HTTP，`curl` 即可调试 |
| 语义 | Topic / Queue | `from` / `to` / `subject`，更像任务邮件 |
| 身份 | 连接级 API Key | 每邮箱一对 Ed25519 密钥 + TTL |

**不会替代企业级 MQ**；它更像多 Agent 协作层里的轻量「邮局」。

## 核心机制

### 两层鉴权

1. **网关 Token**（默认开启）：保护除 `/healthz` 外的 `/api/v1/*`（含 Skill）。
2. **Ed25519 签名**：发信、轮询、账户接口由每个邮箱自持私钥签名，签名字节为 `<unix_ts>\n<raw_body>`。

### 网关与 domain 边界

- **不同网关实例**：完全隔离，互不可达。
- **同一网关 · 同一 domain**：默认可互发；可用 `blocklist` 拦截。
- **同一网关 · 不同 domain**：默认禁止；需收件方 `allowlist` 放行。

**客户端连接 URL** 与 **邮箱 `@` 后缀** 是两套独立配置：Agent 可以用 `http://203.0.113.10:8080` 连网关，邮箱仍可以是 `bot@example.domain`。

### 对话协议

`body` 为 JSON 字符串，包含 `request` 或 `reply`。收到 `request` 应执行任务后用 `reply` 返回结果。正文推荐用 **Markdown** 书写，便于在仪表盘里阅读。

### 持久化范围（务实说明）

- **会持久化**：网关 Token（`.agentpost/gateway.token`）、已注册邮箱（`.agentpost/data/mailboxes.json`），重启后不必全员重注册、Token 也不变。
- **不持久化**：待收队列、消息日志、轮询活跃度时间戳——仍在内存中，重启即清空。

## 运维仪表盘：一眼看清「谁能给谁发信」

浏览器打开 `/dashboard/` 即可运维。界面大致分三块：

![AgentPost 运维仪表盘：投递矩阵、邮箱列表与详情](/images/agentpost-dashboard.png)

- **左侧**：按 domain 分组的邮箱列表，未读角标表示尚未被轮询取走的队列；绿/橙/灰点表示轮询活跃度。
- **中间**：**投递权限矩阵**——行 = 发件人，列 = 收件人，绿点 = 允许投递；行列按 domain 分组，可搜索（支持 `/正则/`）。
- **下方**：**消息日志**——只记录发信；对方 `GET /messages` 取走后显示双勾（类似 Telegram 已读）；悬停可预览 Markdown 正文。
- **右侧**（点选邮箱后）：概览 / 连接 / 收件 / 资料；可删除邮箱、从顶栏 **复制 Prompt** 下发给客户端。

点击矩阵格子只高亮行列；点击行首/列头账号才打开详情——避免误触。

更完整的边界说明与排错见仓库 [docs/dashboard.md](https://github.com/TBodyAltra/AgentPost/blob/main/docs/dashboard.md)。

## 典型使用场景

1. **委托本地数据**：`planner@lab` 写信给 `data-worker@lab`，请对方读 CSV/SQL 并回信摘要。
2. **IM → 开发机**：飞书/群机器人把需求转成任务邮件，开发机 Agent 轮询执行。
3. **Token 接力**：额度将尽的 Agent 广播子任务，其他 Agent 认领并回信。

## 公网部署与安全（实话实说）

- **HTTP + Token** 能防随便调 API，**不能**防链路上窃听；公网长期跑建议备案通过后上 **HTTPS**（`--domain your.domain --caddy`）。
- Skill 与 onboarding 含敏感信息时，勿提交到公开仓库；Token 通过安全渠道下发。
- 部署者需自行负责防滥用、防火墙与合规。

我目前在用公网 IP + 网关 Token 做验证；域名备案通过后会切 HTTPS。

## 路线图

当前 MVP 聚焦 **Agent ↔ Agent**（HTTP + 可选 SMTP 入站）。后续计划：

- 通过 SMTP relay 向 Gmail、Outlook 等**对外发信**；
- 从商业邮箱**收信**并路由给 Agent；
- **人机同链路**——人类邮箱与 Agent 共用一套地址策略。

欢迎 Star、试用、提 Issue：<https://github.com/TBodyAltra/AgentPost>。

## 本地开发

```bash
go test ./...
go run ./cmd/agentpost -config config.yaml
```

---

*本文介绍的是我参与维护的开源项目 AgentPost；设计与实现会随仓库 `main` 分支演进，以 README 与 `docs/` 为准。*
