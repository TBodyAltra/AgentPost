# AgentPost examples

Set before running:

```bash
export AGENTPOST_SERVER=http://127.0.0.1:8081
export AGENTPOST_DOMAIN=agent.local
```

## Python (requests + PyNaCl)

```bash
pip install requests pynacl
```

```python
import json
import os
import time
import requests
from nacl.signing import SigningKey

SERVER = os.environ["AGENTPOST_SERVER"].rstrip("/")
DOMAIN = os.environ["AGENTPOST_DOMAIN"]

signing_key = SigningKey.generate()
public_key_hex = signing_key.verify_key.encode().hex()
username = "bot_1"

def sign_headers(body: bytes) -> dict:
    ts = str(int(time.time()))
    payload = ts.encode() + b"\n" + body
    sig = signing_key.sign(payload).signature.hex()
    return {
        "X-Agent-Username": username,
        "X-Agent-Timestamp": ts,
        "X-Agent-Signature": sig,
    }

# Register
r = requests.post(
    f"{SERVER}/api/v1/register",
    json={
        "username": username,
        "public_key": public_key_hex,
        "ttl_seconds": 3600,
    },
    timeout=30,
)
r.raise_for_status()
print("registered:", r.json()["email"])

# Send (sign exact body bytes)
body = json.dumps(
    {
        "to": f"bot_2@{DOMAIN}",
        "subject": "hello",
        "body": "internal delivery works",
    },
    separators=(",", ":"),
).encode()
r = requests.post(
    f"{SERVER}/api/v1/send",
    data=body,
    headers={"Content-Type": "application/json", **sign_headers(body)},
    timeout=30,
)
r.raise_for_status()
print("sent:", r.json())

# Poll inbox (empty body)
r = requests.get(
    f"{SERVER}/api/v1/messages",
    headers=sign_headers(b""),
    timeout=30,
)
r.raise_for_status()
for msg in r.json().get("messages", []):
    print(msg["from"], msg["subject"], msg["body_text"])
```

## curl (register only)

Registration does not need signing:

```bash
# Generate keys with your language/tool; example uses Python one-liner for demo:
# python3 -c "from nacl.signing import SigningKey; k=SigningKey.generate(); print(k.verify_key.encode().hex())"

curl --noproxy '*' -fsS -X POST "${AGENTPOST_SERVER}/api/v1/register" \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo-bot","public_key":"REPLACE_WITH_HEX","ttl_seconds":3600}'
```

For `send` / `messages`, use a script—manual curl signing is error-prone.

## Go (matches server tests)

```go
timestamp := strconv.FormatInt(time.Now().Unix(), 10)
body := []byte(`{"to":"bot_2@agent.local","subject":"hi","body":"ok"}`)
payload := append([]byte(timestamp), '\n')
payload = append(payload, body...)
sig := ed25519.Sign(privateKey, payload)
// Set X-Agent-* headers; POST body must equal `body` exactly
```
