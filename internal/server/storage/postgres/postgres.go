// Package postgres implements the storage interfaces using PostgreSQL.
package postgres

import (
	"database/sql"
	"errors"
	"io/fs"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

// ErrNotFound is returned when the requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrVersionConflict is returned when an update fails due to version mismatch.
var ErrVersionConflict = errors.New("version conflict")

// ErrLoginTaken is returned when the login is already registered.
var ErrLoginTaken = errors.New("login already taken")

// User represents a registered user row.
type User struct {
	ID           string
	Login        string
	PasswordHash string
	CreatedAt    time.Time
}

// Secret represents a secret row (Data may be nil for binary secrets stored in MinIO).
type Secret struct {
	ID         string
	UserID     string
	Type       string
	Name       string
	Metadata   string
	Data       []byte
	StorageKey string
	FileSize   int64
	Version    int32
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// DB wraps a *sql.DB connection and provides user/secret persistence.
type DB struct {
	conn *sql.DB
}

// New opens a Postgres connection and runs all pending migrations using migrationsFS.
func New(dsn string, migrationsFS fs.FS) (*DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(25)
	conn.SetConnMaxLifetime(5 * time.Minute)

	if err := runMigrations(conn, migrationsFS); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &DB{conn: conn}, nil
}

// Close closes the underlying connection pool.
func (db *DB) Close() error {
	return db.conn.Close()
}

func runMigrations(conn *sql.DB, migrationsFS fs.FS) error {
	src, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return err
	}
	driver, err := migratepostgres.WithInstance(conn, &migratepostgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
