//go:build js && wasm

package fixes

import (
	"syscall/js"
	"unicode/utf8"

	"fyne.io/fyne/v2"
)

// installNonASCIIKeyboard fixes typing non-ASCII characters (Cyrillic and the
// like) into any of the entry fields on this page.
//
// github.com/fyne-io/glfw-js@v0.4.0 (browser_wasm.go, the "keydown" handler)
// tells the control keys ("Enter", "ArrowLeft", ...) apart from the printable
// ones by the length of KeyboardEvent.key in BYTES:
//
//	keyStr := ke.Get("key").String()
//	if len(keyStr) == 1 { ... go w.charCallback(w, []rune(keyStr)[0]) }
//
// js.Value.String() returns UTF-8, so "a" (1 byte) gets through while "й"
// (2 bytes), "é" or "€" do not: charCallback is never called and the rune
// never reaches fyne.Focusable.TypedRune. Only the wasm driver is affected.
//
// The workaround is a second "keydown" listener on the document - glfw-js
// calls preventDefault but not stopPropagation, so the event still reaches us
// - delivering exactly the runes glfw-js dropped: ASCII is left alone, or we
// would get every latin character twice. Delivery goes through `go`, as it
// does in glfw-js, so that the order of a mixed "aйbц" does not break on two
// different delivery paths (fyne.Do would defer the non-ASCII rune to the
// next frame). That is where the "Error in Fyne call thread" console line
// comes from - the same one the driver itself (window_wasm.go) prints for
// every ASCII character, since no input in wasm arrives on the main
// goroutine.
//
// Remove once upstream checks utf8.RuneCountInString instead.
func installNonASCIIKeyboard() {
	js.Global().Get("document").Call("addEventListener", "keydown", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		event := args[0]

		// Ctrl/Alt/Meta mean a shortcut rather than text input (glfw-js does
		// not call charCallback for those either).
		if event.Get("ctrlKey").Bool() || event.Get("altKey").Bool() || event.Get("metaKey").Bool() {
			return nil
		}

		key := event.Get("key").String()
		r, size := utf8.DecodeRuneInString(key)
		switch {
		case r == utf8.RuneError, size < 2:
			return nil // ASCII - glfw-js already delivered it
		case size != len(key):
			return nil // a multi-character key name ("Enter", "Shift", ...)
		}

		go typeRune(r)
		return nil
	}))
}

// typeRune repeats what the driver's (*window).processCharInput does: the rune
// goes to the focused widget, or to the canvas handler when nothing is focused.
func typeRune(r rune) {
	a := fyne.CurrentApp()
	if a == nil {
		return
	}

	for _, w := range a.Driver().AllWindows() {
		c := w.Canvas()
		if c == nil {
			continue
		}
		if focused := c.Focused(); focused != nil {
			focused.TypedRune(r)
			return
		}
		if onRune := c.OnTypedRune(); onRune != nil {
			onRune(r)
			return
		}
	}
}
