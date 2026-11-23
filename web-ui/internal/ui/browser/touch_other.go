//go:build !js || !wasm

package browser

// installTouchInput is a no-op off the browser: the on-screen keyboard and
// drag-to-scroll are phone-page problems, and a native window has neither. The
// stub keeps `go vet` and editors able to type-check this package; see
// touch_wasm.go.
func installTouchInput() {}
