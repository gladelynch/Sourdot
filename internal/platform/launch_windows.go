//go:build windows

package platform

import "os/exec"

// Launch starts the Godot editor for projectPath using binaryPath.
//
// NOTE: written to documented Godot CLI behavior but not yet verified on
// real Windows hardware -- flagged in the plan's open risks for M6's
// cross-platform pass.
func Launch(binaryPath, projectPath string) (*exec.Cmd, error) {
	cmd := exec.Command(binaryPath, "--path", projectPath, "--editor")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
