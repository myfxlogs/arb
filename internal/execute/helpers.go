package execute

import (
	"arb/internal/decimalutil"

	"github.com/shopspring/decimal"
)

// decimalFromFloat converts a float64 to decimal via decimalutil
// to avoid decimal.NewFromFloat precision issues.
func decimalFromFloat(f float64) decimal.Decimal {
	return decimalutil.FromFloat64(f, 8)
}
