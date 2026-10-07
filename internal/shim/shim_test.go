package shim_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gladelynch/sourdot/internal/shim"
)

// fakeGodot writes an executable that prints its name and arguments, so a
// test can see which build the launcher picked and that the arguments
// arrived intact.
func fakeGodot(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\necho " + name + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func mkProject(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestShLauncherRunsEachProjectsBuild runs the generated launcher for real
// from a range of working directories, since the thing that can go wrong
// is the walk itself: which directory it stops at, and what it does when
// it reaches a project it doesn't know.
func TestShLauncherRunsEachProjectsBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh launcher")
	}
	root, err := filepath.EvalSymlinks(t.TempDir()) // the launcher compares physical paths
	if err != nil {
		t.Fatal(err)
	}
	engines := filepath.Join(root, "engines")
	if err := os.MkdirAll(engines, 0o755); err != nil {
		t.Fatal(err)
	}
	godot42 := fakeGodot(t, engines, "godot-4.2")
	godot48 := fakeGodot(t, engines, "godot-4.8")
	godotDefault := fakeGodot(t, engines, "godot-default")

	outer := mkProject(t, filepath.Join(root, "it's outer")) // a quote in the path, for the quoting
	inner := mkProject(t, filepath.Join(outer, "addons", "inner"))
	broken := mkProject(t, filepath.Join(root, "broken"))
	untracked := mkProject(t, filepath.Join(outer, "untracked"))
	testsDir := filepath.Join(outer, "tests", "bin", "Debug")
	nowhere := filepath.Join(root, "nowhere")
	for _, d := range []string{testsDir, nowhere} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	table := shim.Table{
		Projects: []shim.Target{
			{Dir: outer, Exec: godot42},
			{Dir: inner, Exec: godot48},
			{Dir: broken, Problem: `"Broken" needs Godot 4.3, which isn't installed`},
		},
		Default: godotDefault,
	}
	launcher, err := shim.Write(filepath.Join(root, "bin"), runtime.GOOS, table)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	run := func(dir string, args ...string) (string, error) {
		cmd := exec.Command(launcher, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	cases := []struct {
		name, dir string
		args      []string
		want      string
	}{
		{"project root", outer, []string{"--headless"}, "godot-4.2 --headless"},
		{"deep inside a project", testsDir, nil, "godot-4.2"},
		{"nested project wins", inner, nil, "godot-4.8"},
		{"arguments with spaces survive", outer, []string{"--script", "a b.gd"}, "godot-4.2 --script a b.gd"},
		{"--path overrides the working directory", nowhere, []string{"--path", inner}, "godot-4.8 --path " + inner},
		{"outside any project uses the default", nowhere, []string{"--version"}, "godot-default --version"},
	}
	for _, c := range cases {
		got, err := run(c.dir, c.args...)
		if err != nil {
			t.Errorf("%s: %v (output %q)", c.name, err, got)
			continue
		}
		if got != c.want {
			t.Errorf("%s: ran %q, want %q", c.name, got, c.want)
		}
	}

	// A project that can't launch must say why and fail, not run the
	// default instead.
	if out, err := run(broken); err == nil || !strings.Contains(out, "isn't installed") {
		t.Errorf("broken project: err=%v output=%q, want a failure naming the missing version", err, out)
	}
	// An untracked project inside a tracked one must not inherit the
	// outer project's build.
	if out, err := run(untracked); err == nil || !strings.Contains(out, "isn't tracking") {
		t.Errorf("untracked project: err=%v output=%q, want a failure", err, out)
	}

	// Without a default, outside a project is an error too.
	table.Default = ""
	if _, err := shim.Write(filepath.Join(root, "bin"), runtime.GOOS, table); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out, err := run(nowhere); err == nil || !strings.Contains(out, "no default version") {
		t.Errorf("no default: err=%v output=%q, want a failure", err, out)
	}
}

func TestWriteSkipsUnchangedContent(t *testing.T) {
	dir := t.TempDir()
	table := shim.Table{Default: "/opt/godot"}
	path, err := shim.Write(dir, "linux", table)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if _, err := shim.Write(dir, "linux", table); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.ModTime().Equal(past) {
		t.Errorf("identical launcher was rewritten (stat err %v)", err)
	}
}

func TestCmdLauncherEscapesBatchSyntax(t *testing.T) {
	out := string(shim.Render("windows", shim.Table{
		Projects: []shim.Target{
			{Dir: `C:\Games\100% (demo)`, Exec: `C:\Godot\godot_console.exe`},
			{Dir: `C:\Games\broken`, Problem: `"A&B" needs Godot 4.3, which isn't installed`},
		},
	}))
	for _, want := range []string{
		`if /I "%d%"=="C:\Games\100%% (demo)" goto p0`,
		`"C:\Godot\godot_console.exe" %*`,
		`>&2 echo godot ^(sourdot^): "A^&B" needs Godot 4.3, which isn't installed`,
		"\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("launcher is missing %q:\n%s", want, out)
		}
	}
}

func TestSetupForShells(t *testing.T) {
	home := "/home/alex"
	bin := "/home/alex/.config/sourdot/bin"

	bash := shim.SetupFor("linux", "/bin/bash", bin, home)
	if bash.Profile != "~/.bashrc" || bash.Lines[0] != `export PATH="$HOME/.config/sourdot/bin":"$PATH"` ||
		bash.Lines[1] != `export GODOT_BIN="$HOME/.config/sourdot/bin/godot"` {
		t.Errorf("bash setup = %+v", bash)
	}
	if zsh := shim.SetupFor("linux", "/usr/bin/zsh", bin, home); zsh.Profile != "~/.zshrc" {
		t.Errorf("zsh profile = %q", zsh.Profile)
	}
	if fish := shim.SetupFor("linux", "/usr/bin/fish", bin, home); fish.Lines[0] != `fish_add_path "$HOME/.config/sourdot/bin"` {
		t.Errorf("fish setup = %+v", fish)
	}
	if outside := shim.SetupFor("darwin", "/bin/zsh", "/opt/it's/bin", home); outside.Lines[0] != `export PATH='/opt/it'\''s/bin':"$PATH"` {
		t.Errorf("path outside home = %q", outside.Lines[0])
	}
	win := shim.SetupFor("windows", "", `C:\Users\alex\AppData\Roaming\sourdot\bin`, `C:\Users\alex`)
	if win.Profile != "" || !strings.Contains(win.Lines[0], `'C:\Users\alex\AppData\Roaming\sourdot\bin'`) || !strings.Contains(win.Lines[1], "godot.cmd'") {
		t.Errorf("windows setup = %+v", win)
	}
}
