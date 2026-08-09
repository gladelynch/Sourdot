package release

// SeriesGroup is one collapsible section of the Versions page: every
// release sharing a version number, e.g. series "4.7" holding 4.7-stable
// down through 4.7-dev1.
type SeriesGroup struct {
	Series string `json:"series"`

	// Channel of the most stable release in the group, so the UI can label
	// a whole series at a glance ("4.8 — dev only") without expanding it.
	Channel string `json:"channel"`

	Releases []Release `json:"releases"`
}

// GroupBySeries chunks a SortReleases-ordered slice into per-series groups,
// preserving that order (newest series first, most-stable release first
// within each). Releases that aren't sorted still group correctly, but
// group ordering then follows first appearance.
func GroupBySeries(releases []Release) []SeriesGroup {
	var groups []SeriesGroup
	index := make(map[string]int, len(releases))

	for _, rel := range releases {
		i, ok := index[rel.Series]
		if !ok {
			index[rel.Series] = len(groups)
			groups = append(groups, SeriesGroup{Series: rel.Series, Channel: rel.Channel})
			i = len(groups) - 1
		}
		groups[i].Releases = append(groups[i].Releases, rel)
		if channelRank[rel.Channel] > channelRank[groups[i].Channel] {
			groups[i].Channel = rel.Channel
		}
	}
	return groups
}

// ForCatalog strips a release down to what the Versions page actually
// renders: the assets installable on *this* host, and no body text (the UI
// links out to the release notes in a real browser instead of showing them
// inline).
//
// This matters at scale -- the full catalog is ~330 releases carrying 16
// classified assets each, so shipping it whole to the frontend would mean
// megabytes of JSON per page load for data no view ever reads.
func ForCatalog(releases []Release) []Release {
	wantOS, wantArch := HostPlatform()

	out := make([]Release, 0, len(releases))
	for _, rel := range releases {
		trimmed := rel
		trimmed.BodyMD = ""
		trimmed.Assets = nil
		for _, a := range rel.Assets {
			if a.OS == wantOS && a.Arch == wantArch {
				trimmed.Assets = append(trimmed.Assets, a)
			}
		}
		// A release with no asset for this host (e.g. Linux arm64 before
		// 3.6, Windows arm64 before 4.2) is nothing the user could install,
		// so it's dropped rather than shown with dead buttons.
		if len(trimmed.Assets) == 0 {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
