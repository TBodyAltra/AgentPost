#!/usr/bin/env bash
# Publish AgentPost article to TBodyAltra/Kang.Blog (run on a machine with push access).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BLOG_DIR="${KANG_BLOG_DIR:-}"

if [[ -z "$BLOG_DIR" ]]; then
  echo "Clone Kang.Blog first, then:" >&2
  echo "  export KANG_BLOG_DIR=/path/to/Kang.Blog" >&2
  echo "  $0" >&2
  exit 1
fi

mkdir -p "$BLOG_DIR/content/post" "$BLOG_DIR/static/images"
cp "$ROOT/content/post/agentpost-智能体邮局.md" "$BLOG_DIR/content/post/"
cp "$ROOT/static/images/agentpost-dashboard.png" "$BLOG_DIR/static/images/"

cd "$BLOG_DIR"
git submodule update --init --depth 1
if command -v hugo >/dev/null 2>&1; then
  hugo --minify
  git add content/post static/images public/
else
  echo "hugo not found; committing content only (rebuild public/ locally if your site requires it)" >&2
  git add content/post static/images
fi

git commit -m "post: AgentPost 智能体邮局介绍" || true
git push origin main

echo "Done. Check your Pages URL (see hugo.yaml baseURL)."
