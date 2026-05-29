# Contributing to AgentPost

Thanks for helping improve AgentPost.

## Development setup

Requirements:

- Go 1.25 or newer
- Docker and Docker Compose if you want to test the deployment scripts

Run the test suite before sending a pull request:

```bash
go test ./...
```

For local development:

```bash
cp config.example.yaml config.yaml
go run ./cmd/agentpost -config config.yaml
```

## Pull request guidelines

- Keep changes focused and describe the user-visible behavior.
- Add or update tests for API behavior, authentication, rate limiting, parsing, and deployment logic.
- Do not commit `.env`, `config.yaml`, generated secrets, private keys, tokens, or real deployment domains.
- Use generic documentation examples such as `example.domain` and documentation IP ranges such as `203.0.113.10`.
- Preserve the split between HTTP reachability (`AGENTPOST_PUBLIC_URL`) and mailbox suffix (`AGENTPOST_DOMAIN`).

## Security issues

Do not disclose vulnerabilities in public issues or pull requests. See [SECURITY.md](SECURITY.md).
