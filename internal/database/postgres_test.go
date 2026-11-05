package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestConfigFromEnvRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := ConfigFromEnv()

	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf(
			"expected ErrMissingDatabaseURL, got %v",
			err,
		)
	}
}

func TestOpenRejectsInvalidPoolConfiguration(t *testing.T) {
	cfg := Config{
		DatabaseURL:    "postgres://example:example@localhost/example",
		MaxConns:       0,
		MinConns:       0,
		ConnectTimeout: time.Second,
	}

	_, err := Open(context.Background(), cfg)

	if err == nil {
		t.Fatal("expected invalid pool configuration error")
	}
}

func TestPostgresConnection(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	cfg := Config{
		DatabaseURL:    databaseURL,
		MaxConns:       2,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
	}

	ctx := context.Background()

	pool, err := Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
}
