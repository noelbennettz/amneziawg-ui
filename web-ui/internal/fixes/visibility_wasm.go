//go:build js && wasm

package fixes

import (
	"syscall/js"

	"fyne.io/fyne/v2"
)

// installVisibilityRepaint fixes the page going dead after the tab has been in
// the background (another tab in front, the window minimised) for longer than
// a minute: once it is back, nothing on it updates and nothing reacts to
// clicks until the window is resized or the page reloaded.
//
// The page is not stuck. fyne.io/fyne/v2@v2.8.1 (internal/driver/glfw/loop.go)
// draws a frame and then blocks in glfw.Window.SwapBuffers, which on the wasm
// driver (github.com/fyne-io/glfw-js@v0.4.0, browser_wasm.go) waits for the
// next requestAnimationFrame - and browsers do not run animation frames for a
// hidden page. So the frame drawn just before the tab went away sits in
// SwapBuffers until the tab is visible again, minutes later, while the
// polling goroutines keep queueing Refresh calls.
//
// The damage is done right after SwapBuffers returns. The loop calls
// cache.Clean (internal/cache/base.go), which drops every renderer and every
// object-to-canvas record that was not touched within cache.ValidDuration
// (one minute) - and, since a paint is what touches them, that is all of
// them. Widget.Refresh then goes through CanvasForObject, finds no canvas and
// never marks it dirty, so no frame is ever drawn again: the polls, the
// buttons, the hover states all still run, just nowhere visible. A resize
// works because it marks the canvas dirty directly, and the next paint walks
// the whole tree and re-registers every object.
//
// The workaround does the same thing on purpose: when the document reports
// itself visible again, every window's content and overlays are refreshed
// through the canvas itself (fyne.Canvas.Refresh does not look the canvas up)
// on the next turn of the draw loop - that is after the Clean that empties
// the cache, since the loop is still inside SwapBuffers when the event fires,
// and before anything the user can notice. On desktop the loop never blocks
// this long, so the fix is wasm only.
//
// Remove once upstream either keeps the loop ticking while the page is hidden
// or exempts a window from cache expiry while it waits for a frame.
func installVisibilityRepaint(a fyne.App) {
	document := js.Global().Get("document")
	document.Call("addEventListener", "visibilitychange", js.FuncOf(func(js.Value, []js.Value) any {
		if document.Get("visibilityState").String() != "visible" {
			return nil
		}
		fyne.Do(func() {
			for _, w := range a.Driver().AllWindows() {
				c := w.Canvas()
				if content := c.Content(); content != nil {
					c.Refresh(content)
				}
				for _, o := range c.Overlays().List() {
					c.Refresh(o)
				}
			}
		})
		return nil
	}))
}
