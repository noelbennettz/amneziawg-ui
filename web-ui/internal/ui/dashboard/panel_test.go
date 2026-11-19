package dashboard

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
)

func snapshot(rx, tx uint64, busy, total float64) api.TrafficSnapshot {
	return api.TrafficSnapshot{
		ServerTraffic: map[string]api.InterfaceTraffic{
			"a": {RXBytes: rx / 2, TXBytes: tx / 2},
			"b": {RXBytes: rx - rx/2, TXBytes: tx - tx/2},
		},
		System: api.SystemMetrics{
			CPU:     api.CPUMetrics{Cores: 4, BusySeconds: busy, TotalSeconds: total},
			Memory:  api.UsageMetrics{Total: 8 << 30, Used: 2 << 30},
			Storage: api.UsageMetrics{Total: 100 << 30, Used: 25 << 30},
		},
	}
}

// The first poll can only seed the rates; the gauges are drawn from it at
// once. The second poll turns the differences into a speed and a load.
func TestRatesNeedTwoPolls(t *testing.T) {
	p := New(5*time.Second, func() {})
	start := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	p.applyAt(snapshot(1000, 2000, 10, 100), start)
	if p.hero.value.Text != "—" || p.hero.in.Text != "in —" || p.cpu.value.Text != "—" {
		t.Errorf("rates after one poll = %q / %q / %q, want dashes", p.hero.value.Text, p.hero.in.Text, p.cpu.value.Text)
	}
	if !strings.Contains(p.hero.total.Text, "1.95 KB") {
		t.Errorf("totals after one poll = %q, want the cumulative bytes", p.hero.total.Text)
	}
	if p.memory.value.Text != "25.0%" || p.storage.foot.Text != "25.00 / 100.00 GB" {
		t.Errorf("gauges after one poll: memory %q, storage %q", p.memory.value.Text, p.storage.foot.Text)
	}

	// 5 s later: 10240 more bytes out, 5120 in; 2 of 8 CPU seconds busy.
	p.applyAt(snapshot(1000+5120, 2000+10240, 12, 108), start.Add(5*time.Second))
	if p.hero.value.Text != "3.00 KB/s" || p.hero.in.Text != "in 1.00 KB/s" || p.hero.out.Text != "out 2.00 KB/s" {
		t.Errorf("rates = %q (%q, %q)", p.hero.value.Text, p.hero.in.Text, p.hero.out.Text)
	}
	if p.cpu.value.Text != "25.0%" || p.cpu.foot.Text != "4 cores" {
		t.Errorf("cpu = %q, %q", p.cpu.value.Text, p.cpu.foot.Text)
	}
	if got := len(p.hero.chart.series[p.hero.inIdx].values); got != 1 {
		t.Errorf("chart has %d samples after two polls, want 1", got)
	}
}

// The page reloads the counters after every action as well as on its timer,
// so two polls can land milliseconds apart. The second one must not turn
// that sliver into a rate or a chart sample.
func TestPollsTooCloseTogetherOnlyUpdateGauges(t *testing.T) {
	p := New(5*time.Second, func() {})
	start := time.Now()
	p.applyAt(snapshot(1000, 2000, 10, 100), start)
	p.applyAt(snapshot(1000+5120, 2000+10240, 10.01, 100.01), start.Add(20*time.Millisecond))
	if p.hero.value.Text != "—" || p.cpu.value.Text != "—" || len(p.hero.chart.series[0].values) != 0 {
		t.Errorf("a 20ms poll produced %q / %q and %d samples",
			p.hero.value.Text, p.cpu.value.Text, len(p.hero.chart.series[0].values))
	}
	// The interval after that is measured from the first poll, not the burst.
	p.applyAt(snapshot(1000+5120, 2000+10240, 12, 108), start.Add(5*time.Second))
	if p.hero.value.Text != "3.00 KB/s" || p.cpu.value.Text != "25.0%" {
		t.Errorf("rates = %q / %q", p.hero.value.Text, p.cpu.value.Text)
	}
}

// A server that stops takes its interface counters out of the sum, so the
// total goes backwards; that is not negative traffic.
func TestCountersGoingBackwardsReadAsZero(t *testing.T) {
	p := New(5*time.Second, func() {})
	start := time.Now()
	p.applyAt(snapshot(5000, 9000, 10, 100), start)
	p.applyAt(snapshot(1000, 2000, 9, 99), start.Add(5*time.Second))
	if p.hero.value.Text != "0 B/s" || p.hero.in.Text != "in 0 B/s" || p.hero.out.Text != "out 0 B/s" {
		t.Errorf("rates = %q (%q, %q), want zero", p.hero.value.Text, p.hero.in.Text, p.hero.out.Text)
	}
	if p.cpu.value.Text != "—" {
		t.Errorf("cpu = %q, want a dash for counters that went backwards", p.cpu.value.Text)
	}
}

// The chart keeps exactly the window: Window / interval samples, oldest out
// first, per series.
func TestChartKeepsTheWindow(t *testing.T) {
	s := newSparkline(styleColor(), 3, 0, 40)
	in := s.AddSeries(styleColor())
	out := s.AddSeries(styleColor())
	for i := 1; i <= 5; i++ {
		s.Push(in, float64(i))
		s.Push(out, float64(i*10))
	}
	if got := s.series[in].values; len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Errorf("values = %v, want [3 4 5]", got)
	}
	if got := s.series[out].values; len(got) != 3 || got[0] != 30 {
		t.Errorf("second series = %v, want [30 40 50]", got)
	}
	// It also has to rasterise without panicking at any size, full or not.
	for _, w := range []int{0, 1, 2, 50, 300} {
		s.draw(w, 40)
	}
	s.draw(300, 1)
	newSparkline(styleColor(), 60, 1, 44).draw(200, 40)
}

func styleColor() color.NRGBA { return color.NRGBA{R: 1, G: 2, B: 3, A: 255} }

// The four gauges sit beside the traffic card on a wide page and under it on
// the compact one: that is the whole of what SetNarrow changes, and the two
// columns are what makes the page taller than a phone can show.
func TestSetNarrowMovesTheGaugesUnderTheCard(t *testing.T) {
	test.NewApp()

	p := New(5*time.Second, func() {})
	win := test.NewWindow(p.CanvasObject())
	defer win.Close()
	win.Resize(fyne.NewSize(1400, 900))

	card, gauges := p.hero.object, p.body.Objects[1]

	p.panel.Resize(fyne.NewSize(1400, 900))
	if gauges.Position().X <= card.Position().X {
		t.Fatalf("wide: the gauges start at x=%.0f, the card at x=%.0f - want them beside it",
			gauges.Position().X, card.Position().X)
	}
	if gauges.Position().Y != card.Position().Y {
		t.Errorf("wide: the gauges start at y=%.0f, the card at y=%.0f - want them level",
			gauges.Position().Y, card.Position().Y)
	}

	// The compact page: one column, the gauges under the card, both as wide
	// as the page.
	p.SetNarrow(true)
	p.panel.Resize(fyne.NewSize(390, 900))
	if got, want := gauges.Position().X, card.Position().X; got != want {
		t.Errorf("narrow: the gauges start at x=%.0f, the card at x=%.0f - want one column", got, want)
	}
	if gauges.Position().Y < card.Position().Y+card.Size().Height {
		t.Errorf("narrow: the gauges start at y=%.0f, the card ends at y=%.0f - want them under it",
			gauges.Position().Y, card.Position().Y+card.Size().Height)
	}

	// And back: the wide layout is restored, not just the flag.
	p.SetNarrow(false)
	p.panel.Resize(fyne.NewSize(1400, 900))
	if gauges.Position().X <= card.Position().X {
		t.Errorf("back to wide: the gauges start at x=%.0f, the card at x=%.0f",
			gauges.Position().X, card.Position().X)
	}
}

// Folding the panel hides the body and tells the page, which is what re-clamps
// the scroll offset; the button's glyph follows the state so the next tap is
// the one the person expects.
func TestToggleFoldsTheBodyAndAsksForAReflow(t *testing.T) {
	test.NewApp()

	reflows := 0
	p := New(5*time.Second, func() { reflows++ })
	if !p.body.Visible() {
		t.Fatal("a new panel starts folded")
	}

	up := p.toggle.Icon
	p.toggleOpen()
	if p.body.Visible() {
		t.Error("toggling left the body open")
	}
	if reflows != 1 {
		t.Errorf("reflow ran %d times, want once", reflows)
	}
	if p.toggle.Icon == up {
		t.Error("the toggle kept its glyph after folding")
	}
	if !testhook.Has("monitoring.toggle") {
		t.Error("monitoring.toggle is not registered")
	}

	p.toggleOpen()
	if !p.body.Visible() {
		t.Error("toggling a second time did not unfold the body")
	}
	if reflows != 2 {
		t.Errorf("reflow ran %d times, want twice", reflows)
	}
	if p.toggle.Icon != up {
		t.Error("the toggle did not go back to its first glyph")
	}
}
