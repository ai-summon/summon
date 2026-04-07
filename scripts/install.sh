#!/usr/bin/env sh
set -eu

SUMMON_VERSION="${SUMMON_VERSION:-latest}"
SUMMON_INSTALL_PATH="${SUMMON_INSTALL_PATH:-}"
SUMMON_NONINTERACTIVE="${SUMMON_NONINTERACTIVE:-0}"
SUMMON_NO_MODIFY_PATH="${SUMMON_NO_MODIFY_PATH:-0}"
SUMMON_QUIET="${SUMMON_QUIET:-0}"
SUMMON_DOWNLOAD_URL="${SUMMON_DOWNLOAD_URL:-}"
SUMMON_CHECKSUM_URL="${SUMMON_CHECKSUM_URL:-}"
SUMMON_CHECKSUM="${SUMMON_CHECKSUM:-}"
SUMMON_LATEST_VERSION="${SUMMON_LATEST_VERSION:-v0.0.5}"
SUMMON_RELEASE_BASE_URL="${SUMMON_RELEASE_BASE_URL:-https://github.com/ai-summon/summon/releases/download}"

# Track whether the user explicitly set SUMMON_NONINTERACTIVE
_user_set_noninteractive="$SUMMON_NONINTERACTIVE"

# Determine interactive mode: use /dev/tty when stdin is piped (curl | sh),
# fall back to non-interactive only if /dev/tty is unavailable or explicitly disabled
_need_tty=no
if [ "$_user_set_noninteractive" != "1" ]; then
  if [ -t 0 ]; then
    # stdin is a TTY — interactive mode via stdin
    _need_tty=no
  elif [ -e /dev/tty ] && [ -r /dev/tty ]; then
    # stdin is piped but /dev/tty is available (curl | sh pattern)
    _need_tty=yes
  else
    # no TTY available at all — non-interactive
    SUMMON_NONINTERACTIVE="1"
  fi
else
  SUMMON_NONINTERACTIVE="1"
fi

# Detect terminal ANSI capability
_use_style=false
if [ -t 2 ]; then
  if [ "${TERM+set}" = 'set' ]; then
    case "$TERM" in
      xterm*|rxvt*|urxvt*|linux*|vt*) _use_style=true ;;
    esac
  fi
fi
# Honor NO_COLOR convention (https://no-color.org/)
if [ "${NO_COLOR+set}" = 'set' ]; then
  _use_style=false
fi

fail() {
  category="$1"
  message="$2"
  if $_use_style; then
    printf '\033[1;31merror[%s]\033[0m: %s\n' "$category" "$message" >&2
  else
    printf 'ERROR[%s]: %s\n' "$category" "$message" >&2
  fi
  exit 1
}

warn() {
  if $_use_style; then
    printf '\033[1;33mwarn\033[0m: %s\n' "$1" >&2
  else
    printf 'WARN: %s\n' "$1" >&2
  fi
}

say() {
  if [ "$SUMMON_QUIET" = "1" ]; then
    return
  fi
  if $_use_style; then
    printf '\033[1minfo\033[0m: %s\n' "$1"
  else
    printf 'info: %s\n' "$1"
  fi
}

info() {
  if [ "$SUMMON_QUIET" = "1" ]; then
    return
  fi
  printf '%s\n' "$1"
}

bold() {
  if $_use_style; then
    printf '\033[1m%s\033[0m' "$1"
  else
    printf '%s' "$1"
  fi
}

read_input() {
  if [ "$_need_tty" = "yes" ]; then
    read -r _input </dev/tty || _input=""
  else
    read -r _input || _input=""
  fi
  printf '%s' "$_input"
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

resolve_shell_profile() {
  profile="$HOME/.profile"
  if [ "${SHELL:-}" != "" ]; then
    case "$SHELL" in
      */zsh) profile="$HOME/.zprofile" ;;
      */bash) profile="$HOME/.bashrc" ;;
    esac
  fi
  printf '%s' "$profile"
}

detect_existing_version() {
  target="$1"
  if [ -x "$target" ]; then
    version=$("$target" --version 2>/dev/null | head -n 1 || true)
    if [ -n "$version" ]; then
      printf '%s' "$version"
      return
    fi
  fi
  printf ''
}

show_welcome_banner() {
  info ""
  if $_use_style; then
    printf '\033[1mWelcome to Summon!\033[0m\n'
  else
    printf 'Welcome to Summon!\n'
  fi
  info ""
  info "This will install the summon AI package manager."
}

show_install_details() {
  _target="$1"
  _existing="$2"
  info ""
  if [ -n "$_existing" ]; then
    info "An existing installation was detected ($_existing)."
    info "The summon binary will be upgraded at:"
  else
    info "The summon binary will be installed at:"
  fi
  info ""
  info "    $_target"
  info ""
  info "This can be changed with the SUMMON_INSTALL_PATH environment variable."
}

show_path_info() {
  _bin_dir="$1"
  _profile="$2"
  info ""
  if [ "$SUMMON_NO_MODIFY_PATH" = "1" ]; then
    info "PATH modification is disabled (SUMMON_NO_MODIFY_PATH=1)."
    info "Add this to your shell profile manually: export PATH=\"$_bin_dir:\$PATH\""
  else
    case ":$PATH:" in
      *":$_bin_dir:"*)
        info "PATH already includes the install directory ($_bin_dir)."
        ;;
      *)
        info "The PATH will be updated by modifying the profile file located at:"
        info ""
        info "    $_profile"
        ;;
    esac
  fi
}

show_uninstall_info() {
  info ""
  info "You can uninstall at any time with summon self uninstall and"
  info "these changes will be reverted."
}

show_options_summary() {
  _version="$1"
  _os="$2"
  _arch="$3"
  _target="$4"
  _modify_path="$5"
  info ""
  if $_use_style; then
    printf '\033[1mCurrent installation options:\033[0m\n'
  else
    printf 'Current installation options:\n'
  fi
  info ""
  printf '        version: %s\n' "$(bold "$_version")"
  printf '       platform: %s\n' "$(bold "$_os/$_arch")"
  printf '   install path: %s\n' "$(bold "$_target")"
  printf '    modify PATH: %s\n' "$(bold "$_modify_path")"
}

show_pre_install_summary() {
  _version="$1"
  _os="$2"
  _arch="$3"
  _target="$4"
  _existing="$5"
  _bin_dir=$(dirname "$_target")
  _profile=$(resolve_shell_profile)
  _modify_path_label="yes"
  if [ "$SUMMON_NO_MODIFY_PATH" = "1" ]; then
    _modify_path_label="no"
  fi

  show_welcome_banner
  show_install_details "$_target" "$_existing"
  show_path_info "$_bin_dir" "$_profile"
  show_uninstall_info
  show_options_summary "$_version" "$_os" "$_arch" "$_target" "$_modify_path_label"
}

show_menu() {
  info "" >&2
  info "1) Proceed with standard installation (default - just press enter)" >&2
  info "2) Customize installation" >&2
  info "3) Cancel installation" >&2
  printf '> ' >&2
  _choice=$(read_input)
  printf '%s' "$_choice"
}

run_customization() {
  info ""
  printf 'Enter install path [%s]:\n> ' "$TARGET_PATH"
  _new_path=$(read_input)
  if [ -n "$_new_path" ]; then
    TARGET_PATH="$_new_path"
    SUMMON_INSTALL_PATH="$TARGET_PATH"
  fi
  info ""
  printf 'Modify PATH? [Y/n]:\n> '
  _modify=$(read_input)
  case "$_modify" in
    n|N|no|No|NO) SUMMON_NO_MODIFY_PATH="1" ;;
    *) SUMMON_NO_MODIFY_PATH="0" ;;
  esac
}

show_post_install_summary() {
  _target="$1"
  _bin_dir=$(dirname "$_target")
  _profile=$(resolve_shell_profile)
  _path_modified="$2"

  info ""
  if $_use_style; then
    printf '\033[1msummon is installed now. Great!\033[0m\n'
  else
    printf 'summon is installed now. Great!\n'
  fi

  if [ "$_path_modified" = "yes" ]; then
    info ""
    info "To get started you may need to restart your current shell."
    info "This would reload your PATH environment variable to include"
    info "the summon install directory ($_bin_dir)."
    info ""
    info "To configure your current shell, run:"
    info ""
    info "    . \"$_profile\""
  fi

  info ""
  info "To verify your installation:"
  info ""
  info "    summon --version"
  info ""
  info "To upgrade summon, rerun this installer."
  info "To uninstall, run: summon self uninstall"
}

update_path_if_needed() {
  bin_dir="$1"
  _path_was_modified="no"

  if [ "$SUMMON_NO_MODIFY_PATH" = "1" ]; then
    say "PATH update skipped (SUMMON_NO_MODIFY_PATH=1)."
    info "Add this to your shell profile: export PATH=\"$bin_dir:\$PATH\""
    return
  fi

  case ":$PATH:" in
    *":$bin_dir:"*)
      say "PATH already includes $bin_dir"
      return
      ;;
  esac

  profile=$(resolve_shell_profile)

  line="export PATH=\"$bin_dir:\$PATH\""
  touch "$profile" || {
    say "Could not update $profile automatically."
    info "Run manually: $line"
    return
  }

  if ! grep -F "$line" "$profile" >/dev/null 2>&1; then
    printf '\n%s\n' "$line" >>"$profile" || {
      say "Could not update $profile automatically."
      info "Run manually: $line"
      return
    }
    say "Updated PATH in $profile"
    _path_was_modified="yes"
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

  # Detect existing installation
  _existing_version=$(detect_existing_version "$TARGET_PATH")

  # Show welcome banner and pre-install summary (interactive only)
  if [ "$SUMMON_NONINTERACTIVE" != "1" ]; then
    show_pre_install_summary "$VERSION" "$OS_NAME" "$ARCH_NAME" "$TARGET_PATH" "$_existing_version"

    _menu_done=false
    while [ "$_menu_done" = "false" ]; do
      _choice=$(show_menu)
      case "$_choice" in
        1|"")
          _menu_done=true
          ;;
        2)
          run_customization
          TARGET_PATH=$(resolve_target_path)
          ensure_writable_target "$TARGET_PATH"
          _existing_version=$(detect_existing_version "$TARGET_PATH")
          show_pre_install_summary "$VERSION" "$OS_NAME" "$ARCH_NAME" "$TARGET_PATH" "$_existing_version"
          ;;
        3)
          info ""
          info "Installation cancelled."
          exit 0
          ;;
        *)
          info "Invalid option. Please select 1, 2, or 3."
          ;;
      esac
    done
  fi

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

  say "Installing summon $VERSION for $OS_NAME/$ARCH_NAME"
  download_file "$DOWNLOAD_URL" "$ARTIFACT_PATH"

  EXPECTED_CHECKSUM=$(resolve_checksum "$VERSION" "$CANONICAL_ARTIFACT")
  ACTUAL_CHECKSUM=$(sha256_file "$ARTIFACT_PATH")
  [ "$EXPECTED_CHECKSUM" = "$ACTUAL_CHECKSUM" ] || fail checksum "Checksum mismatch. Expected $EXPECTED_CHECKSUM got $ACTUAL_CHECKSUM"

  STAGED_BINARY=$(extract_binary "$ARTIFACT_PATH" "$OS_NAME")
  STAGE_PATH="$TMP_DIR/summon.staged"
  cp "$STAGED_BINARY" "$STAGE_PATH"
  chmod +x "$STAGE_PATH"

  mv "$STAGE_PATH" "$TARGET_PATH" || fail permission "Failed to activate binary at $TARGET_PATH"

  _path_was_modified="no"
  update_path_if_needed "$(dirname "$TARGET_PATH")"
  warn_if_shadowed "$TARGET_PATH"

  if [ "$SUMMON_NONINTERACTIVE" != "1" ]; then
    show_post_install_summary "$TARGET_PATH" "$_path_was_modified"
  else
    say "Installed summon at: $TARGET_PATH"
    info "Verify with: $TARGET_PATH --version"
    info "Upgrade by rerunning this installer command."
  fi
}

main "$@"
