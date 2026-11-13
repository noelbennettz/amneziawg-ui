// Package dialogs is the one way a dialog is opened on this page.
//
// Every dialog goes through widgets.Dialog, which draws the M3 card: a rounded
// surface, a title at heading size, the content and a right-aligned action row.
// fyne's own dialog draws a different chrome - a small title, a hairline border
// and an almost-square card - so none of this page's dialogs call dialog.Show*
// directly; what is kept from fyne's version is the behaviour callers rely on:
// a click outside dismisses and Escape closes.
package dialogs

import (
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"amneziawg-web-ui/web-ui/internal/ui/browser"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/style"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// Show presents content with a single dismiss button.
func Show(win fyne.Window, title, dismiss string, content fyne.CanvasObject, size fyne.Size) {
	view := widgets.NewDialog(win.Canvas(), title, content, size)

	closeButton := widgets.NewButton(dismiss, nil, view.Hide)
	closeButton.Importance = widget.LowImportance
	testhook.Register("dialog.close", closeButton)

	view.SetActions(closeButton)
	view.Show()
}

// ShowForm presents content with the cancel/confirm pair a form needs.
//
// onConfirm reports whether the dialog is finished: returning false leaves it
// open, which is what a form does when what the user typed does not validate -
// closing it first would throw the input away along with the mistake.
func ShowForm(win fyne.Window, title, confirm, dismiss string, content fyne.CanvasObject, size fyne.Size, onConfirm func() bool) {
	view := widgets.NewDialog(win.Canvas(), title, content, size)

	cancel := widgets.NewButton(dismiss, nil, view.Hide)
	cancel.Importance = widget.LowImportance

	ok := widgets.NewButton(confirm, nil, func() {
		if !onConfirm() {
			return
		}
		view.Hide()
	})
	ok.Importance = widget.HighImportance

	testhook.Register("dialog.cancel", cancel)
	testhook.Register("dialog.confirm", ok)

	view.SetActions(cancel, ok)
	view.Show()
}

// Confirm asks a yes/no question.
func Confirm(win fyne.Window, title, message string, onConfirm func()) {
	size := messageSize(win, message)
	view := widgets.NewDialog(win.Canvas(), title, wrappedMessage(message), size)

	no := widgets.NewButton(lang.L("No"), nil, view.Hide)
	no.Importance = widget.LowImportance

	yes := widgets.NewButton(lang.L("Yes"), nil, func() {
		view.Hide()
		onConfirm()
	})
	yes.Importance = widget.HighImportance

	testhook.Register("dialog.cancel", no)
	testhook.Register("dialog.confirm", yes)

	view.SetActions(no, yes)
	view.Show()
}

// ShowError reports a failed call. Must run on the Fyne thread; components
// reach it through env.Notifier.Fail, which takes care of that.
func ShowError(win fyne.Window, err error) {
	message := err.Error()
	if r, size := utf8.DecodeRuneInString(message); r != utf8.RuneError {
		message = string(unicode.ToUpper(r)) + message[size:]
	}

	view := widgets.NewDialog(win.Canvas(), lang.L("Error"), wrappedMessage(message), messageSize(win, message))
	okButton := widgets.NewButton(lang.L("OK"), nil, view.Hide)
	okButton.Importance = widget.HighImportance
	testhook.Register("dialog.confirm", okButton)

	view.SetActions(okButton)
	view.Show()
}

func wrappedMessage(message string) fyne.CanvasObject {
	return &widget.Label{Text: message, Alignment: fyne.TextAlignCenter, Wrapping: fyne.TextWrapWord}
}

// messageSize is wide enough for the message on one line, capped at 600px and
// at most 90% of the window. A word-wrapped label on its own asks for the
// width of its longest word - a two-line-per-sentence sliver.
func messageSize(win fyne.Window, message string) fyne.Size {
	unwrapped := widget.NewLabel(message).MinSize().Width + 48
	width := min(unwrapped, 600, win.Canvas().Size().Width*0.9)
	return fyne.NewSize(width, 240)
}

// Size clamps a preferred size to the browser window. The card adds its own
// padding and title around the content, so this is the content's box.
func Size(win fyne.Window, width, height float32) fyne.Size {
	canvasSize := win.Canvas().Size()

	// A desktop window keeps a wide margin around its dialog. A phone has none
	// to spare: 80 points of it on a 390-point screen would leave a card
	// narrower than the form inside it, so the compact layout takes the space
	// of two grid gaps and nothing more.
	margin := float32(80)
	if canvasSize.Width > 0 && canvasSize.Width < style.Breakpoint {
		margin = style.Space6
	}

	if canvasSize.Width > 0 && width > canvasSize.Width-margin {
		width = canvasSize.Width - margin
	}
	if canvasSize.Height > 0 && height > canvasSize.Height-margin {
		height = canvasSize.Height - margin
	}
	return fyne.NewSize(width, height)
}

// Copy puts text on the clipboard and reports the outcome instead of assuming
// one: the browser is entitled to refuse a clipboard write, and the text stays
// selectable in the dialog for exactly that case.
func Copy(notify env.Notifier, text string) {
	browser.CopyText(text, func(ok bool) {
		if ok {
			notify.OK(lang.L("Copied to clipboard"))
			return
		}
		notify.Warn(lang.L("Could not copy - select the text and press Ctrl+C"))
	})
}
