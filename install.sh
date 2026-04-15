#!/bin/sh
set -eu

REPO="ai-summon/summon"
INSTALL_DIR="${SUMMON_INSTALL_DIR:-$HOME/.summon}"
BIN_DIR="${INSTALL_DIR}/bin"

err() {
    printf "Error: %s\n" "$1" >&2
    exit 1
}

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

add_to_path() {
    if [ "${SUMMON_NO_MODIFY_PATH:-}" ]; then
        return
    fi

    path_entry="export PATH=\"${BIN_DIR}:\$PATH\""
    fish_entry="set -gx PATH ${BIN_DIR} \$PATH"

    modified_profile=false

    for rc_file in "$HOME/.bashrc" "$HOME/.zshrc"; do
        if [ -f "$rc_file" ]; then
            if ! grep -qF "$BIN_DIR" "$rc_file"; then
                printf '\n# Added by summon installer\n%s\n' "$path_entry" >> "$rc_file"
            fi
            modified_profile=true
        fi
    done

    if [ "$modified_profile" = false ] && [ -f "$HOME/.profile" ]; then
        if ! grep -qF "$BIN_DIR" "$HOME/.profile"; then
            printf '\n# Added by summon installer\n%s\n' "$path_entry" >> "$HOME/.profile"
        fi
    fi

    fish_conf_dir="$HOME/.config/fish/conf.d"
    if [ -d "$HOME/.config/fish" ]; then
        mkdir -p "$fish_conf_dir"
        fish_file="${fish_conf_dir}/summon.fish"
        if [ ! -f "$fish_file" ] || ! grep -qF "$BIN_DIR" "$fish_file"; then
            printf '# Added by summon installer\n%s\n' "$fish_entry" > "$fish_file"
        fi
    fi
}

main() {
    OS=$(detect_os)
    ARCH=$(detect_arch)
    DOWNLOAD_TOOL=$(detect_download_tool)

    printf "Detecting platform: %s/%s\n" "$OS" "$ARCH"

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    VERSION=$(get_latest_version)
    printf "Installing summon %s\n" "$VERSION"

    ARCHIVE="summon-${OS}-${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

    printf "Downloading %s...\n" "$ARCHIVE"
    download "$DOWNLOAD_URL" "${TMPDIR}/${ARCHIVE}" || err "Failed to download ${ARCHIVE} (HTTP 404). Version ${VERSION} may not exist."
    download "$CHECKSUMS_URL" "${TMPDIR}/checksums.txt" || err "Failed to download checksums.txt."

    printf "Verifying checksum...\n"
    verify_checksum "${TMPDIR}/${ARCHIVE}" "${TMPDIR}/checksums.txt"

    mkdir -p "$BIN_DIR" || err "Cannot write to ${BIN_DIR}. Set SUMMON_INSTALL_DIR to a writable directory."
    tar xzf "${TMPDIR}/${ARCHIVE}" -C "$BIN_DIR"
    chmod +x "${BIN_DIR}/summon"

    add_to_path

    printf "\nsummon %s installed successfully to %s/summon\n" "$VERSION" "$BIN_DIR"
    if [ "${SUMMON_NO_MODIFY_PATH:-}" ]; then
        printf "Add the following to your shell profile:\n  export PATH=\"%s:\$PATH\"\n" "$BIN_DIR"
    else
        printf "Restart your shell or run: export PATH=\"%s:\$PATH\"\n" "$BIN_DIR"
    fi
}

main
