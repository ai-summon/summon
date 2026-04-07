package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-summon/summon/internal/cli"
)

func main() {
	// GC mode: if the executable name matches summon-gc-*, this is a
	// Windows garbage-collector process spawned during self-uninstall.
	// It waits for the parent to exit, deletes the original binary, then exits.
	exe := filepath.Base(os.Args[0])
	if strings.HasPrefix(exe, "summon-gc-") && len(os.Args) >= 3 && os.Args[1] == "--gc" {
		binaryPath := os.Args[2]
		dataDir := ""
		if len(os.Args) >= 4 {
			dataDir = os.Args[3]
		}
		cli.RunGC(binaryPath, dataDir)
		return
	}
	cli.Execute()
}
