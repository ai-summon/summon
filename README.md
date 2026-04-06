<p align="center">
  <img src="assets/Summon.png" alt="Summon" width="234" />
</p>

# Summon

A cross-platform package manager for AI agent components — skills, agents,
commands, hooks, and MCP servers.

Summon installs packages from GitHub repositories or local paths and
automatically registers them with detected AI platforms. Currently supports
**Claude Code** and **VS Code Copilot** (CLI and Chat).

## Quick Start

```bash
# Install summon
go install github.com/user/summon/cmd/summon@latest

# Install a package from the catalog
summon install superpowers

# Install from GitHub
summon install github:obra/superpowers

# Install from a local directory (symlinked for development)
summon install --path ../my-plugin

# List installed packages
summon list

# Update all packages
summon update

# Remove a package
summon uninstall superpowers
```

After installing, packages are automatically activated on detected platforms —
no manual configuration needed.

## Standalone Installer

Summon also ships first-party installer scripts for direct binary installation.

macOS/Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/user/summon/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -Command "iwr https://raw.githubusercontent.com/user/summon/main/scripts/install.ps1 -UseBasicParsing | iex"
```

Verify install:

```bash
summon --version
```

Installer behavior highlights:

- Verifies SHA-256 checksum before activation and fails closed on mismatch.
- Installs into a user-writable location by default.
- Attempts PATH setup by default and prints exact manual recovery commands when automatic updates are skipped or fail.
- Supports rerun-safe upgrades by running the same installer command again.

Uninstall installer-managed binary:

```bash
rm -f "$HOME/.local/bin/summon"
```

```powershell
Remove-Item "$HOME/.summon/bin/summon.exe" -Force
```

## Scopes

Summon supports three writable scopes that control where a package is installed
and who can see it.

| Scope | Location | Visibility | Default for |
|-------|----------|------------|-------------|
| `local` | `<project>/.summon/local/` | Current checkout only | `summon install` |
| `project` | `<project>/.summon/project/` | All contributors (committable) | `summon install` restore |
| `user` | `~/.summon/user/` | This user everywhere | `--global` flag |

**Precedence**: `local > project > user` (local overrides project which overrides user).

```bash
summon install superpowers               # local scope (default)
summon install --scope project superpowers  # shared with the team
summon install --scope user superpowers     # user-wide
summon install -g superpowers            # shorthand for --scope user

summon install                           # restore project scope (team reproducibility)
summon install --scope user              # restore user scope
```

Project-scope packages live in `.summon/project/` and can be committed to make
them reproducible for all contributors. Local-scope packages live in `.summon/local/`
and are always gitignored — they are personal development state.

## How It Works

Summon manages a **store** of packages and generates platform-specific
marketplace indexes so each AI platform discovers them natively.

```
<project>/
├── .summon/
│   ├── local/
│   │   ├── registry.yaml              # Personal lock file (gitignored)
│   │   ├── store/                     # Package contents
│   │   └── platforms/                 # Generated marketplace indexes
│   ├── project/
│   │   ├── registry.yaml              # Shared lock file (commit this)
│   │   ├── store/                     # Package contents (gitignored)
│   │   └── platforms/                 # Generated marketplace indexes
│   └── user → ~/.summon/user/         # User-wide scope (outside project)
├── .claude/settings.json              # Claude Code project scope
├── .claude/settings.local.json        # Claude Code local scope
└── .vscode/settings.json              # VS Code Copilot integration
```

**Local scope** (default) installs into `.summon/local/`. **Project scope**
(`--scope project`) installs into `.summon/project/` — commit the registry.
**User scope** (`--scope user` or `--global`) installs into `~/.summon/user/`.

Team members clone the project and run `summon install` to restore all
packages from the committed `registry.yaml`.

## Commands

### `summon install [package]`

Install a package or restore all packages from the registry.

```bash
summon install superpowers              # From catalog (local scope)
summon install github:user/repo         # From GitHub
summon install https://github.com/u/r   # Full URL
summon install --path ../local-plugin   # Local path (symlink)
summon install --ref v2.0.0 superpowers # Pin to version
summon install --scope project superpowers  # Install in project scope
summon install --global superpowers     # Install in user scope
summon install --force superpowers      # Skip compatibility checks
summon install                          # Restore project scope packages
summon install --scope user             # Restore user scope packages
```

### `summon uninstall <package>`

Remove a package from the store, registry, and platform settings.

```bash
summon uninstall superpowers                   # Auto-detects scope
summon uninstall --scope local superpowers     # Explicit local scope
summon uninstall --scope project superpowers   # Project scope
summon uninstall --global superpowers          # User scope
```

If the same package is installed in multiple scopes, `--scope` is required.

### `summon update [package]`

Update a specific package or all packages to their latest version.

```bash
summon update superpowers               # Update one package (auto-detects scope)
summon update                           # Update all project scope packages
summon update --scope user              # Update all user scope packages
summon update --global                  # Shorthand for --scope user
```

If the same package is installed in multiple scopes, `--scope` is required.

### `summon list`

Show installed packages with version, scope, and status.

```bash
summon list                  # All visible scopes
summon list --scope local    # Local scope only
summon list --scope project  # Project scope only
summon list --scope user     # User scope only
summon list --json           # JSON output
summon list --global         # Shorthand for --scope user
```

### `summon init`

Scaffold a new summon package in the current directory.

```bash
summon init
summon init --name my-skills
summon init --platform claude --platform copilot
```

Creates a `summon.yaml` manifest and standard directory structure:

```
my-skills/
├── summon.yaml
├── skills/
├── agents/
├── commands/
└── README.md
```

## Package Manifest

Packages are defined by a `summon.yaml` file at the repository root:

```yaml
name: my-skills
version: "1.0.0"
description: "A collection of useful skills"

author:
  name: "Your Name"
  email: "you@example.com"
license: MIT

platforms:
  - claude
  - copilot

components:
  skills: ./skills/
  agents: ./agents/
  commands: ./commands/
  hooks: ./hooks/hooks.json

dependencies:
  superpowers: ">=5.0.0"

summon_version: ">=0.1.0"
```

### Required Fields

| Field | Description |
|-------|-------------|
| `name` | Package name (kebab-case, max 64 chars) |
| `version` | Semantic version (`MAJOR.MINOR.PATCH`) |
| `description` | Short description (max 256 chars) |

### Optional Fields

| Field | Description |
|-------|-------------|
| `author` | Author name and email |
| `license` | License identifier (e.g., `MIT`) |
| `platforms` | Target platforms (`claude`, `copilot`). Omit for all. |
| `components` | Paths to skills, agents, commands, hooks, mcp directories |
| `dependencies` | Other summon packages required, with version constraints |
| `summon_version` | Minimum summon version required |

## Platform Support

Summon auto-detects installed AI platforms and registers packages with each:

| Platform | Detection | Settings |
|----------|-----------|----------|
| **Claude Code** | `~/.claude` directory exists | `extraKnownMarketplaces`, `enabledPlugins` |
| **VS Code Copilot** | VS Code user config directory exists | `chat.pluginLocations`, `chat.plugins.marketplaces` |

Both platforms use the same plugin format (`.claude-plugin/plugin.json`), so
a single package works across platforms without modification.

### Scopes

| | Local (default) | Global (`--global`) |
|---|---|---|
| Store | `.summon/store/` | `~/.summon/store/` |
| Registry | `.summon/registry.yaml` | `~/.summon/registry.yaml` |
| Shared with team | ✅ via `registry.yaml` | ❌ user-specific |

## Built-in Catalog

These packages can be installed by name:

| Name | Description |
|------|-------------|
| `superpowers` | Core skills library — TDD, debugging, collaboration patterns |
| `cc-spex` | Specification framework |
| `get-shit-done` | Productivity skills |
| `spec-kit` | Specification toolkit |

```bash
summon install superpowers
```

## Local Development

For developing packages, use `--path` to create a symlinked installation:

```bash
# In your project directory
summon install --path ../my-plugin

# Changes to ../my-plugin are immediately available — no reinstall needed
```

Or use platform-native tools for quick iteration:

```bash
# Claude Code: load a plugin directory directly
claude --plugin-dir ../my-plugin

# VS Code Copilot: add to user settings
# "chat.pluginLocations": { "/path/to/my-plugin": true }
```

## Publishing

Summon packages are distributed as Git repositories with semantic version
tags:

```bash
cd my-skills
git tag v1.0.0
git push origin v1.0.0
```

Users install via:

```bash
summon install github:yourname/my-skills
```

To add a package to the built-in catalog, submit a PR adding an entry to
`internal/catalog/catalog.yaml`.

## Team Workflow

```bash
# Developer A: install a package
summon install superpowers
git add .summon/registry.yaml
git commit -m "Add superpowers plugin"

# Developer B: clone and restore
git clone <repo>
summon install     # Restores all packages from registry.yaml
```

The `registry.yaml` lock file is committed to the repo. The `store/` and
`platforms/` directories are gitignored (generated on `summon install`).

## Documentation

- [Package Development](docs/development.md) — Creating and publishing
  packages
- [Plugin Discovery Architecture](docs/plugin-discovery.md) — How summon
  integrates with AI platforms

## Building from Source

```bash
git clone https://github.com/user/summon
cd summon
go build -o bin/summon ./cmd/summon
```

Run tests:

```bash
go test ./... -count=1
```

## License

MIT
