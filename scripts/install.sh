#!/usr/bin/env sh
set -eu

SUMMON_VERSION="${SUMMON_VERSION:-latest}"
SUMMON_INSTALL_PATH="${SUMMON_INSTALL_PATH:-}"
SUMMON_NONINTERACTIVE="${SUMMON_NONINTERACTIVE:-0}"
SUMMON_NO_MODIFY_PATH="${SUMMON_NO_MODIFY_PATH:-0}"
SUMMON_DOWNLOAD_URL="${SUMMON_DOWNLOAD_URL:-}"
SUMMON_CHECKSUM_URL="${SUMMON_CHECKSUM_URL:-}"
SUMMON_CHECKSUM="${SUMMON_CHECKSUM:-}"
SUMMON_LATEST_VERSION="${SUMMON_LATEST_VERSION:-v0.1.0}"
SUMMON_RELEASE_BASE_URL="${SUMMON_RELEASE_BASE_URL:-https://github.com/ai-summon/summon/releases/download}"

if [ "$SUMMON_NONINTERACTIVE" != "1" ] && [ ! -t 0 ]; then
  SUMMON_NONINTERACTIVE="1"
fi

fail() {
  category="$1"
  message="$2"
  printf 'ERROR[%s]: %s\n' "$category" "$message" >&2
  exit 1
}

warn() {
  printf 'WARN: %s\n' "$1" >&2
}

info() {
  printf '%s\n' "$1"
}

normalize_os() {
  raw="${SUMMON_TEST_OS:-$(uname -s)}"
  case "$raw" in
    Darwin|darwin) printf 'darwin' ;;
    Linux|linux) printf 'linux' ;;
    *) fail platform "Unsupported operating system: $raw. Supported: macOS and Linux." ;;
  esac
}

normalize_arch() {
  raw="${SUMMON_TEST_ARCH:-$(uname -m)}"
  case "$raw" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) fail platform "Unsupported architecture: $raw. Supported: amd64 and arm64." ;;
  esac
}

resolve_version() {
  if [ "$SUMMON_VERSION" != "latest" ]; then
    printf '%s' "$SUMMON_VERSION"
    return
  fi

  api_url="${SUMMON_TEST_API_URL:-https://api.github.com/repos/ai-summon/summon/releases/latest}"
  if version_json=$(curl --fail --silent --max-time 10 "$api_url" 2>/dev/null); then
    tag=$(printf '%s' "$version_json" | grep '"tag_name"' | sed 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/' | head -n 1)
    if [ -n "$tag" ]; then
      printf '%s' "$tag"
      return
    fi
  fi

  warn "Could not determine latest version from GitHub API. Using fallback: $SUMMON_LATEST_VERSION"
  printf '%s' "$SUMMON_LATEST_VERSION"
}

resolve_target_path() {
  if [ -n "$SUMMON_INSTALL_PATH" ]; then
    printf '%s' "$SUMMON_INSTALL_PATH"
    return
  fi

  if [ "$(normalize_os)" = "darwin" ]; then
    printf '%s' "$HOME/.local/bin/summon"
  else
    printf '%s' "$HOME/.local/bin/summon"
  fi
}

ensure_writable_target() {
  target="$1"
  parent_dir=$(dirname "$target")
  mkdir -p "$parent_dir" 2>/dev/null || fail permission "Cannot create install directory: $parent_dir"
  [ -w "$parent_dir" ] || fail permission "Install directory is not writable: $parent_dir"
}

require_download_tool() {
  if [ "${SUMMON_TEST_DISABLE_DOWNLOAD_TOOL:-0}" = "1" ]; then
    fail download "Missing download tool. Install curl or wget."
  fi
  if command -v curl >/dev/null 2>&1; then
    printf 'curl'
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    printf 'wget'
    return
  fi
  fail download "Missing download tool. Install curl or wget."
}

download_file() {
  url="$1"
  output="$2"
  tool=$(require_download_tool)

  if [ "${SUMMON_TEST_ALLOW_INSECURE_URLS:-0}" = "1" ]; then
    case "$url" in
      https://*|file://*) ;;
      *) fail download "Only HTTPS URLs are allowed for downloads (test override permits file://)." ;;
    esac
  else
    case "$url" in
      https://*) ;;
      *) fail download "Only HTTPS URLs are allowed for downloads." ;;
    esac
  fi

  if [ "$tool" = "curl" ]; then
    curl --fail --location --retry 3 --retry-delay 1 --silent --show-error "$url" -o "$output" || fail download "Failed to download artifact: $url"
  else
    wget -q --tries=3 -O "$output" "$url" || fail download "Failed to download artifact: $url"
  fi
}

resolve_download_url() {
  version="$1"
  os_name="$2"
  arch_name="$3"
  if [ -n "$SUMMON_DOWNLOAD_URL" ]; then
    printf '%s' "$SUMMON_DOWNLOAD_URL"
    return
  fi
  artifact="summon_${version#v}_${os_name}_${arch_name}.tar.gz"
  printf '%s/%s/%s' "$SUMMON_RELEASE_BASE_URL" "$version" "$artifact"
}

resolve_checksum() {
  version="$1"
  artifact_name="$2"

  if [ -n "$SUMMON_CHECKSUM" ]; then
    printf '%s' "$SUMMON_CHECKSUM"
    return
  fi

  if [ -n "$SUMMON_CHECKSUM_URL" ]; then
    manifest_file="$TMP_DIR/checksums.txt"
    download_file "$SUMMON_CHECKSUM_URL" "$manifest_file"
    checksum=$(grep " $artifact_name$" "$manifest_file" | awk '{print $1}' | head -n 1 || true)
    [ -n "$checksum" ] || fail checksum "Checksum entry not found for $artifact_name"
    printf '%s' "$checksum"
    return
  fi

  manifest_url="${SUMMON_RELEASE_BASE_URL}/${version}/summon_${version#v}_checksums.txt"
  manifest_file="$TMP_DIR/checksums.txt"
  download_file "$manifest_url" "$manifest_file"
  checksum=$(grep " $artifact_name$" "$manifest_file" | awk '{print $1}' | head -n 1 || true)
  [ -n "$checksum" ] || fail checksum "Checksum entry not found for $artifact_name in manifest"
  printf '%s' "$checksum"
}

sha256_file() {
  file_path="$1"
  if [ "${SUMMON_TEST_DISABLE_HASH_TOOL:-0}" = "1" ]; then
    fail checksum "Missing checksum tool. Install shasum or sha256sum."
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file_path" | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file_path" | awk '{print $1}'
    return
  fi
  fail checksum "Missing checksum tool. Install shasum or sha256sum."
}

extract_binary() {
  artifact_path="$1"
  os_name="$2"
  extracted_path="$TMP_DIR/summon"

  case "$artifact_path" in
    *.tar.gz)
      tar -xzf "$artifact_path" -C "$TMP_DIR" || fail download "Failed to extract artifact archive"
      [ -f "$TMP_DIR/summon" ] || fail download "Archive did not contain summon binary"
      ;;
    *)
      cp "$artifact_path" "$extracted_path"
      ;;
  esac

  chmod +x "$extracted_path"
  printf '%s' "$extracted_path"

  _unused="$os_name"
}

update_path_if_needed() {
  bin_dir="$1"

  if [ "$SUMMON_NO_MODIFY_PATH" = "1" ]; then
    info "PATH update skipped (SUMMON_NO_MODIFY_PATH=1)."
    info "Add this to your shell profile: export PATH=\"$bin_dir:\$PATH\""
    return
  fi

  case ":$PATH:" in
    *":$bin_dir:"*)
      info "PATH already includes $bin_dir"
      return
      ;;
  esac

  profile="$HOME/.profile"
  if [ "${SHELL:-}" != "" ]; then
    case "$SHELL" in
      */zsh) profile="$HOME/.zprofile" ;;
      */bash) profile="$HOME/.bashrc" ;;
    esac
  fi

  line="export PATH=\"$bin_dir:\$PATH\""
  touch "$profile" || {
    info "Could not update $profile automatically."
    info "Run manually: $line"
    return
  }

  if ! grep -F "$line" "$profile" >/dev/null 2>&1; then
    printf '\n%s\n' "$line" >>"$profile" || {
      info "Could not update $profile automatically."
      info "Run manually: $line"
      return
    }
    info "Updated PATH in $profile"
  fi
}

warn_if_shadowed() {
  installed_path="$1"
  found=$(command -v summon 2>/dev/null || true)
  if [ -n "$found" ] && [ "$found" != "$installed_path" ]; then
    warn "Another summon binary appears earlier in PATH: $found"
  fi
}

main() {
  OS_NAME=$(normalize_os)
  ARCH_NAME=$(normalize_arch)
  VERSION=$(resolve_version)
  TARGET_PATH=$(resolve_target_path)

  ensure_writable_target "$TARGET_PATH"

  TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/summon-install.XXXXXX")
  cleanup() {
    rm -rf "$TMP_DIR"
  }
  trap cleanup EXIT INT TERM

  DOWNLOAD_URL=$(resolve_download_url "$VERSION" "$OS_NAME" "$ARCH_NAME")
  CANONICAL_ARTIFACT="summon_${VERSION#v}_${OS_NAME}_${ARCH_NAME}.tar.gz"
  if [ -n "$SUMMON_DOWNLOAD_URL" ]; then
    ARTIFACT_NAME=$(basename "$SUMMON_DOWNLOAD_URL")
  else
    ARTIFACT_NAME="$CANONICAL_ARTIFACT"
  fi
  ARTIFACT_PATH="$TMP_DIR/$ARTIFACT_NAME"

  info "Installing summon $VERSION for $OS_NAME/$ARCH_NAME"
  download_file "$DOWNLOAD_URL" "$ARTIFACT_PATH"

  EXPECTED_CHECKSUM=$(resolve_checksum "$VERSION" "$CANONICAL_ARTIFACT")
  ACTUAL_CHECKSUM=$(sha256_file "$ARTIFACT_PATH")
  [ "$EXPECTED_CHECKSUM" = "$ACTUAL_CHECKSUM" ] || fail checksum "Checksum mismatch. Expected $EXPECTED_CHECKSUM got $ACTUAL_CHECKSUM"

  STAGED_BINARY=$(extract_binary "$ARTIFACT_PATH" "$OS_NAME")
  STAGE_PATH="$TMP_DIR/summon.staged"
  cp "$STAGED_BINARY" "$STAGE_PATH"
  chmod +x "$STAGE_PATH"

  mv "$STAGE_PATH" "$TARGET_PATH" || fail permission "Failed to activate binary at $TARGET_PATH"

  update_path_if_needed "$(dirname "$TARGET_PATH")"
  warn_if_shadowed "$TARGET_PATH"

  info "Installed summon at: $TARGET_PATH"
  info "Verify with: $TARGET_PATH --version"
  info "Upgrade by rerunning this installer command."

  _unused="$SUMMON_NONINTERACTIVE"
}

main "$@"
