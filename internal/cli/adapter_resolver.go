package cli

import (
	"fmt"
	"io"

	"github.com/ai-summon/summon/internal/config"
	"github.com/ai-summon/summon/internal/platform"
)

// adapterResolverDeps holds the injectable dependencies for adapter resolution.
type adapterResolverDeps struct {
	runner       platform.CommandRunner
	adapters     []platform.Adapter // test injection; if non-nil, skip detection
	configPath   string             // override config path for testing; empty = default
	target       string             // --target flag value
	stderr       io.Writer          // for warnings
	configSaveFn func(string, config.Config) error // optional; defaults to config.Save
}

// resolveEnabledAdapters returns the adapters to use for a command, respecting:
//  1. Test injection (deps.adapters)
//  2. Config file (enabled/disabled platforms)
//  3. Runtime detection (is CLI on PATH?)
//  4. --target override (bypasses config, requires detection)
//
// On first run (no config), it auto-detects and tries to save config (non-fatal).
func resolveEnabledAdapters(deps *adapterResolverDeps) ([]platform.Adapter, error) {
	// 1. Test injection
	if deps.adapters != nil {
		if deps.target != "" {
			return platform.FilterByTarget(deps.adapters, deps.target)
		}
		if len(deps.adapters) == 0 {
			return nil, fmt.Errorf("no supported CLIs detected")
		}
		return deps.adapters, nil
	}

	// 2. Load config
	cfgPath := deps.configPath
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultPath()
		if err != nil {
			// Can't determine config path — fall back to pure detection
			return detectAndFilterTarget(deps)
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		// Config exists but is unreadable — fall back to pure detection
		fmt.Fprintf(deps.stderr, "⚠ could not read config: %v (using auto-detection)\n", err)
		return detectAndFilterTarget(deps)
	}

	// 3. Detect all available adapters on the system
	allDetected := platform.DetectAdapters(deps.runner)

	// 4. --target override: bypass config, use any detected adapter
	if deps.target != "" {
		return filterByTargetWithWarning(allDetected, deps.target, cfg, deps.stderr)
	}

	// 5. If no config exists, bootstrap: enable all detected platforms, try to save
	saveFn := deps.configSaveFn
	if saveFn == nil {
		saveFn = config.Save
	}
	if !cfg.HasPlatforms() {
		return bootstrapFromDetection(allDetected, cfgPath, saveFn, deps.stderr)
	}

	// 6. Apply config filter
	return applyConfigFilter(allDetected, cfg, deps.stderr)
}

// detectAndFilterTarget is a fallback that does pure detection + target filtering.
func detectAndFilterTarget(deps *adapterResolverDeps) ([]platform.Adapter, error) {
	adapters := platform.DetectAdapters(deps.runner)
	if len(adapters) == 0 {
		return nil, fmt.Errorf("no supported CLIs detected")
	}
	if deps.target != "" {
		return platform.FilterByTarget(adapters, deps.target)
	}
	return adapters, nil
}

// filterByTargetWithWarning handles --target, warning if the target is disabled in config.
func filterByTargetWithWarning(detected []platform.Adapter, target string, cfg config.Config, stderr io.Writer) ([]platform.Adapter, error) {
	filtered, err := platform.FilterByTarget(detected, target)
	if err != nil {
		return nil, err
	}
	if enabled, configured := cfg.IsEnabled(target); configured && !enabled {
		fmt.Fprintf(stderr, "⚠ using disabled platform %s (--target override)\n", target)
	}
	return filtered, nil
}

// bootstrapFromDetection enables all detected platforms and saves config.
func bootstrapFromDetection(detected []platform.Adapter, cfgPath string, saveFn func(string, config.Config) error, stderr io.Writer) ([]platform.Adapter, error) {
	if len(detected) == 0 {
		return nil, fmt.Errorf("no supported CLIs detected")
	}

	// Build and save config (non-fatal on save failure)
	var cfg config.Config
	for _, a := range detected {
		_ = cfg.SetPlatform(a.Name(), true)
	}
	if err := saveFn(cfgPath, cfg); err != nil {
		fmt.Fprintf(stderr, "⚠ could not save config: %v\n", err)
	}

	return detected, nil
}

// applyConfigFilter returns only adapters that are enabled in config and detected.
// Warns about enabled-but-undetected platforms.
func applyConfigFilter(detected []platform.Adapter, cfg config.Config, stderr io.Writer) ([]platform.Adapter, error) {
	detectedSet := make(map[string]platform.Adapter, len(detected))
	for _, a := range detected {
		detectedSet[a.Name()] = a
	}

	var result []platform.Adapter
	for _, name := range config.KnownPlatforms() {
		enabled, configured := cfg.IsEnabled(name)
		if !configured || !enabled {
			continue
		}
		if a, ok := detectedSet[name]; ok {
			result = append(result, a)
		} else {
			fmt.Fprintf(stderr, "⚠ %s is enabled but not installed, skipping\n", name)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no enabled platforms are available; use 'summon platform enable <name>' or install a supported CLI")
	}
	return result, nil
}
