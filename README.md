<p align="center">
  <img src="assets/Summon.png" alt="Summon" width="234" />
</p>

# Summon

[![Tests](https://github.com/ai-summon/summon/actions/workflows/test.yml/badge.svg)](https://github.com/ai-summon/summon/actions/workflows/test.yml)
[![Installer Validation](https://github.com/ai-summon/summon/actions/workflows/installer.yml/badge.svg)](https://github.com/ai-summon/summon/actions/workflows/installer.yml)

Summon is a cross-platform package manager for AI agent components: skills,
agents, commands, hooks, and MCP servers.

It installs packages from GitHub or local paths and wires them into supported
platforms automatically. Current platform support includes Claude Code and
VS Code Copilot.

## Highlights

- One CLI for install, update, uninstall, list, and package scaffolding.
- Composable packages via dependencies, with automatic dependency installation
- Supports local, project, and user scopes with deterministic precedence.
- Team-friendly project scope with restorable registry workflow.
- Secure standalone installers for macOS, Linux, and Windows.
- Native platform activation for Claude Code and VS Code Copilot.

## Installation

Install with standalone scripts:

```bash
# macOS and Linux
curl -fsSL https://raw.githubusercontent.com/ai-summon/summon/main/scripts/install.sh | sh
```

```powershell
# Windows PowerShell
powershell -ExecutionPolicy Bypass -Command "iwr https://raw.githubusercontent.com/ai-summon/summon/main/scripts/install.ps1 -UseBasicParsing | iex"
```

Or install from source with Go:

```bash
go install github.com/ai-summon/summon/cmd/summon@latest
```

Verify installation:

```bash
summon --version
```

## Quick Start

### Installing Packages

```bash
# Install from built-in catalog (default: local scope)
summon install superpowers

# Install from GitHub
summon install github:obra/superpowers

# Install from local path (symlinked for development)
summon install --path ../my-plugin

# Install to project scope (committable, team-shared)
summon install -p superpowers

# Install to user scope (available across all workspaces)
summon install -g superpowers

# See installed packages
summon list

# Update packages
summon update

# Remove package
summon uninstall superpowers
```

### Creating Packages

```bash
# Create a new plugin project (generic type)
summon new my-plugin

# Create a specific plugin type
summon new --type skill my-skill
summon new --type agent my-agent
summon new --type command my-command
summon new --type hook my-hook
summon new --type mcp my-mcp-server

# Create with custom display name
summon new --name "My Awesome Plugin" my-plugin-folder

# Create without version control
summon new --vcs none my-plugin
```

## Documentation

- [Package Development](docs/development.md)
- [Plugin Discovery Architecture](docs/plugin-discovery.md)

For CLI details, run `summon --help` or `summon <command> --help`.

## Features

### Scopes

Summon supports three writable scopes:

| Scope | Location | Visibility | Typical Usage |
|---|---|---|---|
| `local` | `<project>/.summon/local/` | Current checkout only | Day-to-day personal setup |
| `project` | `<project>/.summon/project/` | Team-visible in repo | Reproducible team setup |
| `user` | `~/.summon/user/` | User-wide | Shared tools across projects |

Precedence is `local > project > user`.

```bash
summon install superpowers              # local (default)
summon install -p superpowers            # project scope
summon install -g superpowers            # user scope
summon install --scope project superpowers  # equivalent to -p

summon install                # restore project scope
summon install --scope user   # restore user scope
```

### Platform Integration

Summon auto-detects and registers with:

| Platform | Detection | Integration Surface |
|---|---|---|
| Claude Code | `~/.claude` exists | Known marketplaces, enabled plugins |
| VS Code Copilot | VS Code user config exists | Plugin locations, marketplaces |

### Standalone Installer Behavior

- Verifies SHA-256 checksum before activation.
- Defaults to user-writable install paths.
- Updates PATH when possible, and prints exact fallback commands otherwise.
- Supports rerun-safe updates by rerunning the same installer command.

Uninstall installer-managed binary:

```bash
rm -f "$HOME/.local/bin/summon"
```

```powershell
Remove-Item "$HOME/.summon/bin/summon.exe" -Force
```

## Command Overview

### summon install [package]

```bash
summon install superpowers
summon install github:user/repo
summon install --path ../local-plugin
summon install --ref v2.0.0 superpowers
summon install --scope project superpowers
summon install --global superpowers
summon install --force superpowers
```

### summon uninstall <package>

```bash
summon uninstall superpowers
summon uninstall --scope local superpowers
summon uninstall --scope project superpowers
summon uninstall --global superpowers
```

### summon update [package]

```bash
summon update
summon update superpowers
summon update --scope user
```

### summon list

```bash
summon list
summon list --scope project
summon list --scope user
summon list --json
```

### summon init

```bash
summon init
summon init --name my-skills
summon init --platform claude --platform copilot
```

## Package Format

Summon packages are declared with `summon.yaml`:

```yaml
name: my-skills
version: "1.0.0"
description: "A collection of useful skills"

platforms:
  - claude
  - copilot

components:
  skills: ./skills/
  agents: ./agents/
  commands: ./commands/

dependencies:
  superpowers: ">=5.0.0"

summon_version: ">=0.1.0"
```

Required fields are `name`, `version`, and `description`.

## Built-in Catalog

Install by package name:

- `superpowers`
- `cc-spex`
- `get-shit-done`
- `spec-kit`

```bash
summon install superpowers
```

To add a catalog entry, update `internal/catalog/catalog.yaml`.

## Team Workflow

```bash
# Developer A
summon install superpowers
git add .summon/project/registry.yaml
git commit -m "Add superpowers"

# Developer B
git clone <repo>
summon install
```

Commit the project registry; keep generated store/platform files untracked.

## Building from Source

```bash
git clone https://github.com/ai-summon/summon
cd summon
go build -o bin/summon ./cmd/summon
go test ./... -count=1
```

### Development Shell Function

To run the latest source code without a manual build step, add a shell function
that builds a temporary binary and executes it:

```bash
summon() { go -C /path/to/summon build -o /tmp/summon ./cmd/summon && /tmp/summon "$@"; }
```

> **Warning:** Do not use `go run -C … "$@"` as a shortcut. `go run -C` changes
> the process working directory before the program starts, so `os.Getwd()` will
> return the source tree path instead of the caller's directory. This causes
> summon to install packages and create platform files (e.g. `.claude/`,
> `.summon/`) in the wrong location. Note that `-C` is a `go` top-level flag
> and must appear before the subcommand (e.g. `go -C /path build`, not
> `go build -C /path`).

## License

MIT
