package errclass

// Action classifies how the caller should respond to an MT4/MT5 error.
type Action int32

const (
	Retry      Action = iota // retry same request after backoff
	RetryFresh               // retry with fresh prices (requote/price changed)
	Abort                    // do not retry; log and discard
	Halt                     // stop strategy; market closed or trade disabled
)

// ClassifyMT5 maps an MT5 error code string to an Action.
// mt5grpc.ErrorCode values are strings in the proto definition.
func ClassifyMT5(code string) Action {
	switch code {
	case "REQUOTE", "PRICE_CHANGED":
		return RetryFresh
	case "OFF_QUOTES", "NO_PRICES", "TOO_MANY_TRADE_REQUESTS":
		return Retry
	case "MARKET_CLOSED", "TRADE_DISABLED":
		return Halt
	case "NO_MONEY", "INVALID_VOLUME", "INVALID_PRICE", "INVALID_STOPS",
		"INVALID_TRADE_VOLUME", "INVALID_TRADE_REQUEST":
		return Abort
	default:
		return Abort
	}
}

// ClassifyMT4 maps an MT4 error code string to an Action.
func ClassifyMT4(code string) Action {
	switch code {
	case "ERR_REQUOTE", "ERR_PRICE_CHANGED":
		return RetryFresh
	case "ERR_OFF_QUOTES", "ERR_NO_PRICES", "ERR_TOO_MANY_REQUESTS":
		return Retry
	case "ERR_MARKET_CLOSED", "ERR_TRADE_DISABLED":
		return Halt
	case "ERR_NO_MONEY", "ERR_INVALID_TRADE_VOLUME", "ERR_INVALID_PRICE",
		"ERR_INVALID_STOPS", "ERR_INVALID_TRADE_PARAMETERS":
		return Abort
	default:
		return Abort
	}
}
