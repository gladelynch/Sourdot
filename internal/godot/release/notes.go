package release

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Release channels, in the order Godot promotes a version through them.
const (
	ChannelDev    = "dev"
	ChannelAlpha  = "alpha"
	ChannelBeta   = "beta"
	ChannelRC     = "rc"
	ChannelStable = "stable"
)

// channelRank orders channels least-to-most stable, so releases within a
// series sort stable-first (which is also newest-first, since a series is
// always promoted dev -> alpha -> beta -> rc -> stable over time).
var channelRank = map[string]int{
	ChannelDev:    0,
	ChannelAlpha:  1,
	ChannelBeta:   2,
	ChannelRC:     3,
	ChannelStable: 4,
}

var labelPattern = regexp.MustCompile(`^(stable|rc|beta|alpha|dev)(\d*)$`)

// ClassifyLabel maps a tag's label segment ("stable", "rc1", "beta5",
// "dev3") to its channel and sequence number.
//
// An unrecognized label falls back to ChannelDev with ok=false rather than
// being rejected: if Godot ever invents a new pre-release channel name, the
// release still shows up in the UI grouped with the least-stable builds,
// instead of silently vanishing from the catalog.
func ClassifyLabel(label string) (channel string, num int, ok bool) {
	m := labelPattern.FindStringSubmatch(strings.ToLower(label))
	if m == nil {
		return ChannelDev, 0, false
	}
	n := 0
	if m[2] != "" {
		n, _ = strconv.Atoi(m[2])
	}
	return m[1], n, true
}

// CompareLabels orders two tag labels by stability, least-stable first:
// negative when a is less stable (or earlier in its channel) than b, zero
// when they rank equally. So "dev2" < "dev3" < "beta1" < "rc1" < "stable".
//
// This is the only thing that separates two builds of the same series --
// 4.8-dev2 and 4.8-dev3 are both version 4.8.0 -- so anything ranking
// installed versions has to compare on it, not just on the numbers.
func CompareLabels(a, b string) int {
	ca, na, _ := ClassifyLabel(a)
	cb, nb, _ := ClassifyLabel(b)
	if ra, rb := channelRank[ca], channelRank[cb]; ra != rb {
		return ra - rb
	}
	return na - nb
}

var (
	notesLinkPattern     = regexp.MustCompile(`\[Release notes\]\((https://[^\s)]+)\)`)
	changelogLinkPattern = regexp.MustCompile(`\[Complete changelog\]\((https://[^\s)]+)\)`)
)

// LinksFromBody pulls the "Release notes" and "Complete changelog" links out
// of a release's body text. Every godot-builds release body is generated
// from a fixed template (tools/create-release-notes.py) that includes both,
// so this is the authoritative source -- it costs no extra requests and
// survives the Godot team hand-editing a URL, which a reconstructed one
// wouldn't. Either return value is "" when the link isn't present.
func LinksFromBody(body string) (notesURL, changelogURL string) {
	if m := notesLinkPattern.FindStringSubmatch(body); m != nil {
		notesURL = m[1]
	}
	if m := changelogLinkPattern.FindStringSubmatch(body); m != nil {
		changelogURL = m[1]
	}
	return notesURL, changelogURL
}

// ReleaseNotesURL reconstructs the canonical godotengine.org release-notes
// URL for a version/label pair, mirroring get_release_notes_url() in
// godotengine/godot-builds' tools/create-release-notes.py -- the same
// function the Godot team runs to generate the link embedded in each
// release body.
//
// This is only a fallback for when LinksFromBody comes up empty (older
// releases predate the template). Returns "" for an unrecognized label,
// since guessing a URL shape there would just produce a 404.
func ReleaseNotesURL(version, label string) string {
	channel, num, ok := ClassifyLabel(label)
	if !ok {
		return ""
	}
	slug := strings.ReplaceAll(version, ".", "-")

	if channel == ChannelStable {
		// Major (X.0) and minor (X.Y) releases get a /releases/ landing
		// page keyed by the undotted version; only patch releases (X.Y.Z)
		// get a maintenance-release article.
		if strings.Count(version, ".") < 2 {
			return "https://godotengine.org/releases/" + version + "/"
		}
		return "https://godotengine.org/article/maintenance-release-godot-" + slug + "/"
	}
	if channel == ChannelRC {
		return fmt.Sprintf("https://godotengine.org/article/release-candidate-godot-%s-rc-%d/", slug, num)
	}
	// dev, alpha, and beta all publish under the dev-snapshot article slug.
	return fmt.Sprintf("https://godotengine.org/article/dev-snapshot-godot-%s-%s-%d/", slug, channel, num)
}

// InteractiveChangelogURL points at the commit-level changelog browser the
// Godot team hosts for every tag.
func InteractiveChangelogURL(tagName string) string {
	return "https://godotengine.github.io/godot-interactive-changelog/#" + tagName
}
