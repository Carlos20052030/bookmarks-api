package storage

import (
	"database/sql"
	"errors"
)

// ErrNotFound is returned when a query expects one row and finds none.
var ErrNotFound = errors.New("not found")

// ErrEmailTaken is returned when inserting a user with an email that
// already exists. Detected via the Postgres unique-violation code.
var ErrEmailTaken = errors.New("email already taken")

// Store wraps a database connection and exposes typed operations.
// All methods take a context for cancellation.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}
