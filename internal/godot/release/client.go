package release

// Client fetches and ETag-caches Godot releases from the GitHub Releases
// API. Implemented in M1.
//
// Design notes for M1 (see plan's "Open Risks"):
//   - Unauthenticated GitHub API access is capped at 60 req/hr; cache
//     aggressively (store.BucketReleaseCache) before reaching for an
//     optional user-supplied PAT (store.Settings.GitHubToken).
//   - Use If-None-Match with the cached ETag to avoid burning quota on
//     unchanged data.
type Client struct {
	// token, httpClient, etc. added in M1.
}

// NewClient constructs a release Client. token may be empty.
func NewClient(token string) *Client {
	return &Client{}
}
