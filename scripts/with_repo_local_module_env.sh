#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: bash ./scripts/with_repo_local_module_env.sh <command> [args...]" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
git_config="$(mktemp)"
trap 'rm -f "$git_config"' EXIT

cat >"$git_config" <<EOF
[url "file://$repo_root"]
	insteadOf = https://github.com/yiiilin/harness-core
	insteadOf = ssh://git@github.com/yiiilin/harness-core
	insteadOf = git@github.com:yiiilin/harness-core
EOF

export GOPROXY=direct
export GOSUMDB=off
export GONOSUMDB=github.com/yiiilin/harness-core
export GOPRIVATE=github.com/yiiilin/harness-core
export GONOPROXY=github.com/yiiilin/harness-core
export GIT_ALLOW_PROTOCOL=file:https:ssh
export GIT_CONFIG_GLOBAL="$git_config"

exec "$@"
