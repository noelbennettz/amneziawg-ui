package dashboard

import (
	"fmt"

	"amneziawg-web-ui/web-ui/api"
)

// formatBytes is the shared api.FormatBytes: the tiles round the same way
// as the cards and rows below them.
func formatBytes(v float64) string {
	return api.FormatBytes(v)
}

// formatRate is formatBytes per second.
func formatRate(v float64) string {
	return formatBytes(v) + "/s"
}

// formatPercent renders a ratio in [0, 1] as "12.4%".
func formatPercent(ratio float64) string {
	return fmt.Sprintf("%.1f%%", 100*ratio)
}

// formatUsage renders "3.10 / 7.60 GB": both figures in the unit of the total,
// so the pair reads as one fraction.
func formatUsage(used, total float64) string {
	i := 0
	for total >= 1024 && i < len(api.ByteUnits)-1 {
		total /= 1024
		used /= 1024
		i++
	}
	return fmt.Sprintf("%.2f / %.2f %s", used, total, api.ByteUnits[i])
}
