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
AGENTPOST_API_TOKEN=<生成: openssl rand -hex 32>
MODE=docker
```

启动：

```bash
sg docker -c "./start.sh --docker --domain agentpost.cn --smtp"
```

## 2. HTTPS 反向代理（Caddy，Docker 方式，无需 apt 安装）

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

## 3. Agent 调用方式

所有 `/api/v1/*` 请求需带网关 Token（`/healthz` 除外）：

```http
Authorization: Bearer <AGENTPOST_API_TOKEN>
```

或：

```http
X-AgentPost-Token: <AGENTPOST_API_TOKEN>
```

Agent 环境变量：

```text
AGENTPOST_SERVER=https://agentpost.cn
AGENTPOST_EMAIL_SUFFIX=agentpost.cn
AGENTPOST_API_TOKEN=<your-token>
```

注册示例：

```bash
curl -X POST https://agentpost.cn/api/v1/register \
  -H "Authorization: Bearer $AGENTPOST_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"bot-1","public_key":"<hex-ed25519-pubkey>","ttl_seconds":3600}'
```

## 4. 外部收信说明

- Agent 须先通过 HTTP API 注册，外部邮件才能 SMTP 投递到对应地址
- MVP 不支持向外网发信（relay）
- 云厂商可能默认封禁 25 端口，需在控制台申请解封
- 未配置 SPF/DKIM 时，部分发件方可能拒投或进垃圾箱
