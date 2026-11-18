//go:build js && wasm

package fixes

import (
	"strings"
	"sync"
	"syscall/js"

	"fyne.io/fyne/v2"
)

// installClipboard fixes copy, cut and paste killing the whole app when the
// page is not served from a secure origin.
//
// github.com/fyne-io/glfw-js@v0.4.0 (clipboard_wasm.go) reaches for the async
// clipboard API once, at package init:
//
//	var clipboard = js.Global().Get("navigator").Get("clipboard")
//
// navigator.clipboard exists only in a secure context - https, or localhost.
// Over plain http from anything else, which is how the container is normally
// reached, it is undefined, and the first read panics:
//
//	panic: syscall/js: call of Value.Call on undefined
//	github.com/fyne-io/glfw-js.GetClipboardString()
//	fyne.io/fyne/v2/widget.(*Entry).pasteFromClipboard(...)
//
// The panic runs on the goroutine glfw-js starts for the key event, so it
// takes the program with it: the canvas freezes on its last frame, every
// later event logs "Go program has already exited", and only a reload brings
// the app back. Typing still works right up to that point, which is what
// makes it look like a paste bug rather than a crash. Every browser is
// affected, since none of them expose navigator.clipboard over plain http.
//
// The workaround does not need the async API at all. Ctrl/Cmd+V, +C and +X
// (and Shift+Insert / Shift+Delete) are caught on the document in the CAPTURE
// phase, which runs before glfw-js's own keydown listener no matter which of
// the two was registered first, and stopped there: glfw-js never sees the
// key, so it neither preventDefaults it nor hands fyne a shortcut that reads
// navigator.clipboard. Left alone, the browser then performs the copy or
// paste itself and fires a ClipboardEvent, whose clipboardData is the
// synchronous, permission-free half of the clipboard API - the half a page is
// allowed to use over plain http. From there the text goes to the focused
// widget as the very fyne.Shortcut the driver would have sent, so entries
// keep their own behaviour: the selection is replaced, newlines are flattened
// in single line fields, password fields are never copied out.
//
// The Entry context menu ("Paste" on right click) takes the other route,
// fyne.CurrentApp().Clipboard(), with no browser event behind it to hang a
// real clipboard on - it would still panic. So the app is wrapped and that
// clipboard becomes ours too: it remembers what the shortcuts above moved and
// can never crash, at the price of the menu seeing only the text this app
// itself last copied. The keyboard is the path with full system clipboard
// access.
//
// Remove once upstream falls back to ClipboardEvent, or at least checks
// navigator.clipboard before calling into it.
func installClipboard(a fyne.App) {
	if js.Global().Get("navigator").Get("clipboard").Truthy() {
		return // secure context - upstream's own path works
	}

	fyne.SetCurrentApp(appClipboard{App: a})

	document := js.Global().Get("document")
	document.Call("addEventListener", "keydown", js.FuncOf(onClipboardKey), true)
	document.Call("addEventListener", "paste", js.FuncOf(onPaste))
	document.Call("addEventListener", "copy", js.FuncOf(onCopy))
	document.Call("addEventListener", "cut", js.FuncOf(onCut))
}

// onClipboardKey keeps a clipboard shortcut away from glfw-js and lets the
// browser handle it, which is what produces the ClipboardEvent below.
func onClipboardKey(_ js.Value, args []js.Value) any {
	if len(args) == 0 {
		return nil
	}
	if event := args[0]; clipboardShortcut(event) {
		event.Call("stopImmediatePropagation")
	}
	return nil
}

// clipboardShortcut reports whether the keydown is one of the combinations the
// browser answers with a ClipboardEvent. Both KeyboardEvent.key and .code are
// consulted: the first is empty of meaning under a non-latin layout, where
// Ctrl+V arrives as "м", the second under a remapped one.
func clipboardShortcut(event js.Value) bool {
	if event.Get("altKey").Bool() {
		return false
	}

	key := strings.ToLower(event.Get("key").String())
	code := event.Get("code").String()

	if event.Get("ctrlKey").Bool() || event.Get("metaKey").Bool() {
		switch {
		case key == "v", key == "c", key == "x":
			return true
		case code == "KeyV", code == "KeyC", code == "KeyX":
			return true
		}
		return false
	}

	// The old alternatives, still wired to copy and paste in every browser.
	return event.Get("shiftKey").Bool() && (key == "insert" || key == "delete")
}

func onPaste(_ js.Value, args []js.Value) any {
	text, event, ok := clipboardText(args)
	if !ok {
		return nil
	}
	event.Call("preventDefault")
	if text == "" {
		return nil
	}

	pasteboard.SetContent(text)
	deliver(&fyne.ShortcutPaste{Clipboard: constantClipboard(text)})
	return nil
}

func onCopy(_ js.Value, args []js.Value) any { return takeSelection(args, false) }

func onCut(_ js.Value, args []js.Value) any { return takeSelection(args, true) }

// takeSelection asks the focused widget for the text the driver would have put
// on the clipboard and puts it on the event instead. It has to happen while
// the event is being dispatched, so the shortcut is delivered inline rather
// than on a goroutine of its own - unlike TypedRune (see keyboard_wasm.go),
// neither Entry.copyToClipboard nor Entry.cutToClipboard can block.
func takeSelection(args []js.Value, cut bool) any {
	_, event, ok := clipboardText(args)
	if !ok {
		return nil
	}

	var taken capturedClipboard
	if cut {
		deliver(&fyne.ShortcutCut{Clipboard: &taken})
	} else {
		deliver(&fyne.ShortcutCopy{Clipboard: &taken})
	}
	if taken.text == "" {
		return nil // nothing selected, or a password field: leave the clipboard alone
	}

	event.Get("clipboardData").Call("setData", "text/plain", taken.text)
	event.Call("preventDefault")
	pasteboard.SetContent(taken.text)
	return nil
}

// clipboardText pulls the plain text out of a ClipboardEvent, and reports
// whether the event carries clipboardData at all.
func clipboardText(args []js.Value) (text string, event js.Value, ok bool) {
	if len(args) == 0 {
		return "", js.Undefined(), false
	}

	event = args[0]
	data := event.Get("clipboardData")
	if !data.Truthy() {
		return "", event, false
	}

	return data.Call("getData", "text/plain").String(), event, true
}

// deliver hands the shortcut to the focused widget, the way the driver's
// (*window).triggersShortcut does.
func deliver(shortcut fyne.Shortcut) {
	a := fyne.CurrentApp()
	if a == nil {
		return
	}

	for _, w := range a.Driver().AllWindows() {
		c := w.Canvas()
		if c == nil {
			continue
		}
		if target, ok := c.Focused().(fyne.Shortcutable); ok {
			target.TypedShortcut(shortcut)
			return
		}
	}
}

// constantClipboard is the clipboard a paste shortcut is handed: the text the
// browser already gave us, and nowhere to write back to.
type constantClipboard string

func (c constantClipboard) Content() string { return string(c) }

func (c constantClipboard) SetContent(string) {}

// capturedClipboard is the clipboard a copy or cut shortcut is handed: it only
// has to catch what the widget writes.
type capturedClipboard struct{ text string }

func (c *capturedClipboard) Content() string { return c.text }

func (c *capturedClipboard) SetContent(content string) { c.text = content }

// pasteboard stands in for the system clipboard on the paths that have no
// browser event behind them - the Entry context menu. It holds whatever the
// shortcuts above last moved.
var pasteboard = &memoryClipboard{}

type memoryClipboard struct {
	mu   sync.Mutex
	text string
}

func (c *memoryClipboard) Content() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.text
}

func (c *memoryClipboard) SetContent(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.text = content
}

// appClipboard is the app fyne.CurrentApp() returns, identical to the real one
// except for the clipboard it hands out.
type appClipboard struct{ fyne.App }

func (a appClipboard) Clipboard() fyne.Clipboard { return pasteboard }
