// Package dashboard is the monitoring block at the top of the page: one accent
// card with the VPN throughput and both directions on one chart, and four small
// cards - the host's CPU, memory and disk, and the backend's uptime.
//
// The backend sends one instantaneous reading per poll and remembers nothing;
// the history the charts draw lives here, in the page, and starts over with
// every reload.
package dashboard

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// Window is how far back every chart reaches.
const Window = 5 * time.Minute

// Panel is the dashboard. Open by default: it is what the operator looks at
// first, and it folds away when the server list is what matters.
type Panel struct {
	// reflow tells the page its content height changed, so it can re-clamp
	// the scroll offset; see toggleOpen.
	reflow func()

	hero                         *hero
	cpu, memory, storage, uptime *tile

	// The rates are differences between consecutive polls, so the previous
	// reading has to be kept: the byte totals and when they arrived, and
	// the CPU seconds.
	last struct {
		at         time.Time
		rx, tx     uint64
		busy, idle float64
		ok         bool
	}

	toggle *widgets.Button
	body   *fyne.Container
	panel  *fyne.Container

	// narrow is the compact layout: the gauges go under the traffic card
	// instead of beside it; see SetNarrow.
	narrow bool
}

// New builds the panel for a page that polls every interval; the charts hold
// Window worth of samples at that rate. reflow is called whenever the panel
// changes height.
func New(interval time.Duration, reflow func()) *Panel {
	points := int(Window / interval)
	p := &Panel{reflow: reflow}

	p.hero = newHero(points)
	p.hero.title.Text = lang.L("VPN traffic, last 5 minutes")
	p.hero.title.Refresh()

	p.cpu = newTile(lang.L("CPU"), lang.L("Cores"), style.Primary, points, 1, true)
	p.memory = newTile(lang.L("RAM"), lang.L("Used"), style.Accent, points, 1, true)
	p.storage = newTile(lang.L("Storage"), lang.L("Used"), style.Warning, points, 1, true)
	p.uptime = newTile(lang.L("Uptime"), lang.L("backend"), style.Muted, points, 1, false)

	right := container.NewGridWithColumns(2,
		p.cpu.object, p.memory.object, p.storage.object, p.uptime.object)

	row := container.New(&weightedRow{weights: []float32{3, 2}, gap: style.Space4}, p.hero.object, right)

	// The same hand-rolled disclosure as the create form, for the same reason:
	// collapsing has to tell the page to re-clamp its scroll offset.
	p.body = row

	p.toggle = widgets.IconButton(theme.MenuDropUpIcon(), p.toggleOpen)
	testhook.Register("monitoring.toggle", p.toggle)
	head := container.NewBorder(nil, nil,
		container.NewCenter(widgets.SectionTitle(lang.L("Monitoring"))),
		container.NewCenter(p.toggle),
	)

	// The header is a block of its own, the same surface and the same corner
	// radius as the create form and the server cards under it. Floating the
	// title on the page background made the dashboard read as a heading with
	// cards under it, rather than as the first of the page's blocks.
	p.panel = container.NewVBox(widgets.Card(head), p.body)
	return p
}

// CanvasObject is the panel as the page places it.
func (p *Panel) CanvasObject() fyne.CanvasObject { return p.panel }

// SetNarrow stacks the four gauges under the traffic card, which is what the
// compact page uses. The card itself keeps its shape either way: it is the row
// of two blocks that needs the room, not the card.
func (p *Panel) SetNarrow(narrow bool) {
	if p.narrow == narrow {
		return
	}
	p.narrow = narrow

	if narrow {
		p.body.Layout = &widgets.StackLayout{Gap: style.Space4}
	} else {
		p.body.Layout = &weightedRow{weights: []float32{3, 2}, gap: style.Space4}
	}
	p.body.Refresh()
}

func (p *Panel) toggleOpen() {
	if p.body.Visible() {
		p.body.Hide()
		p.toggle.SetIcon(theme.MenuDropDownIcon())
	} else {
		p.body.Show()
		p.toggle.SetIcon(theme.MenuDropUpIcon())
	}
	p.reflow()
}

// SetUptime fills the uptime card from the backend's own status call.
func (p *Panel) SetUptime(seconds float64) {
	p.uptime.set(0, widgets.Uptime(seconds), "")
}

// Apply takes one poll: sums the interface counters, differences them and
// the CPU jiffies against the previous poll, and pushes a sample onto every
// chart. The first poll only seeds the rates, so the throughput figures show a
// dash until the second one; the gauges are drawn at once. Must run on the UI
// goroutine.
func (p *Panel) Apply(snap api.TrafficSnapshot) {
	p.applyAt(snap, time.Now())
}

func (p *Panel) applyAt(snap api.TrafficSnapshot, now time.Time) {
	var rx, tx uint64
	for _, t := range snap.ServerTraffic {
		rx += t.RXBytes
		tx += t.TXBytes
	}
	sys := snap.System
	busy, idle := sys.CPU.BusySeconds, sys.CPU.TotalSeconds-sys.CPU.BusySeconds

	// A poll on the heels of the previous one - the page reloads the counters
	// after every action as well as on its timer - has nothing to say about a
	// rate: a few milliseconds of CPU time is noise, not a load. The gauges
	// are still current; the rates wait for the next interval.
	if p.last.ok && now.Sub(p.last.at) < time.Second {
		setUsage(p.memory, sys.Memory)
		setUsage(p.storage, sys.Storage)
		return
	}

	if p.last.ok {
		seconds := now.Sub(p.last.at).Seconds()
		if seconds > 0 {
			inRate := rate(rx, p.last.rx, seconds)
			outRate := rate(tx, p.last.tx, seconds)
			p.hero.set(inRate, outRate, true, formatBytes(float64(rx)), formatBytes(float64(tx)))
		}
		if load, ok := cpuLoad(busy, idle, p.last.busy, p.last.idle); ok {
			p.cpu.set(load, formatPercent(load), lang.N("{{.Count}} cores", sys.CPU.Cores, map[string]any{"Count": sys.CPU.Cores}))
		}
	} else {
		p.hero.set(0, 0, false, formatBytes(float64(rx)), formatBytes(float64(tx)))
		p.cpu.set(0, "", lang.N("{{.Count}} cores", sys.CPU.Cores, map[string]any{"Count": sys.CPU.Cores}))
	}

	setUsage(p.memory, sys.Memory)
	setUsage(p.storage, sys.Storage)

	p.last.at, p.last.rx, p.last.tx = now, rx, tx
	p.last.busy, p.last.idle = busy, idle
	p.last.ok = true
}

// rate is the bytes per second between two readings of a counter. A counter
// that went backwards - a server was stopped and its interface, with its
// total, disappeared from the sum - reads as zero rather than negative.
func rate(now, before uint64, seconds float64) float64 {
	if now < before {
		return 0
	}
	return float64(now-before) / seconds
}

// cpuLoad is the share of CPU time spent working between two readings;
// false when no time has passed or the counters went backwards.
func cpuLoad(busy, idle, prevBusy, prevIdle float64) (float64, bool) {
	if busy < prevBusy || idle < prevIdle {
		return 0, false
	}
	total := (busy - prevBusy) + (idle - prevIdle)
	if total <= 0 {
		return 0, false
	}
	return (busy - prevBusy) / total, true
}

// setUsage fills a gauge tile from a capacity reading, dash when there is
// none.
func setUsage(t *tile, u api.UsageMetrics) {
	if u.Total == 0 {
		t.set(0, "", "—")
		return
	}
	ratio := float64(u.Used) / float64(u.Total)
	t.set(ratio, formatPercent(ratio), formatUsage(float64(u.Used), float64(u.Total)))
}

// weightedRow lays two children out side by side, each taking the share its
// weight asks for. Fyne has no fr unit, and container.NewBorder would make the
// right column a fixed width rather than the two fifths the design asks for.
type weightedRow struct {
	weights []float32
	gap     float32
}

func (l *weightedRow) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	total := float32(0)
	for _, w := range l.weights {
		total += w
	}
	if total == 0 {
		return
	}

	available := size.Width - l.gap*float32(len(objects)-1)
	x := float32(0)
	for i, o := range objects {
		weight := l.weights[min(i, len(l.weights)-1)]
		width := available * weight / total
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(width, size.Height))
		x += width + l.gap
	}
}

func (l *weightedRow) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var size fyne.Size
	for i, o := range objects {
		min := o.MinSize()
		size.Height = max(size.Height, min.Height)
		size.Width += min.Width
		if i > 0 {
			size.Width += l.gap
		}
	}
	return size
}
