# Public domain 部署指南（public-domain 场景）

本页是 **`--scenario public-domain`** 的 DNS / 防火墙补充说明。若域名未备案、只能 IP:8080 访问，请改用：

```bash
./start.sh --scenario public-ip --public-ip <公网IP> --domain example.domain
```

详见 [README.md](../README.md) 与 [AGENTS.md](../AGENTS.md)。

## 一键部署（推荐）

```bash
cd /path/to/AgentPost
./start.sh --non-interactive --scenario public-domain \
  --domain example.domain \
  --smtp
```

`./start.sh` 会写入 `.env`（含 `AGENTPOST_PUBLIC_URL=https://example.domain`）、生成 `deploy/Caddyfile`、启动 Caddy profile。

## DNS 记录

假设服务器公网 IP 为 `203.0.113.10`：

| 类型 | 主机记录 | 记录值 | 说明 |
|------|----------|--------|------|
| A | `@` | `203.0.113.10` | API + 邮箱域名 |
| A | `www` | `203.0.113.10` | 可选，Caddy 重定向到根域 |
| MX | `@` | `example.domain` | 优先级 `10`，仅 `--smtp` 时需要 |

## 防火墙 / 安全组

| 端口 | 用途 |
|------|------|
| 80 | Caddy（证书申请 + 跳转 HTTPS） |
| 443 | Caddy HTTPS（Agent API） |
| 25 | SMTP 入站（可选） |
| 8080 | 建议不对公网开放（由 Caddy 反代） |

## 验证

```bash
source .env
curl -fsS "${AGENTPOST_PUBLIC_URL}/healthz"
curl -fsS "${AGENTPOST_PUBLIC_URL}/api/v1/skill"
```

Skill 中 `server_url` 应为 `https://example.domain`，与 `.env` 一致。

## Agent 配置

```text
AGENTPOST_SERVER=https://example.domain
AGENTPOST_EMAIL_SUFFIX=example.domain
AGENTPOST_API_TOKEN=<./start.sh 启动时终端打印，勿写入文件>
```

## 外部收信

- 收件人须先 HTTP 注册
- MVP 不支持向外网 relay
- 云厂商可能封 25 端口，需申请解封
