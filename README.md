# Summon

Unified plugin dependency manager for AI CLIs (Copilot CLI and Claude Code CLI).

## Install

### macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/ai-summon/summon/main/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/ai-summon/summon/main/install.ps1 | iex
```

### Custom Install Directory

By default, summon installs to `~/.local/bin/` (following [XDG conventions](https://specifications.freedesktop.org/basedir-spec/latest/), matching [UV](https://docs.astral.sh/uv/)). Override with `SUMMON_INSTALL_DIR`:

```sh
# macOS / Linux
export SUMMON_INSTALL_DIR=/opt/bin
curl -fsSL https://raw.githubusercontent.com/ai-summon/summon/main/install.sh | sh
```

```powershell
# Windows
$env:SUMMON_INSTALL_DIR = "C:\Tools\bin"
irm https://raw.githubusercontent.com/ai-summon/summon/main/install.ps1 | iex
```

### Skip PATH Modification

If you manage your shell profiles manually:

```sh
# macOS / Linux
export SUMMON_NO_MODIFY_PATH=1
curl -fsSL https://raw.githubusercontent.com/ai-summon/summon/main/install.sh | sh
```

```powershell
# Windows
$env:SUMMON_NO_MODIFY_PATH = "1"
irm https://raw.githubusercontent.com/ai-summon/summon/main/install.ps1 | iex
```

## Build from Source

```sh
go build -ldflags "-X main.version=dev" -o summon ./cmd/summon
./summon --version
```

## Verify Installation

```sh
summon --version
```

## Uninstall

### macOS / Linux

```sh
# Remove summon binary
rm -f ~/.local/bin/summon

# Remove summon config data
rm -rf ~/.summon
```

### Windows (PowerShell)

```powershell
# Remove summon binary
Remove-Item -Force "$env:USERPROFILE\.local\bin\summon.exe"

# Remove summon config data
Remove-Item -Recurse -Force "$env:USERPROFILE\.summon"

# Remove the summon PATH entry from your User environment variables
```

## License

See [LICENSE](LICENSE) for details.
