package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// godot-builds carries every stable tag *and* every dev/alpha/beta/rc
	// pre-release (349 releases as of Aug 2026, back to 1.0), with the same
	// asset naming as godotengine/godot. Fetching it alone replaces what
	// used to take a separate stable-only source.
	releasesRepo    = "godotengine/godot-builds"
	releasesPerPage = 100

	// maxReleasePages bounds the paginated walk. The full catalog needs 4
	// pages today; the ceiling leaves room to grow while guaranteeing we
	// can't spin against a misbehaving API.
	maxReleasePages = 8

	// minSupportedMajor drops 1.x/2.x, whose asset filenames predate every
	// pattern in the Classify table (so they'd yield releases with zero
	// installable assets) and which no current project targets.
	minSupportedMajor = 3
)

func releasesURL(page int) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=%d&page=%d",
		releasesRepo, releasesPerPage, page)
}

// Cache lets Client avoid burning GitHub's 60/hr unauthenticated rate
// limit on unchanged data. Defined here (rather than importing
// internal/store) to keep this package dependency-free; *store.DB
// satisfies this interface structurally.
type Cache interface {
	GetReleaseCache(key string) (etag string, body []byte, ok bool)
	PutReleaseCache(key, etag string, body []byte) error
}

// Client fetches and ETag-caches Godot releases from the GitHub Releases
// API.
type Client struct {
	httpClient *http.Client
	token      string
	cache      Cache
}

// NewClient constructs a release Client. token and cache may both be
// empty/nil -- an empty token means unauthenticated (60 req/hr) access,
// and a nil cache disables ETag caching entirely.
func NewClient(token string, cache Cache) *Client {
	return &Client{httpClient: &http.Client{Timeout: 30 * time.Second}, token: token, cache: cache}
}

type rawRelease struct {
	TagName     string     `json:"tag_name"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	PublishedAt time.Time  `json:"published_at"`
	Body        string     `json:"body"`
	Assets      []rawAsset `json:"assets"`
}

type rawAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// tagPattern splits a tag like "4.7.1-stable" or "3.6-stable" into
// major.minor[.patch]-label.
var tagPattern = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?-(.+)$`)

// ListReleases fetches the whole godot-builds catalog -- every channel,
// stable and pre-release alike -- and returns it sorted newest series
// first, with the most stable build of each series leading its series.
// Each release's Assets is filtered down to classified editor zips;
// ChecksumsURL is populated from the release's SHA512-SUMS.txt asset if
// present.
func (c *Client) ListReleases(ctx context.Context) ([]Release, error) {
	var releases []Release

	for page := 1; page <= maxReleasePages; page++ {
		body, err := c.get(ctx, releasesURL(page))
		if err != nil {
			if page > 1 && len(releases) > 0 {
				// Page 1 is the only page that ever gains entries (releases
				// are immutable once published), so a later page failing
				// costs us older history, not currency. Returning the
				// partial catalog beats failing the whole Versions page.
				break
			}
			return nil, err
		}

		var raws []rawRelease
		if err := json.Unmarshal(body, &raws); err != nil {
			return nil, fmt.Errorf("decoding releases page %d: %w", page, err)
		}

		for _, r := range raws {
			if r.Draft {
				continue
			}
			if rel, ok := parseRelease(r); ok {
				releases = append(releases, rel)
			}
		}

		if len(raws) < releasesPerPage {
			break // short page: that was the last one
		}
	}

	SortReleases(releases)
	return releases, nil
}

// SortReleases orders releases newest-series-first, and within a series
// most-stable-first (stable, then rc3, rc2, beta5, ..., dev1) -- which for
// a series is also reverse-chronological, since Godot only ever promotes a
// version up the channel ladder.
func SortReleases(releases []Release) {
	sort.SliceStable(releases, func(i, j int) bool {
		a, b := releases[i], releases[j]
		if a.Major != b.Major {
			return a.Major > b.Major
		}
		if a.Minor != b.Minor {
			return a.Minor > b.Minor
		}
		if a.Patch != b.Patch {
			return a.Patch > b.Patch
		}
		return CompareLabels(a.Label, b.Label) > 0
	})
}

func parseRelease(r rawRelease) (Release, bool) {
	m := tagPattern.FindStringSubmatch(r.TagName)
	if m == nil {
		return Release{}, false
	}
	major, _ := strconv.Atoi(m[1])
	if major < minSupportedMajor {
		return Release{}, false
	}
	minor, _ := strconv.Atoi(m[2])
	patch := 0
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}

	label := m[4]
	channel, _, _ := ClassifyLabel(label)

	// Series is the tag minus its label, kept verbatim rather than rebuilt
	// from major/minor/patch: Godot writes two-component tags for X.0
	// releases ("4.7-stable", never "4.7.0-stable"), and the UI groups on
	// this string, so it has to match the tag exactly.
	series := strings.TrimSuffix(r.TagName, "-"+label)

	notesURL, changelogURL := LinksFromBody(r.Body)
	if notesURL == "" {
		notesURL = ReleaseNotesURL(series, label)
	}
	if changelogURL == "" {
		changelogURL = InteractiveChangelogURL(r.TagName)
	}

	rel := Release{
		TagName:         r.TagName,
		Series:          series,
		Major:           major,
		Minor:           minor,
		Patch:           patch,
		Label:           label,
		Channel:         channel,
		PublishedAt:     r.PublishedAt,
		BodyMD:          r.Body,
		ReleaseNotesURL: notesURL,
		ChangelogURL:    changelogURL,
	}

	for _, a := range r.Assets {
		if a.Name == "SHA512-SUMS.txt" {
			rel.ChecksumsURL = a.BrowserDownloadURL
			continue
		}
		info, ok := Classify(a.Name)
		if !ok {
			continue
		}
		rel.Assets = append(rel.Assets, Asset{
			Name:        a.Name,
			DownloadURL: a.BrowserDownloadURL,
			SizeBytes:   a.Size,
			OS:          info.OS,
			Arch:        info.Arch,
			IsMono:      info.IsMono,
		})
	}
	return rel, true
}

// get performs an ETag-conditional GET, serving stale cached data if the
// API is unreachable or rate-limited and a cached copy exists.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var cachedETag string
	var cachedBody []byte
	if c.cache != nil {
		if etag, body, ok := c.cache.GetReleaseCache(url); ok {
			cachedETag, cachedBody = etag, body
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if cachedETag != "" {
		req.Header.Set("If-None-Match", cachedETag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if cachedBody != nil {
			return cachedBody, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified && cachedBody != nil:
		return cachedBody, nil
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		if cachedBody != nil {
			return cachedBody, nil // rate-limited: serve stale cache rather than failing outright
		}
		return nil, fmt.Errorf("github API rate limit hit (status %d) and no cached data available", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("github API returned status %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if c.cache != nil {
		if etag := resp.Header.Get("ETag"); etag != "" {
			_ = c.cache.PutReleaseCache(url, etag, body)
		}
	}
	return body, nil
}
