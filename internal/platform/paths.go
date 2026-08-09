// Package platform resolves OS-specific filesystem locations and (later, in
// M3) exec mechanics for launching the Godot binary. Path resolution needs
// no per-OS build tags: os.UserConfigDir() already resolves correctly for
// each platform (XDG on Linux, %AppData% on Windows, ~/Library/Application
// Support on macOS). Per-OS launch_*.go files are added when M3 needs them.
package platform

import (
	"os"
	"path/filepath"
)

const appDirName = "sourdot"

// ConfigDir returns the directory Sourdot stores its settings file and
// BoltDB database in, creating it if it doesn't already exist.
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// VersionsDir returns the directory installed Godot versions are extracted
// into, one subdirectory per synthetic version ID (see internal/godot/install).
func VersionsDir() (string, error) {
	base, err := ConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "versions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
