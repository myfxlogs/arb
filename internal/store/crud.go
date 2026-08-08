package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// TickRecord represents a single tick for batch insertion.
type TickRecord struct {
	Time   time.Time
	Broker string
	Symbol string
	Bid    float64
	Ask    float64
}

// InsertTicks uses COPY FROM for bulk tick insertion (zero allocation hot path).
func (s *Store) InsertTicks(ctx context.Context, ticks []TickRecord) error {
	if len(ticks) == 0 {
		return nil
	}
	i := 0
	src := pgx.CopyFromFunc(func() ([]any, error) {
		if i >= len(ticks) {
			return nil, nil
		}
		t := ticks[i]
		i++
		return []any{t.Time, t.Broker, t.Symbol, t.Bid, t.Ask}, nil
	})
	_, err := s.pool.CopyFrom(ctx,
		pgx.Identifier{"ticks"},
		[]string{"ts", "broker", "symbol", "bid", "ask"},
		src,
	)
	if err != nil {
		return fmt.Errorf("copy ticks: %w", err)
	}
	return nil
}

// SignalRecord represents an arbitrage signal for storage.
type SignalRecord struct {
	ID        string
	Ts        time.Time
	Strategy  string
	Legs      string // JSONB
	GrossBps  float64
	NetBps    float64
	Executed  bool
	Dismissed bool
}

// InsertSignal inserts a new signal record.
func (s *Store) InsertSignal(ctx context.Context, r SignalRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO signals (id, ts, strategy, legs, gross_bps, net_bps, executed, dismissed)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, r.Ts, r.Strategy, r.Legs, r.GrossBps, r.NetBps, r.Executed, r.Dismissed)
	return err
}

// UpdateSignalExecuted marks a signal as executed or dismissed.
func (s *Store) UpdateSignalExecuted(ctx context.Context, id string, executed, dismissed bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE signals SET executed = $2, dismissed = $3 WHERE id = $1`,
		id, executed, dismissed)
	return err
}

// QuerySignals retrieves signals within a time range, optionally filtered by strategy.
func (s *Store) QuerySignals(ctx context.Context, from, to time.Time, strategy string, limit int32) ([]SignalRecord, error) {
	if strategy != "" {
		rows, err := s.pool.Query(ctx,
			`SELECT id, ts, strategy, legs, gross_bps, net_bps, executed, dismissed
			 FROM signals WHERE ts >= $1 AND ts <= $2 AND strategy = $3 ORDER BY ts DESC LIMIT $4`,
			from, to, strategy, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanSignals(rows)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, ts, strategy, legs, gross_bps, net_bps, executed, dismissed
		 FROM signals WHERE ts >= $1 AND ts <= $2 ORDER BY ts DESC LIMIT $3`,
		from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSignals(rows)
}

func scanSignals(rows pgx.Rows) ([]SignalRecord, error) {
	var results []SignalRecord
	for rows.Next() {
		var r SignalRecord
		if err := rows.Scan(&r.ID, &r.Ts, &r.Strategy, &r.Legs,
			&r.GrossBps, &r.NetBps, &r.Executed, &r.Dismissed); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// OrderRecord represents an order for storage.
type OrderRecord struct {
	ClientID string
	Broker   string
	Symbol   string
	Side     string
	Volume   float64
	Price    float64
	Ticket   *int64
	Status   string
	Error    *string
}

// UpsertOrder inserts or updates an order by client_id (idempotency).
func (s *Store) UpsertOrder(ctx context.Context, r OrderRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO orders (client_id, broker, symbol, side, volume, price, ticket, status, error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (client_id) DO UPDATE SET
		   ticket = EXCLUDED.ticket,
		   status = EXCLUDED.status,
		   error = EXCLUDED.error`,
		r.ClientID, r.Broker, r.Symbol, r.Side, r.Volume, r.Price,
		r.Ticket, r.Status, r.Error)
	return err
}

// QueryOrders retrieves orders within a time range.
func (s *Store) QueryOrders(ctx context.Context, from, to time.Time, limit int32) ([]OrderRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT client_id, broker, symbol, side, volume, price, ticket, status, error
		 FROM orders WHERE ts >= $1 AND ts <= $2 ORDER BY ts DESC LIMIT $3`,
		from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []OrderRecord
	for rows.Next() {
		var r OrderRecord
		if err := rows.Scan(&r.ClientID, &r.Broker, &r.Symbol, &r.Side,
			&r.Volume, &r.Price, &r.Ticket, &r.Status, &r.Error); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// DailySummary represents a daily P&L summary.
type DailySummary struct {
	Day        time.Time
	TotalPnL   float64
	TradeCount int32
	WinCount   int32
}

// UpsertDailySummary updates or inserts a daily summary.
func (s *Store) UpsertDailySummary(ctx context.Context, r DailySummary) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO daily_summary (day, total_pnl, trade_count, win_count)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (day) DO UPDATE SET
		   total_pnl = EXCLUDED.total_pnl,
		   trade_count = EXCLUDED.trade_count,
		   win_count = EXCLUDED.win_count`,
		r.Day, r.TotalPnL, r.TradeCount, r.WinCount)
	return err
}

// QueryDailySummary retrieves daily summaries for a date range.
func (s *Store) QueryDailySummary(ctx context.Context, from, to time.Time) ([]DailySummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT day, total_pnl, trade_count, win_count FROM daily_summary
		 WHERE day >= $1 AND day <= $2 ORDER BY day DESC`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DailySummary
	for rows.Next() {
		var r DailySummary
		if err := rows.Scan(&r.Day, &r.TotalPnL, &r.TradeCount, &r.WinCount); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CreateNextMonthPartition creates a monthly partition for the ticks table.
func (s *Store) CreateNextMonthPartition(ctx context.Context, year int, month time.Month) error {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	partName := fmt.Sprintf("ticks_%d_%02d", year, month)
	_, err := s.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF ticks
		 FOR VALUES FROM ('%s') TO ('%s')`,
		partName, start.Format("2006-01-02"), end.Format("2006-01-02")))
	return err
}

// EnsureCurrentPartitions creates partitions for the current and next month.
func (s *Store) EnsureCurrentPartitions(ctx context.Context) error {
	now := time.Now()
	if err := s.CreateNextMonthPartition(ctx, now.Year(), now.Month()); err != nil {
		return err
	}
	next := now.AddDate(0, 1, 0)
	return s.CreateNextMonthPartition(ctx, next.Year(), next.Month())
}
