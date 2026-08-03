package decimalutil

import (
	"strconv"

	"github.com/shopspring/decimal"
)

// FromFloat64 converts a float64 to decimal.Decimal via string formatting
// to avoid the precision loss of decimal.NewFromFloat. digits specifies
// the number of decimal places to preserve.
func FromFloat64(f float64, digits int32) decimal.Decimal {
	s := strconv.FormatFloat(f, 'f', int(digits), 64)
	d, _ := decimal.NewFromString(s)
	return d
}

// ToFloat64 converts a decimal.Decimal back to float64 via string parsing.
func ToFloat64(d decimal.Decimal) float64 {
	f, _ := strconv.ParseFloat(d.String(), 64)
	return f
}

// FromString parses a decimal from a string.
func FromString(s string) (decimal.Decimal, error) {
	return decimal.NewFromString(s)
}
