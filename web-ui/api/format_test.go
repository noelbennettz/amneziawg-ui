package api

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := map[float64]string{
		0:                "0 B",
		512:              "512 B",
		1024:             "1.00 KB",
		14060:            "13.73 KB",
		1245540515:       "1.16 GB",
		-5:               "0 B",
		1 << 50 * 3000.0: "3000.00 PB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%v) = %q, want %q", in, got, want)
		}
	}
}
