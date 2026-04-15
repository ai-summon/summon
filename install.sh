#!/bin/sh
set -eu

REPO="ai-summon/summon"

err() {
    printf "Error: %s\n" "$1" >&2
    exit 1
}

# Infer $HOME from passwd if not set (matching UV's get_home logic).
get_home() {
    if [ -n "${HOME:-}" ]; then
        echo "$HOME"
    elif [ -n "${USER:-}" ]; then
        getent passwd "$USER" | cut -d: -f6
    else
        getent passwd "$(id -un)" | cut -d: -f6
    fi
}

INFERRED_HOME=$(get_home)

detect_os() {
    case "$(uname -s)" in
        Linux)  echo "linux" ;;
        Darwin) echo "darwin" ;;
        *)      err "Unsupported operating system: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)         echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *)              err "Unsupported architecture: $(uname -m)" ;;
    esac
}

detect_download_tool() {
    if command -v curl >/dev/null 2>&1; then
        echo "curl"
    elif command -v wget >/dev/null 2>&1; then
        echo "wget"
    else
        err "curl is required but not found. Install curl or wget and try again."
    fi
}

download() {
    url="$1"
    output="$2"
    case "$DOWNLOAD_TOOL" in
        curl) curl -fsSL -o "$output" "$url" ;;
        wget) wget -qO "$output" "$url" ;;
    esac
}

get_latest_version() {
    if [ "${SUMMON_VERSION:-}" ]; then
        echo "$SUMMON_VERSION"
        return
    fi
    url="https://api.github.com/repos/${REPO}/releases/latest"
    tmpfile="${TMPDIR}/api_response"
    download "$url" "$tmpfile"
    version=$(grep '"tag_name"' "$tmpfile" | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    if [ -z "$version" ]; then
        err "Failed to determine latest version from GitHub API."
    fi
    echo "$version"
}

verify_checksum() {
    archive_file="$1"
    checksums_file="$2"
    archive_name=$(basename "$archive_file")

    expected=$(grep "$archive_name" "$checksums_file" | awk '{print $1}')
    if [ -z "$expected" ]; then
        err "No checksum found for ${archive_name} in checksums.txt."
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive_file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive_file" | awk '{print $1}')
    else
        err "sha256sum or shasum is required for checksum verification."
    fi

    if [ "$expected" != "$actual" ]; then
        err "Checksum verification failed! The downloaded file may have been tampered with."
    fi
}

# Resolve the install directory using XDG conventions (matching UV's logic).
# Priority: $SUMMON_INSTALL_DIR > $XDG_BIN_HOME > $XDG_DATA_HOME/../bin > $HOME/.local/bin
resolve_install_dir() {
    if [ "${SUMMON_INSTALL_DIR:-}" ]; then
        echo "$SUMMON_INSTALL_DIR"
        return
    fi
    if [ "${XDG_BIN_HOME:-}" ]; then
        echo "$XDG_BIN_HOME"
        return
    fi
    if [ "${XDG_DATA_HOME:-}" ]; then
        echo "$XDG_DATA_HOME/../bin"
        return
    fi
    echo "$INFERRED_HOME/.local/bin"
}

# Write an env script (sh) that conditionally prepends install_dir to PATH.
# This matches UV/cargo-dist's approach for idempotent PATH management.
write_env_script_sh() {
    _install_dir_expr="$1"
    _env_script_path="$2"
    cat <<EOF > "$_env_script_path"
#!/bin/sh
# add summon to PATH if not already present
case ":\${PATH}:" in
    *:"$_install_dir_expr":*)
        ;;
    *)
        export PATH="$_install_dir_expr:\$PATH"
        ;;
esac
EOF
}

# Write an env script (fish) that conditionally prepends install_dir to PATH.
write_env_script_fish() {
    _install_dir_expr="$1"
    _env_script_path="$2"
    cat <<EOF > "$_env_script_path"
if not contains "$_install_dir_expr" \$PATH
    set -x PATH "$_install_dir_expr" \$PATH
end
EOF
}

# Add install dir to CI PATH ($GITHUB_PATH) if available.
add_install_dir_to_ci_path() {
    _install_dir="$1"
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$_install_dir" >> "$GITHUB_PATH"
    fi
}

# Add install dir to PATH via env-script sourced from rcfiles.
# Creates the env script if it doesn't exist, then adds a source line
# to the first matching rcfile (or creates the first one if none exist).
add_install_dir_to_rcfile() {
    _install_dir_expr="$1"
    _env_script_path="$2"
    _env_script_path_expr="$3"
    _rcfiles="$4"
    _shell="$5"

    _robust_line=". \"$_env_script_path_expr\""

    # Create the env script if it doesn't exist
    if [ ! -f "$_env_script_path" ]; then
        if [ "$_shell" = "sh" ]; then
            write_env_script_sh "$_install_dir_expr" "$_env_script_path"
        else
            write_env_script_fish "$_install_dir_expr" "$_env_script_path"
        fi
    fi

    # Find the first existing rcfile, or default to the first in the list
    _target=""
    for _rcfile_relative in $_rcfiles; do
        # Handle ZDOTDIR for zsh files
        case "$_rcfile_relative" in
            .zsh*)
                _home="${ZDOTDIR:-$INFERRED_HOME}"
                ;;
            *)
                _home="$INFERRED_HOME"
                ;;
        esac
        _rcfile="$_home/$_rcfile_relative"
        if [ -f "$_rcfile" ]; then
            _target="$_rcfile"
            break
        fi
    done
    if [ -z "$_target" ]; then
        _rcfile_relative=$(echo "$_rcfiles" | awk '{ print $1 }')
        # Use the same zsh-aware home for fallback (matching UV)
        case "$_rcfile_relative" in
            .zsh*)
                _home="${ZDOTDIR:-$INFERRED_HOME}"
                ;;
            *)
                _home="$INFERRED_HOME"
                ;;
        esac
        _target="$_home/$_rcfile_relative"
    fi

    # For fish, use 'source' instead of '.'
    if [ "$_shell" = "fish" ]; then
        _line="source \"$_env_script_path_expr\""
    else
        _line="$_robust_line"
    fi

    # Add the source line if not already present
    if ! grep -qF "$_robust_line" "$_target" 2>/dev/null && \
       ! grep -qF "source \"$_env_script_path_expr\"" "$_target" 2>/dev/null; then
        if [ -f "$_env_script_path" ]; then
            echo "" >> "$_target"
            echo "$_line" >> "$_target"
            return 1
        fi
    fi
    return 0
}

# Write source lines to all existing rcfiles in the list (shotgun approach,
# matching UV's behavior for bash rcfiles).
shotgun_add_to_rcfiles() {
    _install_dir_expr="$1"
    _env_script_path="$2"
    _env_script_path_expr="$3"
    _rcfiles="$4"
    _shell="$5"

    _found=false
    _modified=0
    for _rcfile_relative in $_rcfiles; do
        _rcfile="$INFERRED_HOME/$_rcfile_relative"
        if [ -f "$_rcfile" ]; then
            _found=true
            _rc=0
            add_install_dir_to_rcfile "$_install_dir_expr" "$_env_script_path" "$_env_script_path_expr" "$_rcfile_relative" "$_shell" || _rc=$?
            if [ "$_rc" = 1 ]; then
                _modified=1
            fi
        fi
    done

    if [ "$_found" = false ]; then
        add_install_dir_to_rcfile "$_install_dir_expr" "$_env_script_path" "$_env_script_path_expr" "$_rcfiles" "$_shell" || _modified=$?
    fi

    return "$_modified"
}

# Replace $HOME with '$HOME' for late-bound expressions in rcfiles.
replace_home() {
    _str="$1"
    if [ -n "${HOME:-}" ]; then
        echo "$_str" | sed "s,$INFERRED_HOME,\$HOME,"
    else
        echo "$_str"
    fi
}

add_to_path() {
    if [ "${SUMMON_NO_MODIFY_PATH:-}" ]; then
        return
    fi

    # Skip if install dir is already on PATH
    case ":$PATH:" in
        *:"$BIN_DIR":*) return ;;
    esac

    _install_dir_expr=$(replace_home "$BIN_DIR")
    _env_script_path="$BIN_DIR/env"
    _env_script_path_expr=$(replace_home "$_env_script_path")
    _fish_env_script_path="${_env_script_path}.fish"
    _fish_env_script_path_expr="${_env_script_path_expr}.fish"

    add_install_dir_to_ci_path "$BIN_DIR"

    # .profile (primary)
    _r1=0
    add_install_dir_to_rcfile "$_install_dir_expr" "$_env_script_path" "$_env_script_path_expr" ".profile" "sh" || _r1=$?

    # bash rcfiles — write to all that exist
    _r2=0
    shotgun_add_to_rcfiles "$_install_dir_expr" "$_env_script_path" "$_env_script_path_expr" ".bashrc .bash_profile .bash_login" "sh" || _r2=$?

    # zsh rcfiles
    _r3=0
    add_install_dir_to_rcfile "$_install_dir_expr" "$_env_script_path" "$_env_script_path_expr" ".zshrc .zshenv" "sh" || _r3=$?

    # fish
    _r4=0
    mkdir -p "$INFERRED_HOME/.config/fish/conf.d" || true
    add_install_dir_to_rcfile "$_install_dir_expr" "$_fish_env_script_path" "$_fish_env_script_path_expr" ".config/fish/conf.d/summon.env.fish" "fish" || _r4=$?

    if [ "$_r1" = 1 ] || [ "$_r2" = 1 ] || [ "$_r3" = 1 ] || [ "$_r4" = 1 ]; then
        printf "\nTo add %s to your PATH, either restart your shell or run:\n" "$_install_dir_expr"
        printf "\n    source %s (sh, bash, zsh)\n" "$_env_script_path_expr"
        printf "    source %s (fish)\n" "$_fish_env_script_path_expr"
    fi
}

main() {
    OS=$(detect_os)
    ARCH=$(detect_arch)
    DOWNLOAD_TOOL=$(detect_download_tool)

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    VERSION=$(get_latest_version)

    ARCHIVE="summon-${OS}-${ARCH}.tar.gz"
    DOWNLOAD_BASE="${SUMMON_DOWNLOAD_BASE:-https://github.com/${REPO}/releases/download}"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/${VERSION}/${ARCHIVE}"
    CHECKSUMS_URL="${DOWNLOAD_BASE}/${VERSION}/checksums.txt"

    printf "downloading summon %s %s-%s\n" "$VERSION" "$OS" "$ARCH"
    download "$DOWNLOAD_URL" "${TMPDIR}/${ARCHIVE}" || err "Failed to download ${ARCHIVE} (HTTP 404). Version ${VERSION} may not exist."
    download "$CHECKSUMS_URL" "${TMPDIR}/checksums.txt" || err "Failed to download checksums.txt."

    verify_checksum "${TMPDIR}/${ARCHIVE}" "${TMPDIR}/checksums.txt"

    BIN_DIR=$(resolve_install_dir)
    mkdir -p "$BIN_DIR" || err "Cannot write to ${BIN_DIR}. Set SUMMON_INSTALL_DIR to a writable directory."
    tar xzf "${TMPDIR}/${ARCHIVE}" -C "$BIN_DIR"
    chmod +x "${BIN_DIR}/summon"

    printf "installing to %s\n" "$BIN_DIR"
    printf "    summon\n"

    add_to_path

    printf "everything's installed!\n"
}

main
