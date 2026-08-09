package store

import bolt "go.etcd.io/bbolt"

var metaSchemaVersionKey = []byte("schema_version")

// EnsureSchemaVersion stamps BucketMeta with the current SchemaVersion on a
// fresh database, or in future will run migrations when an existing
// database's stored version is behind SchemaVersion. v1 has nothing to
// migrate from yet, so this only handles the stamp-on-first-run case.
func (db *DB) EnsureSchemaVersion() error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketMeta))
		if b.Get(metaSchemaVersionKey) != nil {
			// Present: a real migration runner would compare stored vs.
			// SchemaVersion here and step through migrations. Nothing to
			// do yet.
			return nil
		}
		return b.Put(metaSchemaVersionKey, []byte{byte(SchemaVersion)})
	})
}
