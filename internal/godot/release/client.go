package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const releasesURL = "https://api.github.com/repos/godotengine/godot/releases?per_page=100"

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

// ListStableReleases fetches releases from godotengine/godot and returns
// only stable (non-draft, non-prerelease) ones, newest first -- v1's
// scope, per the plan, is the stable channel only. Each release's Assets
// is filtered down to classified editor zips; ChecksumsURL is populated
// from the release's SHA512-SUMS.txt asset if present.
func (c *Client) ListStableReleases(ctx context.Context) ([]Release, error) {
	body, err := c.get(ctx, releasesURL)
	if err != nil {
		return nil, err
	}

	var raws []rawRelease
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("decoding releases response: %w", err)
	}

	releases := make([]Release, 0, len(raws))
	for _, r := range raws {
		if r.Draft || r.Prerelease {
			continue
		}
		if rel, ok := parseRelease(r); ok {
			releases = append(releases, rel)
		}
	}
	return releases, nil
}

func parseRelease(r rawRelease) (Release, bool) {
	m := tagPattern.FindStringSubmatch(r.TagName)
	if m == nil {
		return Release{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch := 0
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}

	rel := Release{
		TagName:     r.TagName,
		Major:       major,
		Minor:       minor,
		Patch:       patch,
		Label:       m[4],
		PublishedAt: r.PublishedAt,
		BodyMD:      r.Body,
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
