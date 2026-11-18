//go:build !js || !wasm

package fixes

import "fyne.io/fyne/v2"

// installClipboard is a no-op off the browser: the clipboard that panics when
// the page is not on a secure origin belongs to fyne's wasm driver alone, see
// clipboard_wasm.go. The stub only exists so "go vet" and editors can still
// type-check this package natively.
func installClipboard(fyne.App) {}
