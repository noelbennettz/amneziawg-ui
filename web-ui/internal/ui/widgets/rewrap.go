package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

// Rewrapped wraps content so that every layout pass runs twice.
//
// A Paragraph - and the hints, notes and meta lines built on it - is as tall
// as the width it is given allows, which is the whole point of it (see
// paragraph.go). But a fyne layout asks every child for its minimum size
// before it gives any of them a width. So on the pass that narrows the page
// the notes are still measured at the width they had a moment ago, they are
// stacked at that height, and only then are they given the narrower width and
// wrap into two or three more lines than the room they were put in: the text
// of one note is drawn on top of the next, and nothing asks for another pass,
// because no size changed after that. It stays that way until something else
// happens to lay the page out again - a window resize, a panel opening.
//
// Hence the second pass here, at the one place everything on the page hangs
// from: the first gives the whole tree the width it will have, which is when
// the paragraphs re-wrap, and the second stacks it at the heights that came
// out of that. Two is enough because a paragraph's height follows from its
// width alone, and the width does not change between the passes.
//
// The cost is one extra layout of the tree per resize, not per frame: nothing
// repaints here, and the second pass is the one whose result is seen.
//
// Relayout (pair.go) is the same idea for text that grows in place; this one
// is for text that changes height because its width did.
func Rewrapped(content fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&twiceLayout{}, content)
}

// twiceLayout is Rewrapped's layout: layout.NewMaxLayout, run twice.
type twiceLayout struct{}

func (l *twiceLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.Size{}
	}
	return objects[0].MinSize()
}

func (l *twiceLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}

	// The first pass is for the width alone: fyne skips a resize to the size
	// an object already has, so it cannot be the size the second pass asks
	// for, and no height is needed to learn where the text breaks - the box
	// layouts below measure their children rather than fill the space.
	inner := layout.NewMaxLayout()
	inner.Layout(objects, fyne.NewSize(size.Width, 0))
	inner.Layout(objects, size)
}
