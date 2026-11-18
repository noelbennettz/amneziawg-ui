package api

import "fmt"

// ByteUnits are binary steps of 1024, labelled the short way. Every byte
// figure the page shows - the server cards, the client rows, the monitoring
// tiles - goes through FormatBytes, so a total never disagrees with the sum
// of the figures below it by rounding alone.
var ByteUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}

// FormatBytes renders a byte count as "1.16 GB": two decimals from KB up,
// none for plain bytes.
func FormatBytes(v float64) string {
	if v < 0 {
		v = 0
	}
	i := 0
	for v >= 1024 && i < len(ByteUnits)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", v, ByteUnits[i])
	}
	return fmt.Sprintf("%.2f %s", v, ByteUnits[i])
}
