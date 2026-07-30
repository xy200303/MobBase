#!/usr/bin/env bash
set -euo pipefail

RELEASE_API="https://api.github.com/repos/xy200303/MobBase/releases"
VERSION="latest"
SOURCE_PATH=""
INSTALL_DIR=""
NO_PATH=0

usage() {
  cat <<'EOF'
Usage: install.sh [--version <tag|latest>] [--source <path>] [--install-dir <path>] [--no-path]

Install a verified Mob release binary. Go is needed only when --source is used.
The default install directory is $MOB_INSTALL_DIR, $MOB_HOME/bin, or ~/.mob/bin.
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
    *) fail "unknown argument: $1" ;;
  esac
done

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

if [[ -n "$SOURCE_PATH" ]]; then
  command -v go >/dev/null 2>&1 || fail "Go 1.26 or later is required only with --source. Omit it to install a release binary."
  SOURCE_PATH="$(cd -- "$SOURCE_PATH" && pwd)"
  [[ -f "$SOURCE_PATH/go.mod" ]] || fail "--source must point to a Mob source checkout containing go.mod"
  BUILD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mob-install.XXXXXX")"
  trap 'rm -rf "$BUILD_DIR"' EXIT
  (
    cd "$SOURCE_PATH"
    go build -trimpath -o "$BUILD_DIR/mob" ./cmd/mob
  )
  install -m 0755 "$BUILD_DIR/mob" "$INSTALL_DIR/mob"
else
  command -v curl >/dev/null 2>&1 || fail "curl is required to download a Mob release binary."
  case "$(uname -s):$(uname -m)" in
    Darwin:arm64) ASSET_NAME="mob-darwin-arm64" ;;
    Darwin:x86_64) ASSET_NAME="mob-darwin-amd64" ;;
    Linux:x86_64) ASSET_NAME="mob-linux-amd64" ;;
    Linux:aarch64) ASSET_NAME="mob-linux-arm64" ;;
    *) fail "No Mob release artifact is defined for $(uname -s) $(uname -m). Use --source to build locally." ;;
  esac
  if [[ "$VERSION" == "latest" ]]; then
    VERSION="$(curl -fsSL -H 'User-Agent: MobBase-Installer' "$RELEASE_API/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    [[ -n "$VERSION" ]] || fail "Could not resolve the latest GitHub Release tag."
  fi
  ASSET_URL="https://github.com/xy200303/MobBase/releases/download/$VERSION/$ASSET_NAME"
  CACHE_ROOT="${MOB_HOME:-$HOME/.mob}/cache/releases"
  if command -v sha256sum >/dev/null 2>&1; then
    TAG_ID="$(printf '%s' "$VERSION" | sha256sum | awk '{print $1}')"
  else
    TAG_ID="$(printf '%s' "$VERSION" | shasum -a 256 | awk '{print $1}')"
  fi
  CACHE_DIR="$CACHE_ROOT/$ASSET_NAME/$TAG_ID"
  CACHE_BINARY="$CACHE_DIR/$ASSET_NAME"
  CACHE_CHECKSUM="$CACHE_DIR/$ASSET_NAME.sha256"
  BUILD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mob-install.XXXXXX")"
  trap 'rm -rf "$BUILD_DIR"' EXIT
  if [[ -f "$CACHE_BINARY" && -f "$CACHE_CHECKSUM" ]]; then
    EXPECTED="$(grep -Eo '[A-Fa-f0-9]{64}' "$CACHE_CHECKSUM" | head -n 1)"
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL="$(sha256sum "$CACHE_BINARY" | awk '{print $1}')"
    else
      ACTUAL="$(shasum -a 256 "$CACHE_BINARY" | awk '{print $1}')"
    fi
    if [[ "$(printf '%s' "$ACTUAL" | tr '[:upper:]' '[:lower:]')" == "$(printf '%s' "$EXPECTED" | tr '[:upper:]' '[:lower:]')" ]]; then
      install -m 0755 "$CACHE_BINARY" "$INSTALL_DIR/mob"
      printf 'Using cached Mob release %s\n' "$VERSION"
      CACHE_HIT=1
    fi
  fi
  if [[ "${CACHE_HIT:-0}" -ne 1 ]]; then
    curl -fsSL --retry 3 -o "$BUILD_DIR/$ASSET_NAME" "$ASSET_URL"
    EXPECTED="$(curl -fsSL "$ASSET_URL.sha256" | grep -Eo '[A-Fa-f0-9]{64}' | head -n 1)"
    [[ ${#EXPECTED} -eq 64 ]] || fail "Release $VERSION does not provide a valid $ASSET_NAME.sha256 file."
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL="$(sha256sum "$BUILD_DIR/$ASSET_NAME" | awk '{print $1}')"
    else
      ACTUAL="$(shasum -a 256 "$BUILD_DIR/$ASSET_NAME" | awk '{print $1}')"
    fi
    ACTUAL="$(printf '%s' "$ACTUAL" | tr '[:upper:]' '[:lower:]')"
    EXPECTED="$(printf '%s' "$EXPECTED" | tr '[:upper:]' '[:lower:]')"
    [[ "$ACTUAL" == "$EXPECTED" ]] || fail "Downloaded $ASSET_NAME does not match the release SHA-256."
    mkdir -p "$CACHE_DIR"
    install -m 0755 "$BUILD_DIR/$ASSET_NAME" "$CACHE_BINARY"
    printf '%s  %s\n' "$EXPECTED" "$ASSET_NAME" > "$CACHE_CHECKSUM"
    install -m 0755 "$BUILD_DIR/$ASSET_NAME" "$INSTALL_DIR/mob"
  fi
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
