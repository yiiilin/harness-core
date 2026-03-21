#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  bash ./scripts/release-module.sh resolve <module> <version>
  bash ./scripts/release-module.sh tag <module> <version>

Modules:
  root
  builtins
  modules
  adapters
  cli

Environment:
  APPLY=1   Create the annotated local tag for the "tag" command.
EOF
}

require_semver() {
  local version="$1"
  if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
    echo "invalid version: $version" >&2
    exit 1
  fi
}

resolve_module() {
  local module="$1"
  local version="$2"
  case "$module" in
    root)
      MODULE_DIR="."
      MODULE_PATH="github.com/yiiilin/harness-core"
      TAG_NAME="$version"
      ;;
    builtins)
      MODULE_DIR="pkg/harness/builtins"
      MODULE_PATH="github.com/yiiilin/harness-core/pkg/harness/builtins"
      TAG_NAME="pkg/harness/builtins/$version"
      ;;
    modules)
      MODULE_DIR="modules"
      MODULE_PATH="github.com/yiiilin/harness-core/modules"
      TAG_NAME="modules/$version"
      ;;
    adapters)
      MODULE_DIR="adapters"
      MODULE_PATH="github.com/yiiilin/harness-core/adapters"
      TAG_NAME="adapters/$version"
      ;;
    cli)
      MODULE_DIR="cmd/harness-core"
      MODULE_PATH="github.com/yiiilin/harness-core/cmd/harness-core"
      TAG_NAME="cmd/harness-core/$version"
      ;;
    *)
      echo "unknown module: $module" >&2
      usage
      exit 1
      ;;
  esac
}

main() {
  if [[ $# -ne 3 ]]; then
    usage
    exit 1
  fi

  local command="$1"
  local module="$2"
  local version="$3"

  require_semver "$version"
  resolve_module "$module" "$version"

  case "$command" in
    resolve)
      printf 'module=%s\nmodule_dir=%s\nmodule_path=%s\ntag=%s\n' \
        "$module" "$MODULE_DIR" "$MODULE_PATH" "$TAG_NAME"
      ;;
    tag)
      if git rev-parse -q --verify "refs/tags/$TAG_NAME" >/dev/null; then
        echo "tag already exists locally: $TAG_NAME" >&2
        exit 1
      fi
      if [[ "${APPLY:-0}" != "1" ]]; then
        printf 'dry_run=1\nmodule=%s\nmodule_dir=%s\nmodule_path=%s\ntag=%s\n' \
          "$module" "$MODULE_DIR" "$MODULE_PATH" "$TAG_NAME"
        printf 'next=git tag -a %q -m %q\n' "$TAG_NAME" "Release $MODULE_PATH $version"
        exit 0
      fi
      git tag -a "$TAG_NAME" -m "Release $MODULE_PATH $version"
      printf 'created_tag=%s\n' "$TAG_NAME"
      ;;
    *)
      echo "unknown command: $command" >&2
      usage
      exit 1
      ;;
  esac
}

main "$@"
