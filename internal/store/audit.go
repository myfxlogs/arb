package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditEntry represents an audit log record.
type AuditEntry struct {
	Timestamp time.Time
	EventType string
	Broker    string
	Detail    string
}

// AuditLog writes audit entries to the database.
type AuditLog struct {
	pool *pgxpool.Pool
}

// NewAuditLog creates an AuditLog using the same connection pool.
func NewAuditLog(pool *pgxpool.Pool) *AuditLog {
	return &AuditLog{pool: pool}
}

// EnsureAuditTable creates the audit_log table if it doesn't exist.
func (a *AuditLog) EnsureAuditTable(ctx context.Context) error {
	_, err := a.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS audit_log (
    id        BIGSERIAL PRIMARY KEY,
    ts        TIMESTAMPTZ NOT NULL DEFAULT now(),
    event     TEXT NOT NULL,
    broker    TEXT,
    detail    TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_log(ts DESC);
	`)
	return err
}

// Log writes a single audit entry.
func (a *AuditLog) Log(ctx context.Context, entry AuditEntry) error {
	_, err := a.pool.Exec(ctx,
		`INSERT INTO audit_log (ts, event, broker, detail) VALUES ($1, $2, $3, $4)`,
		entry.Timestamp, entry.EventType, entry.Broker, entry.Detail)
	return err
}

// Logf writes an audit entry with a formatted detail string.
func (a *AuditLog) Logf(ctx context.Context, eventType, broker, format string, args ...any) error {
	return a.Log(ctx, AuditEntry{
		Timestamp: time.Now(),
		EventType: eventType,
		Broker:    broker,
		Detail:    fmt.Sprintf(format, args...),
	})
}
