package bus

import "time"

// PlatformType identifies the trading platform.
type PlatformType int32

const (
	PlatformMT4 PlatformType = iota
	PlatformMT5
	PlatformBinance
)

// Quote is the unified market data structure passed through the QuoteBus.
// Hot Path uses float64 Bid/Ask directly — no decimal allocation.
type Quote struct {
	Symbol   string
	Bid      float64
	Ask      float64
	Time     time.Time
	Broker   string
	Platform PlatformType
}
