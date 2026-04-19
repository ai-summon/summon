# Summon

**The cross-platform AI plugin manager.**

> Plugins depend on other plugins. Plugins depend on system tools. Summon resolves it all — across **Claude Code CLI** and **GitHub Copilot CLI**.

---

## Highlights

- **Transitive dependency resolution** — install a plugin and Summon automatically installs every plugin it depends on, and every plugin *those* depend on
- **System dependency checks** — Summon verifies that required system tools (git, node, docker, etc.) are present, so nothing silently breaks
- **Skill collision detection** — Summon detects when multiple plugins define skills with the same name, showing which one wins and which are silently shadowed
- **Health monitoring** — run `summon check` at any time to verify that all plugin and system dependencies are still satisfied
- Supports Claude-style marketplaces and plugins.
- Fully compatible with Claude and Copilot CLIs.
- Supports macOS, Linux, and Windows.

## Install

Install summon with our standalone installers:

### macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/ai-summon/summon/main/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/ai-summon/summon/main/install.ps1 | iex
```

If installed via the standalone installer, summon can update itself to the latest version:

```sh
summon self update
```

## Documentation

uv's documentation is available at docs.ai-summon.com.

Additionally, the command line reference documentation can be viewed with summon help.

## Quick Start

```sh
# Install a plugin
summon install speckit

# See what's installed
summon list

# Check plugin health
summon check

# Update everything
summon update
```

## The `summon.yaml` Manifest

Plugins can include an optional `summon.yaml` to declare dependencies and system requirements:

```yaml
marketplaces:
  acme-marketplace: gh:acme-org/acme-marketplace

dependencies:
  - wingman                      # plugin from the default marketplace
  - speckit@acme-marketplace     # plugin from a named marketplace (registered above)

system_requirements:
  - git                          # required — install fails if missing
  - name: docker
    optional: true
    reason: "Only needed for containerized analysis"
```

That's it. When someone runs `summon install my-plugin`, Summon resolves the full dependency tree — `wingman` and its own dependencies, `speckit` from the right marketplace, and verifies `git` is on the system — all automatically.

| Field | Required | Description |
|---|---|---|
| `dependencies` | | Plugins this plugin depends on |
| `system_requirements` | | System binaries that must be present |
| `marketplaces` | | Named marketplace aliases for dependencies |

## Commands

| Command | Description |
|---|---|
| `summon install <package>` | Install a plugin with full dependency resolution |
| `summon uninstall <package>` | Remove a plugin (warns about reverse dependencies) |
| `summon update [package]` | Update one or all plugins |
| `summon list` | List installed plugins with dependency tree |
| `summon check [package]` | Verify plugin health and system dependencies |
| `summon validate` | Validate a `summon.yaml` manifest in the current directory |
| `summon platform list` | Show which CLIs are enabled |
| `summon platform enable <name>` | Enable a platform (claude or copilot) |
| `summon platform disable <name>` | Disable a platform |

### Target a specific CLI

By default, Summon installs to every CLI it detects. To target just one:

```sh
summon install my-plugin --target claude
summon install my-plugin --target copilot
```

### JSON output

Most commands support `--json` for scripting and CI:

```sh
summon list --json
summon check --json
```

## Marketplaces

Browse and register plugin marketplaces to discover new plugins:

```sh
# List registered marketplaces
summon marketplace list

# Add a custom marketplace
summon marketplace add https://github.com/my-org/my-marketplace

# Browse available plugins
summon marketplace browse my-marketplace

# Remove a marketplace
summon marketplace remove my-marketplace
```

## Self-Management

Summon can update and remove itself:

```sh
# Update summon to the latest version
summon self update

# Remove summon from your system
summon self uninstall
```

## Supported Platforms

| OS | Architectures |
|---|---|
| macOS | arm64, amd64 |
| Linux | arm64, amd64 |
| Windows | amd64 |

## License

See [LICENSE](LICENSE) for details.
