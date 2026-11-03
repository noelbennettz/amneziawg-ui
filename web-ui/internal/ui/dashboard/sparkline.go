package dashboard

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/style"
)

// sparkline is a filled line chart of the last capacity samples, newest at the
// right edge, drawn in one or more series (the hero chart carries upload and
// download together).
//
// It draws itself into one canvas.Raster rather than out of hundreds of
// canvas.Line and canvas.Rectangle objects: the wasm driver uploads a raster as
// a single texture, whereas every separate object is its own draw call on every
// frame.
//
// The window is fixed: a chart with fewer samples than its capacity leaves the
// left part empty instead of stretching what it has, so the horizontal scale
// means the same five minutes from the first sample on.
type sparkline struct {
	widget.BaseWidget

	bg       color.NRGBA
	ceiling  float64
	capacity int
	height   float32
	series   []*sparkSeries
	raster   *canvas.Raster
}

type sparkSeries struct {
	color  color.NRGBA
	values []float64
}

// newSparkline builds an empty chart on the given background (the colour of
// the card it sits on, so it looks drawn into the card rather than pasted over
// it). ceiling pins the top of the chart; zero means scale to the largest
// sample in view.
func newSparkline(bg color.NRGBA, capacity int, ceiling float64, height float32) *sparkline {
	s := &sparkline{bg: bg, capacity: max(capacity, 2), ceiling: ceiling, height: height}
	s.ExtendBaseWidget(s)
	s.raster = canvas.NewRaster(s.draw)
	return s
}

// AddSeries appends a line to the chart and returns its index, which is what
// Push takes. A chart with no series draws only its background.
func (s *sparkline) AddSeries(c color.NRGBA) int {
	s.series = append(s.series, &sparkSeries{color: c})
	return len(s.series) - 1
}

// Push appends a sample to one series, dropping the oldest once the window is
// full.
func (s *sparkline) Push(series int, v float64) {
	if series < 0 || series >= len(s.series) {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		v = 0
	}

	line := s.series[series]
	line.values = append(line.values, v)
	if len(line.values) > s.capacity {
		line.values = line.values[len(line.values)-s.capacity:]
	}
	s.raster.Refresh()
}

func (s *sparkline) CreateRenderer() fyne.WidgetRenderer {
	return &sparklineRenderer{s: s}
}

// draw rasterises the chart at the pixel size the driver asks for. Each pixel
// column between two samples is interpolated, filled down to the baseline in a
// translucent shade of the line colour, and capped with a two-pixel stroke.
func (s *sparkline) draw(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	fill(img, s.bg)
	if w < 2 || h < 2 {
		return img
	}

	top := s.ceiling
	if top <= 0 {
		for _, line := range s.series {
			for _, v := range line.values {
				top = math.Max(top, v)
			}
		}
		// Headroom so the peak does not touch the frame; a flat zero
		// series sits on the baseline rather than dividing by zero.
		top *= 1.15
		if top == 0 {
			top = 1
		}
	}

	// Vertical margins keep the stroke inside the raster at both extremes.
	margin := math.Min(3, float64(h)/8)
	usable := float64(h-1) - 2*margin
	yOf := func(v float64) float64 {
		return margin + usable*(1-math.Min(v/top, 1))
	}

	// Samples are spaced so that a full window spans the whole width; a
	// partial one occupies the right-hand end.
	step := float64(w-1) / float64(s.capacity-1)

	for _, line := range s.series {
		n := len(line.values)
		if n == 0 {
			continue
		}
		x0 := float64(w-1) - step*float64(n-1)
		area := style.WithAlpha(line.color, 0x33)

		for i := 0; i < n; i++ {
			xa := x0 + step*float64(i)
			ya := yOf(line.values[i])
			xb, yb := xa, ya
			if i+1 < n {
				xb = x0 + step*float64(i+1)
				yb = yOf(line.values[i+1])
			}
			for x := int(math.Round(xa)); x <= int(math.Round(xb)) && x < w; x++ {
				t := 0.0
				if xb > xa {
					t = (float64(x) - xa) / (xb - xa)
				}
				y := ya + (yb-ya)*t
				yi := int(math.Round(y))
				for py := yi; py < h; py++ {
					img.SetNRGBA(x, py, blend(img.NRGBAAt(x, py), area))
				}
				img.SetNRGBA(x, yi, line.color)
				if yi+1 < h {
					img.SetNRGBA(x, yi+1, line.color)
				}
			}
		}
	}
	return img
}

// blend puts src over dst, so two translucent series do not erase each other
// where they cross.
func blend(dst, src color.NRGBA) color.NRGBA {
	a := float64(src.A) / 255
	return color.NRGBA{
		R: uint8(float64(src.R)*a + float64(dst.R)*(1-a)),
		G: uint8(float64(src.G)*a + float64(dst.G)*(1-a)),
		B: uint8(float64(src.B)*a + float64(dst.B)*(1-a)),
		A: 0xff,
	}
}

func fill(img *image.NRGBA, c color.NRGBA) {
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

type sparklineRenderer struct {
	s *sparkline
}

func (r *sparklineRenderer) Layout(size fyne.Size) { r.s.raster.Resize(size) }
func (r *sparklineRenderer) MinSize() fyne.Size {
	return fyne.NewSize(80, r.s.height)
}
func (r *sparklineRenderer) Refresh() { r.s.raster.Refresh() }
func (r *sparklineRenderer) Destroy() {}

func (r *sparklineRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.s.raster}
}
