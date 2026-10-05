// Package store implements OpenSave's persistence layer: an embedded
// SQLite database (replacing the original single-JSON-file db.js) plus a
// one-time importer for existing users' legacy JSON data.
package store

import (
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the SQLite connection and exposes entity-scoped query
// methods (see settings.go, games.go, branches.go, snapshots.go, peers.go,
// cloudtokens.go).
type Store struct {
	db *sqlx.DB
}

// Open creates (if needed) and opens the SQLite database at path, applying
// any migrations that haven't run yet.
func Open(path string) (*Store, error) {
	// _pragma params ensure foreign keys are enforced (SQLite defaults them
	// off per-connection) and busy_timeout avoids spurious SQLITE_BUSY
	// errors from the watcher/api/p2p goroutines all touching the DB.
	// Treat the supplied path as a literal filename, not SQLite URI syntax.
	// In particular #, ? and percent escapes must not select a different DB
	// or inject connection options. Keep the special in-memory database.
	dsnPath := ":memory:"
	if path != ":memory:" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve sqlite database path: %w", err)
		}
		dsnPath = "file:" + (&url.URL{Path: filepath.ToSlash(absolute)}).EscapedPath()
	}
	dsn := dsnPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1) // modernc.org/sqlite + a single file: avoid concurrent-writer lock contention

	// Tolerate columns the struct doesn't know about. Queries here use
	// SELECT *, so without this a database written by a NEWER build hard-fails
	// an older one ("missing destination name <col>") and the app won't start
	// at all — a downgrade, or just testing a beta and going back, bricks it.
	// Ignoring unmapped columns makes the schema forward-compatible instead.
	db = db.Unsafe()

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied int
		if err := s.db.Get(&applied, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := s.db.Beginx()
		if err != nil {
			return fmt.Errorf("begin migration tx %s: %w", name, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
