package postgres

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/lib/pq"
)

// CreateUser inserts a new user and returns the generated UUID.
func (db *DB) CreateUser(login, passwordHash string) (string, error) {
	var id string
	err := db.conn.QueryRow(
		`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`,
		login, passwordHash,
	).Scan(&id)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return "", ErrLoginTaken
		}
		return "", err
	}
	return id, nil
}

// GetUserByLogin returns the user with the given login.
func (db *DB) GetUserByLogin(login string) (*User, error) {
	u := &User{}
	err := db.conn.QueryRow(
		`SELECT id, login, password_hash, created_at FROM users WHERE login = $1`,
		login,
	).Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}
