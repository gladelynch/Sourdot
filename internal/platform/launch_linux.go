//go:build linux

package platform

import "os/exec"

// Launch starts the Godot editor for projectPath using binaryPath (a
// direct executable, as returned by install.ResolveBinaryPath on Linux).
// --editor is required: --path alone runs the project as a game rather
// than opening it for editing -- verified against a real 4.7.1-stable
// binary's --help output ("-e, --editor: Start the editor instead of
// running the scene").
//
// Returns the started *os.Process without waiting on it: the editor is
// meant to keep running independently of GodotVM.
func Launch(binaryPath, projectPath string) (*exec.Cmd, error) {
	cmd := exec.Command(binaryPath, "--path", projectPath, "--editor")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
