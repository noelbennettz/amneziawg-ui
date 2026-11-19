package dialogs

import (
	"errors"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// newDialogWindow is a window of the given size, with a page in it, for the
// dialogs to open over.
func newDialogWindow(t *testing.T, width, height float32) fyne.Window {
	t.Helper()

	test.NewApp()
	win := test.NewWindow(container.NewVBox())
	win.Resize(fyne.NewSize(width, height))
	return win
}

// press taps the widget registered under id, the way the browser suite would.
func press(t *testing.T, id string) {
	t.Helper()

	button, ok := testhook.Object(id).(*widgets.Button)
	if !ok {
		t.Fatalf("%s is not a button, it is %T", id, testhook.Object(id))
	}
	test.Tap(button)
}

// Size clamps a preferred size to the window: a desktop keeps a wide margin, a
// phone keeps almost none, and neither lets a card hang off the screen.
func TestSizeClampsToTheWindow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		canvas        fyne.Size
		width, height float32
		want          fyne.Size
	}{
		{
			name:   "desktop, the card fits",
			canvas: fyne.NewSize(1500, 950),
			width:  600, height: 400,
			want: fyne.NewSize(600, 400),
		},
		{
			name:   "desktop, the card is wider than the window allows",
			canvas: fyne.NewSize(1500, 950),
			width:  1450, height: 500,
			want: fyne.NewSize(1420, 500), // 80 points of margin
		},
		{
			name:   "phone, the card takes what is left",
			canvas: fyne.NewSize(390, 844),
			width:  600, height: 500,
			want: fyne.NewSize(390-style.Space6, 500),
		},
		{
			name:   "phone, taller than the screen",
			canvas: fyne.NewSize(390, 844),
			width:  300, height: 2000,
			want: fyne.NewSize(300, 844-style.Space6),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			win := newDialogWindow(t, tc.canvas.Width, tc.canvas.Height)
			if got := Size(win, tc.width, tc.height); got != tc.want {
				t.Errorf("Size(%.0f, %.0f) on a %.0fx%.0f window = %v, want %v",
					tc.width, tc.height, tc.canvas.Width, tc.canvas.Height, got, tc.want)
			}
		})
	}
}

// Confirm asks a yes/no question: both answers are reachable from the browser
// suite by id, and the callback runs only after the card is gone.
func TestConfirmRegistersBothAnswers(t *testing.T) {
	win := newDialogWindow(t, 1500, 950)

	called := 0
	Confirm(win, "Delete server?", "The keys on disk go with it.", func() { called++ })

	if top := win.Canvas().Overlays().Top(); top == nil {
		t.Fatal("Confirm opened no card")
	}
	for _, id := range []string{"dialog.confirm", "dialog.cancel"} {
		if !testhook.Has(id) {
			t.Errorf("%s is not registered, so the browser suite cannot press it", id)
		}
	}

	press(t, "dialog.confirm")
	if called != 1 {
		t.Errorf("the callback ran %d times, want once", called)
	}
	if win.Canvas().Overlays().Top() != nil {
		t.Error("the card is still open after Yes")
	}
}

// A form's confirm handler decides whether the card closes: returning false is
// what a rejected input looks like, and throwing the card away with it would
// lose what the person typed.
func TestShowFormKeepsTheCardOpenWhileTheHandlerSaysNo(t *testing.T) {
	win := newDialogWindow(t, 1500, 950)

	accepted := false
	ShowForm(win, "New server", "Create", "Cancel", widget.NewLabel("form"), fyne.NewSize(600, 400), func() bool {
		return accepted
	})
	if win.Canvas().Overlays().Top() == nil {
		t.Fatal("ShowForm opened no card")
	}
	if !testhook.Has("dialog.cancel") || !testhook.Has("dialog.confirm") {
		t.Fatalf("the form registered cancel=%v confirm=%v",
			testhook.Has("dialog.cancel"), testhook.Has("dialog.confirm"))
	}

	press(t, "dialog.confirm")
	if win.Canvas().Overlays().Top() == nil {
		t.Fatal("the card closed although the handler rejected the input")
	}

	accepted = true
	press(t, "dialog.confirm")
	if win.Canvas().Overlays().Top() != nil {
		t.Error("the card stayed open after the handler accepted")
	}
}

// ShowError opens a card with one button, and closes it on OK.
func TestShowError(t *testing.T) {
	win := newDialogWindow(t, 1500, 950)

	ShowError(win, errors.New("the server did not answer"))

	if win.Canvas().Overlays().Top() == nil {
		t.Fatal("ShowError opened no card")
	}
	if !testhook.Has("dialog.confirm") {
		t.Error("dialog.confirm is not registered")
	}
	press(t, "dialog.confirm")
	if win.Canvas().Overlays().Top() != nil {
		t.Error("the error card stayed open after OK")
	}
}

// The message is a word-wrapped, centred label: a label of its own would be as
// wide as the sentence, and as tall as one word per line where the card is
// narrow.
func TestWrappedMessage(t *testing.T) {
	label, ok := wrappedMessage("the server did not answer").(*widget.Label)
	if !ok {
		t.Fatalf("wrappedMessage returned %T, want a label", wrappedMessage("x"))
	}
	if label.Text != "the server did not answer" {
		t.Errorf("wrappedMessage text = %q", label.Text)
	}
	if label.Wrapping != fyne.TextWrapWord || label.Alignment != fyne.TextAlignCenter {
		t.Errorf("wrappedMessage wrapping = %v, alignment = %v", label.Wrapping, label.Alignment)
	}
}

// messageSize is wide enough for the message on one line, capped at 600 points
// and at 90% of the window.
func TestMessageSizeIsCapped(t *testing.T) {
	win := newDialogWindow(t, 1500, 950)

	if short := messageSize(win, "Delete server?"); short.Width < 100 {
		t.Errorf("a short message got %.0f points of width", short.Width)
	}

	long := messageSize(win, "This message is long enough to want more than the 600-point ceiling, "+
		"which is what keeps an error from stretching the card across a wide screen.")
	if long.Width != 600 {
		t.Errorf("a long message got %.0f points of width, want the 600 ceiling", long.Width)
	}

	small := newDialogWindow(t, 390, 844)
	if got := messageSize(small, "x").Width; got > 390*0.9+0.5 {
		t.Errorf("on a phone the message got %.0f points, more than 90%% of the screen", got)
	}
}
