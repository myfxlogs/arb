package errclass

import "testing"

func TestClassifyMT5(t *testing.T) {
	cases := []struct {
		code string
		want Action
	}{
		{"REQUOTE", RetryFresh},
		{"PRICE_CHANGED", RetryFresh},
		{"OFF_QUOTES", Retry},
		{"NO_PRICES", Retry},
		{"TOO_MANY_TRADE_REQUESTS", Retry},
		{"MARKET_CLOSED", Halt},
		{"TRADE_DISABLED", Halt},
		{"NO_MONEY", Abort},
		{"INVALID_VOLUME", Abort},
		{"INVALID_PRICE", Abort},
		{"INVALID_STOPS", Abort},
		{"UNKNOWN_ERROR", Abort},
	}
	for _, c := range cases {
		got := ClassifyMT5(c.code)
		if got != c.want {
			t.Errorf("ClassifyMT5(%q) = %d, want %d", c.code, got, c.want)
		}
	}
}

func TestClassifyMT4(t *testing.T) {
	cases := []struct {
		code string
		want Action
	}{
		{"ERR_REQUOTE", RetryFresh},
		{"ERR_PRICE_CHANGED", RetryFresh},
		{"ERR_OFF_QUOTES", Retry},
		{"ERR_NO_PRICES", Retry},
		{"ERR_TOO_MANY_REQUESTS", Retry},
		{"ERR_MARKET_CLOSED", Halt},
		{"ERR_TRADE_DISABLED", Halt},
		{"ERR_NO_MONEY", Abort},
		{"ERR_INVALID_TRADE_VOLUME", Abort},
		{"ERR_INVALID_PRICE", Abort},
		{"ERR_INVALID_STOPS", Abort},
		{"ERR_INVALID_TRADE_PARAMETERS", Abort},
		{"ERR_UNKNOWN", Abort},
	}
	for _, c := range cases {
		got := ClassifyMT4(c.code)
		if got != c.want {
			t.Errorf("ClassifyMT4(%q) = %d, want %d", c.code, got, c.want)
		}
	}
}
