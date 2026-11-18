//go:build !js || !wasm

package fixes

// installNonASCIIKeyboard is a no-op off the browser: the dropped non-ASCII
// input is a bug in fyne's wasm driver alone, see keyboard_wasm.go. The stub
// only exists so "go vet" and editors can still type-check this package
// natively.
func installNonASCIIKeyboard() {}
