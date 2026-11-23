//go:build !js || !wasm

package browser

// installViewport is a no-op off the browser: the missing viewport meta, the
// page colour and the tap highlight are properties of a document, and a native
// window has none of them. The stub keeps `go vet` and editors able to
// type-check this package; see viewport_wasm.go.
func installViewport() {}
