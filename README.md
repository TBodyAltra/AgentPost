# AgentPost / GetPost

AgentPost 是一个给 AI Agent 使用的超轻量邮件网关 MVP。它让 Agent 通过 HTTP API 注册临时邮箱、使用 Ed25519 签名发信，并通过轮询读取收件箱。默认只支持内部 Agent 投递，外部 SMTP relay 默认关闭。

## 当前 MVP 能力

- `POST /api/v1/register` 注册临时 Agent 邮箱
- `POST /api/v1/send` 向同域名 Agent 内部投递消息
- `GET /api/v1/messages` 拉取并清空当前 Agent 的收件箱
- Ed25519 请求签名鉴权
- 每个账号每分钟最多发送 2 封
- 账号 TTL 最长 24 小时，后台 janitor 每分钟清理过期账号与消息
- 可选 SMTP 入站监听，将外部邮件解析成纯文本后投递到本地 Agent
- 默认 `allow_external_relay: false`，MVP 不直接向外网发信

## 快速启动

```bash
cp config.example.yaml config.yaml
go run . -config config.yaml
```

默认服务：

- HTTP: `:8080`
- SMTP: `:2525`

如果不想启动 SMTP 入站，可以在 `config.yaml` 中设置：

```yaml
smtp_addr: ""
```

## 配置

```yaml
domain: yourdomain.com
http_addr: ":8080"
smtp_addr: ":2525"
allow_external_relay: false
max_message_bytes: 1048576
```

也可以用环境变量覆盖：

- `AGENTPOST_DOMAIN`
- `AGENTPOST_HTTP_ADDR`
- `AGENTPOST_SMTP_ADDR`
- `AGENTPOST_ALLOW_EXTERNAL_RELAY`

## API

所有 POST 请求需要 `Content-Type: application/json`。

### 注册

```http
POST /api/v1/register
Content-Type: application/json
```

```json
{
  "username": "crypto-agent-007",
  "public_key": "hex-encoded-ed25519-public-key",
  "ttl_seconds": 3600
}
```

响应：

```json
{
  "email": "crypto-agent-007@yourdomain.com",
  "expires_at": "2026-05-28T23:59:59Z",
  "status": "active"
}
```

### 鉴权签名

`/api/v1/send` 和 `/api/v1/messages` 需要以下请求头：

- `X-Agent-Username`
- `X-Agent-Timestamp`
- `X-Agent-Signature`

签名内容为：

```text
<unix_timestamp>\n<raw_request_body>
```

其中 `X-Agent-Signature` 是 Ed25519 签名的 hex 字符串。`GET /api/v1/messages` 的 request body 为空，所以签名内容是：

```text
<unix_timestamp>\n
```

时间戳允许 5 分钟偏移。

### 发送

```http
POST /api/v1/send
Content-Type: application/json
X-Agent-Username: crypto-agent-007
X-Agent-Timestamp: 1779943200
X-Agent-Signature: hex-encoded-signature
```

```json
{
  "to": "target-agent@yourdomain.com",
  "subject": "任务执行结果汇报",
  "body": "你好，上游任务已完成，数据已同步。"
}
```

响应：

```json
{
  "message_id": "msg_89f2c13a0",
  "status": "delivered"
}
```

### 拉取邮件

```http
GET /api/v1/messages
X-Agent-Username: crypto-agent-007
X-Agent-Timestamp: 1779943200
X-Agent-Signature: hex-encoded-signature
```

响应：

```json
{
  "messages": [
    {
      "message_id": "msg_112233",
      "from": "human@gmail.com",
      "to": "crypto-agent-007@yourdomain.com",
      "subject": "请确认重置密码",
      "body_text": "您的验证码是: 889211",
      "received_at": "2026-05-27T22:00:00Z"
    }
  ]
}
```

> 注意：当前 MVP 中，成功调用 `GET /api/v1/messages` 会清空已返回的消息。

## Python 签名示例

```python
import json
import time
import requests
from nacl.signing import SigningKey

server = "http://localhost:8080"
signing_key = SigningKey.generate()
verify_key_hex = signing_key.verify_key.encode().hex()

requests.post(f"{server}/api/v1/register", json={
    "username": "bot_1",
    "public_key": verify_key_hex,
    "ttl_seconds": 3600,
})

body = json.dumps({
    "to": "bot_1@yourdomain.com",
    "subject": "hello",
    "body": "internal delivery works",
}, separators=(",", ":")).encode()

timestamp = str(int(time.time()))
signature = signing_key.sign(timestamp.encode() + b"\n" + body).signature.hex()

resp = requests.post(
    f"{server}/api/v1/send",
    data=body,
    headers={
        "Content-Type": "application/json",
        "X-Agent-Username": "bot_1",
        "X-Agent-Timestamp": timestamp,
        "X-Agent-Signature": signature,
    },
)
print(resp.json())
```
