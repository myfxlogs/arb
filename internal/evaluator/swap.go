package evaluator

import (
	"errors"

	"github.com/shopspring/decimal"

	"arb/internal/listing"
)

// ErrUncalibratedSwap is returned when the SwapType is not among the
// calibrated modes {0,1,3,4,5,6}. The caller must treat the opportunity
// as non-executable (12 §4.1: cannot guarantee cost accuracy).
var ErrUncalibratedSwap = errors.New("swap mode not calibrated")

// DailySwap computes the per-day swap charge in the leg's profit currency
// for a given direction and lot size (12 §4.1).
//
// InPoints (1): S × Points × ContractSize × Lots  [verified EURUSD/XAUUSD]
// MarginCurrency (3): S × Lots
// Currency (4): S × Lots
// PercCurPrice (5): S% × Price × ContractSize × Lots / 100 / 365
// PercOpenPrice (6): same as 5 (using open price ≈ price)
// SwapNone (0): zero
// SymInfo_s408 (2) / PointClosePrice (7) / PointBidPrice (8): ErrUncalibratedSwap
//
// For Sell, the swap value is negated (SwapShort is typically positive
// meaning you receive, but MT5 convention: SwapShort is the rate applied
// to short positions — we use it directly, then negate because selling
// swaps the sign).
func DailySwap(l *listing.Listing, direction BuySell, lots decimal.Decimal, price decimal.Decimal) (decimal.Decimal, error) {
	if l == nil {
		return decimal.Zero, errors.New("nil listing")
	}

	st := l.Swap.SwapType
	if st == listing.SwapNone {
		return decimal.Zero, nil
	}

	s := l.Swap.SwapLong
	if direction == Sell {
		s = l.Swap.SwapShort
	}

	switch st {
	case listing.SwapInPoints:
		// S × Points × ContractSize × Lots
		return s.Mul(l.Points).Mul(l.ContractSize).Mul(lots), nil

	case listing.SwapMarginCcy, listing.SwapCurrency:
		// S × Lots (profit-currency or margin-currency amount)
		return s.Mul(lots), nil

	case listing.SwapPctCurPrice, listing.SwapPctOpenPrice:
		// S% × Price × ContractSize × Lots / 100 / 365
		return s.Mul(price).Mul(l.ContractSize).Mul(lots).
			Div(decimal.NewFromInt(100)).Div(decimal.NewFromInt(365)), nil

	case listing.SwapSymInfoS408, listing.SwapPointClose, listing.SwapPointBid:
		return decimal.Zero, ErrUncalibratedSwap

	default:
		return decimal.Zero, ErrUncalibratedSwap
	}
}
