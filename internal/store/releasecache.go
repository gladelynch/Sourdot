package store

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

type releaseCacheEntry struct {
	ETag string `json:"etag"`
	Body []byte `json:"body"`
}

// GetReleaseCache and PutReleaseCache satisfy release.Cache structurally
// (internal/godot/release doesn't import this package, to keep it
// dependency-free) so *DB can be passed directly to release.NewClient.
func (db *DB) GetReleaseCache(key string) (etag string, body []byte, ok bool) {
	_ = db.bolt.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte(BucketReleaseCache)).Get([]byte(key))
		if raw == nil {
			return nil
		}
		var entry releaseCacheEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil
		}
		etag, body, ok = entry.ETag, entry.Body, true
		return nil
	})
	return etag, body, ok
}

func (db *DB) PutReleaseCache(key, etag string, body []byte) error {
	raw, err := json.Marshal(releaseCacheEntry{ETag: etag, Body: body})
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketReleaseCache)).Put([]byte(key), raw)
	})
}

// releaseIndexKey holds the *parsed* release catalog, as distinct from the
// raw ETag'd API page bodies above. Versioned so a future change to the
// Release struct invalidates stale entries instead of decoding them into
// half-populated values.
const releaseIndexKey = "__parsed_index_v1"

type releaseIndexEntry struct {
	FetchedAt time.Time       `json:"fetchedAt"`
	Releases  json.RawMessage `json:"releases"`
}

// GetReleaseIndex returns the last-persisted parsed release catalog and
// when it was fetched. Godot releases are immutable once published, so
// this is what lets the Versions page paint the full history instantly on
// launch -- and keep working entirely offline -- rather than blocking on
// four GitHub API round-trips.
func (db *DB) GetReleaseIndex() (body []byte, fetchedAt time.Time, ok bool) {
	_ = db.bolt.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte(BucketReleaseCache)).Get([]byte(releaseIndexKey))
		if raw == nil {
			return nil
		}
		var entry releaseIndexEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil
		}
		body, fetchedAt, ok = entry.Releases, entry.FetchedAt, true
		return nil
	})
	return body, fetchedAt, ok
}

// PutReleaseIndex persists the parsed release catalog, stamped with the
// current time.
func (db *DB) PutReleaseIndex(body []byte) error {
	raw, err := json.Marshal(releaseIndexEntry{FetchedAt: time.Now(), Releases: body})
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketReleaseCache)).Put([]byte(releaseIndexKey), raw)
	})
}
