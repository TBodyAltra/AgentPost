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
ENABLE_SMTP="${AGENTPOST_ENABLE_SMTP:-0}"
ENABLE_CADDY="${AGENTPOST_ENABLE_CADDY:-0}"
REQUIRE_TOKEN="${AGENTPOST_REQUIRE_TOKEN:-}"
SMTP_PORT="${AGENTPOST_SMTP_PORT:-2525}"
INTERACTIVE=1
CONFIGURE_ONLY=0
TOKEN_GENERATED=0
TOKEN_FROM_FILE=0
AGENTPOST_DIR="${AGENTPOST_DIR:-.agentpost}"
TOKEN_FILE="${AGENTPOST_TOKEN_FILE:-${AGENTPOST_DIR}/gateway.token}"
LAN_IP=""
PUBLIC_IP=""
CONNECT_LOCALHOST=""
CONNECT_LAN=""
CONNECT_PUBLIC=""
CONNECT_DOMAIN=""
TOKEN_POLICY="auto" # auto | yes | no
SMTP_FLAG_SET=0
SKIP_LAN_DETECT=0
SKIP_PUBLIC_DETECT=0
CLI_HTTP_PORT=""
CLI_DOMAIN=""
CLI_ENABLE_CADDY=""
CLI_ENABLE_SMTP=""
CLI_LAN_IP=""
CLI_PUBLIC_IP=""

usage() {
  cat <<'EOF'
Usage: ./start.sh [command] [options]

Commands:
  up          Start AgentPost (default)
  configure   Write .env / config (no start)
  stop        Stop Docker deployment
  status      Show health and endpoint info
  logs        Follow Docker logs (docker mode only)
  help        Show this help

Deployment:
  ./start.sh up
  Gateway listens on :8080. Onboarding lists every URL the server can advertise
  (localhost, optional LAN / public IP, optional HTTPS domain). Each client
  picks the base URL it can reach.

Options:
  --domain NAME         Mailbox @ suffix; with --caddy, HTTPS site name
  --lan-ip IP           Include LAN URL in onboarding (auto-detect if omitted)
  --no-lan-detect       Do not auto-detect LAN IP
  --public-ip IP        Include public IP URL in onboarding (auto-detect if omitted)
  --no-public-detect    Do not auto-detect public IP
  --http-port PORT      Host HTTP port (default: 8080)
  --smtp                Enable SMTP inbound
  --no-smtp             Disable SMTP inbound
  --token               Require gateway token (default)
  --no-token            Disable gateway token
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
  ./start.sh up
  ./start.sh up --no-token
  ./start.sh up --domain example.domain --caddy --smtp
  ./start.sh up --lan-ip 192.168.1.100 --public-ip 203.0.113.10
  ./start.sh configure --non-interactive
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

resolve_connection_endpoints() {
  CONNECT_LOCALHOST="http://127.0.0.1:${HTTP_PORT}"
  CONNECT_LAN=""
  CONNECT_PUBLIC=""
  CONNECT_DOMAIN=""

  if [[ "$SKIP_LAN_DETECT" != "1" && -z "$LAN_IP" ]]; then
    LAN_IP="$(detect_lan_ip 2>/dev/null || true)"
  fi
  if [[ -n "$LAN_IP" ]]; then
    CONNECT_LAN="http://${LAN_IP}:${HTTP_PORT}"
  fi

  if [[ "$SKIP_PUBLIC_DETECT" != "1" && -z "$PUBLIC_IP" ]]; then
    PUBLIC_IP="$(detect_public_ip 2>/dev/null || true)"
  fi
  if [[ -n "$PUBLIC_IP" ]]; then
    CONNECT_PUBLIC="http://${PUBLIC_IP}:${HTTP_PORT}"
  fi

  if [[ "$ENABLE_CADDY" == "1" && -n "$DOMAIN" ]]; then
    CONNECT_DOMAIN="https://${DOMAIN}"
  fi
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
    if [[ -z "$CLI_DOMAIN" ]]; then
      DOMAIN="${AGENTPOST_DOMAIN:-$DOMAIN}"
    fi
    if [[ -z "$CLI_HTTP_PORT" ]]; then
      HTTP_PORT="${AGENTPOST_HTTP_PORT:-$HTTP_PORT}"
    fi
    if [[ -z "$CLI_ENABLE_SMTP" ]]; then
      ENABLE_SMTP="${AGENTPOST_ENABLE_SMTP:-$ENABLE_SMTP}"
    fi
    if [[ -z "$CLI_ENABLE_CADDY" ]]; then
      ENABLE_CADDY="${AGENTPOST_ENABLE_CADDY:-$ENABLE_CADDY}"
    fi
    REQUIRE_TOKEN="${AGENTPOST_REQUIRE_TOKEN:-$REQUIRE_TOKEN}"
    SMTP_PORT="${AGENTPOST_SMTP_PORT:-$SMTP_PORT}"
    CONNECT_LOCALHOST="${AGENTPOST_CONNECT_LOCALHOST:-$CONNECT_LOCALHOST}"
    CONNECT_LAN="${AGENTPOST_CONNECT_LAN:-$CONNECT_LAN}"
    CONNECT_PUBLIC="${AGENTPOST_CONNECT_PUBLIC:-$CONNECT_PUBLIC}"
    CONNECT_DOMAIN="${AGENTPOST_CONNECT_DOMAIN:-$CONNECT_DOMAIN}"
    MODE="${MODE:-auto}"
  fi
  if [[ -n "$saved_token" ]]; then
    AGENTPOST_API_TOKEN="$saved_token"
  else
    unset AGENTPOST_API_TOKEN
  fi
  if [[ "${AGENTPOST_REQUIRE_TOKEN:-}" == "0" ]]; then
    unset AGENTPOST_API_TOKEN
  fi
  if [[ -n "$CLI_HTTP_PORT" ]]; then
    HTTP_PORT="$CLI_HTTP_PORT"
  fi
  if [[ -n "$CLI_DOMAIN" ]]; then
    DOMAIN="$CLI_DOMAIN"
  fi
  if [[ -n "$CLI_ENABLE_SMTP" ]]; then
    ENABLE_SMTP="$CLI_ENABLE_SMTP"
  fi
  if [[ -n "$CLI_ENABLE_CADDY" ]]; then
    ENABLE_CADDY="$CLI_ENABLE_CADDY"
  fi
  if [[ -n "$CLI_LAN_IP" ]]; then
    LAN_IP="$CLI_LAN_IP"
  fi
  if [[ -n "$CLI_PUBLIC_IP" ]]; then
    PUBLIC_IP="$CLI_PUBLIC_IP"
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
        REQUIRE_TOKEN=1
      fi
      ;;
    *)
      die "unknown token policy: $TOKEN_POLICY"
      ;;
  esac
}

apply_deployment_defaults() {
  DOMAIN="${DOMAIN:-agent.local}"
  ENABLE_SMTP="$(normalize_bool "$ENABLE_SMTP")"
  ENABLE_CADDY="$(normalize_bool "$ENABLE_CADDY")"

  if [[ "$ENABLE_CADDY" == "1" ]]; then
    [[ -n "$DOMAIN" && "$DOMAIN" != "agent.local" ]] || die "--caddy requires --domain (HTTPS site name)"
  fi

  if [[ "$SMTP_FLAG_SET" != "1" ]]; then
    ENABLE_SMTP=0
  fi

  apply_token_policy
  resolve_connection_endpoints
}

choose_deployment_interactive() {
  DOMAIN="$(prompt "Mailbox suffix (@domain)" "agent.local")"
  if [[ "$SKIP_LAN_DETECT" != "1" ]]; then
    LAN_IP="$(detect_lan_ip 2>/dev/null || true)"
    if [[ -n "$LAN_IP" ]]; then
      log "Detected LAN IP: ${LAN_IP}"
    fi
  fi
  if [[ "$SKIP_PUBLIC_DETECT" != "1" ]]; then
    PUBLIC_IP="$(detect_public_ip 2>/dev/null || true)"
    if [[ -n "$PUBLIC_IP" ]]; then
      log "Detected public IP: ${PUBLIC_IP}"
    fi
  fi
  if prompt_yes_no "Enable HTTPS with a public domain (Caddy)?" "n"; then
    DOMAIN="$(prompt "HTTPS domain (DNS A record required)" "$DOMAIN")"
    [[ -n "$DOMAIN" ]] || die "domain is required for HTTPS"
    ENABLE_CADDY=1
    if prompt_yes_no "Enable SMTP inbound (port 25)?" "n"; then
      ENABLE_SMTP=1
      SMTP_FLAG_SET=1
    fi
  fi
  if prompt_yes_no "Require gateway API token?" "y"; then
    TOKEN_POLICY=yes
  else
    TOKEN_POLICY=no
  fi
}

resolve_deployment() {
  if [[ "$INTERACTIVE" == "1" ]] && is_tty; then
    choose_deployment_interactive
  fi
  apply_deployment_defaults
}

write_env_file() {
  cat >"$ENV_FILE" <<EOF
# Generated by ./start.sh
AGENTPOST_DOMAIN=${DOMAIN}
AGENTPOST_HTTP_PORT=${HTTP_PORT}
AGENTPOST_ENABLE_SMTP=${ENABLE_SMTP}
AGENTPOST_ENABLE_CADDY=${ENABLE_CADDY}
AGENTPOST_REQUIRE_TOKEN=${REQUIRE_TOKEN}
AGENTPOST_SMTP_PUBLISH_PORT=${AGENTPOST_SMTP_PUBLISH_PORT:-25}
AGENTPOST_SMTP_PORT=${SMTP_PORT}
AGENTPOST_CONNECT_LOCALHOST=${CONNECT_LOCALHOST}
MODE=${MODE}
EOF
  if [[ -n "$CONNECT_LAN" ]]; then
    printf 'AGENTPOST_CONNECT_LAN=%s\n' "$CONNECT_LAN" >>"$ENV_FILE"
  fi
  if [[ -n "$CONNECT_PUBLIC" ]]; then
    printf 'AGENTPOST_CONNECT_PUBLIC=%s\n' "$CONNECT_PUBLIC" >>"$ENV_FILE"
  fi
  if [[ -n "$CONNECT_DOMAIN" ]]; then
    printf 'AGENTPOST_CONNECT_DOMAIN=%s\n' "$CONNECT_DOMAIN" >>"$ENV_FILE"
  fi
  log "Wrote ${ENV_FILE} (localhost=${CONNECT_LOCALHOST}, token=${REQUIRE_TOKEN})"
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
  if [[ -f "$TOKEN_FILE" ]]; then
    AGENTPOST_API_TOKEN="$(tr -d '[:space:]' <"$TOKEN_FILE")"
    if [[ -n "$AGENTPOST_API_TOKEN" ]]; then
      TOKEN_FROM_FILE=1
      export AGENTPOST_API_TOKEN
      return
    fi
  fi
  if [[ -z "${AGENTPOST_API_TOKEN+x}" ]]; then
    AGENTPOST_API_TOKEN="$(openssl rand -hex 32)"
    TOKEN_GENERATED=1
    export AGENTPOST_API_TOKEN
    mkdir -p "$(dirname "$TOKEN_FILE")"
    printf '%s\n' "$AGENTPOST_API_TOKEN" >"$TOKEN_FILE"
    chmod 600 "$TOKEN_FILE" 2>/dev/null || true
    return
  fi
  unset AGENTPOST_API_TOKEN
}

print_api_token() {
  if [[ "$REQUIRE_TOKEN" != "1" ]]; then
    log "Gateway token disabled."
    return
  fi
  if [[ "$TOKEN_GENERATED" == "1" ]]; then
    echo ""
    log "AGENTPOST_API_TOKEN (new — saved for next restart):"
    printf '%s\n' "$AGENTPOST_API_TOKEN"
    echo ""
    log "Stored in ${TOKEN_FILE} (not in .env). Clients can keep using this token after redeploy."
    echo ""
  elif [[ "$TOKEN_FROM_FILE" == "1" ]]; then
    log "Using gateway token from ${TOKEN_FILE} (same as last deploy)."
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
  if [[ -n "$CONNECT_DOMAIN" ]]; then
    cat <<EOF

Firewall: allow inbound TCP 80 and 443 (Caddy). Optional TCP 25 if SMTP is enabled.
Agents on the public internet should use ${CONNECT_DOMAIN}.
EOF
  fi
  if [[ -n "$CONNECT_PUBLIC" ]]; then
    cat <<EOF

Firewall: allow inbound TCP ${HTTP_PORT} on ${PUBLIC_IP:-the public IP}.
EOF
  fi
  if [[ -n "$CONNECT_LAN" ]]; then
    cat <<EOF

Firewall: allow inbound TCP ${HTTP_PORT} on ${LAN_IP:-the gateway host} for LAN clients.
EOF
  fi
}

print_endpoints() {
  cat <<EOF

AgentPost is running.

  Mailbox:   @${DOMAIN}
  Local:     ${CONNECT_LOCALHOST}
EOF
  if [[ -n "$CONNECT_LAN" ]]; then
    echo "  LAN:       ${CONNECT_LAN}"
  fi
  if [[ -n "$CONNECT_PUBLIC" ]]; then
    echo "  Public IP: ${CONNECT_PUBLIC}"
  fi
  if [[ -n "$CONNECT_DOMAIN" ]]; then
    echo "  Domain:    ${CONNECT_DOMAIN}"
  fi
  cat <<EOF

Agents fetch skill from the base URL they can reach, for example:
EOF
  if [[ "$REQUIRE_TOKEN" == "1" && -n "${AGENTPOST_API_TOKEN:-}" ]]; then
    echo "  curl -fsS -H \"Authorization: Bearer \${AGENTPOST_API_TOKEN}\" ${CONNECT_LOCALHOST}/api/v1/skill"
  else
    echo "  curl -fsS ${CONNECT_LOCALHOST}/api/v1/skill"
  fi
  if [[ "$ENABLE_SMTP" == "1" ]]; then
    echo "  SMTP inbound: :${AGENTPOST_SMTP_PUBLISH_PORT:-25} (host) -> :2525 (container)"
  fi
  if [[ "$REQUIRE_TOKEN" == "1" ]]; then
    echo ""
    echo "  Gateway token: required on /api/v1/* except /healthz"
  fi
  print_firewall_hints
}

print_agent_prompt() {
  if ! have_cmd go; then
    die "Go is required to print the Agent onboarding prompt. Install Go 1.25+ or use the dashboard Copy prompt button."
  fi
  go run ./cmd/agentpost -config "$CONFIG_FILE" -print-onboarding
}

print_configure_summary() {
  cat <<EOF

Configuration written. Next:
  ./start.sh up
  # or non-interactive:
  ./start.sh --non-interactive up

Client base URL (pick one your agent can reach):
  ${CONNECT_LOCALHOST}
EOF
  if [[ -n "$CONNECT_LAN" ]]; then
    echo "  ${CONNECT_LAN}"
  fi
  if [[ -n "$CONNECT_PUBLIC" ]]; then
    echo "  ${CONNECT_PUBLIC}"
  fi
  if [[ -n "$CONNECT_DOMAIN" ]]; then
    echo "  ${CONNECT_DOMAIN}"
  fi
  echo ""
  echo "  AGENTPOST_EMAIL_SUFFIX=${DOMAIN}"
  if [[ "$REQUIRE_TOKEN" == "1" ]]; then
    echo "  AGENTPOST_API_TOKEN=<printed when you run ./start.sh up>"
  fi
}

cmd_configure() {
  prepare_deployment
  print_configure_summary
}

export_runtime_env() {
  export AGENTPOST_DATA_DIR="${AGENTPOST_DATA_DIR:-${ROOT}/${AGENTPOST_DIR}/data}"
  export AGENTPOST_DOMAIN="$DOMAIN"
  export AGENTPOST_HTTP_PORT="$HTTP_PORT"
  export AGENTPOST_CONNECT_LOCALHOST="$CONNECT_LOCALHOST"
  export AGENTPOST_CONNECT_LAN="${CONNECT_LAN:-}"
  export AGENTPOST_CONNECT_PUBLIC="${CONNECT_PUBLIC:-}"
  export AGENTPOST_CONNECT_DOMAIN="${CONNECT_DOMAIN:-}"
  unset AGENTPOST_PUBLIC_URL
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

  log "Starting with Docker Compose..."
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
  log "Starting with go run on :${HTTP_PORT} ..."
  log "Press Ctrl+C to stop."
  go run ./cmd/agentpost -config "$CONFIG_FILE"
}

prepare_deployment() {
  load_env_file
  if [[ ! -f "$ENV_FILE" && -f .env.example ]]; then
    cp .env.example "$ENV_FILE"
    log "Created ${ENV_FILE} from .env.example"
    load_env_file
  fi
  resolve_deployment
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
  local url="${CONNECT_LOCALHOST:-http://127.0.0.1:${HTTP_PORT}}/healthz"
  if curl -fsS "$url"; then
    echo
    resolve_connection_endpoints
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
      --domain) DOMAIN="$2"; CLI_DOMAIN="$2"; shift ;;
      --lan-ip) LAN_IP="$2"; CLI_LAN_IP="$2"; shift ;;
      --public-ip) PUBLIC_IP="$2"; CLI_PUBLIC_IP="$2"; shift ;;
      --no-lan-detect) SKIP_LAN_DETECT=1 ;;
      --no-public-detect) SKIP_PUBLIC_DETECT=1 ;;
      --http-port) HTTP_PORT="$2"; CLI_HTTP_PORT="$2"; shift ;;
      --smtp) ENABLE_SMTP=1; CLI_ENABLE_SMTP=1; SMTP_FLAG_SET=1 ;;
      --no-smtp) ENABLE_SMTP=0; CLI_ENABLE_SMTP=0; SMTP_FLAG_SET=1 ;;
      --token) TOKEN_POLICY=yes ;;
      --no-token) TOKEN_POLICY=no ;;
      --caddy) ENABLE_CADDY=1; CLI_ENABLE_CADDY=1 ;;
      --no-caddy) ENABLE_CADDY=0; CLI_ENABLE_CADDY=0 ;;
      --docker) MODE=docker ;;
      --native) MODE=native ;;
      --configure-only) CONFIGURE_ONLY=1 ;;
      --non-interactive) INTERACTIVE=0 ;;
      --scenario|--public-url)
        die "removed: use ./start.sh up (optional --domain --caddy --lan-ip --public-ip). See ./start.sh help"
        ;;
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
