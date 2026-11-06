//go:build !js || !wasm

package fixes

// installWheelInput is a no-op off the browser: the wheel deltas the page has
// to convert are the ones fyne's wasm driver reads through glfw-js, see
// wheel_wasm.go. The stub only exists so "go vet" and editors can still
// type-check this package natively.
func installWheelInput() {}
