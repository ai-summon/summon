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

By default, summon installs to `~/.summon/bin/`. Override with `SUMMON_INSTALL_DIR`:

```sh
# macOS / Linux
export SUMMON_INSTALL_DIR=/opt/summon
curl -fsSL https://raw.githubusercontent.com/ai-summon/summon/main/install.sh | sh
```

```powershell
# Windows
$env:SUMMON_INSTALL_DIR = "C:\Tools\summon"
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

## License

See [LICENSE](LICENSE) for details.
