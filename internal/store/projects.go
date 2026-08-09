package store

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"

	"github.com/gladelynch/sourdot/internal/project"
)

// PutProject upserts a record, keyed by its ID.
func (db *DB) PutProject(p project.Project) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketProjects)).Put([]byte(p.ID), raw)
	})
}

// GetProject looks up a record by ID.
func (db *DB) GetProject(id string) (project.Project, bool, error) {
	var p project.Project
	found := false
	err := db.bolt.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte(BucketProjects)).Get([]byte(id))
		if raw == nil {
			return nil
		}
		found = true
		return json.Unmarshal(raw, &p)
	})
	return p, found, err
}

// ListProjects returns every tracked project.
func (db *DB) ListProjects() ([]project.Project, error) {
	var out []project.Project
	err := db.bolt.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketProjects)).ForEach(func(_, v []byte) error {
			var p project.Project
			if err := json.Unmarshal(v, &p); err != nil {
				return err
			}
			out = append(out, p)
			return nil
		})
	})
	return out, err
}

// DeleteProject removes a project record by ID (this only stops tracking
// it -- it never touches the project's files on disk).
func (db *DB) DeleteProject(id string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketProjects)).Delete([]byte(id))
	})
}
