// Package store is the local persistence layer: a BoltDB file for
// versions/projects/release-cache records (chosen over a naively-rewritten
// JSON file specifically to avoid the "state resets on restart" bug class
// seen in competitor tools — see the plan's rationale), plus a small JSON
// settings file for the singleton app config.
package store

import (
	"time"

	bolt "go.etcd.io/bbolt"
)

// Bucket names. Kept as constants so callers never typo a bucket key.
const (
	BucketInstalledVersions = "installed_versions"
	BucketProjects          = "projects"
	BucketReleaseCache      = "release_cache"
	BucketMeta              = "meta"
)

// SchemaVersion is bumped whenever the on-disk record shapes change in a
// way that needs a migration. Recorded in BucketMeta from the very first
// run, even though v1 has nothing to migrate from yet.
const SchemaVersion = 1

// DB wraps a BoltDB handle with Sourdot's bucket schema.
type DB struct {
	bolt *bolt.DB
}

// Open opens (creating if necessary) the BoltDB file at path, ensures all
// known buckets exist, and stamps the schema version on first run.
func Open(path string) (*DB, error) {
	bdb, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}

	db := &DB{bolt: bdb}
	if err := db.bolt.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{BucketInstalledVersions, BucketProjects, BucketReleaseCache, BucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.bolt.Close()
		return nil, err
	}

	return db, nil
}

// Close closes the underlying BoltDB file handle.
func (db *DB) Close() error {
	return db.bolt.Close()
}
