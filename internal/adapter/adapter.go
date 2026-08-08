package adapter

import (
	"context"
	"time"

	"arb/internal/bus"
	"github.com/shopspring/decimal"
)

// PlatformAdapter abstracts MT4/MT5/Binance connections.
type PlatformAdapter interface {
	Connect(ctx context.Context) (token string, err error)
	Disconnect() error
	Stop()
	HealthCheck(ctx context.Context) error

	Subscribe(ctx context.Context, symbols []string) error
	QuoteStream(ctx context.Context, b *bus.QuoteBus)

	AccountSummary(ctx context.Context) (*Account, error)
	OpenOrders(ctx context.Context) ([]Order, error)
	OrderHistory(ctx context.Context, from, to time.Time) ([]Order, error)
	AllSymbols(ctx context.Context) ([]string, error)
	SymbolDigits(ctx context.Context, symbols []string) (map[string]int32, error)

	PlaceOrder(ctx context.Context, req OrderRequest) (*OrderResult, error)
	CancelOrder(ctx context.Context, ticket int64) error
	CloseOrder(ctx context.Context, ticket int64, lots decimal.Decimal, price float64, slippage int32) (*OrderResult, error)

	Platform() bus.PlatformType
	BrokerName() string
	SetOnReconnect(fn func(ctx context.Context) error)
}

// OrderOperation specifies buy or sell.
type OrderOperation int32

const (
	OpBuy OrderOperation = iota
	OpSell
)

// OrderRequest is the unified order submission request.
type OrderRequest struct {
	ClientID    string
	Symbol      string
	Operation   OrderOperation
	Volume      decimal.Decimal
	Price       float64
	Slippage    int32
	StopLoss    float64
	TakeProfit  float64
}

// OrderState classifies the result state of an order.
type OrderState int32

const (
	StateFilled OrderState = iota
	StatePartial
	StateRejected
	StateUnknown
)

// OrderResult is the unified order result.
type OrderResult struct {
	ClientID    string
	Ticket      int64
	Symbol      string
	Operation   OrderOperation
	State       OrderState
	Volume      decimal.Decimal
	CloseVolume decimal.Decimal
	Error       error
}

// IsFullFill returns true if the order was fully filled.
func (r *OrderResult) IsFullFill() bool {
	return r.State == StateFilled && r.CloseVolume.Equal(r.Volume)
}

// Order represents an open or historical order.
type Order struct {
	Ticket     int64
	Symbol     string
	Type       OrderOperation
	Lots       decimal.Decimal
	State      OrderState
	OpenPrice  float64
	ClosePrice float64
	StopLoss   float64
	TakeProfit float64
	Profit     float64
	Swap       float64
	Commission float64
	Comment    string
	OpenTime   time.Time
	CloseTime  time.Time
}

// Account holds account summary data.
type Account struct {
	Balance     decimal.Decimal
	Equity      decimal.Decimal
	Margin      decimal.Decimal
	FreeMargin  decimal.Decimal
	Currency    string
	Credit      float64
	Profit      float64
	MarginLevel float64
	Leverage    int32
	Company     string
	Platform    string
	Login       int64
}
