//go:build js && wasm

package browser

import (
	"syscall/js"
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
	/* The page cannot scroll and every gesture on the canvas is ours: saying
	   so lets the browser stop negotiating whether a drag is a scroll before
	   it hands the events over, which is the delay a phone feels as a drag
	   that starts late. */
	canvas { display: block; touch-action: none; }
	/* A tap on the canvas used to flash the browser's own highlight over the
	   whole page: Android Chrome paints -webkit-tap-highlight-color, a
	   translucent #33b5e5, over the box of whatever was tapped, and the
	   canvas covers the viewport. iOS Safari draws the same flash in grey.
	   The panel has its own press states, so the browser's is only a
	   full-screen wash on a dark page. The outline goes for the same reason:
	   a focused element - the off-screen input the keyboard fix focuses
	   above all - would draw a focus ring around every pixel of the page. */
	html, body, canvas, #dummyEntry {
		-webkit-tap-highlight-color: transparent;
	}
	canvas, #dummyEntry { outline: none; }`)
	head.Call("appendChild", style)

	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("resize"))
}
