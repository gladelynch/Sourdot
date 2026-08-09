package release

import (
	"reflect"
	"testing"
)

// rel builds a minimal Release for ordering/grouping tests, deriving the
// same fields parseRelease would.
func rel(series, label string, major, minor, patch int) Release {
	channel, _, _ := ClassifyLabel(label)
	return Release{
		TagName: series + "-" + label,
		Series:  series,
		Major:   major,
		Minor:   minor,
		Patch:   patch,
		Label:   label,
		Channel: channel,
	}
}

// TestSortReleases covers the ordering the Versions page depends on:
// newest series first, and within a series the most stable build first.
// The input is deliberately shuffled and mixes 3.x with 4.x.
func TestSortReleases(t *testing.T) {
	releases := []Release{
		rel("4.7", "beta2", 4, 7, 0),
		rel("3.6", "stable", 3, 6, 0),
		rel("4.8", "dev3", 4, 8, 0),
		rel("4.7", "stable", 4, 7, 0),
		rel("4.7.1", "stable", 4, 7, 1),
		rel("4.7", "rc1", 4, 7, 0),
		rel("4.8", "dev10", 4, 8, 0),
		rel("4.7", "beta10", 4, 7, 0),
	}
	SortReleases(releases)

	got := make([]string, len(releases))
	for i, r := range releases {
		got[i] = r.TagName
	}
	want := []string{
		"4.8-dev10", // dev10 > dev3: sequence compares numerically, not as text
		"4.8-dev3",
		"4.7.1-stable", // 4.7.1 outranks 4.7
		"4.7-stable",   // within 4.7: stable > rc > beta
		"4.7-rc1",
		"4.7-beta10",
		"4.7-beta2",
		"3.6-stable",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortReleases order:\n got: %v\nwant: %v", got, want)
	}
}

func TestGroupBySeries(t *testing.T) {
	releases := []Release{
		rel("4.8", "dev3", 4, 8, 0),
		rel("4.8", "dev2", 4, 8, 0),
		rel("4.7", "stable", 4, 7, 0),
		rel("4.7", "rc1", 4, 7, 0),
		rel("3.6", "stable", 3, 6, 0),
	}
	groups := GroupBySeries(releases)

	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}
	if groups[0].Series != "4.8" || len(groups[0].Releases) != 2 {
		t.Errorf("group 0 = %s with %d releases, want 4.8 with 2", groups[0].Series, len(groups[0].Releases))
	}
	// A series that never went stable must advertise its best channel, so
	// the collapsed header can say "dev" without being expanded.
	if groups[0].Channel != ChannelDev {
		t.Errorf("group 4.8 channel = %q, want %q", groups[0].Channel, ChannelDev)
	}
	if groups[1].Series != "4.7" || groups[1].Channel != ChannelStable {
		t.Errorf("group 1 = (%s, %s), want (4.7, stable)", groups[1].Series, groups[1].Channel)
	}
	if groups[2].Series != "3.6" {
		t.Errorf("group 2 = %s, want 3.6", groups[2].Series)
	}
}

// TestForCatalog checks the two things the trim has to get right: it keeps
// only assets installable on this host, and it drops releases left with
// none (rather than surfacing them with dead Install buttons).
func TestForCatalog(t *testing.T) {
	hostOS, hostArch := HostPlatform()

	withAssets := rel("4.7", "stable", 4, 7, 0)
	withAssets.BodyMD = "a long release-notes body"
	withAssets.Assets = []Asset{
		{Name: "host-standard", OS: hostOS, Arch: hostArch, IsMono: false},
		{Name: "host-mono", OS: hostOS, Arch: hostArch, IsMono: true},
		{Name: "other-platform", OS: "sunos", Arch: "sparc"},
	}

	otherOnly := rel("3.5", "stable", 3, 5, 0)
	otherOnly.Assets = []Asset{{Name: "other-platform", OS: "sunos", Arch: "sparc"}}

	got := ForCatalog([]Release{withAssets, otherOnly})

	if len(got) != 1 {
		t.Fatalf("got %d releases, want 1 (the host-installable one)", len(got))
	}
	if len(got[0].Assets) != 2 {
		t.Errorf("got %d assets, want 2 (host standard + mono)", len(got[0].Assets))
	}
	for _, a := range got[0].Assets {
		if a.OS != hostOS || a.Arch != hostArch {
			t.Errorf("kept a non-host asset: %+v", a)
		}
	}
	if got[0].BodyMD != "" {
		t.Error("BodyMD should be stripped from the catalog payload")
	}
	// The trim must not mutate its input, which is the cached index.
	if withAssets.BodyMD == "" || len(withAssets.Assets) != 3 {
		t.Error("ForCatalog mutated the source release")
	}
}
