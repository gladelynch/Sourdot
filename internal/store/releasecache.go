package store

import (
	"encoding/json"

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
