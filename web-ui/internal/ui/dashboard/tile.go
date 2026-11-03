package dashboard

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// caption is the small muted line a tile opens with, set the way M3 sets one:
// sentence case, label size, no shouting.
func caption(text string, c color.Color, size float32) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextSize = size
	return t
}

// hero is the accent card at the top of the dashboard: the headline
// throughput, the two directions under it, and both lines on one chart.
type hero struct {
	title  *canvas.Text
	value  *canvas.Text
	in     *canvas.Text
	out    *canvas.Text
	total  *canvas.Text
	chart  *sparkline
	inIdx  int
	outIdx int

	object fyne.CanvasObject
}

func newHero(capacity int) *hero {
	subtle := style.WithAlpha(style.OnPrimaryContainer, 0xd0)
	h := &hero{
		title: caption("", style.OnPrimaryContainer, style.SizeLabel),
		value: caption("—", style.OnPrimaryContainer, 38),
		in:    caption("", subtle, style.SizeLabel),
		out:   caption("", subtle, style.SizeLabel),
		total: caption("", subtle, style.SizeLabel),
	}
	h.chart = newSparkline(style.PrimaryContainer, capacity, 0, 92)
	h.inIdx = h.chart.AddSeries(style.OnPrimaryContainer)
	h.outIdx = h.chart.AddSeries(style.WithAlpha(style.OnPrimaryContainer, 0x8c))

	valueRow := container.NewHBox(container.NewCenter(h.value))

	dot := caption("·", subtle, style.SizeLabel)
	stats := container.NewHBox(
		container.NewCenter(h.in), dot,
		container.NewCenter(h.out), caption("·", subtle, style.SizeLabel),
		container.NewCenter(h.total),
	)

	// The figures ride at the top and the chart takes the rest of the card, so
	// the accent card is filled whatever height the row of gauges next to it
	// ends up having.
	h.object = widgets.HeroCard(container.NewBorder(
		container.NewVBox(h.title, valueRow, stats), nil, nil, nil, h.chart,
	))
	return h
}

// set writes the figures and pushes one sample per direction. haveRates is
// false on the first poll, which only seeds the difference the next one takes:
// the headline then shows a dash and the chart stays empty.
func (h *hero) set(inRate, outRate float64, haveRates bool, inTotal, outTotal string) {
	if haveRates {
		h.value.Text = formatRate(inRate + outRate)
		h.in.Text = "in " + formatRate(inRate)
		h.out.Text = "out " + formatRate(outRate)
		h.chart.Push(h.inIdx, inRate)
		h.chart.Push(h.outIdx, outRate)
	} else {
		h.value.Text = "—"
		h.in.Text = "in —"
		h.out.Text = "out —"
	}
	h.total.Text = "total " + inTotal + " / " + outTotal

	for _, t := range []*canvas.Text{h.value, h.in, h.out, h.total} {
		t.Refresh()
	}
}

// tile is one of the four small cards under the hero: a caption, the current
// reading, its history and the figure that does not move much.
type tile struct {
	caption *canvas.Text
	value   *canvas.Text
	foot    *canvas.Text
	chart   *sparkline
	object  fyne.CanvasObject
}

// newTile builds a small card. capacity is how many samples the chart keeps
// and ceiling pins its top (zero to autoscale); see sparkline. A nil chart
// gives a text-only tile, which is what the uptime reading is.
func newTile(label, footCaption string, tint color.NRGBA, capacity int, ceiling float64, withChart bool) *tile {
	t := &tile{
		caption: caption(label, style.Muted, style.SizeLabel),
		value:   caption("—", style.Text, 26),
		foot:    caption(footCaption, style.Muted, style.SizeCaption),
	}

	rows := []fyne.CanvasObject{t.caption, t.value, t.foot}
	if withChart {
		t.chart = newSparkline(style.Surface, capacity, ceiling, 44)
		t.chart.AddSeries(tint)
		rows = []fyne.CanvasObject{t.caption, t.value, t.foot, t.chart}
	}
	t.object = widgets.Card(container.NewVBox(rows...))
	return t
}

// set pushes a sample onto the chart and shows both figures. An empty value
// leaves the chart alone and shows a dash, for a reading that needs a second
// sample before it means anything. Must run on the UI goroutine.
func (t *tile) set(sample float64, value, foot string) {
	if value == "" {
		value = "—"
	} else if t.chart != nil {
		t.chart.Push(0, sample)
	}
	t.value.Text = value
	t.value.Refresh()
	if foot != "" {
		t.foot.Text = foot
		t.foot.Refresh()
	}
}
