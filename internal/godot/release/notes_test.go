package release

import "testing"

func TestClassifyLabel(t *testing.T) {
	cases := []struct {
		label   string
		channel string
		num     int
		ok      bool
	}{
		{"stable", ChannelStable, 0, true},
		{"rc1", ChannelRC, 1, true},
		{"rc12", ChannelRC, 12, true},
		{"beta5", ChannelBeta, 5, true},
		{"alpha1", ChannelAlpha, 1, true},
		{"dev3", ChannelDev, 3, true},
		// Unknown labels must survive as dev rather than be dropped, so a
		// channel name Godot invents later still reaches the UI.
		{"nightly7", ChannelDev, 0, false},
		{"alpha0-unofficial", ChannelDev, 0, false},
	}
	for _, c := range cases {
		channel, num, ok := ClassifyLabel(c.label)
		if channel != c.channel || num != c.num || ok != c.ok {
			t.Errorf("ClassifyLabel(%q) = (%q, %d, %v), want (%q, %d, %v)",
				c.label, channel, num, ok, c.channel, c.num, c.ok)
		}
	}
}

// TestReleaseNotesURL pins the reconstruction against real godotengine.org
// URLs, each verified to return HTTP 200 in Aug 2026. This mirrors
// get_release_notes_url() in godot-builds' tools/create-release-notes.py;
// if Godot changes that function, these are the cases that should catch it.
func TestReleaseNotesURL(t *testing.T) {
	cases := []struct {
		version, label, want string
	}{
		// Minor and major stables get a /releases/ landing page...
		{"4.3", "stable", "https://godotengine.org/releases/4.3/"},
		{"4.7", "stable", "https://godotengine.org/releases/4.7/"},
		{"4.0", "stable", "https://godotengine.org/releases/4.0/"},
		// ...but patch stables get a maintenance-release article.
		{"4.7.1", "stable", "https://godotengine.org/article/maintenance-release-godot-4-7-1/"},
		// Release candidates have their own slug.
		{"4.7.2", "rc1", "https://godotengine.org/article/release-candidate-godot-4-7-2-rc-1/"},
		// dev/alpha/beta all share the dev-snapshot slug.
		{"4.4", "beta1", "https://godotengine.org/article/dev-snapshot-godot-4-4-beta-1/"},
		{"4.7", "beta1", "https://godotengine.org/article/dev-snapshot-godot-4-7-beta-1/"},
		{"4.8", "dev3", "https://godotengine.org/article/dev-snapshot-godot-4-8-dev-3/"},
		{"4.1", "alpha2", "https://godotengine.org/article/dev-snapshot-godot-4-1-alpha-2/"},
		// An unrecognized label yields no URL rather than a guessed 404.
		{"4.9", "nightly1", ""},
	}
	for _, c := range cases {
		if got := ReleaseNotesURL(c.version, c.label); got != c.want {
			t.Errorf("ReleaseNotesURL(%q, %q) = %q, want %q", c.version, c.label, got, c.want)
		}
	}
}

// TestLinksFromBody uses the real body text of godot-builds' 4.8-dev3
// release, since that template is where every catalog entry's links
// actually come from.
func TestLinksFromBody(t *testing.T) {
	body := "**Godot 4.8 dev 3** is a dev snapshot for the 4.8 feature release.\n\n" +
		"Report bugs on GitHub after checking that they haven't been reported:\n" +
		"- https://github.com/godotengine/godot/issues\n\n----\n\n" +
		"- [Release notes](https://godotengine.org/article/dev-snapshot-godot-4-8-dev-3/)\n" +
		"- [Complete changelog](https://godotengine.github.io/godot-interactive-changelog/#4.8-dev3)\n"

	notes, changelog := LinksFromBody(body)
	if want := "https://godotengine.org/article/dev-snapshot-godot-4-8-dev-3/"; notes != want {
		t.Errorf("notes URL = %q, want %q", notes, want)
	}
	if want := "https://godotengine.github.io/godot-interactive-changelog/#4.8-dev3"; changelog != want {
		t.Errorf("changelog URL = %q, want %q", changelog, want)
	}

	// A stable body carries a third link; the two we want must still be
	// picked out precisely rather than the regex swallowing the wrong one.
	stableBody := "- [Release notes](https://godotengine.org/releases/4.7/)\n" +
		"- [Curated changelog](https://github.com/godotengine/godot/blob/4.7-stable/CHANGELOG.md)\n" +
		"- [Complete changelog](https://godotengine.github.io/godot-interactive-changelog/#4.7-stable)\n"
	notes, changelog = LinksFromBody(stableBody)
	if want := "https://godotengine.org/releases/4.7/"; notes != want {
		t.Errorf("stable notes URL = %q, want %q", notes, want)
	}
	if want := "https://godotengine.github.io/godot-interactive-changelog/#4.7-stable"; changelog != want {
		t.Errorf("stable changelog URL = %q, want %q", changelog, want)
	}

	if notes, changelog := LinksFromBody("no links here"); notes != "" || changelog != "" {
		t.Errorf("expected empty results for a bodyless release, got (%q, %q)", notes, changelog)
	}
}
