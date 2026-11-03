package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Pair lays two blocks out side by side when there is room for both, and one
// under the other when there is not.
//
// A card row is what this exists for: a title block on the left, its actions on
// the right, and on a phone the actions under the title. Fyne has no responsive
// container, and it cannot have one in the usual sense - a layout is handed the
// size it got, but its minimum size is what decided that size, so a layout
// cannot ask for less room than the two halves need side by side without being
// told which arrangement it is in. So the page tells it: one breakpoint for the
// whole page (style.Breakpoint), flipped through SetNarrow on every Pair at
// once, so a row never restacks a few pixels before its neighbours do.
type Pair struct {
	// Narrow stacks the second block under the first.
	Narrow bool
	// Indent pushes the second block in when stacked, so it lines up under
	// the text of the first block rather than under its avatar.
	Indent float32
	// Gap separates the two blocks, both ways.
	Gap float32
}

// Layout places objects[0] and objects[1].
func (p *Pair) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	first, second := objects[0], objects[1]
	firstMin, secondMin := first.MinSize(), second.MinSize()

	if p.Narrow {
		first.Move(fyne.NewPos(0, 0))
		first.Resize(fyne.NewSize(size.Width, firstMin.Height))

		second.Move(fyne.NewPos(p.Indent, firstMin.Height+p.Gap))
		second.Resize(fyne.NewSize(max(size.Width-p.Indent, secondMin.Width), secondMin.Height))
		return
	}

	// The second block keeps its own size at the right edge - an action row
	// squeezed by a long name would only push its own content around - and
	// the first takes whatever is left of the width.
	second.Resize(secondMin)
	second.Move(fyne.NewPos(size.Width-secondMin.Width, (size.Height-secondMin.Height)/2))

	first.Move(fyne.NewPos(0, (size.Height-firstMin.Height)/2))
	first.Resize(fyne.NewSize(size.Width-secondMin.Width-p.Gap, firstMin.Height))
}

// MinSize is the row's size side by side and the column's when stacked. The
// page refreshes the containers around this one when the breakpoint flips,
// which is what carries the new height up to the scroll content.
func (p *Pair) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 2 {
		return fyne.Size{}
	}
	first, second := objects[0].MinSize(), objects[1].MinSize()

	if p.Narrow {
		return fyne.NewSize(max(first.Width, second.Width+p.Indent), first.Height+p.Gap+second.Height)
	}
	return fyne.NewSize(first.Width+p.Gap+second.Width, max(first.Height, second.Height))
}

// PairBox is a Pair as an object, with the switch the page flips.
//
// A widget rather than a *fyne.Container with a field on the side: fyne walks
// the object tree with a type switch over *fyne.Container and fyne.Widget, and
// a struct that embeds the first is neither - its children would be laid out
// and then never drawn.
type PairBox struct {
	widget.BaseWidget

	layout *Pair
	box    *fyne.Container
}

// NewPair builds a row that stacks on a narrow page; indent is where the second
// block starts once it is stacked.
func NewPair(gap, indent float32, first, second fyne.CanvasObject) *PairBox {
	box := &PairBox{layout: &Pair{Gap: gap, Indent: indent}}
	box.box = container.New(box.layout, first, second)
	box.ExtendBaseWidget(box)
	return box
}

func (b *PairBox) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.box)
}

// SetNarrow restacks the pair; the page calls it from its breakpoint.
func (b *PairBox) SetNarrow(narrow bool) {
	if b.layout.Narrow == narrow {
		return
	}
	b.layout.Narrow = narrow
	b.Refresh()
}

// StackLayout puts its children one under the other, each at its own minimum
// height and the full width of the column: the stacked form of a WeightedRow,
// and what the dashboard turns into on a narrow page.
type StackLayout struct {
	Gap float32
}

func (s *StackLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objects {
		height := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, height))
		y += height + s.Gap
	}
}

func (s *StackLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var size fyne.Size
	for i, o := range objects {
		min := o.MinSize()
		size.Width = max(size.Width, min.Width)
		if i > 0 {
			size.Height += s.Gap
		}
		size.Height += min.Height
	}
	return size
}

// SplitRow lays three blocks out as one line, or as two on a narrow page: the
// first and the third share the top line - the third at the right edge, which
// is where a menu button belongs - and the middle one gets the second line.
//
// It is the shape of a client row's right-hand side: the state chips, the
// counters that say what the peer is doing, and the overflow menu. All three on
// one line need more width than a phone has, and the counters are the part that
// can wait for its own line.
type SplitRow struct {
	Narrow bool
	Gap    float32
}

func (s *SplitRow) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 3 {
		return
	}
	first, middle, last := objects[0], objects[1], objects[2]

	if !s.Narrow {
		x := float32(0)
		for _, o := range objects {
			min := o.MinSize()
			o.Move(fyne.NewPos(x, (size.Height-min.Height)/2))
			o.Resize(min)
			x += min.Width + s.Gap
		}
		return
	}

	firstSize, lastSize := first.MinSize(), last.MinSize()
	first.Move(fyne.NewPos(0, 0))
	first.Resize(firstSize)
	last.Move(fyne.NewPos(size.Width-lastSize.Width, 0))
	last.Resize(lastSize)

	middleSize := middle.MinSize()
	middle.Move(fyne.NewPos(0, max(firstSize.Height, lastSize.Height)+s.Gap/2))
	middle.Resize(middleSize)
}

func (s *SplitRow) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 3 {
		return fyne.Size{}
	}
	first, middle, last := objects[0].MinSize(), objects[1].MinSize(), objects[2].MinSize()

	if s.Narrow {
		top := max(first.Height, last.Height)
		return fyne.NewSize(max(first.Width+s.Gap+last.Width, middle.Width), top+s.Gap/2+middle.Height)
	}
	return fyne.NewSize(first.Width+s.Gap+middle.Width+s.Gap+last.Width,
		max(first.Height, max(middle.Height, last.Height)))
}

// SplitRowBox is a SplitRow as an object, with the switch the page flips. A
// widget for the reason PairBox is one.
type SplitRowBox struct {
	widget.BaseWidget

	layout *SplitRow
	box    *fyne.Container
}

// NewSplitRow builds the three-block row that folds onto two lines when narrow.
func NewSplitRow(gap float32, first, middle, last fyne.CanvasObject) *SplitRowBox {
	box := &SplitRowBox{layout: &SplitRow{Gap: gap}}
	box.box = container.New(box.layout, first, middle, last)
	box.ExtendBaseWidget(box)
	return box
}

func (b *SplitRowBox) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.box)
}

// SetNarrow folds or unfolds the row.
func (b *SplitRowBox) SetNarrow(narrow bool) {
	if b.layout.Narrow == narrow {
		return
	}
	b.layout.Narrow = narrow
	b.Refresh()
}

// Relayout lays every container below obj out again at the size it already has.
//
// Fyne lays a container out only when it is resized, and a canvas.Text that
// grows does not resize the row it sits in. So a pill whose text arrived after
// its row was laid out - the public IP replacing "unknown", a counter going
// from "0 B" to "1.82 MB" - keeps drawing at its new width while its neighbours
// stay where the old width put them, and they overlap. Anything that changes a
// text in place calls this, bottom up, so a parent measures the new minimum
// before it places its children.
func Relayout(obj fyne.CanvasObject) {
	switch node := obj.(type) {
	case *fyne.Container:
		for _, child := range node.Objects {
			Relayout(child)
		}
		if node.Layout != nil {
			node.Layout.Layout(node.Objects, node.Size())
		}
	case holder:
		Relayout(node.holds())
	}
}

// holder is a widget of this package that wraps a container of its own: the
// tree walk above cannot see inside a widget, and a row is a widget.
type holder interface {
	holds() *fyne.Container
}

func (b *PairBox) holds() *fyne.Container { return b.box }

func (b *SplitRowBox) holds() *fyne.Container { return b.box }
