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

### Scaffolding a New Package

Use `summon new` to scaffold a new package project:

```sh
# Create a generic plugin package
summon new my-package

# Create a specific type of plugin
summon new --type skill my-skill-plugin
summon new --type agent my-agent-plugin
summon new --type command my-command-plugin
summon new --type hook my-hook-plugin
summon new --type mcp my-mcp-server

# Create with custom display name
summon new --name "My Plugin" my-plugin-folder

# Create without Git initialization
summon new --vcs none my-package
```

The `summon new` command:
- Creates the directory structure with appropriate folders for your plugin type
- Generates a `summon.yaml` manifest with metadata (name, version, author, description)
- Creates a templated `README.md` with guidelines for your plugin type
- Optionally initializes a Git repository (default behavior with `--vcs git`)
- Creates a `.gitignore` file configured for plugin development

For more options, run `summon new --help`.

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

### Uninstalling Summon

To fully uninstall summon, remove the binary and clean up any PATH modifications made by the installer.

**1. Remove the binary**

macOS/Linux:

```sh
rm "$HOME/.local/bin/summon"
```

Windows (PowerShell):

```powershell
Remove-Item "$HOME\.summon\bin\summon.exe"
```

If you used `SUMMON_INSTALL_PATH` during installation, remove the binary from that custom location instead.

**2. Remove the PATH entry from your shell profile**

The installer appends an `export PATH` line to your shell profile. Remove it from the appropriate file:

- **zsh**: `~/.zprofile`
- **bash**: `~/.bashrc`
- **other**: `~/.profile`

Delete the line:

```
export PATH="$HOME/.local/bin:$PATH"
```

On Windows, remove `$HOME\.summon\bin` from your user PATH via System Settings or:

```powershell
# View current PATH entries
$env:Path -split ';'
# Then set PATH without the summon entry
setx Path ($env:Path -replace [regex]::Escape("$HOME\.summon\bin;"), '')
```

**3. Remove data directories (optional)**

To remove all installed packages and user-scope data:

```sh
rm -rf "$HOME/.summon"
```

To remove project-local package data, delete the `.summon/` directory in each project:

```sh
rm -rf .summon/
```

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

## Release Process

Summon uses [GoReleaser](https://goreleaser.com/) and a GitHub Actions workflow to automate releases.

### Creating a Release

1. Ensure all tests pass: `go test ./... -count=1`
2. Tag the commit with a semantic version: `git tag v0.1.0`
3. Push the tag: `git push origin v0.1.0`

The release workflow automatically:
- Runs the full test suite as a gate
- Cross-compiles for 6 platforms (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64)
- Generates a SHA-256 checksum manifest
- Publishes binaries and the manifest to GitHub Releases

### Version Stamping

The version is embedded at build time via ldflags:

```
-X github.com/ai-summon/summon/internal/cli.version={{.Version}}
```

The `var version` in `internal/cli/root.go` defaults to `"0.1.0"` for development builds. Tagged releases override this with the actual tag version.

### Release Artifact Structure

Each release publishes:

| Artifact | Format |
|----------|--------|
| `summon_{version}_linux_amd64.tar.gz` | tar.gz |
| `summon_{version}_linux_arm64.tar.gz` | tar.gz |
| `summon_{version}_darwin_amd64.tar.gz` | tar.gz |
| `summon_{version}_darwin_arm64.tar.gz` | tar.gz |
| `summon_{version}_windows_amd64.zip` | zip |
| `summon_{version}_windows_arm64.zip` | zip |
| `summon_{version}_checksums.txt` | SHA-256 manifest |

Where `{version}` is without the `v` prefix (e.g., `0.1.0`).

### Installer Version Resolution

The install scripts (`scripts/install.sh` and `scripts/install.ps1`) dynamically resolve the latest release version from the GitHub Releases API. If the API is unreachable, they fall back to the hardcoded `SUMMON_LATEST_VERSION` value and warn the user.

Checksum verification uses the release's checksum manifest automatically — no need to manually update checksums in the install scripts when publishing a new release.

