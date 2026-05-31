#!/usr/bin/env bash
# AgentPost one-click launcher (native Go or Docker).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

ENV_FILE="${ENV_FILE:-.env}"
CONFIG_FILE="${CONFIG_FILE:-config.yaml}"
CADDYFILE="${CADDYFILE:-deploy/Caddyfile}"
MODE="${MODE:-auto}" # auto | docker | native
HTTP_PORT="${AGENTPOST_HTTP_PORT:-8080}"
DOMAIN="${AGENTPOST_DOMAIN:-agent.local}"
SCENARIO="${AGENTPOST_SCENARIO:-}"
PUBLIC_URL="${AGENTPOST_PUBLIC_URL:-}"
ENABLE_SMTP="${AGENTPOST_ENABLE_SMTP:-0}"
ENABLE_CADDY="${AGENTPOST_ENABLE_CADDY:-0}"
REQUIRE_TOKEN="${AGENTPOST_REQUIRE_TOKEN:-}"
SMTP_PORT="${AGENTPOST_SMTP_PORT:-2525}"
INTERACTIVE=1
CONFIGURE_ONLY=0
TOKEN_GENERATED=0
LAN_IP=""
PUBLIC_IP=""
TOKEN_POLICY="auto" # auto | yes | no
SMTP_FLAG_SET=0

usage() {
  cat <<'EOF'
Usage: ./start.sh [command] [options]

Commands:
  up          Start AgentPost (default)
  configure   Apply a deployment scenario and write .env / config (no start)
  stop        Stop Docker deployment
  status      Show health and endpoint info
  logs        Follow Docker logs (docker mode only)
  help        Show this help

Deployment modes (--scenario):
  http (default)     HTTP on a host:port; set --public-url to the address clients use
  public-domain      HTTPS on a real domain (Caddy + Let's Encrypt)

  Legacy aliases (still supported): local, lan, public-ip — they only preset --public-url.

Options:
  --scenario NAME       http | public-domain (or legacy local | lan | public-ip)
  --domain NAME         Mailbox @ suffix (e.g. agent.local, example.domain)
  --public-url URL      AGENTPOST_PUBLIC_URL / skill server_url (how clients reach HTTP)
  --lan-ip IP           Shortcut for --public-url http://LAN_IP:PORT (legacy scenario=lan)
  --public-ip IP        Shortcut for --public-url http://PUBLIC_IP:PORT (legacy public-ip)
  --http-port PORT      Host HTTP port (default: 8080)
  --smtp                Enable SMTP inbound
  --no-smtp             Disable SMTP inbound
  --token               Require gateway token (default for public scenarios)
  --no-token            Disable gateway token (default for local/lan)
  --caddy               Enable Caddy HTTPS reverse proxy
  --no-caddy            Disable Caddy
  --docker              Force Docker Compose
  --native              Force local "go run"
  --configure-only      Write config and exit (alias: configure command)
  --non-interactive     Fail instead of prompting when inputs are missing

Environment:
  Reads .env if present (AGENTPOST_API_TOKEN is never loaded from .env).
  Set AGENTPOST_API_TOKEN in the shell before ./start.sh to reuse a token.
  See .env.example and AGENTS.md.

Examples:
  ./start.sh up                                 # http://127.0.0.1:8080, no token
  ./start.sh up --public-url http://203.0.113.10:8080 --token
  ./start.sh configure --non-interactive --public-url http://192.168.1.100:8080
  ./start.sh --scenario public-domain --domain example.domain --smtp
  AGENTPOST_API_TOKEN=$(openssl rand -hex 32) ./start.sh --scenario public-domain --domain example.domain
  # legacy: ./start.sh --scenario local | lan --lan-ip … | public-ip --public-ip …
EOF
}

log() {
  printf '[agentpost] %s\n' "$*"
}

warn() {
  printf '[agentpost] warning: %s\n' "$*" >&2
}

die() {
  printf '[agentpost] error: %s\n' "$*" >&2
  exit 1
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

is_tty() {
  [[ -t 0 ]]
}

prompt() {
  local message="$1"
  local default="${2:-}"
  local reply
  if [[ -n "$default" ]]; then
    printf '%s [%s]: ' "$message" "$default" >&2
  else
    printf '%s: ' "$message" >&2
  fi
  read -r reply
  if [[ -z "$reply" ]]; then
    printf '%s' "$default"
  else
    printf '%s' "$reply"
  fi
}

prompt_yes_no() {
  local message="$1"
  local default="${2:-n}"
  local hint="y/N"
  [[ "$default" == "y" || "$default" == "yes" ]] && hint="Y/n"
  local reply
  printf '%s (%s): ' "$message" "$hint" >&2
  read -r reply
  reply="${reply:-$default}"
  case "${reply,,}" in
    y|yes) return 0 ;;
    *) return 1 ;;
  esac
}

detect_lan_ip() {
  if have_cmd hostname; then
    hostname -I 2>/dev/null | awk '{print $1; exit}'
  fi
}

detect_public_ip() {
  if have_cmd curl; then
    curl -fsS --max-time 4 https://api.ipify.org 2>/dev/null || \
      curl -fsS --max-time 4 https://ifconfig.me/ip 2>/dev/null || true
  fi
}

normalize_bool() {
  case "${1,,}" in
    1|true|yes|on) printf '1' ;;
    *) printf '0' ;;
  esac
}

url_host() {
  local url="$1" host
  host="${url#*://}"
  host="${host%%/*}"
  host="${host%%:*}"
  printf '%s' "$host"
}

url_is_loopback_or_private() {
  local host="$1"
  case "$host" in
    localhost|127.0.0.1|::1) return 0 ;;
    10.*|192.168.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*) return 0 ;;
    *) return 1 ;;
  esac
}

public_url_suggests_token() {
  local host
  host="$(url_host "$PUBLIC_URL")"
  [[ -n "$host" ]] || return 1
  if url_is_loopback_or_private "$host"; then
    return 1
  fi
  return 0
}

normalize_scenario() {
  case "$SCENARIO" in
    ""|http) SCENARIO=http ;;
    local|lan|public-ip|public-domain) ;;
    *) die "unknown scenario: ${SCENARIO}. Use http | public-domain (or legacy local | lan | public-ip)" ;;
  esac
}

scenario_label() {
  case "$1" in
    http) printf 'http (host:port)' ;;
    local) printf 'local (legacy → http://127.0.0.1)' ;;
    lan) printf 'lan (legacy → LAN IP)' ;;
    public-ip) printf 'public-ip (legacy → public IP)' ;;
    public-domain) printf 'public-domain (HTTPS domain)' ;;
    *) printf '%s' "$1" ;;
  esac
}

load_env_file() {
  local saved_token=""
  if [[ -n "${AGENTPOST_API_TOKEN:-}" ]]; then
    saved_token="$AGENTPOST_API_TOKEN"
  fi
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    set -a
    source "$ENV_FILE"
    set +a
    SCENARIO="${AGENTPOST_SCENARIO:-$SCENARIO}"
    DOMAIN="${AGENTPOST_DOMAIN:-$DOMAIN}"
    HTTP_PORT="${AGENTPOST_HTTP_PORT:-$HTTP_PORT}"
    PUBLIC_URL="${AGENTPOST_PUBLIC_URL:-$PUBLIC_URL}"
    ENABLE_SMTP="${AGENTPOST_ENABLE_SMTP:-$ENABLE_SMTP}"
    ENABLE_CADDY="${AGENTPOST_ENABLE_CADDY:-$ENABLE_CADDY}"
    REQUIRE_TOKEN="${AGENTPOST_REQUIRE_TOKEN:-$REQUIRE_TOKEN}"
    SMTP_PORT="${AGENTPOST_SMTP_PORT:-$SMTP_PORT}"
    MODE="${MODE:-auto}"
  fi
  if [[ -n "$saved_token" ]]; then
    AGENTPOST_API_TOKEN="$saved_token"
  else
    unset AGENTPOST_API_TOKEN
  fi
  # Do not let a stale shell token override .env when this deployment disables gateway auth.
  if [[ "${AGENTPOST_REQUIRE_TOKEN:-}" == "0" ]]; then
    unset AGENTPOST_API_TOKEN
  fi
}

apply_token_policy() {
  case "$TOKEN_POLICY" in
    yes) REQUIRE_TOKEN=1 ;;
    no) REQUIRE_TOKEN=0 ;;
    auto)
      if [[ -n "$REQUIRE_TOKEN" ]]; then
        REQUIRE_TOKEN="$(normalize_bool "$REQUIRE_TOKEN")"
      else
        case "$SCENARIO" in
          public-domain|public-ip) REQUIRE_TOKEN=1 ;;
          http|local|lan)
            if public_url_suggests_token; then
              REQUIRE_TOKEN=1
            else
              REQUIRE_TOKEN=0
            fi
            ;;
          *) REQUIRE_TOKEN=0 ;;
        esac
      fi
      ;;
    *)
      die "unknown token policy: $TOKEN_POLICY"
      ;;
  esac
}

apply_scenario_defaults() {
  if [[ "$SCENARIO" == local || "$SCENARIO" == lan || "$SCENARIO" == public-ip ]]; then
    LEGACY_SCENARIO="${LEGACY_SCENARIO:-$SCENARIO}"
  fi
  normalize_scenario
  case "$SCENARIO" in
    http|local|lan|public-ip)
      SCENARIO=http
      DOMAIN="${DOMAIN:-agent.local}"
      ENABLE_CADDY=0
      case "${LEGACY_SCENARIO:-}" in
        local)
          PUBLIC_URL="http://127.0.0.1:${HTTP_PORT}"
          ;;
        lan)
          if [[ -n "$LAN_IP" ]]; then
            PUBLIC_URL="http://${LAN_IP}:${HTTP_PORT}"
          elif [[ -z "$PUBLIC_URL" ]]; then
            LAN_IP="$(detect_lan_ip)"
            if [[ -z "$LAN_IP" ]]; then
              if [[ "$INTERACTIVE" == "1" ]] && is_tty; then
                LAN_IP="$(prompt "LAN IP address for agents to connect" "")"
              else
                die "scenario=lan requires --lan-ip, --public-url, or a detectable LAN address"
              fi
            fi
            PUBLIC_URL="http://${LAN_IP}:${HTTP_PORT}"
          fi
          ;;
        public-ip)
          if [[ -n "$PUBLIC_IP" ]]; then
            PUBLIC_URL="http://${PUBLIC_IP}:${HTTP_PORT}"
          elif [[ -z "$PUBLIC_URL" ]]; then
            PUBLIC_IP="$(detect_public_ip)"
            if [[ -z "$PUBLIC_IP" ]]; then
              if [[ "$INTERACTIVE" == "1" ]] && is_tty; then
                PUBLIC_IP="$(prompt "Public IP address for agents to connect" "")"
              else
                die "scenario=public-ip requires --public-ip, --public-url, or network access to detect a public IP"
              fi
            fi
            PUBLIC_URL="http://${PUBLIC_IP}:${HTTP_PORT}"
          fi
          if [[ -z "$DOMAIN" || "$DOMAIN" == "agent.local" ]]; then
            if [[ "$INTERACTIVE" == "1" ]] && is_tty; then
              DOMAIN="$(prompt "Mailbox suffix (@domain, logical only — no DNS required)" "agent.local")"
            else
              DOMAIN="${DOMAIN:-agent.local}"
            fi
          fi
          TOKEN_POLICY="${TOKEN_POLICY:-yes}"
          ;;
        *)
          if [[ -z "$PUBLIC_URL" ]]; then
            PUBLIC_URL="http://127.0.0.1:${HTTP_PORT}"
          fi
          ;;
      esac
      ;;
    public-domain)
      if [[ -z "$DOMAIN" || "$DOMAIN" == "agent.local" ]]; then
        if [[ "$INTERACTIVE" == "1" ]] && is_tty; then
          DOMAIN="$(prompt "Public domain name (DNS A record required)" "")"
        else
          die "scenario=public-domain requires --domain"
        fi
      fi
      [[ -n "$DOMAIN" ]] || die "scenario=public-domain requires a domain"
      PUBLIC_URL="https://${DOMAIN}"
      ENABLE_CADDY=1
      ;;
    *)
      die "unknown scenario: ${SCENARIO:-<empty>}. Use http | public-domain"
      ;;
  esac

  if [[ -n "${PUBLIC_URL_OVERRIDE:-}" ]]; then
    PUBLIC_URL="$PUBLIC_URL_OVERRIDE"
  fi

  ENABLE_SMTP="$(normalize_bool "$ENABLE_SMTP")"
  ENABLE_CADDY="$(normalize_bool "$ENABLE_CADDY")"

  if [[ "$SMTP_FLAG_SET" != "1" && "$SCENARIO" == "http" ]]; then
    ENABLE_SMTP=0
  fi

  apply_token_policy
}

choose_scenario_interactive() {
  cat >&2 <<'EOF'

Select deployment mode:
  1) http            HTTP gateway on a host:port (default http://127.0.0.1:8080)
  2) public-domain   HTTPS on a real domain (Caddy + certificate)

EOF
  local choice
  choice="$(prompt "Choice" "1")"
  case "$choice" in
    1|http|local|lan|public-ip|publicip|public_ip) SCENARIO=http ;;
    2|public-domain|publicdomain|public_domain) SCENARIO=public-domain ;;
    *) die "invalid scenario choice: $choice" ;;
  esac

  case "$SCENARIO" in
    http)
      local default_url="http://127.0.0.1:${HTTP_PORT}"
      PUBLIC_URL="$(prompt "Public URL for agents (server_url in Skill)" "$default_url")"
      [[ -n "$PUBLIC_URL" ]] || die "public URL is required"
      if public_url_suggests_token; then
        TOKEN_POLICY=yes
      else
        TOKEN_POLICY=no
      fi
      if prompt_yes_no "Enable SMTP inbound (port 25)?" "n"; then
        ENABLE_SMTP=1
      fi
      ;;
    public-domain)
      DOMAIN="$(prompt "Public domain (HTTPS)" "")"
      [[ -n "$DOMAIN" ]] || die "domain is required"
      TOKEN_POLICY=yes
      if prompt_yes_no "Enable SMTP inbound (port 25)?" "n"; then
        ENABLE_SMTP=1
      fi
      ;;
  esac
}

resolve_scenario() {
  if [[ -n "$PUBLIC_IP" && -z "${LEGACY_SCENARIO:-}" ]]; then
    LEGACY_SCENARIO=public-ip
  fi
  if [[ -n "$LAN_IP" && -z "${LEGACY_SCENARIO:-}" ]]; then
    LEGACY_SCENARIO=lan
  fi
  if [[ -n "$SCENARIO" && "$SCENARIO" != "http" && "$SCENARIO" != "public-domain" ]]; then
    LEGACY_SCENARIO="$SCENARIO"
  fi
  if [[ -z "$SCENARIO" ]]; then
    if [[ "$INTERACTIVE" == "1" ]] && is_tty; then
      choose_scenario_interactive
    else
      SCENARIO=http
    fi
  fi
  apply_scenario_defaults
}

write_env_file() {
  cat >"$ENV_FILE" <<EOF
# Generated by ./start.sh — deployment scenario: ${SCENARIO}
AGENTPOST_SCENARIO=${SCENARIO}
AGENTPOST_DOMAIN=${DOMAIN}
AGENTPOST_HTTP_PORT=${HTTP_PORT}
AGENTPOST_PUBLIC_URL=${PUBLIC_URL}
AGENTPOST_ENABLE_SMTP=${ENABLE_SMTP}
AGENTPOST_ENABLE_CADDY=${ENABLE_CADDY}
AGENTPOST_REQUIRE_TOKEN=${REQUIRE_TOKEN}
AGENTPOST_SMTP_PUBLISH_PORT=${AGENTPOST_SMTP_PUBLISH_PORT:-25}
AGENTPOST_SMTP_PORT=${SMTP_PORT}
MODE=${MODE}
EOF
  log "Wrote ${ENV_FILE} (scenario=$(scenario_label "$SCENARIO"), public_url=${PUBLIC_URL})"
}

write_config() {
  local smtp_addr=""
  if [[ "$ENABLE_SMTP" == "1" ]]; then
    smtp_addr=":2525"
  fi

  cat >"$CONFIG_FILE" <<EOF
domain: ${DOMAIN}
http_addr: ":8080"
smtp_addr: "${smtp_addr}"
allow_external_relay: false
max_message_bytes: 1048576
EOF
  log "Wrote ${CONFIG_FILE} (domain=${DOMAIN}, smtp=${smtp_addr:-disabled})"
}

write_caddyfile() {
  if [[ "$ENABLE_CADDY" != "1" ]]; then
    return
  fi
  mkdir -p "$(dirname "$CADDYFILE")"
  cat >"$CADDYFILE" <<EOF
{
	servers {
		protocols h1 h2
	}
}

${DOMAIN} {
	encode gzip
	reverse_proxy agentpost:8080
}

www.${DOMAIN} {
	redir https://${DOMAIN}{uri} permanent
}
EOF
  log "Wrote ${CADDYFILE} for ${DOMAIN}"
}

ensure_api_token() {
  if [[ "$REQUIRE_TOKEN" != "1" ]]; then
    unset AGENTPOST_API_TOKEN
    return
  fi
  if [[ -n "${AGENTPOST_API_TOKEN:-}" ]]; then
    export AGENTPOST_API_TOKEN
    return
  fi
  if [[ -z "${AGENTPOST_API_TOKEN+x}" ]]; then
    AGENTPOST_API_TOKEN="$(openssl rand -hex 32)"
    TOKEN_GENERATED=1
    export AGENTPOST_API_TOKEN
    return
  fi
  unset AGENTPOST_API_TOKEN
}

print_api_token() {
  if [[ "$REQUIRE_TOKEN" != "1" ]]; then
    log "Gateway token disabled for this scenario."
    return
  fi
  if [[ "$TOKEN_GENERATED" == "1" ]]; then
    echo ""
    log "AGENTPOST_API_TOKEN (shown once — not saved to any file):"
    printf '%s\n' "$AGENTPOST_API_TOKEN"
    echo ""
    log "Save this token now. Reuse it with: AGENTPOST_API_TOKEN=<token> ./start.sh"
    echo ""
  elif [[ -n "${AGENTPOST_API_TOKEN:-}" ]]; then
    log "Using AGENTPOST_API_TOKEN from the current shell environment."
  fi
}

detect_mode() {
  case "$MODE" in
    docker|native) ;;
    auto)
      if have_cmd docker && docker compose version >/dev/null 2>&1; then
        MODE=docker
      elif have_cmd go; then
        MODE=native
      else
        die "Need Docker Compose or Go. Install one of them, or set MODE=docker|native."
      fi
      ;;
    *)
      die "Unknown MODE=${MODE}"
      ;;
  esac
}

wait_for_health() {
  local url="http://127.0.0.1:${HTTP_PORT}/healthz"
  local i
  for i in $(seq 1 30); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

print_firewall_hints() {
  case "$SCENARIO" in
    http)
      if public_url_suggests_token; then
        cat <<EOF

Firewall: allow inbound TCP ${HTTP_PORT} on the gateway host for clients using ${PUBLIC_URL}.
Domain / ICP filing is NOT required when using an IP:port URL.
EOF
      fi
      ;;
    public-domain)
      cat <<EOF

Firewall: allow inbound TCP 80 and 443 (Caddy). Optional TCP 25 if SMTP is enabled.
Agents should use ${PUBLIC_URL} (not raw :8080 on the public internet).
EOF
      ;;
  esac
}

print_endpoints() {
  cat <<EOF

AgentPost is running.

  Scenario:  $(scenario_label "$SCENARIO")
  Server URL: ${PUBLIC_URL}
  Mailbox:    @${DOMAIN}

  Health:   ${PUBLIC_URL}/healthz
  Skill:    ${PUBLIC_URL}/api/v1/skill
  Register: POST ${PUBLIC_URL}/api/v1/register
  Send:     POST ${PUBLIC_URL}/api/v1/send
  Inbox:    GET  ${PUBLIC_URL}/api/v1/messages

Agents should fetch the skill document first (URLs match deployment parameters):
  curl -fsS ${PUBLIC_URL}/api/v1/skill
EOF
  if [[ "$ENABLE_SMTP" == "1" ]]; then
    echo "  SMTP inbound: :${AGENTPOST_SMTP_PUBLISH_PORT:-25} (host) -> :2525 (container)"
  fi
  if [[ "$REQUIRE_TOKEN" == "1" ]]; then
    echo ""
    echo "  Gateway token: required on /api/v1/* except /healthz and /api/v1/skill"
  fi
  print_firewall_hints
}

print_agent_prompt() {
  cat <<EOF

--- Agent onboarding prompt (copy below) ---

You are connecting to an AgentPost mail gateway on this deployment.

1. Read the skill document first (authoritative API reference):
   ${PUBLIC_URL}/api/v1/skill

   curl -fsS ${PUBLIC_URL}/api/v1/skill

2. Gateway credentials (use on all /api/v1/* except /healthz and /api/v1/skill):
   AGENTPOST_SERVER=${PUBLIC_URL}
   AGENTPOST_EMAIL_SUFFIX=${DOMAIN}
EOF
  if [[ "$REQUIRE_TOKEN" == "1" && -n "${AGENTPOST_API_TOKEN:-}" ]]; then
    cat <<EOF
   AGENTPOST_API_TOKEN=${AGENTPOST_API_TOKEN}

   Header: Authorization: Bearer ${AGENTPOST_API_TOKEN}
EOF
  fi
  cat <<EOF

3. Workflow:
   - Generate an Ed25519 keypair; keep the private key secret.
   - POST /api/v1/register with your public key hex (optional profile, optional domain).
   - GET /api/v1/agents to discover other agents.
   - POST /api/v1/send and GET /api/v1/messages with signed requests.
   - Every message body MUST be JSON with exactly one of: "request" (ask) or "reply" (answer).
   - After human approval, start a background subagent to poll your inbox; on "request", execute it fully then reply with results (never send empty acknowledgments).
   - Poll with code/scripts (not AI loops on empty inbox) to avoid wasting LLM Token Plan; wake AI only when mail arrives.
   - Sign bytes: "<unix_timestamp>\\n<raw_request_body>" (empty body for GET/DELETE).
   - Use X-Agent-Email: you@your-domain for auth headers.

4. Rules:
   - Use AGENTPOST_SERVER exactly as above; do not substitute another host.
   - Any valid domain suffix is allowed at register; mailbox user@domain must be unique.
   - Same-domain mail is allowed by default; cross-domain requires recipient allowlist.
   - Request/reply protocol: one inbound request + one outbound reply = one conversation turn.
   - Poll is destructive: fetched messages are removed from the server.
   - Max TTL 24h; re-register before expiry.

5. Operator dashboard: ${PUBLIC_URL}/dashboard/

--- end prompt ---

EOF
}

print_configure_summary() {
  cat <<EOF

Configuration written. Next:
  ./start.sh up
  # or non-interactive:
  ./start.sh --non-interactive up

Agent environment (from this deployment):
  AGENTPOST_SERVER=${PUBLIC_URL}
  AGENTPOST_EMAIL_SUFFIX=${DOMAIN}
EOF
  if [[ "$REQUIRE_TOKEN" == "1" ]]; then
    echo "  AGENTPOST_API_TOKEN=<printed when you run ./start.sh up>"
  fi
}

cmd_configure() {
  prepare_deployment
  print_configure_summary
}

export_runtime_env() {
  export AGENTPOST_SCENARIO="$SCENARIO"
  export AGENTPOST_DOMAIN="$DOMAIN"
  export AGENTPOST_HTTP_PORT="$HTTP_PORT"
  export AGENTPOST_PUBLIC_URL="$PUBLIC_URL"
  export AGENTPOST_ENABLE_CADDY="$ENABLE_CADDY"
  export AGENTPOST_REQUIRE_TOKEN="$REQUIRE_TOKEN"
  export AGENTPOST_SMTP_PORT="$SMTP_PORT"
  export AGENTPOST_SMTP_PUBLISH_PORT="${AGENTPOST_SMTP_PUBLISH_PORT:-25}"
  if [[ "$REQUIRE_TOKEN" == "1" ]]; then
    export AGENTPOST_API_TOKEN="${AGENTPOST_API_TOKEN:-}"
  else
    unset AGENTPOST_API_TOKEN
    export AGENTPOST_API_TOKEN=""
  fi
  if [[ "$ENABLE_SMTP" == "1" ]]; then
    export AGENTPOST_SMTP_ADDR=":2525"
  else
    export AGENTPOST_SMTP_ADDR=""
  fi
}

cmd_up_docker() {
  ensure_api_token
  export_runtime_env

  local compose_args=()
  if [[ "$ENABLE_CADDY" == "1" ]]; then
    compose_args+=(--profile caddy)
  fi

  log "Starting with Docker Compose (scenario=$(scenario_label "$SCENARIO"))..."
  docker compose "${compose_args[@]}" up -d --build

  if wait_for_health; then
    print_api_token
    print_endpoints
    print_agent_prompt
  else
    log "Service started but health check timed out. Run: ./start.sh logs"
    exit 1
  fi
}

cmd_up_native() {
  ensure_api_token
  export_runtime_env
  export AGENTPOST_HTTP_ADDR=":${HTTP_PORT}"
  export AGENTPOST_ALLOW_EXTERNAL_RELAY=false

  if ! have_cmd go; then
    die "Go is not installed. Use ./start.sh --docker or install Go 1.25+."
  fi

  print_api_token
  log "Starting with go run on :${HTTP_PORT} (scenario=$(scenario_label "$SCENARIO")) ..."
  log "Press Ctrl+C to stop."
  go run ./cmd/agentpost -config "$CONFIG_FILE"
}

prepare_deployment() {
  load_env_file
  if [[ -z "$SCENARIO" && ! -f "$ENV_FILE" && -f .env.example ]]; then
    cp .env.example "$ENV_FILE"
    log "Created ${ENV_FILE} from .env.example"
    load_env_file
  fi
  resolve_scenario
  write_env_file
  write_config
  write_caddyfile
}

cmd_up() {
  prepare_deployment
  if [[ "$CONFIGURE_ONLY" == "1" ]]; then
    print_configure_summary
    return
  fi
  detect_mode
  if [[ "$MODE" == "docker" ]]; then
    cmd_up_docker
  else
    cmd_up_native
  fi
}

cmd_stop() {
  if have_cmd docker && docker compose version >/dev/null 2>&1; then
    docker compose --profile caddy down
    log "Stopped Docker deployment."
  else
    log "Docker Compose not available; nothing to stop."
  fi
}

cmd_status() {
  load_env_file
  local url="${PUBLIC_URL:-http://127.0.0.1:${HTTP_PORT}}/healthz"
  if curl -fsS "$url"; then
    echo
    print_endpoints
  else
    log "AgentPost does not appear to be running at ${url}"
    exit 1
  fi
}

cmd_logs() {
  docker compose logs -f --tail=100
}

main() {
  local command=up
  while [[ $# -gt 0 ]]; do
    case "$1" in
      up|start) command=up ;;
      configure|config) command=configure ;;
      stop|down) command=stop ;;
      status|health) command=status ;;
      logs) command=logs ;;
      help|-h|--help) usage; exit 0 ;;
      --scenario) SCENARIO="$2"; shift ;;
      --domain) DOMAIN="$2"; shift ;;
      --public-url) PUBLIC_URL_OVERRIDE="$2"; shift ;;
      --lan-ip) LAN_IP="$2"; shift ;;
      --public-ip) PUBLIC_IP="$2"; shift ;;
      --http-port) HTTP_PORT="$2"; shift ;;
      --smtp) ENABLE_SMTP=1; SMTP_FLAG_SET=1 ;;
      --no-smtp) ENABLE_SMTP=0; SMTP_FLAG_SET=1 ;;
      --token) TOKEN_POLICY=yes ;;
      --no-token) TOKEN_POLICY=no ;;
      --caddy) ENABLE_CADDY=1 ;;
      --no-caddy) ENABLE_CADDY=0 ;;
      --docker) MODE=docker ;;
      --native) MODE=native ;;
      --configure-only) CONFIGURE_ONLY=1 ;;
      --non-interactive) INTERACTIVE=0 ;;
      *) die "Unknown argument: $1 (try ./start.sh help)" ;;
    esac
    shift
  done

  case "$command" in
    up) cmd_up ;;
    configure) cmd_configure ;;
    stop) cmd_stop ;;
    status) cmd_status ;;
    logs) cmd_logs ;;
  esac
}

main "$@"
