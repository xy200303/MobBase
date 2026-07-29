#!/usr/bin/env bash
set -euo pipefail

MODULE_PATH="github.com/xy200303/MobBase/cmd/mob"
VERSION="latest"
VERSION_EXPLICIT=0
SOURCE_PATH=""
INSTALL_DIR=""
NO_PATH=0

usage() {
  cat <<'EOF'
Usage: install.sh [--version <version>] [--source <path>] [--install-dir <path>] [--no-path]

Install Mob from a local source checkout or the Go module. The default install
directory is $MOB_INSTALL_DIR, $MOB_HOME/bin, or ~/.mob/bin.
EOF
}

fail() {
  printf 'Mob installation failed: %s\n' "$1" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || fail "--version requires a value"
      VERSION="$2"
      VERSION_EXPLICIT=1
      shift 2
      ;;
    --source)
      [[ $# -ge 2 ]] || fail "--source requires a path"
      SOURCE_PATH="$2"
      shift 2
      ;;
    --install-dir)
      [[ $# -ge 2 ]] || fail "--install-dir requires a path"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --no-path)
      NO_PATH=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

command -v go >/dev/null 2>&1 || fail "Go 1.26 or later is required. Install Go, then rerun this script."

if [[ -z "$INSTALL_DIR" ]]; then
  if [[ -n "${MOB_INSTALL_DIR:-}" ]]; then
    INSTALL_DIR="$MOB_INSTALL_DIR"
  elif [[ -n "${MOB_HOME:-}" ]]; then
    INSTALL_DIR="$MOB_HOME/bin"
  else
    INSTALL_DIR="$HOME/.mob/bin"
  fi
fi
mkdir -p "$INSTALL_DIR"

if [[ -z "$SOURCE_PATH" && $VERSION_EXPLICIT -eq 0 ]]; then
  SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
  CANDIDATE="$(cd -- "$SCRIPT_DIR/.." && pwd)"
  if [[ -f "$CANDIDATE/go.mod" ]]; then
    SOURCE_PATH="$CANDIDATE"
  fi
fi

if [[ -n "$SOURCE_PATH" ]]; then
  SOURCE_PATH="$(cd -- "$SOURCE_PATH" && pwd)"
  [[ -f "$SOURCE_PATH/go.mod" ]] || fail "--source must point to a Mob source checkout containing go.mod"
  BUILD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mob-install.XXXXXX")"
  trap 'rm -rf "$BUILD_DIR"' EXIT
  (
    cd "$SOURCE_PATH"
    go build -o "$BUILD_DIR/mob" ./cmd/mob
  )
  install -m 0755 "$BUILD_DIR/mob" "$INSTALL_DIR/mob"
else
  GOBIN="$INSTALL_DIR" go install "$MODULE_PATH@$VERSION"
fi

add_to_path() {
  local profile
  case "${SHELL:-}" in
    */zsh) profile="$HOME/.zprofile" ;;
    *) profile="$HOME/.bashrc" ;;
  esac
  local escaped_dir path_line
  escaped_dir=$(printf '%q' "$INSTALL_DIR")
  path_line="export PATH=${escaped_dir}:\$PATH"
  if [[ -f "$profile" ]] && grep -Fqx "$path_line" "$profile"; then
    return
  fi
  printf '\n# Mob CLI\n%s\n' "$path_line" >> "$profile"
  export PATH="$INSTALL_DIR:$PATH"
  printf 'Added Mob to PATH in %s\n' "$profile"
}

if [[ $NO_PATH -eq 0 ]]; then
  add_to_path
fi

"$INSTALL_DIR/mob" help >/dev/null
printf 'Mob installed to %s/mob\n' "$INSTALL_DIR"
if [[ $NO_PATH -eq 1 ]]; then
  printf 'Add %s to PATH before running mob from a new shell.\n' "$INSTALL_DIR"
else
  printf 'Open a new shell, or source your profile, to use mob from PATH.\n'
fi
