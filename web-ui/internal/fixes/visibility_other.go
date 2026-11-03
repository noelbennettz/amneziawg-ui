//go:build !js || !wasm

package fixes

import "fyne.io/fyne/v2"

// installVisibilityRepaint is a no-op off the browser: only the wasm driver
// parks the draw loop in SwapBuffers while the page is hidden, see
// visibility_wasm.go. The stub only exists so "go vet" and editors can still
// type-check this package natively.
func installVisibilityRepaint(fyne.App) {}
