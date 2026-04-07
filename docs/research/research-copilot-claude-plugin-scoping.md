# Research Report: Plugin Scoping Behavior — Copilot vs Claude

**Date:** 2026-04-06  
**Status:** VS Code visibility fixed; scope correctness still open  
**Issue:** Plugin scoping inconsistencies between Copilot CLI, VS Code Copilot Chat, and Claude Code

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Claude Code Plugin Scoping (Reference Behavior)](#2-claude-code-plugin-scoping-reference-behavior)
3. [Copilot Plugin Scoping — Official Mechanisms](#3-copilot-plugin-scoping--official-mechanisms)
4. [Current Summon Implementation Analysis](#4-current-summon-implementation-analysis)
5. [Root Cause of Observed Issues](#5-root-cause-of-observed-issues)
6. [Proposed Solutions](#6-proposed-solutions)
7. [Recommendations](#7-recommendations)
8. [References](#8-references)

---

## 1. Problem Statement

After running `summon install github:ismaelJimenez/superpowers`, the following behavior is observed:

| Platform | Expected | Actual | Status |
|---|---|---|---|
| Claude Code CLI | Plugin visible only in project directory | Plugin visible only in project directory | ✅ Works |
| VS Code Claude Chat | Plugin visible only when workspace open | Plugin visible only when workspace open | ✅ Works |
| Copilot CLI (terminal) | Plugin visible only in project directory | Plugin visible in **ALL** folders | ❌ Broken (scope leak) |
| VS Code Copilot Chat | Plugin visible when workspace is open | Plugin visible (user-global, not workspace-scoped) | ⚠️ Fixed (see below) |

**VS Code Copilot Chat fix (applied):** The root cause was that summon never set `chat.plugins.enabled: true` — the master toggle for the agent plugins preview feature. Without it, VS Code ignored all plugin configuration. This has been fixed in `internal/platform/copilot.go` by calling `ensurePluginsEnabled()` during `Register()`. Plugins now appear in VS Code Copilot Chat. However, they are registered at the **user level** (not workspace-scoped), which is a separate issue tracked below.

Additionally: `~/.copilot/config.json` (with `installed_plugins`) is exclusive to Copilot CLI. VS Code Copilot Chat does **NOT** read from this file — the two systems have **separate plugin discovery paths**.

---

## 2. Claude Code Plugin Scoping (Reference Behavior)

Claude Code has a clean, well-defined scoping model. Its `claude plugin install` command accepts a `--scope` flag with four values:

| Scope | Settings File | Use Case |
|---|---|---|
| `user` (default) | `~/.claude/settings.json` | Personal plugins, available across all projects |
| `project` | `.claude/settings.json` | Team plugins, shared via version control |
| `local` | `.claude/settings.local.json` | Project-specific, gitignored |
| `managed` | Managed settings (read-only) | Enterprise-managed plugins |

### How It Works

Both Claude Code CLI and VS Code Claude Chat (when Copilot Chat Extension for Claude is active) read from the **same settings files**:

```
# User-scoped
~/.claude/settings.json
  → extraKnownMarketplaces: { "marketplace-name": { source: {...} } }
  → enabledPlugins: { "plugin@marketplace": true }

# Project-scoped
<project>/.claude/settings.json
  → Same keys, project-local scope
```

**Key insight**: Claude Code CLI and VS Code Claude Chat share the same settings file format and discovery mechanism. There is a single source of truth per scope level, and both the CLI and IDE read it.

### How Summon Implements Claude (Working Correctly)

The `ClaudeAdapter` in `internal/platform/claude.go`:
- **Local scope**: Writes `extraKnownMarketplaces` and `enabledPlugins` to `<project>/.claude/settings.json`
- **Global scope**: Writes the same keys to `~/.claude/settings.json`
- Both CLI and VS Code read from the same file → consistent behavior

---

## 3. Copilot Plugin Scoping — Official Mechanisms

Copilot has **two independent plugin discovery systems** that do NOT share configuration, which is the root of the problem.

### 3.1 Copilot CLI Plugin System

**Config file**: `~/.copilot/config.json`  
**Plugin storage**: `~/.copilot/installed-plugins/`  
**Install command**: `copilot plugin install <spec>`

Copilot CLI stores plugins **exclusively at the user level**. There is no `--scope workspace` flag for `copilot plugin install`. From the official docs:

> Plugins installed from a marketplace are stored at: `~/.copilot/installed-plugins/MARKETPLACE/PLUGIN-NAME/`

The `~/.copilot/config.json` contains:
```json
{
  "installed_plugins": [
    {
      "name": "superpowers",
      "marketplace": "summon-local",
      "version": "5.0.7",
      "enabled": true,
      "cache_path": "/Users/.../.copilot/installed-plugins/summon-local/superpowers"
    }
  ]
}
```

**However**, Copilot CLI DOES support project-scoped **skills and agents** (not full plugins) via directory-based discovery:

```
CUSTOM AGENTS (first-found-wins):
  1. ~/.copilot/agents/              (user)
  2. <project>/.github/agents/       (project)
  3. <parents>/.github/agents/       (inherited, monorepo)
  4. ~/.claude/agents/               (user, .claude convention)
  5. <project>/.claude/agents/       (project)
  6. <parents>/.claude/agents/       (inherited)
  7. PLUGIN: agents/ dirs            (plugin, by install order)

AGENT SKILLS (first-found-wins):
  1. <project>/.github/skills/       (project)
  2. <project>/.agents/skills/       (project)
  3. <project>/.claude/skills/       (project)
  4. <parents>/.github/skills/       (inherited)
  5. ~/.copilot/skills/              (user)
  6. ~/.agents/skills/               (user)
  7. ~/.claude/skills/               (user)
  8. PLUGIN: skills/ dirs            (plugin)
```

So Copilot CLI's skills/agents ARE workspace-scoped — but the **plugin mechanism** (which bundles skills + agents + hooks + MCP servers) is ALWAYS user-global.

### 3.2 VS Code Copilot Chat Plugin System

**Config files**: VS Code `settings.json` (user-level and workspace-level)  
**Key settings**:
- `chat.plugins.marketplaces` — marketplace discovery (list of repo refs or `file:///` URIs)
- `chat.pluginLocations` — direct plugin path registration (path → enabled boolean)
- `chat.plugins.enabled` — master toggle for plugin support (preview feature)

**Important**: These are **application-scoped** settings in VS Code, meaning VS Code only reads them from user-level settings. Workspace-level equivalents exist only for recommendations:

```json
// .vscode/settings.json (workspace) — for recommendations only
{
  "extraKnownMarketplaces": { ... },
  "enabledPlugins": { ... }
}
```

**Cross-reading behavior observed**: `~/.copilot/config.json` is **NOT** read by VS Code Copilot Chat. It is exclusive to Copilot CLI. The two systems have completely **separate plugin discovery paths**.

### 3.3 Summary of Copilot Scoping Mechanisms

| Mechanism | Copilot CLI | VS Code Copilot Chat | Scope |
|---|---|---|---|
| `~/.copilot/config.json` `installed_plugins` | ✅ Reads | ❌ Does NOT read | User (global) |
| `~/.copilot/installed-plugins/` | ✅ Reads | ❌ Does NOT read | User (global) |
| `~/.copilot/skills/`, `~/.copilot/agents/` | ✅ Reads | ❓ Unknown | User (global) |
| `<project>/.github/skills/` | ✅ Reads | ✅ Reads | Workspace |
| `<project>/.github/agents/` | ✅ Reads | ✅ Reads | Workspace |
| `<project>/.claude/skills/` | ✅ Reads | ✅ Reads | Workspace |
| `<project>/.claude/agents/` | ✅ Reads | ✅ Reads | Workspace |
| VS Code user `settings.json` `chat.pluginLocations` | ❌ No | ✅ Reads (requires `chat.plugins.enabled: true`) | User (global) |
| VS Code user `settings.json` `chat.plugins.marketplaces` | ❌ No | ✅ Reads (requires `chat.plugins.enabled: true`) | User (global) |
| VS Code user `settings.json` `chat.plugins.enabled` | ❌ No | ✅ **MASTER TOGGLE** — must be `true` | User (global) |
| `.vscode/settings.json` `enabledPlugins` | ❌ No | ⚠️ Recommendations only (requires master toggle) | Workspace (soft) |

---

## 4. Current Summon Implementation Analysis

### Current CopilotAdapter Behavior (`internal/platform/copilot.go`)

The `CopilotAdapter` targets **VS Code settings** exclusively:

**For ALL installs (both local and global scope)**:
1. Sets `chat.plugins.enabled: true` → VS Code user-level `settings.json` *(master toggle, added by fix)*
2. Writes `chat.plugins.marketplaces` (file URI) → VS Code user-level `settings.json`
3. Writes `chat.pluginLocations` (plugin path → true) → VS Code user-level `settings.json`

**Additionally for local scope**:
4. Writes `chat.plugins.marketplaces` → workspace `.vscode/settings.json`
5. Writes `chat.pluginLocations` → workspace `.vscode/settings.json`
6. Writes `extraKnownMarketplaces` → workspace `.vscode/settings.json`
7. Writes `enabledPlugins` → workspace `.vscode/settings.json`

### What Was Missing (Now Fixed)

The adapter still does **NOT**:
- Write to `~/.copilot/config.json` (exclusive to Copilot CLI — not read by VS Code)
- Leverage Copilot CLI's native skill/agent directory-based discovery for workspace scoping

~~The adapter previously did not set `chat.plugins.enabled: true` — this has been fixed.~~

### How the Plugin Ends Up in `~/.copilot/config.json`

Despite summon not writing to `~/.copilot/config.json`, the plugin appears there. This happens because:
1. Summon writes a `file:///` marketplace URI to VS Code user settings
2. Copilot CLI discovers the marketplace from the `file:///` URI (it reads VS Code marketplace config)
3. Copilot CLI auto-installs/caches the plugin to `~/.copilot/installed-plugins/` and registers it in `config.json`
4. This makes it **globally visible** in Copilot CLI across ALL folders

This is the indirect mechanism causing the "visible everywhere in CLI" behavior.

---

## 5. Root Cause of Observed Issues

### Issue 1: Copilot CLI sees plugins in ALL folders

**Root cause**: Copilot CLI's plugin system is inherently user-global. Once the plugin is discovered (via the marketplace URI in VS Code settings), it gets cached to `~/.copilot/installed-plugins/` and registered in `~/.copilot/config.json` — making it globally available. There is no native project-scoped plugin mechanism in Copilot CLI.

**Status**: ❌ Remains open — requires architectural changes (see Section 6).

### Issue 2: VS Code Copilot Chat didn't see plugins

**Root cause**: The adapter never set the **master toggle** `chat.plugins.enabled: true`. From the official VS Code docs:

> Agent plugins are currently in preview. Enable or disable support for agent plugins with the `chat.plugins.enabled` setting.

Without this setting, VS Code **ignored all plugin configuration entirely** — `chat.plugins.marketplaces`, `chat.pluginLocations`, `enabledPlugins`, and `extraKnownMarketplaces` were all inert.

**Status**: ✅ **FIXED** — `ensurePluginsEnabled()` now sets `chat.plugins.enabled: true` in VS Code user settings during `Register()`. Verified: plugins now appear in VS Code Copilot Chat.

**Note**: The fix makes plugins visible at **user-global** scope in VS Code (not workspace-scoped). This is because `chat.pluginLocations` and `chat.plugins.marketplaces` are application-scoped settings — VS Code only reads them from user-level settings. Workspace scoping for VS Code Copilot Chat remains a separate challenge (see Section 6).

### The Fundamental Architecture Difference

| Aspect | Claude Code | Copilot |
|---|---|---|
| CLI + IDE share config? | ✅ Same `settings.json` files | ❌ No — CLI uses `~/.copilot/config.json`, VS Code uses `settings.json` |
| Plugin scoping in CLI | ✅ user/project/local via `--scope` | ❌ User-global only |
| Plugin scoping in IDE | ✅ Via settings file hierarchy | ⚠️ User-global + workspace recommendations |
| Project-scoped skills | ✅ Via `.claude/skills/` | ✅ Via `.github/skills/`, `.claude/skills/` |
| Project-scoped agents | ✅ Via `.claude/agents/` | ✅ Via `.github/agents/`, `.claude/agents/` |
| Project-scoped hooks | ✅ Part of plugin scope | ❌ Only via plugins (user-global) |
| Project-scoped MCP | ✅ Via `.mcp.json` in project | ✅ Via `.copilot/mcp-config.json` or `.vscode/mcp.json` |

---

## 6. Proposed Solutions

### Approach A: Dual-Target Adapter (Recommended)

Split the Copilot adapter into two distinct registration targets:

#### A1. User/Global Scope (`summon install --global`)

Write to `~/.copilot/config.json` directly:
```json
{
  "installed_plugins": [
    {
      "name": "superpowers",
      "marketplace": "summon-global",
      "version": "5.0.7",
      "installed_at": "2026-04-06T...",
      "enabled": true,
      "cache_path": "/Users/.../.copilot/installed-plugins/summon-global/superpowers"
    }
  ]
}
```

And copy/symlink the plugin to `~/.copilot/installed-plugins/summon-global/superpowers`.

**Result**: Plugin visible in both Copilot CLI (all folders) AND VS Code Copilot Chat. Matches the expected behavior for global installs.

#### A2. Workspace/Project Scope (`summon install` — default)

**For Copilot CLI**: Leverage native directory-based discovery. Instead of (or in addition to) the plugin mechanism, create symlinks/copies in the project:

```
<project>/.github/skills/      → skill components from the plugin
<project>/.github/agents/      → agent components from the plugin
<project>/.copilot/mcp-config.json → MCP servers from the plugin
<project>/.github/hooks/hooks.json → hooks from the plugin
```

This makes skills and agents workspace-scoped for Copilot CLI.

**For VS Code Copilot Chat**: Write workspace recommendations:
```json
// .vscode/settings.json
{
  "extraKnownMarketplaces": {
    "summon-local": {
      "source": {
        "source": "directory",
        "path": "/absolute/path/.summon/platforms/copilot"
      }
    }
  },
  "enabledPlugins": {
    "superpowers@summon-local": true
  },
  "chat.pluginLocations": {
    "/absolute/path/.summon/store/superpowers": true
  }
}
```

**Critical**: Do NOT write `chat.plugins.marketplaces` or `chat.pluginLocations` to VS Code **user-level** settings for local-scope installs. This is what causes the "visible everywhere" leak.

### Approach B: Copilot CLI Native Registration

Use Copilot CLI's own plugin install command as part of summon's installation:

```bash
# After generating marketplace, use copilot CLI to install
copilot plugin install /path/to/.summon/platforms/copilot
```

**Pros**: Uses official Copilot API, future-proof  
**Cons**: Requires `copilot` CLI to be installed and accessible; still user-global  

### Approach C: Decomposed Plugin Architecture

For workspace scope, decompose the plugin into Copilot-native project structures instead of using the plugin mechanism:

```
<project>/
├── .github/
│   ├── agents/           ← agent .md files from plugin
│   ├── skills/           ← skill directories from plugin
│   ├── hooks/hooks.json  ← hooks from plugin
│   └── mcp.json          ← MCP servers from plugin
├── .copilot/
│   └── mcp-config.json   ← MCP servers (alternative location)
└── .summon/
    └── (internal tracking)
```

**Pros**: True workspace scoping for ALL components (skills, agents, hooks, MCP). Works perfectly with Copilot CLI's native discovery. Project-scoped components take precedence over user-scoped.

**Cons**: Plugin components are scattered across standard directories; conflicts possible with user's own agents/skills; harder to cleanly uninstall. Need `.gitignore` management.

### Approach D: Hybrid Approach (Best of Both Worlds)

Combine approaches for optimal behavior:

1. **For `--global`**: Use Approach A1 — write to `~/.copilot/config.json` + `~/.copilot/installed-plugins/`
2. **For local (default)**: Use Approach C for skills/agents (native discovery) + write MCP config to project-level `.copilot/mcp-config.json` or `.vscode/mcp.json` + handle hooks
3. **For VS Code**: Write workspace recommendations in `.vscode/settings.json` (`enabledPlugins`, `extraKnownMarketplaces`)
4. **Stop writing** `chat.plugins.marketplaces` and `chat.pluginLocations` to **user-level** VS Code settings for local installs

---

## 7. Recommendations

### Immediate Fixes (Short-term)

0. ~~**Set `chat.plugins.enabled: true` in VS Code user-level settings.**~~  
   ✅ **DONE** — `ensurePluginsEnabled()` added to `Register()` in `copilot.go`. Plugins now appear in VS Code Copilot Chat.

1. **Stop writing to VS Code user-level settings for local-scope installs.**  
   Currently, the adapter always writes `chat.plugins.marketplaces` and `chat.pluginLocations` to user-level VS Code settings (lines 93-96 and 141-143 of `copilot.go`). For local scope, this should be removed. This is what causes the "visible in ALL folders" leak through Copilot CLI's marketplace auto-discovery.

2. **Write to `~/.copilot/config.json` for global-scope installs.**  
   Add `installed_plugins` entries and copy the plugin to `~/.copilot/installed-plugins/`. This makes the plugin visible to Copilot CLI. Note: this does NOT help VS Code — they have separate discovery.

### Architecture Changes (Medium-term)

3. **Add a `CopilotCLIAdapter`** separate from VS Code settings.  
   The current `CopilotAdapter` conflates two systems. Consider splitting:
   - `CopilotCLIAdapter` → manages `~/.copilot/config.json` and `~/.copilot/installed-plugins/`
   - `CopilotVSCodeAdapter` → manages VS Code `settings.json` (both user and workspace)
   
   Or keep one adapter but handle both targets internally.

4. **Implement workspace-scoped Copilot support via decomposition.**  
   For local-scope installs, generate `.github/skills/` and `.github/agents/` symlinks pointing into `.summon/store/<plugin>/`. This gives true project-scoped behavior for the two most important components.

### Future Considerations

5. **Watch for Copilot CLI workspace-scoped plugin support.**  
   GitHub may add `--scope workspace` to `copilot plugin install` in the future. The loading order diagram already shows project-level directories taking precedence. When this arrives, summon should adopt it.

6. **Consider `.copilot/` as an alternative to `.github/` for workspace config.**  
   Copilot CLI reads from both `.github/` and `.copilot/` directories. Using `.copilot/` may better signal intent and avoid conflicts with CI/CD configurations in `.github/`.

---

## 8. References

### Official GitHub Copilot Documentation

1. **GitHub Copilot CLI Plugin Reference** — Plugin manifest schema, file locations, loading order and precedence  
   https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference

2. **Finding and Installing Plugins for GitHub Copilot CLI** — Install commands, marketplace management  
   https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/plugins-finding-installing

3. **About Plugins for GitHub Copilot CLI** — Plugin concepts, contents, comparison with manual config  
   https://docs.github.com/en/copilot/concepts/agents/copilot-cli/about-cli-plugins

4. **GitHub Copilot CLI Configuration Directory Reference** — `~/.copilot/` structure, `config.json`, `installed-plugins/`  
   https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference

5. **Creating a Plugin for GitHub Copilot CLI** — Plugin structure, `plugin.json` format  
   https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/plugins-creating

### VS Code Copilot Chat Documentation

6. **Agent Plugins in VS Code (Preview)** — Plugin discovery, installation, enable/disable, marketplace config, workspace recommendations  
   https://code.visualstudio.com/docs/copilot/customization/agent-plugins

6b. **GitHub Copilot in VS Code Settings Reference** — Full list of `chat.*` settings including `chat.plugins.enabled` master toggle  
   https://code.visualstudio.com/docs/copilot/reference/copilot-settings

### Claude Code Documentation

7. **Claude Code Plugins Reference** — Plugin components, installation scopes (`user`/`project`/`local`/`managed`), manifest schema, CLI commands  
   https://code.claude.com/docs/en/plugins-reference

8. **Claude Code MCP Docs** — MCP server configuration, scoping  
   https://code.claude.com/docs/en/mcp

### Community & Analysis Resources

9. **Creating Agent Plugins for VS Code and Copilot CLI** (Ken Muse) — Cross-platform plugin development  
   https://www.kenmuse.com/blog/creating-agent-plugins-for-vs-code-and-copilot-cli/

10. **GitHub Copilot CLI Extensions Complete Guide** (htek.dev) — Extensions system, workspace vs user scoping  
    https://htek.dev/articles/github-copilot-cli-extensions-complete-guide/

11. **Plugin System & Skills — DeepWiki** — Plugin discovery mechanics, loading order  
    https://deepwiki.com/github/copilot-cli/5.5-plugin-system-and-skills

12. **Configuration System Overview — DeepWiki** — Configuration hierarchy, precedence rules  
    https://deepwiki.com/github/copilot-cli/5.1-configuration-system-overview

13. **Agent Skills, Plugins and Marketplace: Complete Guide** (Chris Ayers) — End-to-end plugin lifecycle  
    https://chris-ayers.com/posts/agent-skills-plugins-marketplace/

14. **The Complete .claude Directory Guide** — Claude Code project structure and scoping  
    https://computingforgeeks.com/claude-code-dot-claude-directory-guide/

### Summon Source Code (Internal)

15. `internal/platform/copilot.go` — Current VS Code Copilot adapter implementation
16. `internal/platform/claude.go` — Claude Code adapter implementation (reference for correct scoping)
17. `internal/platform/platform.go` — Adapter interface definition
18. `internal/installer/installer.go` — Installation orchestration, `ResolvePaths()`, platform registration

---

## Appendix: Copilot Loading Order (from official docs)

```
┌──────────────────────────────────────────────────────────────────┐
│  BUILT-IN - HARDCODED, ALWAYS PRESENT                            │
│  • tools: bash, view, apply_patch, glob, rg, task, ...           │
│  • agents: explore, task, code-review, general-purpose, research │
└────────────────────────┬─────────────────────────────────────────┘
                         │
  ┌──────────────────────▼──────────────────────────────────────────────┐
  │  CUSTOM AGENTS - FIRST LOADED IS USED (dedup by ID)                 │
  │  1. ~/.copilot/agents/           (user, .github convention)         │
  │  2. <project>/.github/agents/    (project)                          │
  │  3. <parents>/.github/agents/    (inherited, monorepo)              │
  │  4. ~/.claude/agents/            (user, .claude convention)         │
  │  5. <project>/.claude/agents/    (project)                          │
  │  6. <parents>/.claude/agents/    (inherited, monorepo)              │
  │  7. PLUGIN: agents/ dirs         (plugin, by install order)         │
  │  8. Remote org/enterprise agents (remote, via API)                  │
  └──────────────────────┬──────────────────────────────────────────────┘
                         │
  ┌──────────────────────▼──────────────────────────────────────────────┐
  │  AGENT SKILLS - FIRST LOADED IS USED (dedup by name)                │
  │  1. <project>/.github/skills/        (project)                      │
  │  2. <project>/.agents/skills/        (project)                      │
  │  3. <project>/.claude/skills/        (project)                      │
  │  4. <parents>/.github/skills/ etc.   (inherited)                    │
  │  5. ~/.copilot/skills/               (personal-copilot)             │
  │  6. ~/.agents/skills/                (personal-agents)              │
  │  7. ~/.claude/skills/                (personal-claude)              │
  │  8. PLUGIN: skills/ dirs             (plugin)                       │
  │  9. COPILOT_SKILLS_DIRS env + config (custom)                       │
  └──────────────────────┬──────────────────────────────────────────────┘
                         │
  ┌──────────────────────▼──────────────────────────────────────────────┐
  │  MCP SERVERS - LAST LOADED IS USED (dedup by server name)           │
  │  1. ~/.copilot/mcp-config.json       (lowest priority)              │
  │  2. .vscode/mcp.json                 (workspace)                    │
  │  3. PLUGIN: MCP configs              (plugins)                      │
  │  4. --additional-mcp-config flag     (highest priority)             │
  └─────────────────────────────────────────────────────────────────────┘
```

Source: [GitHub Copilot CLI Plugin Reference — Loading order and precedence](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference)
