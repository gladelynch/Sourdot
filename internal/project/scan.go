package project

import (
	"fmt"
	"os"
	"path/filepath"
)

// FindProjectFile looks for a project.godot file directly inside dir --
// v1 scope is an explicit single-folder add, not recursive discovery (see
// the plan's backlog for an optional recursive "scan subfolders" toggle).
func FindProjectFile(dir string) (string, error) {
	path := filepath.Join(dir, "project.godot")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no project.godot found in %s", dir)
	}
	return path, nil
}
