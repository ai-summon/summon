# Package Development Guide

This guide documents how to develop summon packages and the platform-native mechanisms for local development.

## Package Structure

A summon package requires a `summon.yaml` manifest and optional component directories:

```
my-package/
  summon.yaml
  skills/
  agents/
  commands/
  README.md
```

Scaffold a new package:

```sh
summon init --name my-package --platform claude --platform copilot
```

## summon.yaml Manifest

```yaml
name: my-package
version: "1.0.0"
description: "What this package does"
author:
  name: "Your Name"
  email: "you@example.com"
license: MIT
platforms:
  - claude
  - copilot
components:
  skills: skills/
  agents: agents/
  commands: commands/
dependencies:
  other-package: ">=1.0.0"
summon_version: ">=0.1.0"
```

### Validation Rules

- **name**: kebab-case, max 64 characters
- **version**: semantic versioning (MAJOR.MINOR.PATCH)
- **description**: required, max 256 characters
- **platforms**: `claude` and/or `copilot`
- **components**: paths must exist in the package directory

## Local Development

### Claude Code: `--plugin-dir`

During development, use Claude Code's native `--plugin-dir` flag to load your package directly without installing:

```sh
claude --plugin-dir /path/to/my-package
```

This tells Claude Code to treat the directory as a plugin source, picking up skills, agents, and commands from the package without copying or symlinking it into the store.

### VS Code Copilot: `chat.pluginLocations`

For Copilot development, add your package directory to VS Code's settings:

```json
{
  "chat.pluginLocations": [
    "file:///path/to/my-package"
  ]
}
```

This can be set in:
- **Workspace settings** (`.vscode/settings.json`) for project-scoped development
- **User settings** for global access during development

### Using `summon install --path`

For a more integrated development workflow, install your local package via symlink:

```sh
summon install --path ../my-package
```

This creates a symlink in the store pointing to your source directory. Changes to your package are reflected immediately without reinstalling.

## Publishing

Packages are distributed via Git repositories. To make your package installable:

1. Host the repository on GitHub
2. Tag releases with semantic versions: `git tag v1.0.0 && git push --tags`
3. Users install with: `summon install github:user/my-package`

### Catalog Registration

To add your package to the built-in catalog for name-based installation (`summon install my-package`), submit an entry to `internal/catalog/catalog.yaml`:

```yaml
- name: my-package
  repository: https://github.com/user/my-package
  description: "Short description"
  platforms: [claude, copilot]
```

## Testing Your Package

Verify your package validates correctly:

```sh
# Install locally and check for validation warnings (local scope by default)
summon install --path . --force

# Install for all contributors in the team (committable)
summon install --scope project --path . --force

# List to confirm it appears (shows all scopes)
summon list

# Uninstall when done testing
summon uninstall my-package
```

## Scopes

Summon uses three scopes for package installation:

| Scope | Default for | Location | Notes |
|-------|-------------|----------|-------|
| `local` | `install <pkg>` | `<project>/.summon/local/` | Personal; always gitignored |
| `project` | `install` (restore) | `<project>/.summon/project/` | Team-shared; commit the registry |
| `user` | `--global` | `~/.summon/user/` | User-wide across all projects |

Use `--scope <name>` to select a scope explicitly:

```sh
summon install --scope project my-package   # share with team
summon install --scope user my-package      # available in every project
```

When the same package is installed in more than one scope, commands like `update`
and `uninstall` require an explicit `--scope` flag to avoid ambiguity:

```sh
summon update --scope local my-package
summon uninstall --scope project my-package
```

## Standalone Installer Lifecycle

Summon includes platform-native installer scripts for direct CLI bootstrap:

- macOS/Linux: `scripts/install.sh`
- Windows: `scripts/install.ps1`

Common lifecycle steps:

1. Install by running the platform script.
2. Verify with `summon --version`.
3. Upgrade by rerunning the same install script.
4. Uninstall by removing the installed binary path.

Default install targets:

- macOS/Linux: `$HOME/.local/bin/summon`
- Windows: `$HOME/.summon/bin/summon.exe`

Override and automation inputs:

- `SUMMON_VERSION`: explicit version (default latest).
- `SUMMON_INSTALL_PATH`: full destination path override.
- `SUMMON_NONINTERACTIVE=1`: deterministic no-prompt behavior.
- `SUMMON_NO_MODIFY_PATH=1`: skip automatic PATH update.
- `SUMMON_DOWNLOAD_URL` and `SUMMON_CHECKSUM_URL`: source overrides for CI and prerelease validation.

### PATH Recovery Commands

If automatic PATH updates fail or are disabled, use manual shell commands.

macOS/Linux (bash/zsh profile):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Windows (user PATH):

```powershell
setx Path "$HOME/.summon/bin;$env:Path"
```

### Troubleshooting

- `ERROR[platform]`: unsupported OS/architecture. Use a supported runner or host.
- `ERROR[download]`: network/tooling/source issue. Validate URL and HTTPS access.
- `ERROR[checksum]`: integrity mismatch or checksum source issue. Re-fetch checksum source and retry.
- `ERROR[permission]`: destination not writable. Use a user-writable path or set `SUMMON_INSTALL_PATH`.

