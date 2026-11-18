package widgets

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

const (
	// A paragraph that wraps into a very different number of lines at the two
	// widths the page is laid out at - a wide window and one under the
	// breakpoint.
	rewrapLong = "One value for all four paddings is what the docs recommend while RandomTrailers is on, " +
		"because it is what keeps the receiver from reading one message type as another. It does leave " +
		"the gaps between the types at WireGuard's own 148/92/64/32, which an observer can recover from " +
		"the smallest packet of each type; rolling them separately hides that, and gives up the " +
		"receiver's margin in exchange."
	rewrapShort = "The placeholders are what the engine accepts, not what is wise. On top of them S1 " +
		"must fit MTU-148, S2 must fit MTU-92, S1+56 must not equal S2, and H1-H4 must all differ."
)

// A note that wraps and the note under it must not be drawn on top of each
// other, at whichever width the stack is laid out: the second one starts below
// the last line the first one draws.
//
// This is what Rewrapped is for: a plain container.NewVBox stacks them at the
// height the first note had before it was given its width, so the pass that
// narrows the page leaves the two overlapping - the bug this test was written
// for, seen in the create form's obfuscation block.
func TestRewrappedKeepsWrappedNotesApart(t *testing.T) {
	test.NewApp()

	for _, widths := range [][]float32{
		{1320, 700}, // a wide window, then one under the breakpoint
		{700, 1320}, // and the other way round
		{1320, 400, 1320, 500},
	} {
		note := NewParagraph(rewrapLong, 13, nil, fyne.TextStyle{Italic: true}, 220)
		next := NewParagraph(rewrapShort, 13, nil, fyne.TextStyle{Italic: true}, 220)
		page := Rewrapped(container.NewVBox(note, next))

		for _, width := range widths {
			page.Resize(fyne.NewSize(width, 900))
			assertStackedBelow(t, note, next, width)
		}
	}
}

// assertStackedBelow checks that the first paragraph was given the room its
// lines take, and that the second starts under them.
func assertStackedBelow(t *testing.T, first, second *Paragraph, width float32) {
	t.Helper()

	need := float32(len(first.lines)) * lineHeight(first.size, first.style)
	if first.Size().Height+0.5 < need {
		t.Errorf("at %.0f wide: %d lines need %.0f points, the layout gave %.0f",
			width, len(first.lines), need, first.Size().Height)
	}
	if second.Position().Y+0.5 < first.Position().Y+need {
		t.Errorf("at %.0f wide: the second note starts at %.0f, the first one draws until %.0f",
			width, second.Position().Y, first.Position().Y+need)
	}
}

// The wrapper is invisible to the layouts above it: it takes the size of the
// object it holds.
func TestRewrappedMinSizeIsItsContent(t *testing.T) {
	test.NewApp()

	content := container.NewVBox(
		NewParagraph(rewrapShort, 13, nil, fyne.TextStyle{}, 220),
		NewParagraph(strings.Repeat("a fairly long sentence that wraps ", 3), 13, nil, fyne.TextStyle{}, 220),
	)
	page := Rewrapped(content)

	if got, want := page.MinSize(), content.MinSize(); got != want {
		t.Errorf("min size %.0fx%.0f, want the content's %.0fx%.0f",
			got.Width, got.Height, want.Width, want.Height)
	}
}
