# Feature Specification: Plugin.json Manifest Fallback

**Feature Branch**: `004-plugin-json-fallback`
**Created**: 2026-04-06
**Status**: Draft

## Problem Statement

Summon currently requires every installable package to have a `summon.yaml`
manifest. This creates an adoption barrier: the vast majority of existing Claude
Code and Copilot plugins already ship with `.claude-plugin/plugin.json` (and
optionally `.claude-plugin/marketplace.json`) but have no `summon.yaml`. Package
authors must add a summon-specific file before their package can be installed,
even though the platform-native metadata already provides enough information for
installation.

Real-world examples from the built-in catalog:

- **superpowers** (`github:ismaelJimenez/superpowers`): has
  `.claude-plugin/plugin.json` at root. A `summon.yaml` was added manually on a
  feature branch but is not yet merged upstream.
- **cc-spex** (`github:rhuss/cc-spex`): marketplace repo with
  `.claude-plugin/marketplace.json` at root and the actual plugin at `./spex/`
  with its own `.claude-plugin/plugin.json`. No `summon.yaml` exists.

## Design

### Manifest Resolution Chain

When loading a package, the system tries the following sources in order:

1. **`summon.yaml`** — full-fidelity manifest (current behavior, unchanged)
2. **`.claude-plugin/plugin.json`** — single-plugin repo fallback
3. **`.claude-plugin/marketplace.json`** — marketplace repo fallback (resolves
   plugin subdirectory from `plugins[0].source`, then uses that subdirectory's
   `plugin.json`)
4. **None found** — error (unchanged)

Each source produces the same `*Manifest` struct. Downstream code (platform
adapters, marketplace generation, registry, store) does not know or care which
source was used.

### Metadata Mapping

#### From plugin.json

| Manifest field | plugin.json field | Default |
|---|---|---|
| `Name` | `name` | — (required) |
| `Version` | `version` | — (required) |
| `Description` | `description` | — (required) |
| `Author` | `author` | nil |
| `License` | `license` | "" |
| `Homepage` | `homepage` | "" |
| `Repository` | `repository` | "" |
| `Platforms` | — | `["claude", "copilot"]` |
| `Components` | — | auto-detected (see below) |
| `Dependencies` | — | none |

#### From marketplace.json (case 3)

The marketplace.json is used only to locate the plugin subdirectory via
`plugins[0].source`. Once the plugin directory is resolved, its
`.claude-plugin/plugin.json` is read using the same mapping above.

### Component Auto-Detection

For inferred manifests (no `summon.yaml`), components are discovered by probing
the plugin root directory:

| Component | Probe | Logic |
|---|---|---|
| Skills | `skills/` | Directory exists and is non-empty |
| Agents | `agents/` | Directory exists and is non-empty |
| Commands | `commands/` | Directory exists and is non-empty |
| Hooks | `hooks/hooks.json` or `hooks.json` | File exists (Claude or Copilot format) |
| MCP | `.mcp.json` | File exists |

Auto-detected components skip the strict path-existence validation that
`summon.yaml` components receive. The manifest records relative paths (e.g.,
`skills/`) in the same format as explicit declarations.

### Marketplace Repo Handling

When `.claude-plugin/marketplace.json` exists at the repo root:

1. Parse `marketplace.json` and read `plugins[0].source` (e.g., `"./spex"`)
2. Resolve the plugin directory: `<repo>/<source>/`
3. Load `.claude-plugin/plugin.json` from the plugin subdirectory
4. Move only the plugin subdirectory to the store — not the entire repo

Store result for cc-spex:
```
.summon/local/store/spex/
├── .claude-plugin/plugin.json
├── commands/
├── skills/
├── hooks.json
├── scripts/
└── overlays/
```

Multi-plugin marketplaces: for this version, only `plugins[0]` is installed.
Future work could support `summon install github:user/repo:plugin-name`.

### Plugin.json Preservation

When `.claude-plugin/plugin.json` already exists in the plugin directory, the
installer must not overwrite it with `GeneratePluginJSON()`. The existing file
is authoritative and may contain fields beyond what summon generates (e.g.,
`keywords`, `hooks`, `strict`).

### Catalog Update

The `cc-spex` catalog entry is renamed to `spex` to match the actual plugin
name from `plugin.json`. The installed package name always comes from the
plugin's own metadata.

### Validation

Inferred manifests use lighter validation (`ValidateInferred`):
- Name, version, description must be non-empty
- No path-existence checks on auto-detected components
- No platform allowlist enforcement (defaults to both platforms)
- No dependency or summon_version constraint checking

## Limitations

- **No dependency support**: plugin.json has no dependency fields. Packages
  requiring dependency chaining must use `summon.yaml`.
- **No platform filtering**: inferred manifests default to both platforms. A
  package that only works on Claude must use `summon.yaml` to declare this.
- **No version constraints**: `summon_version` is not expressible in
  plugin.json.
- **Single plugin per marketplace**: only `plugins[0]` is extracted.

## Code Changes

### `internal/manifest/manifest.go`

New public function:

```go
func LoadOrInfer(dir string) (*Manifest, string, error)
```

Returns `(manifest, pluginRoot, error)`. `pluginRoot` equals `dir` for cases 1
and 2, and the resolved subdirectory for case 3.

New internal helper:

```go
func inferFromPluginJSON(pluginRoot string) (*Manifest, error)
```

Reads `.claude-plugin/plugin.json`, maps fields to `Manifest`, auto-detects
components, defaults platforms to `["claude", "copilot"]`.

### `internal/installer/installer.go`

- Replace `manifest.Load(cloneDest)` with `manifest.LoadOrInfer(cloneDest)`
- When `pluginRoot != cloneDest` (marketplace case), move `pluginRoot` to the
  store instead of the entire clone directory
- Skip `ValidateFull()` for inferred manifests; use `ValidateInferred()` which
  only checks name/version/description are non-empty
- Skip `GeneratePluginJSON()` if `.claude-plugin/plugin.json` already exists

### `internal/catalog/catalog.yaml`

- Rename `cc-spex` entry to `spex`

### No changes to

Platform adapters, marketplace generation, registry, store layout, resolver.
These all operate on the `*Manifest` struct which is identical regardless of
source.

## Testing

### Unit tests — `internal/manifest/manifest_test.go`

1. `TestLoadOrInfer_SummonYaml` — summon.yaml takes priority, `pluginRoot == dir`
2. `TestLoadOrInfer_PluginJSON` — no summon.yaml, `.claude-plugin/plugin.json`
   exists → synthesized manifest with correct fields, platforms default to
   `["claude","copilot"]`, `pluginRoot == dir`
3. `TestLoadOrInfer_MarketplaceJSON` — no summon.yaml,
   `.claude-plugin/marketplace.json` with `source: "./subdir"` → resolves to
   subdir's `plugin.json`, `pluginRoot == dir/subdir`
4. `TestLoadOrInfer_NothingFound` — no summon.yaml, no plugin.json, no
   marketplace.json → error
5. `TestInferFromPluginJSON_AutoDetectComponents` — temp dir with `skills/`,
   `commands/`, `hooks.json` → inferred manifest has matching component paths
6. `TestInferFromPluginJSON_NoComponents` — empty plugin dir → manifest with no
   components (valid, just metadata)
7. `TestLoadOrInfer_SummonYamlWins` — dir has both summon.yaml and plugin.json →
   summon.yaml is used

### Unit tests — `internal/installer/installer_test.go`

8. `TestInstall_PluginJSONFallback` — cloned repo with only plugin.json → install
   succeeds and store has correct files
9. `TestInstall_MarketplaceExtraction` — marketplace repo → only the plugin
   subdir is moved to the store
10. `TestInstall_SkipGeneratePluginJSON` — `.claude-plugin/plugin.json` already
    exists → not overwritten

### Catalog test — `internal/catalog/catalog_test.go`

11. Verify the renamed `spex` entry resolves correctly

### E2E — `tests/e2e/cli_test.go`

12. Install from a local path with only `.claude-plugin/plugin.json` → verify
    full round-trip (install, list, uninstall)
