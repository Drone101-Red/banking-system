package db

import (
    "context"
    "database/sql"
    "fmt"

    _ "github.com/lib/pq"
)

type PostgresStore struct {
    db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, fmt.Errorf("abrir postgres: %w", err)
    }
    db.SetMaxOpenConns(20)
    db.SetMaxIdleConns(5)

    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("ping postgres: %w", err)
    }
    return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error { return s.db.Close() }

func (s *PostgresStore) Ping(ctx context.Context) error {
    return s.db.PingContext(ctx)
}