# agentpost.cn 部署指南

## DNS 记录（在域名服务商控制台添加）

假设服务器公网 IP 为 `203.10.99.50`（请替换为你的实际 IP）：

| 类型 | 主机记录 | 记录值 | 说明 |
|------|----------|--------|------|
| A | `@` | `203.10.99.50` | API + 邮箱域名 |
| A | `www` | `203.10.99.50` | 可选，Caddy 会重定向到根域 |
| MX | `@` | `agentpost.cn` | 优先级 `10`，外部邮件入站 |

> 邮箱地址格式：`bot-name@agentpost.cn`

## 防火墙 / 安全组

| 端口 | 用途 |
|------|------|
| 80 | Caddy HTTP（自动跳转 HTTPS + 申请证书） |
| 443 | Caddy HTTPS（Agent API） |
| 25 | SMTP 入站（外部邮箱投递） |
| 8080 | 可选，建议仅本机访问（Caddy 反代） |

## 1. 配置 AgentPost

```bash
cd /home/kxu/AgentPost
cp .env.example .env
```

编辑 `.env`：

```bash
AGENTPOST_DOMAIN=agentpost.cn
AGENTPOST_ENABLE_SMTP=1
AGENTPOST_SMTP_PUBLISH_PORT=25
AGENTPOST_API_TOKEN=$(openssl rand -hex 32)   # 仅当前 shell；不要写入 .env
AGENTPOST_PUBLIC_URL=https://agentpost.cn
MODE=docker
```

启动后 Token 会打印到终端（若未在 shell 中预设）。**不要将 Token 写入 `.env` 或 skill 文件。**

```bash
./start.sh --docker --domain agentpost.cn --smtp
```

## 2. HTTPS 反向代理（Caddy，Docker 方式）

AgentPost 的 `docker-compose.yml` 已包含 Caddy 服务，与 AgentPost 一起启动即可：

```bash
sg docker -c "cd /home/kxu/AgentPost && docker compose up -d"
```

Caddy 会自动申请 Let's Encrypt 证书（需 DNS A 记录已生效且 80/443 已放行）。

验证：

```bash
curl https://agentpost.cn/healthz
# {"status":"ok"}
```

> 若坚持用系统包安装 Caddy（Ubuntu 22.04 默认源无 caddy），需先添加官方 apt 源：
> https://caddyserver.com/docs/install#debian-ubuntu-raspbian

## 3. Agent 获取 skill 与调用 API

先拉取本部署的使用说明（**不含 Token**）：

```bash
curl -fsS https://agentpost.cn/api/v1/skill
curl -fsS -H 'Accept: application/json' https://agentpost.cn/api/v1/skill
```

所有 `/api/v1/*` 请求（除 `/skill`）需带网关 Token：

```http
Authorization: Bearer <AGENTPOST_API_TOKEN>
```

Token 在 `./start.sh` 启动时打印到终端，需由运维安全分发给 Agent。

Agent 环境变量：

```text
AGENTPOST_SERVER=https://agentpost.cn
AGENTPOST_EMAIL_SUFFIX=agentpost.cn
AGENTPOST_API_TOKEN=<部署时终端打印的值>
```

## 4. 外部收信说明

- Agent 须先通过 HTTP API 注册，外部邮件才能 SMTP 投递到对应地址
- MVP 不支持向外网发信（relay）
- 云厂商可能默认封禁 25 端口，需在控制台申请解封
- 未配置 SPF/DKIM 时，部分发件方可能拒投或进垃圾箱
