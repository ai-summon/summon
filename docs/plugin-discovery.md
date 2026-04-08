# Plugin Discovery & Auto-Activation

How `summon install` makes packages visible and active in Claude Code and VS Code Copilot.

## Reference Documentation

| Topic | URL |
|-------|-----|
| Claude Code plugins overview | https://docs.anthropic.com/en/docs/claude-code/plugins |
| Claude Code marketplace format | https://docs.anthropic.com/en/docs/claude-code/plugins#marketplace-format |
| Claude Code plugin marketplace schema | https://code.claude.com/docs/en/plugin-marketplaces |
| Claude Code settings (enabledPlugins) | https://docs.anthropic.com/en/docs/claude-code/settings |
| VS Code Copilot agent plugins | https://code.visualstudio.com/docs/copilot/customization/agent-plugins |
| VS Code Copilot marketplace config | https://code.visualstudio.com/docs/copilot/customization/agent-plugins#_configure-plugin-marketplaces |
| VS Code Copilot workspace recommendations | https://code.visualstudio.com/docs/copilot/customization/agent-plugins#_workspace-plugin-recommendations |
| VS Code Copilot local plugins | https://code.visualstudio.com/docs/copilot/customization/agent-plugins#_use-local-plugins |
| VS Code settings scopes | https://code.visualstudio.com/docs/configure/settings |

Both platforms share the same plugin/marketplace JSON schema (the `.claude-plugin` convention).
VS Code explicitly references the [Claude Code plugin marketplace documentation](https://code.claude.com/docs/en/plugin-marketplaces)
for the full schema, confirming format compatibility.

## Architecture

```
summon install github:user/pkg
        │
        ▼
┌──────────────────┐
│  installer.go    │  Orchestrates the full flow
│  Install()       │
└────────┬─────────┘
         │
   ┌─────┴──────────────────────────────────────────────┐
   │  1. Clone repo → temp dir                         │
   │  2. Resolve version (tag/ref/HEAD)                 │
   │  3. Load manifest (resolution chain below)          │
   │  4. Move to store (.summon/store/<name>)            │  ← Cross-device-safe: uses rename-first with copy/fallback for different filesystems
   │  5. Generate plugin descriptor if missing           │
   │  6. Record in registry (.summon/registry.yaml)      │
   │  7. Generate marketplace (marketplace.json)         │
   │  8. Register marketplace with platforms             │
   │  9. Auto-enable plugin on platforms                 │
   └────────────────────────────────────────────────────┘
```

### Manifest Resolution Chain

When loading a package, `manifest.LoadOrInfer()` tries these sources in order:

1. **`summon.yaml`** — Full manifest with explicit configuration. Uses `Validate()` + `ValidateFull()`.
2. **`.claude-plugin/plugin.json`** — Claude/Copilot plugin descriptor. Fields are mapped to a Manifest with permissive validation (`ValidateInferred`). Platforms default to `["claude", "copilot"]`. Components are auto-detected by probing the directory.
3. **`.claude-plugin/marketplace.json`** — Marketplace index listing multiple plugins. Each plugin's `source` directory is resolved and its `plugin.json` is loaded via step 2. All plugins are installed as separate registry entries.
4. **Error** — If none of the above exist, installation fails.

### Component Auto-Detection

When inferring from `plugin.json`, the following directories/files are probed to populate `components`:

| Probe | Component | Value |
|-------|-----------|-------|
| `skills/` directory exists | `components.skills` | `"skills"` |
| `agents/` directory exists | `components.agents` | `"agents"` |
| `commands/` directory exists | `components.commands` | `"commands"` |
| `hooks.json` file at root | `components.hooks` | `"."` |
| `hooks/hooks.json` file | `components.hooks` | `"hooks"` |
| `.mcp.json` file exists | `components.mcp` | `"."` |

If no probes match, `components` is left nil (the plugin has no materializable components).

### Validation Differences

| Check | `summon.yaml` | `plugin.json` / `marketplace.json` |
|-------|--------------|-------------------------------------|
| Required fields (name, version, description) | Yes (`Validate`) | Yes (`ValidateInferred`) |
| Component paths exist on disk | Yes (`ValidateFull`) | No |
| Semantic version format | Yes | No |
| summon_version constraint | Yes | No |

### Directory Layout (local scope)

```
<project>/
├── .summon/
│   ├── registry.yaml                    # Lock file: all installed packages
│   ├── store/
│   │   └── superpowers/                 # Package contents (cloned repo)
│   │       ├── summon.yaml
│   │       ├── skills/
│   │       └── .claude-plugin/
│   │           └── plugin.json          # Step 5: per-plugin descriptor
│   └── platforms/
│       ├── claude/
│       │   ├── .claude-plugin/
│       │   │   └── marketplace.json     # Step 7: marketplace index
│       │   └── plugins/
│       │       └── superpowers → ../../store/superpowers  # symlink
│       └── copilot/
│           ├── .claude-plugin/
│           │   └── marketplace.json
│           └── plugins/
│               └── superpowers → ../../store/superpowers
├── .claude/
│   └── settings.json                   # Steps 8-9: Claude Code reads this
└── .vscode/
    └── settings.json                   # Steps 8-9: VS Code Copilot reads this
```

## Step-by-Step Flow

### Step 5: Plugin Descriptor (`plugin.json`)

**File:** `internal/marketplace/plugin.go` — `GeneratePluginJSON()`

Creates `<store>/<name>/.claude-plugin/plugin.json` from the package's `summon.yaml` manifest:

```json
{
  "name": "superpowers",
  "description": "AI coding superpowers",
  "version": "1.0.0"
}
```

Both platforms look for `.claude-plugin/plugin.json` inside each plugin directory to identify it.

### Step 7: Marketplace Index (`marketplace.json`)

**File:** `internal/marketplace/marketplace.go` — `Generate()`

Creates one marketplace per platform at `<platforms>/<platform>/.claude-plugin/marketplace.json`. The `source` field uses `./`-prefixed relative paths (absolute paths and `../` traversal are rejected by both platforms):

```json
{
  "name": "summon-local",
  "description": "Summon summon-local package marketplace",
  "owner": { "name": "summon" },
  "plugins": [
    {
      "name": "superpowers",
      "description": "AI coding superpowers",
      "version": "1.0.0",
      "source": "./plugins/superpowers"
    }
  ]
}
```

Symlinks are created at `<platforms>/<platform>/plugins/<name>` pointing to `<store>/<name>` so the relative `./plugins/<name>` path resolves correctly from the marketplace root.

**Why per-platform marketplaces?** Packages declare compatible platforms in `summon.yaml`. A Claude-only package should not appear in the Copilot marketplace and vice versa.

### Step 8: Platform Registration

**File:** `internal/platform/claude.go` and `copilot.go` — `Register()`

Writes to the platform's settings file so it knows where to find the marketplace.

**Claude Code** — `.claude/settings.json` (local) or `~/.claude/settings.json` (global):
```json
{
  "extraKnownMarketplaces": {
    "summon-local": {
      "source": {
        "source": "directory",
        "path": "/absolute/path/to/.summon/platforms/claude"
      }
    }
  }
}
```

**VS Code Copilot** — `.vscode/settings.json` (workspace) AND VS Code user `settings.json` (application):

VS Code settings have different scopes ([docs](https://code.visualstudio.com/docs/configure/settings)).
`chat.plugins.marketplaces` and `chat.pluginLocations` are **application-scoped** —
VS Code only reads them from user-level settings, not from `.vscode/settings.json`.

For **local (workspace) scope**, summon writes to **two** settings files:

1. **User-level settings** (`~/Library/.../Code/User/settings.json`) — application-scoped
   keys that VS Code actually reads for plugin activation:
   ```json
   {
     "chat.plugins.marketplaces": [
       "file:///absolute/path/to/.summon/platforms/copilot"
     ]
   }
   ```

2. **Workspace settings** (`.vscode/settings.json`) — workspace-scoped keys for
   team sharing and recommendations ([source: "Workspace plugin recommendations"](https://code.visualstudio.com/docs/copilot/customization/agent-plugins#_workspace-plugin-recommendations)):
   ```json
   {
     "extraKnownMarketplaces": {
       "summon-local": {
         "source": {
           "source": "directory",
           "path": "/absolute/path/to/.summon/platforms/copilot"
         }
       }
     },
     "chat.plugins.marketplaces": [
       "file:///absolute/path/to/.summon/platforms/copilot"
     ]
   }
   ```

For **global scope**, only user-level settings are written (the application-scoped
keys are already in the right place).

### Step 9: Auto-Enable

**File:** `internal/platform/claude.go` and `copilot.go` — `EnablePlugin()`

Without this step, the platform discovers the marketplace but the plugin remains disabled until the user manually activates it.

**Claude Code** — uses `enabledPlugins` in the same settings file:
```json
{
  "enabledPlugins": {
    "superpowers@summon-local": true
  }
}
```

**VS Code Copilot** — for **local scope**, writes to **two** settings files:

1. **User-level settings** — `chat.pluginLocations` (application-scoped, actually
   activates the plugin) ([source: "Use local plugins"](https://code.visualstudio.com/docs/copilot/customization/agent-plugins#_use-local-plugins)):
   ```json
   {
     "chat.pluginLocations": {
       "/absolute/path/to/.summon/store/superpowers": true
     }
   }
   ```

2. **Workspace settings** — `enabledPlugins` (team recommendations) +
   `chat.pluginLocations` (backward compat):
   ```json
   {
     "enabledPlugins": {
       "superpowers@summon-local": true
     },
     "chat.pluginLocations": {
       "/absolute/path/to/.summon/store/superpowers": true
     }
   }
   ```

For **global scope**, only user-level settings are written.

On `summon uninstall`, `DisablePlugin()` removes the corresponding key from each platform's settings.

## Platform Adapter Interface

**File:** `internal/platform/platform.go`

```go
type Adapter interface {
    Name() string
    Detect() bool
    SettingsPath(scope Scope) string
    Register(marketplacePath, marketplaceName string, scope Scope) error
    Unregister(marketplaceName string, scope Scope) error
    EnablePlugin(pluginName, marketplaceName, storeDir string, scope Scope) error
    DisablePlugin(pluginName, marketplaceName, storeDir string, scope Scope) error
}
```

- **Detect()** checks if the platform is installed (e.g., `~/.claude` exists for Claude Code, VS Code user data dir exists for Copilot)
- **Register/Unregister** — Claude: manages `extraKnownMarketplaces`; Copilot: manages `chat.plugins.marketplaces` and (for local scope) `extraKnownMarketplaces`
- **EnablePlugin/DisablePlugin** — Claude: manages `enabledPlugins`; Copilot: manages `chat.pluginLocations` and (for local scope) `enabledPlugins`

## Why Copilot Writes to Two Settings Files (Local Scope)

VS Code has three settings scopes ([docs](https://code.visualstudio.com/docs/configure/settings)):
application, workspace, and resource. The `chat.plugins.marketplaces` and
`chat.pluginLocations` settings are **application-scoped** — VS Code only reads
them from user-level settings, not from `.vscode/settings.json`.

For workspace-level recommendations, VS Code Copilot Chat reads
`extraKnownMarketplaces` and `enabledPlugins` from `.vscode/settings.json`
([source](https://code.visualstudio.com/docs/copilot/customization/agent-plugins#_workspace-plugin-recommendations)).
These are the **same keys** that Claude Code uses — both platforms share the format.

For **local scope** installs, the Copilot adapter therefore writes to **two files**:

| Key | File | Purpose |
|-----|------|---------|
| `chat.plugins.marketplaces` | User settings (app-scoped) | **Actual** marketplace registration |
| `chat.pluginLocations` | User settings (app-scoped) | **Actual** plugin activation |
| `extraKnownMarketplaces` | `.vscode/settings.json` | Workspace recommendations / team sharing |
| `enabledPlugins` | `.vscode/settings.json` | Workspace recommendations / team sharing |
| `chat.plugins.marketplaces` | `.vscode/settings.json` | Backward compatibility |
| `chat.pluginLocations` | `.vscode/settings.json` | Backward compatibility |

For **global scope** installs, only user-level settings are written (the
application-scoped keys are already in the right place).

## Scope Model

Summon supports three writable scopes. Each scope uses a separate store and registry.

| Scope | Flag | Store path | Scope alias |
|-------|------|------------|-------------|
| `local` | *(default for install)* | `<project>/.summon/local/` | — |
| `project` | `--scope project` | `<project>/.summon/project/` | *(default for restore)* |
| `user` | `--scope user` / `--global` | `~/.summon/user/` | `global` |

**Precedence**: `local > project > user`. When listing packages without a `--scope` filter, all scopes are shown.

### Claude Scope Settings

| Scope | Settings file |
|-------|--------------|
| `user` | `~/.claude/settings.json` |
| `project` | `.claude/settings.json` |
| `local` | `.claude/settings.local.json` |

### Copilot Scope Settings and Materialization

Copilot is split into two behaviors based on scope:

**User scope** (`--scope user` / `--global`): writes application-scoped keys to
VS Code user settings only — the same as the original local-scope behavior.

**Project scope** and **local scope**: must not write user-level settings because
that would invisibly activate project packages for all workspaces. Instead, the adapter:

1. Writes workspace-scoped keys to `.vscode/settings.json` (team recommendations).
2. **Materializes** skills and agents as symlinks into documented workspace paths:
   - Project scope → `.github/skills/` and `.github/agents/`
   - Local scope → `.claude/skills/` and `.claude/agents/`

In addition, `local` scope propagates to VS Code user settings for activation
(since VS Code's `chat.pluginLocations` is application-scoped).

#### Why materialization instead of native plugin registration?

Two approaches were considered for Copilot project and local scopes:

| Approach | How it works | Tradeoff |
|----------|-------------|----------|
| **Native user-global plugin install** (rejected for project/local) | Write `chat.pluginLocations` and `chat.plugins.marketplaces` to VS Code user settings | Simple, but activates the package in *every* workspace for the current user — the package leaks out of the intended project. This violates the scope contract. |
| **Workspace materialization** (chosen) | Symlink skills/agents into documented per-project paths (`.github/skills/`, `.github/agents/`); write team-recommendation keys to `.vscode/settings.json` only | Keeps the package visible only inside the intended workspace. Requires the symlinks to be present on disk; skills and agents are activated, but hooks and MCP have no equivalent workspace-scoped mechanism in the documented Copilot API. |

The materialization approach was chosen because it is the only way to avoid scope leaks for project/local
installs. The tradeoff is that hooks and (for local scope) MCP cannot be fully honoured —
see Known Limitations below.

**Known limitations**:
- Hooks are not supported at project or local scope (Copilot has no workspace-scoped hook mechanism).
- MCP is not supported at local scope; use project or user scope for MCP packages.

Full scope/settings matrix:

| Key | User scope | Project scope | Local scope |
|-----|-----------|--------------|-------------|
| Claude settings file | `~/.claude/settings.json` | `.claude/settings.json` | `.claude/settings.local.json` |
| Copilot materialization dir (skills) | *(none)* | `.github/skills/` | `.claude/skills/` |
| Copilot materialization dir (agents) | *(none)* | `.github/agents/` | `.claude/agents/` |
| Copilot workspace settings | *(none)* | `.vscode/settings.json` | `.vscode/settings.json` |
| Copilot user settings | `~/Library/.../Code/User/settings.json` | *(workspace only)* | `~/Library/.../Code/User/settings.json` |

## Key Constraints (from platform docs)

1. **Marketplace must be at `.claude-plugin/marketplace.json`** — both platforms look for this exact path relative to the marketplace root directory.
2. **Plugin sources must use `./` prefix** — relative paths only, no `../` traversal. This is why symlinks are needed to bring store packages into the marketplace directory tree.
3. **Each plugin needs `.claude-plugin/plugin.json`** — the platform uses this to identify a directory as a valid plugin.
4. **Settings files are JSON** — both platforms read/write standard JSON settings files. Summon merges keys non-destructively:
   - Claude: touches `extraKnownMarketplaces` and `enabledPlugins`
   - Copilot: touches `chat.plugins.marketplaces`, `chat.pluginLocations`, and (for local scope) `extraKnownMarketplaces` and `enabledPlugins`
5. **VS Code setting scopes matter** — `chat.plugins.marketplaces` and `chat.pluginLocations` are application-scoped (user-level only). Workspace-level `.vscode/settings.json` must use `extraKnownMarketplaces` and `enabledPlugins` for VS Code Copilot Chat discovery.

## Uninstall Flow

`summon uninstall <name>`:

1. Remove package from store
2. Remove entry from `registry.yaml`
3. Regenerate all `marketplace.json` files (package disappears from index)
4. `DisablePlugin()` on all active platforms:
   - Claude: removes from `enabledPlugins`
   - Copilot: removes from `chat.pluginLocations` and `enabledPlugins`
5. If no packages remain, `Unregister()` removes marketplace entries:
   - Claude: removes from `extraKnownMarketplaces`
   - Copilot: removes from `chat.plugins.marketplaces` and `extraKnownMarketplaces`
