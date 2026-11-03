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
}

func newMenuRow(item MenuItem, onTapped func()) *menuRow {
	row := &menuRow{item: item, onTapped: onTapped}
	row.ExtendBaseWidget(row)
	return row
}

func (r *menuRow) Tapped(*fyne.PointEvent) { r.onTapped() }

func (r *menuRow) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (r *menuRow) MouseIn(*fyne.PointEvent) {
	r.hovered = true
	r.Refresh()
}

func (r *menuRow) MouseMoved(*fyne.PointEvent) {}

func (r *menuRow) MouseOut() {
	r.hovered = false
	r.Refresh()
}

func (r *menuRow) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(nil)
	bg.CornerRadius = style.RadiusSM

	label := canvas.NewText(r.item.Label, style.Text)
	label.TextSize = style.SizeBody

	row := []fyne.CanvasObject{bg}
	left := container.NewHBox()
	if r.item.Icon != nil {
		left.Add(container.NewCenter(Icon(r.item.Icon, 18)))
	}
	left.Add(container.NewCenter(label))

	content := container.NewStack(bg, container.New(&insetLayout{h: style.Space3, v: style.Space2}, left))
	row = []fyne.CanvasObject{content}

	rr := &menuRowRenderer{r: r, bg: bg, objects: row}
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
	if m.r.hovered {
		m.bg.FillColor = style.SurfaceHigh
	} else {
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
