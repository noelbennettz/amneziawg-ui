package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/style"
)

// menuRow is one line of an overflow menu: the label, its glyph, a hovered
// rounded highlight and a tap that runs the action.
type menuRow struct {
	widget.BaseWidget

	item     MenuItem
	onTapped func()
	hovered  bool
	pressed  bool
}

func newMenuRow(item MenuItem, onTapped func()) *menuRow {
	row := &menuRow{item: item, onTapped: onTapped}
	row.ExtendBaseWidget(row)
	return row
}

// The two interfaces the row is meant to be driven by, asserted so that a
// parameter type drifting from *desktop.MouseEvent to something else - which is
// exactly how this row spent its life never being hovered - stops the build
// instead of silently switching the highlight off.
var (
	_ desktop.Hoverable = (*menuRow)(nil)
	_ desktop.Mouseable = (*menuRow)(nil)
)

func (r *menuRow) Tapped(*fyne.PointEvent) { r.onTapped() }

// MouseDown and MouseUp give the row the state a tap needs to be visible at
// all: the highlight above is hover, and a finger never hovers. On a phone the
// items of an overflow menu were tapped with nothing on screen saying the tap
// had landed - the menu just vanished under the dialog that opened behind it.
// A browser does synthesise the mouse events behind a tap, so this is the whole
// fix.
func (r *menuRow) MouseDown(*desktop.MouseEvent) {
	r.pressed = true
	r.Refresh()
}

func (r *menuRow) MouseUp(*desktop.MouseEvent) {
	r.pressed = false
	r.Refresh()
}

func (r *menuRow) Cursor() desktop.Cursor { return desktop.PointerCursor }

// The three hover handlers take a *desktop.MouseEvent, not the *fyne.PointEvent
// of Tapped. It has to be that type and no other: desktop.Hoverable is defined
// over it, and a row taking the wrong one is a row the driver never treats as
// hoverable - which is exactly what this was, so the highlight below was dead
// code and an item gave no sign at all that the pointer was on it.
func (r *menuRow) MouseIn(*desktop.MouseEvent) {
	r.hovered = true
	r.Refresh()
}

func (r *menuRow) MouseMoved(*desktop.MouseEvent) {}

func (r *menuRow) MouseOut() {
	r.hovered = false
	r.pressed = false
	r.Refresh()
}

func (r *menuRow) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(nil)
	bg.CornerRadius = style.RadiusSM

	label := canvas.NewText(r.item.Label, style.Text)
	label.TextSize = style.SizeBody

	left := container.NewHBox()
	if r.item.Icon != nil {
		left.Add(container.NewCenter(Icon(r.item.Icon, 18)))
	}
	left.Add(container.NewCenter(label))

	content := container.NewStack(bg, container.New(&insetLayout{h: style.Space3, v: style.Space2}, left))

	rr := &menuRowRenderer{r: r, bg: bg, objects: []fyne.CanvasObject{content}}
	rr.Refresh()
	return rr
}

type menuRowRenderer struct {
	r       *menuRow
	bg      *canvas.Rectangle
	objects []fyne.CanvasObject
}

func (m *menuRowRenderer) Layout(size fyne.Size) {
	for _, o := range m.objects {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (m *menuRowRenderer) MinSize() fyne.Size {
	min := fyne.NewSize(140, 40)
	for _, o := range m.objects {
		min = min.Max(o.MinSize())
	}
	return min
}

func (m *menuRowRenderer) Refresh() {
	// A press is the accent laid over the surface, a hover one neutral step up:
	// the menu is dark enough that a neutral pressed state would read as a
	// change of background rather than as a reaction to the finger.
	switch {
	case m.r.pressed:
		m.bg.FillColor = style.WithAlpha(style.Primary, 0x33)
	case m.r.hovered:
		m.bg.FillColor = style.SurfaceHigh
	default:
		m.bg.FillColor = nil
	}
	m.bg.Refresh()
}

func (m *menuRowRenderer) Destroy()                     {}
func (m *menuRowRenderer) Objects() []fyne.CanvasObject { return m.objects }

// showMenu lays the entries out in the page's own menu surface and opens it
// under the button that asked for it, flipped up and left when the window edge
// is closer than the menu is.
func showMenuAt(from fyne.CanvasObject, items []MenuItem, canvasFor fyne.Canvas) {
	rows := container.NewVBox()
	popup := &widget.PopUp{}

	for _, item := range items {
		item := item
		rows.Add(newMenuRow(item, func() {
			popup.Hide()
			if item.Do != nil {
				item.Do()
			}
		}))
	}

	card := surface(rows, style.SurfaceAlt, style.RadiusMD)
	popup = widget.NewPopUp(card, canvasFor)

	size := card.MinSize()
	size.Width += style.Space1
	popup.Resize(size)

	anchor := fyne.CurrentApp().Driver().AbsolutePositionForObject(from)
	pos := anchor.Add(fyne.NewPos(0, from.Size().Height+style.Space1))

	canvasSize := canvasFor.Size()
	if pos.X+size.Width > canvasSize.Width {
		pos.X = canvasSize.Width - size.Width - style.Space2
	}
	if pos.Y+size.Height > canvasSize.Height {
		pos.Y = anchor.Y - size.Height - style.Space1
	}
	if pos.X < 0 {
		pos.X = 0
	}
	if pos.Y < 0 {
		pos.Y = 0
	}

	popup.ShowAtPosition(pos)
}
