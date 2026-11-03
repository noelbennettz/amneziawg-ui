package dashboard

import "testing"

func TestFormatRate(t *testing.T) {
	if got := formatRate(14060); got != "13.73 KB/s" {
		t.Errorf("formatRate = %q", got)
	}
}

func TestFormatUsage(t *testing.T) {
	if got := formatUsage(3.1*1024*1024*1024, 7.6*1024*1024*1024); got != "3.10 / 7.60 GB" {
		t.Errorf("formatUsage = %q", got)
	}
	if got := formatPercent(0.1237); got != "12.4%" {
		t.Errorf("formatPercent = %q", got)
	}
}
