package release

import "testing"

// Fixtures are real asset filenames pulled from the godotengine/godot
// GitHub API (checked Aug 2026): 3.5.3-stable, 3.6-stable, 4.0-stable, and
// 4.7.1-stable, spanning exactly the 3.x/4.x and standard/mono naming
// variance the plan flagged as a risk to verify during M1.
func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		want ClassifiedAsset // zero value means "must not classify"
	}{
		// 4.x standard
		{"Godot_v4.7.1-stable_linux.x86_64.zip", ClassifiedAsset{"linux", "x86_64", false}},
		{"Godot_v4.7.1-stable_linux.x86_32.zip", ClassifiedAsset{"linux", "x86_32", false}},
		{"Godot_v4.7.1-stable_linux.arm64.zip", ClassifiedAsset{"linux", "arm64", false}},
		{"Godot_v4.7.1-stable_linux.arm32.zip", ClassifiedAsset{"linux", "arm32", false}},
		{"Godot_v4.7.1-stable_macos.universal.zip", ClassifiedAsset{"macos", "universal", false}},
		{"Godot_v4.7.1-stable_win64.exe.zip", ClassifiedAsset{"windows", "x86_64", false}},
		{"Godot_v4.7.1-stable_win32.exe.zip", ClassifiedAsset{"windows", "x86_32", false}},
		{"Godot_v4.7.1-stable_windows_arm64.exe.zip", ClassifiedAsset{"windows", "arm64", false}},

		// 4.x mono
		{"Godot_v4.7.1-stable_mono_linux_x86_64.zip", ClassifiedAsset{"linux", "x86_64", true}},
		{"Godot_v4.7.1-stable_mono_macos.universal.zip", ClassifiedAsset{"macos", "universal", true}},
		{"Godot_v4.7.1-stable_mono_win64.zip", ClassifiedAsset{"windows", "x86_64", true}},
		{"Godot_v4.7.1-stable_mono_win32.zip", ClassifiedAsset{"windows", "x86_32", true}},
		{"Godot_v4.7.1-stable_mono_windows_arm64.zip", ClassifiedAsset{"windows", "arm64", true}},

		// 3.x standard -- note "x11"/"osx" naming, not "linux"/"macos"
		{"Godot_v3.6-stable_x11.64.zip", ClassifiedAsset{"linux", "x86_64", false}},
		{"Godot_v3.6-stable_x11.32.zip", ClassifiedAsset{"linux", "x86_32", false}},
		{"Godot_v3.6-stable_osx.universal.zip", ClassifiedAsset{"macos", "universal", false}},
		{"Godot_v3.6-stable_win64.exe.zip", ClassifiedAsset{"windows", "x86_64", false}},

		// 3.x mono
		{"Godot_v3.6-stable_mono_x11_64.zip", ClassifiedAsset{"linux", "x86_64", true}},
		{"Godot_v3.6-stable_mono_osx.universal.zip", ClassifiedAsset{"macos", "universal", true}},
		{"Godot_v3.6-stable_mono_win64.zip", ClassifiedAsset{"windows", "x86_64", true}},

		// must NOT classify as an installable editor zip
		{"godot-4.7.1-stable.tar.xz", ClassifiedAsset{}},
		{"godot-4.7.1-stable.tar.xz.sha256", ClassifiedAsset{}},
		{"Godot_v4.7.1-stable_export_templates.tpz", ClassifiedAsset{}},
		{"Godot_v4.7.1-stable_mono_export_templates.tpz", ClassifiedAsset{}},
		{"Godot_v4.7.1-stable_android_editor.apk", ClassifiedAsset{}},
		{"Godot_v4.7.1-stable_web_editor.zip", ClassifiedAsset{}},
		{"godot-lib.4.7.1.stable.template_release.aar", ClassifiedAsset{}},
		{"SHA512-SUMS.txt", ClassifiedAsset{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Classify(tc.name)
			wantOK := tc.want != (ClassifiedAsset{})
			if ok != wantOK {
				t.Fatalf("Classify(%q) ok = %v, want %v", tc.name, ok, wantOK)
			}
			if ok && got != tc.want {
				t.Fatalf("Classify(%q) = %+v, want %+v", tc.name, got, tc.want)
			}
		})
	}
}
