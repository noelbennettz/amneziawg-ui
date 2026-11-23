// install.go is the one entry point of this package's page integration: the
// frontend calls Install once, before the window exists, and the page is
// prepared for the panel.
//
// What follows is not a patch set bolted onto the product; it is the product's
// half of the browser contract. The wasm driver (fyne.io/fyne/v2 v2.8.1 with
// github.com/fyne-io/glfw-js v0.4.0) leaves a handful of page-level jobs to
// the application - a viewport meta, an on-screen keyboard, drag scrolling, a
// wheel event per frame, non-ASCII key delivery, a clipboard that works over
// plain http, and a repaint when a hidden tab comes back - and each file below
// does one of them. Every file opens with the upstream behaviour it answers and
// the condition under which it can be dropped, so a fyne upgrade can be
// checked against it instead of against memory.
package browser

import "fyne.io/fyne/v2"

// Install prepares the page; call it on the application before the window
// starts, since one of these calls replaces the clipboard the app hands out
// and another has to have wrapped document.addEventListener before the driver
// registers its own listeners.
func Install(a fyne.App) {
	installNonASCIIKeyboard()
	installClipboard(a)
	installVisibilityRepaint(a)
	installViewport()
	installWheelInput()
	installTouchInput()
}
