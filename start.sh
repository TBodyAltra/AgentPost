#!/usr/bin/env bash
# AgentPost one-click launcher (native Go or Docker).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

ENV_FILE="${ENV_FILE:-.env}"
CONFIG_FILE="${CONFIG_FILE:-config.yaml}"
MODE="${MODE:-auto}" # auto | docker | native
HTTP_PORT="${AGENTPOST_HTTP_PORT:-8080}"
DOMAIN="${AGENTPOST_DOMAIN:-agent.local}"
ENABLE_SMTP="${AGENTPOST_ENABLE_SMTP:-0}"
SMTP_PORT="${AGENTPOST_SMTP_PORT:-2525}"

usage() {
  cat <<'EOF'
Usage: ./start.sh [command] [options]

Commands:
  up        Start AgentPost (default)
  stop      Stop Docker deployment
  status    Show health and endpoint info
  logs      Follow Docker logs (docker mode only)
  help      Show this help

Options:
  --docker          Force Docker Compose
  --native          Force local "go run"
  --domain NAME     Mailbox domain suffix (default: agent.local)
  --http-port PORT  Published HTTP port (default: 8080)
  --smtp            Enable SMTP inbound on :2525

Environment:
  Reads .env if present. See .env.example.

Examples:
  ./start.sh
  ./start.sh --native
  ./start.sh --docker --domain post.my-team.internal --http-port 8080
  AGENTPOST_DOMAIN=agent.local ./start.sh up
EOF
}

log() {
  printf '[agentpost] %s\n' "$*"
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

load_env_file() {
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    set -a
    source "$ENV_FILE"
    set +a
    DOMAIN="${AGENTPOST_DOMAIN:-$DOMAIN}"
    HTTP_PORT="${AGENTPOST_HTTP_PORT:-$HTTP_PORT}"
    ENABLE_SMTP="${AGENTPOST_ENABLE_SMTP:-$ENABLE_SMTP}"
    SMTP_PORT="${AGENTPOST_SMTP_PORT:-$SMTP_PORT}"
    MODE="${MODE:-auto}"
  fi
}

write_config() {
  local smtp_addr=""
  if [[ "$ENABLE_SMTP" == "1" || "$ENABLE_SMTP" == "true" || "$ENABLE_SMTP" == "yes" ]]; then
    smtp_addr=":2525"
  fi
  local api_token="${AGENTPOST_API_TOKEN:-}"

  cat >"$CONFIG_FILE" <<EOF
domain: ${DOMAIN}
http_addr: ":8080"
smtp_addr: "${smtp_addr}"
allow_external_relay: false
max_message_bytes: 1048576
api_token: "${api_token}"
EOF
  log "Wrote ${CONFIG_FILE} (domain=${DOMAIN}, smtp=${smtp_addr:-disabled}, api_token=${api_token:+enabled})"
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
        log "Need Docker Compose or Go. Install one of them, or set MODE=docker|native."
        exit 1
      fi
      ;;
    *)
      log "Unknown MODE=${MODE}"
      exit 1
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

print_endpoints() {
  cat <<EOF

AgentPost is running.

  Health:  http://127.0.0.1:${HTTP_PORT}/healthz
  Register: POST http://127.0.0.1:${HTTP_PORT}/api/v1/register
  Send:     POST http://127.0.0.1:${HTTP_PORT}/api/v1/send
  Inbox:    GET  http://127.0.0.1:${HTTP_PORT}/api/v1/messages

Mailbox domain suffix: @${DOMAIN}
Example address: agent-a@${DOMAIN}

Tip: Agents only need outbound HTTP to this host; use your server IP if remote:
  http://<your-server-ip>:${HTTP_PORT}
EOF
  if [[ "$ENABLE_SMTP" == "1" || "$ENABLE_SMTP" == "true" || "$ENABLE_SMTP" == "yes" ]]; then
    echo "  SMTP inbound: :${AGENTPOST_SMTP_PUBLISH_PORT:-25} (host) -> :2525 (container)"
  fi
}

cmd_up_docker() {
  if [[ ! -f "$ENV_FILE" && -f .env.example ]]; then
    cp .env.example "$ENV_FILE"
    log "Created ${ENV_FILE} from .env.example — edit if needed."
    load_env_file
    write_config
  fi

  write_config
  export AGENTPOST_DOMAIN="$DOMAIN"
  export AGENTPOST_HTTP_PORT="$HTTP_PORT"
  export AGENTPOST_SMTP_PORT="$SMTP_PORT"
  export AGENTPOST_SMTP_PUBLISH_PORT="${AGENTPOST_SMTP_PUBLISH_PORT:-25}"
  export AGENTPOST_API_TOKEN="${AGENTPOST_API_TOKEN:-}"
  if [[ "$ENABLE_SMTP" == "1" || "$ENABLE_SMTP" == "true" || "$ENABLE_SMTP" == "yes" ]]; then
    export AGENTPOST_SMTP_ADDR=":2525"
  else
    export AGENTPOST_SMTP_ADDR=""
  fi

  log "Starting with Docker Compose..."
  docker compose up -d --build

  if wait_for_health; then
    print_endpoints
  else
    log "Service started but health check timed out. Run: ./start.sh logs"
    exit 1
  fi
}

cmd_up_native() {
  write_config
  if ! have_cmd go; then
    log "Go is not installed. Use ./start.sh --docker or install Go 1.25+."
    exit 1
  fi

  export AGENTPOST_DOMAIN="$DOMAIN"
  export AGENTPOST_HTTP_ADDR=":${HTTP_PORT}"
  if [[ "$ENABLE_SMTP" == "1" || "$ENABLE_SMTP" == "true" || "$ENABLE_SMTP" == "yes" ]]; then
    export AGENTPOST_SMTP_ADDR=":2525"
  else
    export AGENTPOST_SMTP_ADDR=""
  fi
  export AGENTPOST_ALLOW_EXTERNAL_RELAY=false

  log "Starting with go run on :${HTTP_PORT} ..."
  log "Press Ctrl+C to stop."
  go run . -config "$CONFIG_FILE"
}

cmd_up() {
  load_env_file
  detect_mode
  if [[ "$MODE" == "docker" ]]; then
    cmd_up_docker
  else
    cmd_up_native
  fi
}

cmd_stop() {
  if have_cmd docker && docker compose version >/dev/null 2>&1; then
    docker compose down
    log "Stopped Docker deployment."
  else
    log "Docker Compose not available; nothing to stop."
  fi
}

cmd_status() {
  local url="http://127.0.0.1:${HTTP_PORT}/healthz"
  if curl -fsS "$url"; then
    echo
    print_endpoints
  else
    log "AgentPost does not appear to be running on ${url}"
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
      stop|down) command=stop; shift; main "$command"; return ;;
      status|health) command=status; shift; main "$command"; return ;;
      logs) command=logs; shift; main "$command"; return ;;
      help|-h|--help) usage; exit 0 ;;
      --docker) MODE=docker ;;
      --native) MODE=native ;;
      --domain) DOMAIN="$2"; shift ;;
      --http-port) HTTP_PORT="$2"; shift ;;
      --smtp) ENABLE_SMTP=1 ;;
      *) log "Unknown argument: $1"; usage; exit 1 ;;
    esac
    shift
  done

  case "$command" in
    up) cmd_up ;;
    stop) cmd_stop ;;
    status) cmd_status ;;
    logs) cmd_logs ;;
  esac
}

main "$@"
