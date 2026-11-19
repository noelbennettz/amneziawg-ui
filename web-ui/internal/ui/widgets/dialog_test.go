package widgets

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// newDialogCanvas is a window of the given size to hang a dialog over.
func newDialogCanvas(t *testing.T, width, height float32) fyne.Window {
	t.Helper()

	test.NewApp()
	win := test.NewWindow(container.NewVBox())
	win.Resize(fyne.NewSize(width, height))

	return win
}

// layoutCard resizes the card's content to the card, which is the pass the
// popup's renderer runs when it is drawn: the card's own children - the title,
// the scrolled content and the action row - are placed then, and not before.
func layoutCard(d *Dialog) {
	d.popup.Content.Resize(d.popup.Size())
}

// The size a caller asks for is an upper bound, not a promise: a short message
// in a tall box gets a card the size of its content, so it does not read as a
// half-empty page.
func TestDialogShrinksToItsContent(t *testing.T) {
	win := newDialogCanvas(t, 1500, 950)
	c := win.Canvas()
	content := widget.NewLabel("S1-S4 must be at least 12.")

	requested := fyne.NewSize(900, 700)
	d := NewDialog(c, "Obfuscation", content, requested)

	got := d.popup.Size()
	if got.Width > requested.Width || got.Height > requested.Height {
		t.Errorf("card is %.0fx%.0f, larger than the %.0fx%.0f asked for",
			got.Width, got.Height, requested.Width, requested.Height)
	}
	if got.Height >= requested.Height {
		t.Errorf("card is %.0f tall for a one-line message, want less than the %.0f allowed",
			got.Height, requested.Height)
	}
	if got.Width < content.MinSize().Width {
		t.Errorf("card is %.0f wide, narrower than the content's minimum %.0f",
			got.Width, content.MinSize().Width)
	}
}

// A card never grows past the canvas it is drawn on: that is what keeps a
// desktop-sized form usable on a phone.
func TestDialogStaysInsideTheCanvas(t *testing.T) {
	for _, canvas := range []fyne.Size{
		{Width: 1500, Height: 950},
		{Width: 390, Height: 844}, // the narrow layout's test phone
	} {
		win := newDialogCanvas(t, canvas.Width, canvas.Height)
		c := win.Canvas()
		long := widget.NewLabel("A line long enough to want more room than a phone has, " +
			"repeated so the content's minimum width is well past the screen.")
		long.Wrapping = fyne.TextWrapWord

		d := NewDialog(c, "Client config", long, fyne.NewSize(900, 700))
		got := d.popup.Size()

		if want := canvas.Width - cardPadH; got.Width > want+0.5 {
			t.Errorf("on a %.0f-wide canvas the card is %.0f wide, want at most %.0f",
				canvas.Width, got.Width, want)
		}
		if got.Height > canvas.Height {
			t.Errorf("on a %.0f-tall canvas the card is %.0f tall", canvas.Height, got.Height)
		}
	}
}

// Show puts the card on the canvas, Hide takes it off: the overlay stack is
// where a modal lives.
func TestDialogShowAndHide(t *testing.T) {
	win := newDialogCanvas(t, 1500, 950)
	c := win.Canvas()
	d := NewDialog(c, "Delete server", widget.NewLabel("Sure?"), fyne.NewSize(400, 240))

	if d.popup.Visible() {
		t.Error("the card is visible before Show")
	}
	d.Show()
	if !d.popup.Visible() {
		t.Fatal("Show left the card hidden")
	}
	if top := c.Overlays().Top(); top == nil {
		t.Fatal("Show put nothing on the overlay stack")
	}
	d.Hide()
	if d.popup.Visible() {
		t.Error("Hide left the card on the canvas")
	}
}

// The actions are a row in the bottom right corner, under the content: the
// WhatsApp-style order a person expects, whatever the card's size.
func TestDialogActionsSitBottomRight(t *testing.T) {
	win := newDialogCanvas(t, 1500, 950)
	c := win.Canvas()
	d := NewDialog(c, "Delete server", widget.NewLabel("Sure?"), fyne.NewSize(600, 400))

	cancel := NewButton("Cancel", nil, nil)
	confirm := NewButton("Delete", nil, nil)
	d.SetActions(cancel, confirm)
	d.Show()
	// The test canvas lays a popup out when it draws it; capturing the canvas
	// forces that pass, which is what makes the positions readable without a
	// frame loop.
	layoutCard(d)

	card := d.popup.Size()
	at := fyne.CurrentApp().Driver().AbsolutePositionForObject
	cancelAt, confirmAt := at(cancel), at(confirm)

	if confirmAt.Y+cancel.Size().Height <= card.Height/2 {
		t.Errorf("the action row is at %.0f of a %.0f-tall card, want it in the lower half",
			confirmAt.Y, card.Height)
	}
	if confirmAt.X+confirm.Size().Width <= card.Width/2 {
		t.Errorf("the actions end at %.0f of a %.0f-wide card, want them on the right",
			confirmAt.X+confirm.Size().Width, card.Width)
	}
	if cancelAt.X >= confirmAt.X {
		t.Errorf("cancel starts at %.0f, confirm at %.0f: want cancel first", cancelAt.X, confirmAt.X)
	}

	// SetActions replaces rather than appends.
	d.SetActions(confirm)
	if got := len(d.actions.Objects); got != 1 {
		t.Errorf("after SetActions one button the row holds %d", got)
	}
}
