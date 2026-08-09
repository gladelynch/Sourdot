package core

import (
	"testing"

	"github.com/gladelynch/sourdot/internal/godot/release"
)

// seriesRelease builds a catalog entry carrying one asset for whatever host
// the tests are running on, so pickSeriesRelease's installability filter
// sees it as a real candidate.
func seriesRelease(tag string, major, minor, patch int, label string) release.Release {
	osName, arch := release.HostPlatform()
	return release.Release{
		TagName: tag,
		Series:  tag,
		Major:   major,
		Minor:   minor,
		Patch:   patch,
		Label:   label,
		Channel: channelOf(label),
		Assets: []release.Asset{
			{Name: tag + "-standard", OS: osName, Arch: arch},
			{Name: tag + "-mono", OS: osName, Arch: arch, IsMono: true},
		},
	}
}

func channelOf(label string) string {
	channel, _, _ := release.ClassifyLabel(label)
	return channel
}

// TestPickSeriesReleaseFollowsTheSeries covers what a project means when it
// records "4.8" and nothing else. Until 4.8 ships, "4.8" is the most recent
// 4.8 build there is -- filtering the catalog to stable left such a project
// with nothing installable at all, which is the dead end this pins shut.
func TestPickSeriesReleaseFollowsTheSeries(t *testing.T) {
	tests := []struct {
		name     string
		catalog  []release.Release
		want     string
		wantNone bool
	}{
		{
			name: "series still in pre-release takes the latest build",
			catalog: []release.Release{
				seriesRelease("4.8-dev5", 4, 8, 0, "dev5"),
				seriesRelease("4.8-beta1", 4, 8, 0, "beta1"),
				seriesRelease("4.8-beta2", 4, 8, 0, "beta2"),
				seriesRelease("4.8-rc1", 4, 8, 0, "rc1"),
			},
			want: "4.8-rc1",
		},
		{
			name: "dev builds only, still installable",
			catalog: []release.Release{
				seriesRelease("4.8-dev1", 4, 8, 0, "dev1"),
				seriesRelease("4.8-dev3", 4, 8, 0, "dev3"),
				seriesRelease("4.8-dev2", 4, 8, 0, "dev2"),
			},
			want: "4.8-dev3",
		},
		{
			name: "stable wins over a newer pre-release of the same series",
			catalog: []release.Release{
				seriesRelease("4.8-rc1", 4, 8, 0, "rc1"),
				seriesRelease("4.8-stable", 4, 8, 0, "stable"),
			},
			want: "4.8-stable",
		},
		{
			// The reason stability is ranked ahead of the patch number: a
			// project happily on 4.8-stable must not be dragged onto a 4.8.1
			// release candidate the day one is published.
			name: "shipped stable wins over a higher-patch release candidate",
			catalog: []release.Release{
				seriesRelease("4.8-stable", 4, 8, 0, "stable"),
				seriesRelease("4.8.1-rc1", 4, 8, 1, "rc1"),
			},
			want: "4.8-stable",
		},
		{
			name: "newest patch wins between stable builds",
			catalog: []release.Release{
				seriesRelease("4.8-stable", 4, 8, 0, "stable"),
				seriesRelease("4.8.2-stable", 4, 8, 2, "stable"),
				seriesRelease("4.8.1-stable", 4, 8, 1, "stable"),
			},
			want: "4.8.2-stable",
		},
		{
			name: "other series are never candidates",
			catalog: []release.Release{
				seriesRelease("4.9-stable", 4, 9, 0, "stable"),
				seriesRelease("4.7.2-stable", 4, 7, 2, "stable"),
			},
			wantNone: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found := pickSeriesRelease(tc.catalog, 4, 8, false)
			if tc.wantNone {
				if found {
					t.Fatalf("picked %s, want no match", got.TagName)
				}
				return
			}
			if !found {
				t.Fatal("no release picked, want " + tc.want)
			}
			if got.TagName != tc.want {
				t.Errorf("picked %s, want %s", got.TagName, tc.want)
			}
		})
	}
}

// TestPickSeriesReleaseSkipsUninstallableBuilds covers the other half of
// the broken install flow: the newest build in a series is no use if it
// ships nothing this host can run, and picking it anyway would fail at
// download time while a build that does work sits right behind it.
func TestPickSeriesReleaseSkipsUninstallableBuilds(t *testing.T) {
	newest := seriesRelease("4.8-rc1", 4, 8, 0, "rc1")
	newest.Assets = nil // published, but nothing for this host/variant

	catalog := []release.Release{newest, seriesRelease("4.8-beta2", 4, 8, 0, "beta2")}

	got, found := pickSeriesRelease(catalog, 4, 8, false)
	if !found {
		t.Fatal("no release picked; an installable older build was available")
	}
	if got.TagName != "4.8-beta2" {
		t.Errorf("picked %s, want 4.8-beta2 -- the newest build installable on this host", got.TagName)
	}

	// A C# project needs a .NET asset specifically, so a release carrying
	// only the standard build isn't a candidate for it.
	standardOnly := seriesRelease("4.8-rc2", 4, 8, 0, "rc2")
	standardOnly.Assets = standardOnly.Assets[:1]
	got, found = pickSeriesRelease([]release.Release{standardOnly, seriesRelease("4.8-beta2", 4, 8, 0, "beta2")}, 4, 8, true)
	if !found || got.TagName != "4.8-beta2" {
		t.Errorf("picked %s (found=%v) for a .NET target, want 4.8-beta2", got.TagName, found)
	}
}
