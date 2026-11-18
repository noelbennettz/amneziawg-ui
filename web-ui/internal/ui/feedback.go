package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"amneziawg-web-ui/web-ui/internal/ui/dialogs"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// feedback is the page's env.Notifier: outcomes go to a snackbar floating over
// the bottom of the window, failures open an error dialog. Every method is safe
// from any goroutine.
type feedback struct {
	win   fyne.Window
	snack *snackbar
}

func (f *feedback) OK(message string) {
	f.snack.show(message, style.Success)
}

func (f *feedback) Warn(message string) {
	f.snack.show(message, style.Warning)
}

func (f *feedback) Fail(err error) {
	fyne.Do(func() { dialogs.ShowError(f.win, err) })
}

// snackbar is the M3 snackbar: an inverse-surface card that appears over the
// bottom of the page, says one thing and takes itself away again. The footer it
// replaced shared a line with the copyright, so the two overwrote each other.
type snackbar struct {
	box   *fyne.Container
	label *canvas.Text
	dot   *canvas.Circle
	seq   int
}

func newSnackbar() *snackbar {
	s := &snackbar{
		label: widgets.LabelText("", style.Background),
		dot:   canvas.NewCircle(style.Success),
	}

	content := container.NewHBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(8, 8), s.dot)),
		container.NewCenter(s.label),
	)

	surface := canvas.NewRectangle(style.Text)
	surface.CornerRadius = style.RadiusSM

	s.box = container.NewStack(surface, container.New(&snackPadding{}, content))
	s.box.Hide()
	return s
}

// CanvasObject is the snackbar as the page stacks it over the body.
func (s *snackbar) CanvasObject() fyne.CanvasObject { return s.box }

// show displays the message, replacing whatever was there, and hides it again
// after a few seconds unless another message has arrived in the meantime.
func (s *snackbar) show(message string, c color.NRGBA) {
	fyne.Do(func() {
		s.seq++
		seq := s.seq

		s.label.Text = message
		s.label.Refresh()
		s.dot.FillColor = c
		s.dot.Refresh()
		s.box.Show()

		time.AfterFunc(5*time.Second, func() {
			fyne.Do(func() {
				if s.seq != seq {
					return
				}
				s.box.Hide()
			})
		})
	})
}

// snackPadding is the snackbar's inner spacing: wider than a card's, because a
// single line of text in a pill needs room at both ends.
type snackPadding struct{}

func (p *snackPadding) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(style.Space5, style.Space3))
		o.Resize(size.Subtract(fyne.NewSize(style.Space5*2, style.Space3*2)))
	}
}

func (p *snackPadding) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var min fyne.Size
	for _, o := range objects {
		min = min.Max(o.MinSize())
	}
	return min.Add(fyne.NewSize(style.Space5*2, style.Space3*2))
}
