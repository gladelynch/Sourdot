package project

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFeatureVersion(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"typical Godot 4 project", `PackedStringArray("4.3", "Forward Plus")`, "4.3"},
		{"beta build still tags the minor version", `PackedStringArray("4.8", "GL Compatibility")`, "4.8"},
		{"version not first", `PackedStringArray("C#", "4.2")`, "4.2"},
		{"no version token", `PackedStringArray("Forward Plus")`, ""},
		{"empty array", `PackedStringArray()`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFeatureVersion(tc.value); got != tc.want {
				t.Errorf("parseFeatureVersion(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestParseProjectGodotIconAndFeatures(t *testing.T) {
	dir := t.TempDir()
	const contents = "config_version=5\n\n[application]\n\n" +
		"config/name=\"Thumbnail Test\"\n" +
		"config/icon=\"res://icon.svg\"\n" +
		"config/features=PackedStringArray(\"4.8\", \"Forward Plus\")\n"
	path := filepath.Join(dir, "project.godot")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := ParseProjectGodot(path)
	if err != nil {
		t.Fatalf("ParseProjectGodot: %v", err)
	}
	if info.Icon != "res://icon.svg" {
		t.Errorf("Icon = %q, want res://icon.svg", info.Icon)
	}
	if info.FeatureVersion != "4.8" {
		t.Errorf("FeatureVersion = %q, want 4.8", info.FeatureVersion)
	}
}

func TestResolveThumbnailUsesConfigIcon(t *testing.T) {
	dir := t.TempDir()
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), svg, 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveThumbnail(dir, "res://icon.svg")
	want := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)
	if got != want {
		t.Errorf("ResolveThumbnail = %q, want %q", got, want)
	}
}

func TestResolveThumbnailFallsBackToDefaultCandidate(t *testing.T) {
	dir := t.TempDir()
	png := []byte("not a real png but bytes are bytes")
	if err := os.WriteFile(filepath.Join(dir, "icon.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}

	// No config/icon at all -- should still find icon.png by convention.
	got := ResolveThumbnail(dir, "")
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	if got != want {
		t.Errorf("ResolveThumbnail = %q, want %q", got, want)
	}
}

func TestResolveThumbnailNoIconFound(t *testing.T) {
	dir := t.TempDir()
	if got := ResolveThumbnail(dir, ""); got != "" {
		t.Errorf("ResolveThumbnail with no icon files = %q, want empty", got)
	}
}

func TestResolveThumbnailRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := []byte("outside the project dir")
	if err := os.WriteFile(filepath.Join(root, "secret.png"), secret, 0o644); err != nil {
		t.Fatal(err)
	}

	// A malicious/broken config/icon pointing outside the project root
	// must not be read, and must not fall through to a false match either.
	if got := ResolveThumbnail(dir, "res://../secret.png"); got != "" {
		t.Errorf("ResolveThumbnail with path-escaping icon = %q, want empty", got)
	}
}
