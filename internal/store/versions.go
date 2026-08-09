package store

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"

	"github.com/gladelynch/sourdot/internal/godot/install"
)

// PutInstalledVersion upserts a record, keyed by its ID.
func (db *DB) PutInstalledVersion(v install.InstalledVersion) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketInstalledVersions)).Put([]byte(v.ID), raw)
	})
}

// GetInstalledVersion looks up a record by ID.
func (db *DB) GetInstalledVersion(id string) (install.InstalledVersion, bool, error) {
	var iv install.InstalledVersion
	found := false
	err := db.bolt.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte(BucketInstalledVersions)).Get([]byte(id))
		if raw == nil {
			return nil
		}
		found = true
		return json.Unmarshal(raw, &iv)
	})
	return iv, found, err
}

// ListInstalledVersions returns every persisted InstalledVersion.
func (db *DB) ListInstalledVersions() ([]install.InstalledVersion, error) {
	var out []install.InstalledVersion
	err := db.bolt.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketInstalledVersions)).ForEach(func(_, v []byte) error {
			var iv install.InstalledVersion
			if err := json.Unmarshal(v, &iv); err != nil {
				return err
			}
			out = append(out, iv)
			return nil
		})
	})
	return out, err
}

// DeleteInstalledVersion removes a record by ID. Callers are responsible
// for removing the on-disk install directory first.
func (db *DB) DeleteInstalledVersion(id string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketInstalledVersions)).Delete([]byte(id))
	})
}
