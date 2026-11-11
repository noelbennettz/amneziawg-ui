//go:build js && wasm

package fixes

import (
	"syscall/js"

	"amneziawg-web-ui/web-ui/internal/ui/browser"
)

// wheelScript is the page-side half of installWheelInput.
//
// It does two things, both of them to the driver's own wheel listener
// (github.com/fyne-io/glfw-js@v0.4.0, browser_wasm.go):
//
// Make its preventDefault land. The listener is registered on the document
// with no options, and since Chrome 73 a wheel or touchmove listener on
// window, document or body is passive unless it says otherwise
// (chromestatus.com/feature/6662647093133312). preventDefault in a passive
// listener is thrown away and logs "[Intervention] Unable to preventDefault
// inside passive event listener" for every event - one line per wheel tick,
// which on a scroll is a console in flames and work in the main thread that
// buys nothing. Only the document's own addEventListener is wrapped, only for
// the passive-by-default types, and only when the caller left the flag out:
// an explicit { passive: true } is still respected (the touch fix below uses
// it).
//
// Take the wheel input over. glfw-js hands the driver the browser's delta as
// if it were a GLFW notch offset - the deltaMode switch that would convert it
// is commented out upstream, so the multiplier is always 1 - and fyne's
// scroll arithmetic (internal/driver/glfw/window.go, processMouseScrolled)
// then multiplies any offset of 5 or more by 10 and every offset by 0.2
// (scroll_speed_wasm.go). A wheel notch is 100 pixels in Chrome, so it
// arrives as 200 points: eight times the 25 a notch moves a fyne window on
// the desktop. A trackpad sends the same 100 pixels spread over dozens of
// events of one or two pixels, and those fall under the cutoff, so each one
// moves 0.2 to 0.8 points instead. Scrolling therefore jumps in 200 point
// steps, with an occasional crawl between them, and every event costs a
// goroutine and a full walk of the object tree before the frame it dirties.
//
// So the real events are swallowed here, their deltas converted to points
// (one per CSS pixel, 16 per line for the browsers that report deltaMode 1)
// and summed, and at most one wheel event per animation frame is dispatched
// to the canvas carrying the total. The driver still does the scrolling and
// still picks the widget under the pointer - it only ever sees one event a
// frame, of a size it turns into exactly the points that were asked for: it
// doubles anything from 5 up, hence the halving below, and the leftover below
// 10 points waits for the next event rather than being rounded up into a jump.
//
// On a phone this is also the path the touch fix feeds: its synthetic wheel
// events are coalesced here like real ones, so a drag costs one event a frame
// instead of one per touchmove.
//
// What is lost: with no native scrolling left in the page (installViewport
// hides the overflow) nothing needs preventDefault on the real events, so they
// are not cancelled - if the page ever becomes scrollable again the browser
// would scroll it as well. Remove once upstream reads WheelEvent.deltaMode,
// and once the registration on the document passes { passive: false }.
const wheelScript = `
(function () {
	var doc = document;

	var add = doc.addEventListener;
	var passiveByDefault = ['wheel', 'mousewheel', 'touchstart', 'touchmove'];
	doc.addEventListener = function (type, fn, opts) {
		var unset = opts === undefined || opts === null || typeof opts === 'boolean' ||
			opts.passive === undefined;
		if (passiveByDefault.indexOf(type) >= 0 && unset) {
			opts = {
				capture: typeof opts === 'boolean' ? opts : !!(opts && opts.capture),
				passive: false
			};
		}
		return add.call(doc, type, fn, opts);
	};

	// One point per CSS pixel, which is the unit the canvas is drawn in; a
	// browser that reports lines or pages gets a plausible size for each.
	var PIXEL_PER_LINE = 16;
	var PIXEL_PER_PAGE = 800;

	// The driver doubles every offset from 5 up before scaling it by 0.2, so
	// the event that carries n points has to say n/2, and anything under 10
	// points cannot be said at all.
	var MIN_POINTS = 10;

	var pendingX = 0, pendingY = 0, frame = 0;

	var points = function (e) {
		var toPixel = 1;
		if (e.deltaMode === 1) {
			toPixel = PIXEL_PER_LINE;
		} else if (e.deltaMode === 2) {
			toPixel = PIXEL_PER_PAGE;
		}
		return [e.deltaX * toPixel, e.deltaY * toPixel];
	};

	var flush = function () {
		frame = 0;
		if (Math.abs(pendingX) < MIN_POINTS && Math.abs(pendingY) < MIN_POINTS) {
			return; // too small to say: it rides along with the next event
		}

		var canvas = doc.querySelector('canvas');
		if (!canvas) {
			pendingX = pendingY = 0;
			return;
		}

		var ev = new WheelEvent('wheel', {
			deltaX: pendingX / 2,
			deltaY: pendingY / 2,
			deltaMode: 0,
			bubbles: true,
			cancelable: true
		});
		ev.__awgWheel = true;
		pendingX = pendingY = 0;
		canvas.dispatchEvent(ev);
	};

	doc.addEventListener('wheel', function (e) {
		if (e.__awgWheel) {
			return; // ours, on its way to the driver
		}
		if (!doc.querySelector('canvas')) {
			return;
		}

		var p = points(e);
		pendingX += p[0];
		pendingY += p[1];
		e.stopImmediatePropagation();
		if (!frame) {
			frame = window.requestAnimationFrame(flush);
		}
	}, { capture: true, passive: true });
})();
`

// installWheelInput replaces the page's wheel handling with one that scrolls
// in the same steps a desktop fyne window does, one event per frame; see
// wheel_wasm.go on the driver's own handling for the reasons.
func installWheelInput() {
	document := js.Global().Get("document")
	script := document.Call("createElement", "script")
	script.Set("textContent", wheelScript)
	// The page's policy allows one inline script by nonce; this is another
	// one, so it has to carry the value the server put in the markup.
	if nonce := browser.CSPNonce(); nonce != "" {
		script.Set("nonce", nonce)
	}
	document.Get("head").Call("appendChild", script)
}
