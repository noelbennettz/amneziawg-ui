//go:build js && wasm

package fixes

import (
	"syscall/js"

	"amneziawg-web-ui/web-ui/internal/ui/browser"
)

// installViewport makes the page use the phone's own width.
//
// The loader page "fyne package" emits carries no viewport meta at all, so a
// phone lays it out at the browser's 980-point default and shrinks the whole
// canvas down to fit the screen: text at two thirds of its size, and - worse -
// a Fyne canvas 980 points wide, which the page then draws its two-column
// layout into before clipping it to the 390 points of glass it has. Adding the
// meta is the difference between a page that fits and one that is a scaled-down
// desktop window.
//
// The stylesheet next to it takes the loader's centring off the body: it is
// there for the spinner, and it also leaves the page scrollable, which a canvas
// that owns the viewport must not be.
//
// The resize event at the end is what tells the driver to size its canvas
// again - the meta was applied after the canvas was created, so as far as the
// driver is concerned nothing has happened yet.
func installViewport() {
	document := js.Global().Get("document")
	head := document.Get("head")

	const viewport = "width=device-width, initial-scale=1, viewport-fit=cover, interactive-widget=resizes-content"
	if existing := document.Call("querySelector", "meta[name=viewport]"); existing.Truthy() {
		existing.Call("setAttribute", "content", viewport)
	} else {
		meta := document.Call("createElement", "meta")
		meta.Call("setAttribute", "name", "viewport")
		meta.Call("setAttribute", "content", viewport)
		head.Call("appendChild", meta)
	}

	style := document.Call("createElement", "style")
	style.Set("textContent", `html, body {
		margin: 0; padding: 0; height: 100%;
		overflow: hidden; overscroll-behavior: none;
		background-color: #131314; text-align: left;
	}
	canvas { display: block; }`)
	head.Call("appendChild", style)

	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("resize"))
}

// installTouchInput gives a phone two things the wasm driver does not: a
// keyboard that appears when a field is tapped, and a page that scrolls when
// dragged.
//
// The keyboard: glfw-js ships the hook that raises it - showVirtualKeyboard in
// fyne's device_wasm.go focuses the loader page's #dummyEntry input - but
// nothing calls it, so a tapped entry gets a caret and no keyboard, and the
// keystrokes that never arrive are the ones the user is typing. The input
// itself is parked at top:-100pt, which a phone will not focus either: a
// control that is not on screen cannot take focus. So the input is moved into
// the corner at a pixel square, focused when a Fyne widget takes focus, and its
// own events are fed back into Fyne - a soft keyboard reports what was typed
// through "input" events, never through the keydown codes the driver reads.
//
// The scrolling: glfw-js's touch handler is commented out upstream, so a drag
// on the canvas does nothing and the page cannot be scrolled at all on a phone.
// Touches are turned into the wheel events the driver does handle.
//
// Neither applies to a desktop browser: there a hardware keyboard would be
// typed twice (the driver's keydown and the input's own event), and a mouse
// does not need dragging turned into wheeling.
func installTouchInput() {
	if !isTouchDevice() {
		return
	}

	js.Global().Set("awgFocused", js.FuncOf(func(js.Value, []js.Value) any {
		return js.ValueOf(widgetFocused())
	}))
	js.Global().Set("awgType", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		text := args[0].String()
		go func() {
			for _, r := range text {
				typeRune(r)
			}
		}()
		return nil
	}))
	js.Global().Set("awgKey", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		name := args[0].String()
		go typeKey(name)
		return nil
	}))

	document := js.Global().Get("document")
	script := document.Call("createElement", "script")
	script.Set("textContent", touchScript)
	// The page is served under a policy that allows one inline script by
	// nonce, and this is the second one. The value is the server's, put in
	// the loader page's markup for exactly this.
	if nonce := browser.CSPNonce(); nonce != "" {
		script.Set("nonce", nonce)
	}
	document.Get("head").Call("appendChild", script)

	go followFocus()
}
