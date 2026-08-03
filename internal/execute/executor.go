package execute

import (
	"context"
	"fmt"

	"arb/internal/adapter"
	"github.com/shopspring/decimal"
)

// OrderExecutor wraps an adapter with a channel-based semaphore for
// concurrent order rate limiting.
type OrderExecutor struct {
	adapter adapter.PlatformAdapter
	sem     chan struct{}
}

// NewOrderExecutor creates an OrderExecutor with a concurrency limit.
func NewOrderExecutor(a adapter.PlatformAdapter, maxConcurrent int) *OrderExecutor {
	return &OrderExecutor{
		adapter: a,
		sem:     make(chan struct{}, maxConcurrent),
	}
}

// Execute acquires the semaphore and delegates to PlaceOrder.
func (e *OrderExecutor) Execute(ctx context.Context, req adapter.OrderRequest) (*adapter.OrderResult, error) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return e.adapter.PlaceOrder(ctx, req)
}

// CloseOrder acquires the semaphore and delegates to CloseOrder.
func (e *OrderExecutor) CloseOrder(ctx context.Context, ticket int64, lots decimal.Decimal, price float64, slippage int32) (*adapter.OrderResult, error) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return e.adapter.CloseOrder(ctx, ticket, lots, price, slippage)
}

// BrokerName returns the underlying adapter's broker name.
func (e *OrderExecutor) BrokerName() string {
	return e.adapter.BrokerName()
}

// String returns a debug string.
func (e *OrderExecutor) String() string {
	return fmt.Sprintf("OrderExecutor(%s)", e.adapter.BrokerName())
}
