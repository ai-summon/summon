//go:build windows

package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// cleanPath removes the summon binary directory from the user-level PATH
// environment variable on Windows.
func cleanPath(binaryPath string) error {
	binDir := filepath.Dir(binaryPath)

	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil // no Environment key, nothing to clean
		}
		return fmt.Errorf("cannot open registry key: %w", err)
	}
	defer key.Close()

	val, valType, err := key.GetStringValue("Path")
	if err != nil {
		if err == registry.ErrNotExist {
			return nil // no PATH value, nothing to clean
		}
		return fmt.Errorf("cannot read PATH: %w", err)
	}

	// Guard against non-string types
	if valType != registry.SZ && valType != registry.EXPAND_SZ {
		return fmt.Errorf("PATH registry value has unexpected type %d; skipping to avoid corruption", valType)
	}

	// Filter out the summon bin directory
	parts := strings.Split(val, ";")
	var filtered []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if strings.EqualFold(trimmed, binDir) {
			continue
		}
		// Also handle expanded form
		expanded := os.ExpandEnv(trimmed)
		if strings.EqualFold(expanded, binDir) {
			continue
		}
		filtered = append(filtered, p)
	}

	if len(filtered) == len(parts) {
		return nil // summon dir not found in PATH, no-op
	}

	if len(filtered) == 0 {
		// PATH is empty after removal — delete the key
		if err := key.DeleteValue("Path"); err != nil {
			return fmt.Errorf("cannot delete empty PATH: %w", err)
		}
	} else {
		newPath := strings.Join(filtered, ";")
		// Preserve REG_EXPAND_SZ type
		if err := key.SetExpandStringValue("Path", newPath); err != nil {
			return fmt.Errorf("cannot write PATH: %w", err)
		}
	}

	// Broadcast WM_SETTINGCHANGE so existing terminals pick up the change
	broadcastSettingChange()
	return nil
}

// broadcastSettingChange sends WM_SETTINGCHANGE to all top-level windows.
func broadcastSettingChange() {
	user32 := windows.NewLazyDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")

	envStr, _ := syscall.UTF16PtrFromString("Environment")
	const HWND_BROADCAST = 0xFFFF
	const WM_SETTINGCHANGE = 0x001A
	const SMTO_ABORTIFHUNG = 0x0002

	sendMessageTimeout.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(envStr)),
		uintptr(SMTO_ABORTIFHUNG),
		uintptr(5000),
		0,
	)
}

// removeBinary removes the summon binary on Windows using the
// FILE_FLAG_DELETE_ON_CLOSE GC helper pattern (rustup approach).
// On Windows, the GC process also handles data dir removal because the
// binary is inside the data dir and locked while running.
func removeBinary(binaryPath, dataDir string, keepData bool) error {
	// Place GC exe in system temp dir (not inside data dir, which will be removed)
	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	gcName := fmt.Sprintf("summon-gc-%s.exe", hex.EncodeToString(randBytes))
	gcPath := filepath.Join(os.TempDir(), gcName)

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine running executable: %w", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		return fmt.Errorf("cannot read binary for copy: %w", err)
	}
	if err := os.WriteFile(gcPath, data, 0o755); err != nil {
		return fmt.Errorf("cannot create GC exe: %w", err)
	}

	// Open GC exe with FILE_FLAG_DELETE_ON_CLOSE so OS auto-deletes it
	gcPathW, _ := syscall.UTF16PtrFromString(gcPath)
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle:      1, // inheritable
		SecurityDescriptor: nil,
	}
	handle, err := windows.CreateFile(
		gcPathW,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE,
		sa,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_DELETE_ON_CLOSE,
		0,
	)
	if err != nil {
		os.Remove(gcPath)
		return fmt.Errorf("cannot open GC exe with delete-on-close: %w", err)
	}
	_ = handle // kept open; inherited by child processes

	// Spawn GC exe to wait for us and delete the original binary (and data dir)
	gcArgs := []string{"--gc", binaryPath}
	if !keepData {
		gcArgs = append(gcArgs, dataDir)
	}
	gcCmd := exec.Command(gcPath, gcArgs...)
	gcCmd.SysProcAttr = &syscall.SysProcAttr{} // inherit handles
	if err := gcCmd.Start(); err != nil {
		windows.CloseHandle(handle)
		os.Remove(gcPath)
		return fmt.Errorf("cannot start GC process: %w", err)
	}

	// Spawn a system binary to inherit the delete-on-close handle
	// so the GC exe is auto-deleted when it exits
	netExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "net.exe")
	inheritCmd := exec.Command(netExe, "help")
	inheritCmd.SysProcAttr = &syscall.SysProcAttr{} // inherit handles
	inheritCmd.Start()

	time.Sleep(100 * time.Millisecond)
	return nil
}

// completeWindowsUninstall is the GC mode entry point. It waits for the parent
// process to exit, then deletes the original binary and optionally the data dir.
func completeWindowsUninstall(binaryPath, dataDir string) {
	// Find parent PID
	ppid := uint32(os.Getppid())

	// Wait for parent to exit
	hProcess, err := windows.OpenProcess(windows.SYNCHRONIZE, false, ppid)
	if err == nil {
		windows.WaitForSingleObject(hProcess, windows.INFINITE)
		windows.CloseHandle(hProcess)
	}

	// Delete the original binary
	os.Remove(binaryPath)

	// Delete the data directory if requested
	if dataDir != "" {
		os.RemoveAll(dataDir)
	}
}

// dataDirRemovedByGC returns true on Windows — the GC process handles data dir
// removal because the binary is inside the data dir and locked while running.
func dataDirRemovedByGC() bool {
	return true
}

// pathCleanupDescription returns a description for the removal plan display.
func pathCleanupDescription() string {
	return "PATH entry from user environment variable"
}

// pathCleanupSuccessMessage returns the success message for PATH cleanup.
func pathCleanupSuccessMessage() string {
	return "Removed PATH entry from user environment"
}

// binaryRemovalSuccessMessage returns the success message for binary removal.
func binaryRemovalSuccessMessage(_ string) string {
	return "Scheduled binary removal (will complete after this process exits)"
}
