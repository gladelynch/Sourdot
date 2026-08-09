package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gladelynch/sourdot/internal/godot/install"
	"github.com/gladelynch/sourdot/internal/godot/release"
)

// sortsBefore reports whether a belongs ahead of b: newest series first,
// most-stable build first within a series (stable, rc2, rc1, ... dev1),
// standard ahead of its mono twin. It is the installed-version counterpart
// of release.SortReleases, so the installed shelf reads in the same order
// as the catalog under it.
//
// It's also what "newer" means everywhere in this package. Comparing only
// major/minor/patch isn't enough: every build in a series carries the same
// version number, so 4.8-dev2 and 4.8-dev3 tie on the numbers and the
// label is the only thing that separates them.
func sortsBefore(a, b install.InstalledVersion) bool {
	if a.Major != b.Major {
		return a.Major > b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor > b.Minor
	}
	if a.Patch != b.Patch {
		return a.Patch > b.Patch
	}
	if c := release.CompareLabels(a.Label, b.Label); c != 0 {
		return c > 0
	}
	return !a.IsMono && b.IsMono
}

// sortInstalled orders versions in place, newest and most stable first.
func sortInstalled(versions []install.InstalledVersion) {
	sort.SliceStable(versions, func(i, j int) bool {
		return sortsBefore(versions[i], versions[j])
	})
}

// sameVersion compares two version strings by their numeric components, so
// a two-component version matches its three-component spelling. Both forms
// are in circulation: install records always carry all three ("4.4.0"),
// while Godot's own tags and the pin files people hand-write drop a zero
// patch ("4.4").
func sameVersion(a, b string) bool {
	return normalizeVersion(a) == normalizeVersion(b)
}

// normalizeVersion rewrites a version string to its full major.minor.patch
// form, returning it untouched if it isn't parseable as one (an unusable
// pin then simply matches nothing, rather than matching the wrong thing).
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return v
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v
		}
		nums[i] = n
	}
	return fmt.Sprintf("%d.%d.%d", nums[0], nums[1], nums[2])
}
