package core

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/godot/release"
	"github.com/gladelynch/sourdot/internal/project"
	"github.com/gladelynch/sourdot/internal/shim"
)

// WriteShim regenerates the `godot` launcher in binDir from projects, which
// must come from ListProjects: the launcher is built from each project's
// Resolution, and only ListProjects fills that in. Returns the launcher's
// path.
func (pm *ProjectManager) WriteShim(binDir string, projects []project.Project) (string, error) {
	osName, _ := release.HostPlatform()
	return shim.Write(binDir, runtime.GOOS, pm.ShimTable(projects, osName))
}

// ShimTable decides what `godot` runs for each project, by the same rules
// as OpenProject: a project runs its resolved build or reports why it
// can't, and never falls back to a different one.
//
// It is one step stricter than the Open button in one case. A project
// whose resolution would upgrade it to a newer series is refused rather
// than run, because the Open button gets to ask first and a headless test
// run doesn't -- and the upgrade can't be undone. Opening it from Sourdot
// once performs the upgrade, after which the project declares the new
// series and the launcher runs it.
func (pm *ProjectManager) ShimTable(projects []project.Project, osName string) shim.Table {
	var t shim.Table
	for _, p := range projects {
		if p.Missing || p.Resolution == nil {
			continue
		}
		target := shim.Target{}
		res := p.Resolution
		switch {
		case res.Status == project.StatusUnknown:
			target.Problem = fmt.Sprintf("%q doesn't record which Godot version it uses -- choose one for it in Sourdot", p.Name)
		case res.Status == project.StatusMissing:
			target.Problem = fmt.Sprintf("%q needs Godot %s, which isn't installed -- install it from Sourdot", p.Name, res.Label)
		case res.WillUpgrade:
			target.Problem = fmt.Sprintf("%q is a Godot %s project, and running it in %s would upgrade it for good -- open it from Sourdot once to confirm the upgrade", p.Name, p.DeclaredVersion, res.Label)
		default:
			iv, found, err := pm.db.GetInstalledVersion(res.VersionID)
			if err != nil || !found {
				target.Problem = fmt.Sprintf("Godot %s is no longer installed", res.Label)
			} else {
				target.Exec = install.CLIExecutable(iv.BinaryPath, osName)
			}
		}

		// The launcher compares against the physical working directory
		// (pwd -P), so a project added through a symlinked path needs its
		// resolved form listed too.
		target.Dir = p.Path
		t.Projects = append(t.Projects, target)
		if real, err := filepath.EvalSymlinks(p.Path); err == nil && real != p.Path {
			target.Dir = real
			t.Projects = append(t.Projects, target)
		}
	}
	// Sorted so the rendered script only changes when an answer does --
	// shim.Write skips the rewrite when the bytes match.
	sort.Slice(t.Projects, func(i, j int) bool { return t.Projects[i].Dir < t.Projects[j].Dir })

	if iv, ok := pm.defaultVersion(); ok {
		t.Default = install.CLIExecutable(iv.BinaryPath, osName)
	}
	return t
}
