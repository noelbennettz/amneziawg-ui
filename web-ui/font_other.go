//go:build !(js && wasm)

package main

// installCJKFaces has nothing to do outside the browser: the wasm build is the
// only frontend there is, and the host build exists for tests and tooling that
// never draw a window.
func installCJKFaces() error { return nil }
