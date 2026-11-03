//go:build js && wasm

package fixes

import (
	"syscall/js"
	"time"

	"fyne.io/fyne/v2"
)

// touchScript is the page-side half of installTouchInput. It waits for the
// driver's canvas, which is created when the window is shown - later than this
// code runs.
const touchScript = `
(function () {
	var dummy = document.getElementById('dummyEntry');
	if (dummy) {
		// The loader page parks it outside the viewport; a phone will not
		// focus an off-screen control, so it sits in the corner instead.
		dummy.style.cssText = 'position:fixed;left:0;bottom:0;width:1px;height:1px;' +
			'padding:0;margin:0;border:0;opacity:0;font-size:16px;background:transparent;' +
			'color:transparent;caret-color:transparent;';
		dummy.setAttribute('autocomplete', 'off');
		dummy.setAttribute('autocapitalize', 'none');
		dummy.setAttribute('autocorrect', 'off');
		dummy.setAttribute('spellcheck', 'false');
		dummy.setAttribute('tabindex', '-1');
		dummy.setAttribute('aria-hidden', 'true');

		var composing = false;
		var send = function (text) {
			if (text && window.awgType) window.awgType(text);
		};

		// A tablet with a keyboard attached sends both a keydown, which the
		// driver already turns into a rune, and the input event below - the
		// same character twice. The keydown is stamped, and an input event
		// that follows one closely is left alone.
		var printedKeyAt = 0;

		dummy.addEventListener('compositionstart', function () { composing = true; });
		dummy.addEventListener('compositionend', function (e) {
			composing = false;
			dummy.value = '';
			send(e.data);
		});
		dummy.addEventListener('input', function (e) {
			var text = dummy.value;
			dummy.value = '';
			if (composing || e.isComposing) return;
			if (Date.now() - printedKeyAt < 100) return;
			send(text);
		});
		dummy.addEventListener('keydown', function (e) {
			if (e.key && Array.from(e.key).length === 1) printedKeyAt = Date.now();
			// A soft keyboard reports characters through 'input' and sends
			// no usable keydown for them; only the keys that produce no text
			// are handled here, and they are kept from doing anything to the
			// input itself.
			var keys = ['Backspace', 'Delete', 'Enter', 'Escape', 'Tab', 'Home', 'End',
				'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'];
			if (keys.indexOf(e.key) < 0) return;
			e.preventDefault();
			if (window.awgKey) window.awgKey(e.key);
		});

		// The tap that focuses a field has to reach the input inside the
		// gesture that started it, or the phone refuses to open the
		// keyboard: by the next frame Fyne has processed the tap, so ask
		// then rather than waiting for the focus poller.
		document.addEventListener('pointerup', function () {
			requestAnimationFrame(function () {
				if (window.awgFocused && window.awgFocused()) dummy.focus();
			});
		}, true);
	}

	var attach = function () {
		var canvas = document.querySelector('canvas');
		if (!canvas) return false;

		var startX = 0, startY = 0, lastY = 0, dragging = false;
		// The driver scrolls whatever sits under the cursor it last saw, and a
		// touch never moves it: the mousemove in front of every wheel event is
		// what tells it where the finger is. Without it a drag scrolls the
		// top-left corner of the page, which is to say nothing at all.
		var point = function (x, y) {
			document.dispatchEvent(new MouseEvent('mousemove', {
				clientX: x, clientY: y, bubbles: true, cancelable: true
			}));
		};
		var wheel = function (dy, x, y) {
			point(x, y);
			canvas.dispatchEvent(new WheelEvent('wheel', {
				deltaY: dy, deltaMode: 0, clientX: x, clientY: y,
				bubbles: true, cancelable: true
			}));
		};

		canvas.addEventListener('touchstart', function (e) {
			if (e.touches.length !== 1) return;
			var t = e.touches[0];
			startX = t.clientX;
			startY = lastY = t.clientY;
			dragging = false;
		}, { passive: true });

		canvas.addEventListener('touchmove', function (e) {
			if (e.touches.length !== 1) return;
			var t = e.touches[0];
			if (!dragging) {
				var dy = Math.abs(t.clientY - startY);
				if (dy < 8 || Math.abs(t.clientX - startX) > dy) return;
				dragging = true;
			}
			e.preventDefault();
			wheel(lastY - t.clientY, t.clientX, t.clientY);
			lastY = t.clientY;
		}, { passive: false });

		return true;
	};

	if (!attach()) {
		var observer = new MutationObserver(function () {
			if (attach()) observer.disconnect();
		});
		observer.observe(document.body, { childList: true, subtree: true });
	}
})();
`

// followFocus keeps the hidden input in step with the page's focus: focused
// while a field wants keys, and blurred - which puts the keyboard away - when
// nothing does.
func followFocus() {
	shown := false
	for range time.Tick(120 * time.Millisecond) {
		focused := widgetFocused()
		if focused == shown {
			continue
		}
		shown = focused

		dummy := js.Global().Get("document").Call("getElementById", "dummyEntry")
		if !dummy.Truthy() {
			continue
		}
		if focused {
			dummy.Call("focus")
		} else {
			dummy.Call("blur")
		}
	}
}

// widgetFocused reports whether any window has a focused widget.
func widgetFocused() bool {
	app := fyne.CurrentApp()
	if app == nil {
		return false
	}
	for _, w := range app.Driver().AllWindows() {
		if c := w.Canvas(); c != nil && c.Focused() != nil {
			return true
		}
	}
	return false
}

// typeKey repeats what the driver does with a named key: it goes to the focused
// widget.
func typeKey(name string) {
	key := &fyne.KeyEvent{Name: fyne.KeyName(name)}
	switch name {
	case "Escape":
		key.Name = fyne.KeyEscape
	case "Enter":
		key.Name = fyne.KeyReturn
	case "Tab":
		key.Name = fyne.KeyTab
	case "Home":
		key.Name = fyne.KeyHome
	case "End":
		key.Name = fyne.KeyEnd
	}

	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	for _, w := range app.Driver().AllWindows() {
		c := w.Canvas()
		if c == nil {
			continue
		}
		if focused := c.Focused(); focused != nil {
			focused.TypedKey(key)
			return
		}
	}
}

// isTouchDevice is the gate for everything above: a phone, a tablet, or a
// browser on a touch screen.
func isTouchDevice() bool {
	if app := fyne.CurrentApp(); app != nil {
		if device := app.Driver().Device(); device != nil && device.IsMobile() {
			return true
		}
	}
	return js.Global().Get("navigator").Get("maxTouchPoints").Int() > 0
}
