//go:build js && wasm

package testhook

import "syscall/js"

// The browser suite reaches the page through these two globals: awgTarget
// returns a widget's rectangle (or null) and awgAction runs a named action.
// They are only compiled into the WebAssembly bundle, which is what the suite
// drives; the native build of this package carries no JS at all.
func init() {
	js.Global().Set("awgTarget", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 {
			return nil
		}
		x, y, w, h, ok := Rect(args[0].String())
		if !ok {
			return nil
		}
		return js.ValueOf(map[string]any{"x": x, "y": y, "w": w, "h": h})
	}))

	js.Global().Set("awgHas", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 {
			return false
		}
		return js.ValueOf(Has(args[0].String()))
	}))

	js.Global().Set("awgAction", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 {
			return false
		}
		return js.ValueOf(Invoke(args[0].String()))
	}))
}
