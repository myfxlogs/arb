package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgx connection pool for all database operations.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store with a connection pool.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close closes the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Pool returns the underlying pgxpool for direct access.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// EnsureMigrations runs the initial migration SQL if tables don't exist.
func (s *Store) EnsureMigrations(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS ticks (
    ts        TIMESTAMPTZ NOT NULL,
    broker    TEXT NOT NULL,
    symbol    TEXT NOT NULL,
    bid       DOUBLE PRECISION NOT NULL,
    ask       DOUBLE PRECISION NOT NULL
) PARTITION BY RANGE (ts);

CREATE TABLE IF NOT EXISTS signals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    strategy    TEXT NOT NULL,
    legs        JSONB NOT NULL,
    pnl         DOUBLE PRECISION,
    status      TEXT NOT NULL DEFAULT 'pending'
);
CREATE TABLE IF NOT EXISTS orders (
    client_id   TEXT PRIMARY KEY,
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    broker      TEXT NOT NULL,
    symbol      TEXT NOT NULL,
    side        TEXT NOT NULL,
    volume      DOUBLE PRECISION NOT NULL,
    price       DOUBLE PRECISION NOT NULL,
    ticket      BIGINT,
    status      TEXT NOT NULL DEFAULT 'submitted',
    error       TEXT
);

CREATE TABLE IF NOT EXISTS daily_summary (
    day         DATE PRIMARY KEY,
    total_pnl   DOUBLE PRECISION NOT NULL DEFAULT 0,
    trade_count INTEGER NOT NULL DEFAULT 0,
    win_count   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS broker_accounts (
    name        TEXT PRIMARY KEY,
    platform    INTEGER NOT NULL,
    host        TEXT NOT NULL DEFAULT '',
    server      TEXT NOT NULL DEFAULT '',
    port        INTEGER NOT NULL DEFAULT 443,
    login       BIGINT NOT NULL DEFAULT 0,
    password    TEXT NOT NULL DEFAULT '',
    company     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
	`)
	return err
}
