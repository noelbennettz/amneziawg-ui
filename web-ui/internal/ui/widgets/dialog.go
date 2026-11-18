package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/style"
)

// Dialog is the page's modal card: a rounded surface travelling above the page
// with a title, the content and a right-aligned row of actions.
//
// It exists because fyne's dialog draws its own chrome: a small title, a
// hairline border and an almost-square card, none of which belongs to the M3
// surface the rest of the page is made of. The behaviour fyne's dialog brought
// along - a click outside dismisses, Escape closes - is kept by the popup and
// the key hook below.
type Dialog struct {
	canvas fyne.Canvas
	popup  *widget.PopUp

	actions *fyne.Container
}

// NewDialog builds the card, sized to size and centred over the canvas.
func NewDialog(c fyne.Canvas, title string, content fyne.CanvasObject, size fyne.Size) *Dialog {
	heading := canvas.NewText(title, style.Text)
	heading.TextSize = style.SizeTitle

	actions := container.NewHBox()
	actionRow := container.NewBorder(nil, nil, layout.NewSpacer(), container.NewCenter(actions))

	// The content scrolls instead of pushing the actions off the card. A
	// dialog taller than the window - the client's config page on a phone,
	// with a QR code above a column of config - used to grow past the screen
	// and take its only button with it; the card now keeps the room it was
	// given and the part that does not fit scrolls.
	scroll := container.NewVScroll(Rewrapped(content))
	scroll.SetMinSize(fyne.NewSize(0, 0))

	frame := container.NewBorder(
		container.NewVBox(heading, Spacer(style.Space3)),
		container.NewVBox(Spacer(style.Space5), actionRow),
		nil, nil,
		scroll,
	)

	d := &Dialog{canvas: c, actions: actions}
	d.popup = widget.NewPopUp(surface(frame, style.SurfaceAlt, style.RadiusXL), c)

	// size is the room the caller allows, not the size the card takes: a short
	// form in a tall box leaves the dialog looking half empty. What the card
	// would need is measured from its parts rather than from the frame, whose
	// scrolled centre reports no height of its own.
	need := fyne.NewSize(
		max(heading.MinSize().Width, content.MinSize().Width, actionRow.MinSize().Width),
		heading.MinSize().Height+style.Space3+content.MinSize().Height+style.Space5+actionRow.MinSize().Height,
	).Add(fyne.NewSize(cardPadH, cardPadV))

	if need.Height < size.Height {
		size.Height = need.Height
	}
	if need.Width < size.Width && need.Width > 0 {
		size.Width = need.Width
	}
	d.popup.Resize(size)
	d.popup.Hide()
	return d
}

// The padding surface() puts around a dialog's frame.
const (
	cardPadH = (style.Space5 - style.Space1) * 2
	cardPadV = style.Space4 * 2
)

// SetActions replaces the action row. The caller keeps the widgets, which is
// what lets a button be named for the browser suite.
func (d *Dialog) SetActions(buttons ...fyne.CanvasObject) {
	d.actions.Objects = nil
	for _, button := range buttons {
		d.actions.Add(button)
	}
	d.actions.Refresh()
}

// Show puts the card on the canvas and lets Escape close it.
func (d *Dialog) Show() {
	d.popup.Show()
	d.canvas.SetOnTypedKey(func(event *fyne.KeyEvent) {
		if event.Name == fyne.KeyEscape {
			d.Hide()
		}
	})
}

// Hide takes the card away again.
func (d *Dialog) Hide() {
	d.canvas.SetOnTypedKey(nil)
	d.popup.Hide()
}
