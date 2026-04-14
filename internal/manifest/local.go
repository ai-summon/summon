package manifest

import (
	"os"
	"path/filepath"
)

// LocalManifestFetcher reads summon.yaml from a local plugin directory.
type LocalManifestFetcher struct{}

// NewLocalManifestFetcher creates a new LocalManifestFetcher.
func NewLocalManifestFetcher() *LocalManifestFetcher {
	return &LocalManifestFetcher{}
}

// FetchManifest reads and parses summon.yaml from the given local directory.
// Returns (nil, nil) if the file does not exist (valid — no summon-specific deps).
func (f *LocalManifestFetcher) FetchManifest(pluginDir string) (*Manifest, error) {
	path := filepath.Join(pluginDir, "summon.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	return LoadFile(path)
}
