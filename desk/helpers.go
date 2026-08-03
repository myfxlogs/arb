package desk

import (
	"fmt"
	"strconv"
)

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseInt(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

func formatInt64(i int64) string {
	return fmt.Sprintf("%d", i)
}
