package decimalutil

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestFromFloat64Precision(t *testing.T) {
	got := FromFloat64(1.05000, 5)
	want, _ := decimal.NewFromString("1.05000")
	if !got.Equal(want) {
		t.Errorf("FromFloat64(1.05000, 5) = %s, want %s", got.String(), want.String())
	}
}

func TestFromFloat64RoundTrip(t *testing.T) {
	d := FromFloat64(1.05000, 5)
	f := ToFloat64(d)
	if f != 1.05 {
		t.Errorf("round-trip ToFloat64 = %v, want 1.05", f)
	}
}

func TestFromFloat64Boundaries(t *testing.T) {
	cases := []struct {
		f      float64
		digits int32
		want   string
	}{
		{0, 0, "0"},
		{-1.5, 1, "-1.5"},
		{1e15, 0, "1000000000000000"},
		{1e-10, 10, "0.0000000001"},
	}
	for _, c := range cases {
		got := FromFloat64(c.f, c.digits)
		want, _ := decimal.NewFromString(c.want)
		if !got.Equal(want) {
			t.Errorf("FromFloat64(%v, %d) = %s, want %s", c.f, c.digits, got.String(), c.want)
		}
	}
}

func TestFromFloat64NotEqualNewFromFloat(t *testing.T) {
	// 1.2345678901234567 has 17 significant digits.
	// decimal.NewFromFloat preserves full precision: 1.2345678901234567
	// FromFloat64 with digits=5 truncates to 1.23457 — must NOT be equal.
	f := 1.2345678901234567
	got := FromFloat64(f, 5)
	bad := decimal.NewFromFloat(f)
	if got.Equal(bad) {
		t.Errorf("FromFloat64(%.20f, 5) == decimal.NewFromFloat; got %s, bad %s; must not be equal",
			f, got.String(), bad.String())
	}
}
