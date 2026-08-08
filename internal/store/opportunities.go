package store

import (
"context"
"encoding/json"
"fmt"
"time"

"github.com/jackc/pgx/v5"
)

type OpportunityRecord struct {
ID              string
Type            string
Status          string
Legs            string
GrossProfit     string
SpreadCost      string
CommissionCost  string
SlippageCost    string
SwapCost        string
NetProfit       string
NetBps          string
QuoteTime       time.Time
ExpiresAt       time.Time
Confidence      float32
FilledAt        *time.Time
ActualSwap      *string
ActualCommission *string
ActualSlippage  *string
ActualNetProfit *string
ActualFillPrice *string
}

func (s *Store) WriteOpportunity(ctx context.Context, r OpportunityRecord) error {
legsJSON := []byte(r.Legs)
if len(legsJSON) == 0 {
legsJSON = []byte("[]")
}
_, err := s.pool.Exec(ctx,
`INSERT INTO opportunities
   (id, type, status, legs,
    gross_profit, spread_cost, commission_cost, slippage_cost, swap_cost,
    net_profit, net_bps, quote_time, expires_at, confidence)
 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
 ON CONFLICT (id) DO UPDATE SET
   status = EXCLUDED.status,
   legs = EXCLUDED.legs,
   gross_profit = EXCLUDED.gross_profit,
   net_profit = EXCLUDED.net_profit,
   net_bps = EXCLUDED.net_bps,
   expires_at = EXCLUDED.expires_at`,
r.ID, r.Type, r.Status, legsJSON,
r.GrossProfit, r.SpreadCost, r.CommissionCost, r.SlippageCost, r.SwapCost,
r.NetProfit, r.NetBps, r.QuoteTime, r.ExpiresAt, r.Confidence)
return err
}

func (s *Store) UpdateOpportunityFilled(ctx context.Context, id, status string, filledAt time.Time, actualSwap, actualCommission, actualSlippage, actualNetProfit, actualFillPrice string) error {
_, err := s.pool.Exec(ctx,
`UPDATE opportunities SET
   status = $2,
   exec_filled_at = $3,
   exec_actual_swap = $4,
   exec_actual_commission = $5,
   exec_actual_slippage = $6,
   exec_actual_net_profit = $7,
   exec_actual_fill_price = $8
 WHERE id = $1`,
id, status, filledAt, actualSwap, actualCommission, actualSlippage, actualNetProfit, actualFillPrice)
return err
}

func (s *Store) UpdateOpportunityStatus(ctx context.Context, id, status string) error {
_, err := s.pool.Exec(ctx,
`UPDATE opportunities SET status = $2 WHERE id = $1`,
id, status)
return err
}

func (s *Store) QueryOpportunities(ctx context.Context, from, to time.Time, status string, limit int32) ([]OpportunityRecord, error) {
if limit <= 0 {
limit = 100
}
q := `SELECT id, type, status, legs,
       gross_profit, spread_cost, commission_cost, slippage_cost, swap_cost,
       net_profit, net_bps, quote_time, expires_at, confidence,
       exec_filled_at, exec_actual_swap, exec_actual_commission,
       exec_actual_slippage, exec_actual_net_profit, exec_actual_fill_price
FROM opportunities WHERE created_at >= $1 AND created_at <= $2`
args := []any{from, to}
if status != "" {
q += " AND status = $3"
args = append(args, status)
}
q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args)+1)
args = append(args, limit)

rows, err := s.pool.Query(ctx, q, args...)
if err != nil {
return nil, err
}
defer rows.Close()

var results []OpportunityRecord
for rows.Next() {
var r OpportunityRecord
if err := rows.Scan(&r.ID, &r.Type, &r.Status, &r.Legs,
&r.GrossProfit, &r.SpreadCost, &r.CommissionCost, &r.SlippageCost, &r.SwapCost,
&r.NetProfit, &r.NetBps, &r.QuoteTime, &r.ExpiresAt, &r.Confidence,
&r.FilledAt, &r.ActualSwap, &r.ActualCommission,
&r.ActualSlippage, &r.ActualNetProfit, &r.ActualFillPrice); err != nil {
return nil, err
}
results = append(results, r)
}
return results, rows.Err()
}

func (s *Store) GetOpportunity(ctx context.Context, id string) (*OpportunityRecord, error) {
row := s.pool.QueryRow(ctx,
`SELECT id, type, status, legs,
 gross_profit, spread_cost, commission_cost, slippage_cost, swap_cost,
 net_profit, net_bps, quote_time, expires_at, confidence,
 exec_filled_at, exec_actual_swap, exec_actual_commission,
 exec_actual_slippage, exec_actual_net_profit, exec_actual_fill_price
 FROM opportunities WHERE id = $1`, id)

var r OpportunityRecord
err := row.Scan(&r.ID, &r.Type, &r.Status, &r.Legs,
&r.GrossProfit, &r.SpreadCost, &r.CommissionCost, &r.SlippageCost, &r.SwapCost,
&r.NetProfit, &r.NetBps, &r.QuoteTime, &r.ExpiresAt, &r.Confidence,
&r.FilledAt, &r.ActualSwap, &r.ActualCommission,
&r.ActualSlippage, &r.ActualNetProfit, &r.ActualFillPrice)
if err == pgx.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
return &r, nil
}

func MarshalLegs(legs []map[string]any) (string, error) {
b, err := json.Marshal(legs)
if err != nil {
return "", fmt.Errorf("marshal legs: %w", err)
}
return string(b), nil
}
