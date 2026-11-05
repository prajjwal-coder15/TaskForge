package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMissingDatabaseURL = errors.New(
	"DATABASE_URL environment variable is required",
)

type Config struct {
	DatabaseURL    string
	MaxConns       int32
	MinConns       int32
	ConnectTimeout time.Duration
}

func ConfigFromEnv() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, ErrMissingDatabaseURL
	}

	return Config{
		DatabaseURL:    databaseURL,
		MaxConns:       10,
		MinConns:       2,
		ConnectTimeout: 5 * time.Second,
	}, nil
}

func Open(
	ctx context.Context,
	cfg Config,
) (*pgxpool.Pool, error) {
	if cfg.DatabaseURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	if cfg.MaxConns <= 0 {
		return nil, errors.New("MaxConns must be positive")
	}

	if cfg.MinConns < 0 || cfg.MinConns > cfg.MaxConns {
		return nil, errors.New(
			"MinConns must be between zero and MaxConns",
		)
	}

	if cfg.ConnectTimeout <= 0 {
		return nil, errors.New(
			"ConnectTimeout must be positive",
		)
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf(
			"parse PostgreSQL configuration: %w",
			err,
		)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf(
			"create PostgreSQL pool: %w",
			err,
		)
	}

	pingCtx, cancel := context.WithTimeout(
		ctx,
		cfg.ConnectTimeout,
	)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf(
			"connect to PostgreSQL: %w",
			err,
		)
	}

	return pool, nil
}
