//go:build !js || !wasm

package fixes

// installViewport and installTouchInput are no-ops off the browser: the missing
// viewport meta, the on-screen keyboard and drag-to-scroll are properties of a
// page, and none of them exist in a desktop window. The stubs only exist so "go
// vet" and editors can still type-check this package natively; see
// mobile_wasm.go and touch_wasm.go.
func installViewport() {}

func installTouchInput() {}
