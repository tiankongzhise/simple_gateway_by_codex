package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"simple_gateway_by_codex/internal/config"
)

// Store owns PostgreSQL access for the gateway.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL.
func Open(ctx context.Context, database config.DatabaseConfig) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseConnString(database))
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	poolConfig.ConnConfig.Password = database.Password

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func databaseConnString(database config.DatabaseConfig) string {
	parts := []string{
		"host=" + quoteConnStringValue(database.Host),
		"port=" + strconv.Itoa(database.Port),
		"dbname=" + quoteConnStringValue(database.Name),
		"user=" + quoteConnStringValue(database.User),
		"sslmode=" + quoteConnStringValue(database.SSLMode),
	}
	return strings.Join(parts, " ")
}

func quoteConnStringValue(value string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + "'"
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
