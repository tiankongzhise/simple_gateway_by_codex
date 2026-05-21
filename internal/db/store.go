package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store owns PostgreSQL access for the gateway.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Pool exposes the underlying pgx pool to repositories.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Close closes database connections.
func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}
