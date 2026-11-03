// Package testhook names the interactive parts of the page so the browser
// suite can reach them without a coordinate.
//
// The frontend draws into one WebAssembly canvas, so there is no DOM to select:
// a test that clicks by coordinate breaks every time a card grows a row or a
// button moves into a menu, and every redesign costs a round of measuring. A
// component registers the widget it wants reachable under a stable id instead,
// and the suite asks for that widget's rectangle (or runs the action, for the
// things that are not a clickable widget of their own).
//
// Registration is a plain map: a widget that is rebuilt registers again and the
// id keeps pointing at the newest one, and an id whose widget has been hidden
// simply reports no rectangle.
package testhook

import (
	"sync"

	"fyne.io/fyne/v2"
)

var (
	mu      sync.RWMutex
	targets = map[string]fyne.CanvasObject{}
	actions = map[string]func(){}
)

// Register names a widget, so a test can click its centre.
func Register(id string, obj fyne.CanvasObject) {
	if id == "" || obj == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	targets[id] = obj
}

// RegisterAction names an operation that has no widget of its own to click:
// switching a tab, choosing an entry of a pop-up menu that is not open yet.
func RegisterAction(id string, do func()) {
	if id == "" || do == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	actions[id] = do
}

// Rect is the widget's rectangle on the canvas, in points, or false for an id
// that is unknown or whose widget is not visible.
func Rect(id string) (x, y, w, h float32, ok bool) {
	mu.RLock()
	obj := targets[id]
	mu.RUnlock()
	if obj == nil || !obj.Visible() {
		return 0, 0, 0, 0, false
	}

	app := fyne.CurrentApp()
	if app == nil {
		return 0, 0, 0, 0, false
	}
	pos := app.Driver().AbsolutePositionForObject(obj)
	size := obj.Size()
	// A widget inside a collapsed block keeps a size but is never placed, so
	// it sits at the origin with nothing drawn there. Reporting that position
	// would send the click to the top-left corner of the page.
	if size.Width <= 0 || size.Height <= 0 || (pos.X == 0 && pos.Y == 0) {
		return 0, 0, 0, 0, false
	}
	return pos.X, pos.Y, size.Width, size.Height, true
}

// Has reports whether an action or a target is registered under id, without
// doing anything. A test uses it to tell "the widget is gone" from "the click
// landed nowhere".
func Has(id string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, isTarget := targets[id]
	_, isAction := actions[id]
	return isTarget || isAction
}

// Invoke runs the action registered under id, reporting whether there was one.
func Invoke(id string) bool {
	mu.RLock()
	do := actions[id]
	mu.RUnlock()
	if do == nil {
		return false
	}
	do()
	return true
}
