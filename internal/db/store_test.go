package db

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"simple_gateway_by_codex/internal/config"
)

func TestDatabaseConnStringOmitsPassword(t *testing.T) {
	database := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "simple_gateway",
		User:     "simple_gateway",
		Password: "pa:ss@word with spaces",
		SSLMode:  "disable",
	}

	connString := databaseConnString(database)

	if strings.Contains(connString, database.Password) {
		t.Fatalf("connection string includes raw password: %q", connString)
	}
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	poolConfig.ConnConfig.Password = database.Password

	if poolConfig.ConnConfig.Host != database.Host {
		t.Fatalf("Host = %q", poolConfig.ConnConfig.Host)
	}
	if int(poolConfig.ConnConfig.Port) != database.Port {
		t.Fatalf("Port = %d", poolConfig.ConnConfig.Port)
	}
	if poolConfig.ConnConfig.Database != database.Name {
		t.Fatalf("Database = %q", poolConfig.ConnConfig.Database)
	}
	if poolConfig.ConnConfig.User != database.User {
		t.Fatalf("User = %q", poolConfig.ConnConfig.User)
	}
	if poolConfig.ConnConfig.Password != database.Password {
		t.Fatalf("Password = %q", poolConfig.ConnConfig.Password)
	}
}
