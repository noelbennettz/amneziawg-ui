package frontend

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
)

func gzipped(t *testing.T, data string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatal(err)
	}
	w.Close()
	return buf.Bytes()
}

func newApp(t *testing.T) *fiber.App {
	t.Helper()
	files := fstest.MapFS{
		"bundle.wasm.gz":                 {Data: gzipped(t, "\x00asm")},
		"font/NotoSansSC-Regular.otf.gz": {Data: gzipped(t, "\x00otf")},
		"icon.png":                       {Data: []byte("\x89PNG")},
		"wasm_exec.js":                   {Data: []byte("// Go")},
	}
	app := fiber.New()
	assets := Open(files, "")
	app.Use(assets.Compression())
	assets.Mount(app)
	return app
}

func get(t *testing.T, app *fiber.App, path string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The bundle only exists gzipped on disk, and is served under its unpacked
// name: verbatim to a browser, inflated to a client that did not ask for
// gzip, and as application/wasm either way.
func TestPackedBundleIsServedBothWays(t *testing.T) {
	app := newApp(t)

	resp := get(t, app, "/bundle.wasm", map[string]string{"Accept-Encoding": "gzip"})
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "gzip" || resp.Header.Get("Content-Type") != "application/wasm" {
		t.Errorf("gzip client: %d %v", resp.StatusCode, resp.Header)
	}
	if r, err := gzip.NewReader(bytes.NewReader(body)); err != nil {
		t.Errorf("body is not gzip: %v", err)
	} else if raw, _ := io.ReadAll(r); string(raw) != "\x00asm" {
		t.Errorf("inflated body = %q", raw)
	}

	resp = get(t, app, "/bundle.wasm", nil)
	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "" || string(body) != "\x00asm" {
		t.Errorf("plain client: %d %v %q", resp.StatusCode, resp.Header, body)
	}
}

// Every asset carries a content-derived weak ETag, and a matching
// If-None-Match is answered 304 - under the unpacked name too.
func TestAssetsRevalidateByContentHash(t *testing.T) {
	app := newApp(t)

	for _, path := range []string{"/", "/index.html", "/bundle.wasm"} {
		resp := get(t, app, path, nil)
		tag := resp.Header.Get("ETag")
		if resp.StatusCode != 200 || tag == "" || resp.Header.Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d %v", path, resp.StatusCode, resp.Header)
			continue
		}
		if resp.Header.Get("Last-Modified") != "" {
			t.Errorf("%s: Last-Modified sent alongside the hash", path)
		}

		resp = get(t, app, path, map[string]string{"If-None-Match": tag})
		if resp.StatusCode != http.StatusNotModified {
			t.Errorf("%s with matching tag: %d, want 304", path, resp.StatusCode)
		}
	}
}

// The page has to name the assets it loads with the version the server answers
// them with, or a refresh revalidates 12 MB of wasm and 15 MB of faces instead
// of reading them from the browser's cache. The bundle and the runtime are the
// page's own references; the faces are published as metas for the wasm side.
func TestLoaderNamesVersionedAssets(t *testing.T) {
	app := newApp(t)
	body, _ := io.ReadAll(get(t, app, "/", nil).Body)
	page := string(body)

	for _, path := range []string{"/bundle.wasm", "/wasm_exec.js", "/icon.png", "/font/NotoSansSC-Regular.otf"} {
		tag := get(t, app, path, nil).Header.Get("ETag")
		if tag == "" {
			t.Fatalf("%s has no tag to name it with", path)
		}
		want := strings.TrimPrefix(path, "/") + "?v=" + versionOf(tag)
		if !strings.Contains(page, want) {
			t.Errorf("the page does not name %s:\n%s", want, page)
		}
	}

	if !strings.Contains(page, `<meta name="asset-version" content="/font/NotoSansSC-Regular.otf?v=`) {
		t.Errorf("the face is not published for the wasm side:\n%s", page)
	}
	if strings.Contains(page, "{{") {
		t.Errorf("the page still holds an unresolved placeholder:\n%s", page)
	}
}

// A URL that names the content is what a refresh asks for, and the answer can
// be kept for as long as the browser likes - the bytes behind that URL cannot
// change. A tag from another build (or no tag at all) is not that promise, so
// it keeps the revalidating answer the bare names get.
func TestVersionedAssetIsImmutable(t *testing.T) {
	app := newApp(t)

	tag := get(t, app, "/font/NotoSansSC-Regular.otf", nil).Header.Get("ETag")
	version := versionOf(tag)

	fresh := get(t, app, "/font/NotoSansSC-Regular.otf?v="+version, map[string]string{"Accept-Encoding": "gzip"})
	if fresh.StatusCode != http.StatusOK || fresh.Header.Get("Cache-Control") != immutableCacheControl {
		t.Errorf("versioned font: %d %q", fresh.StatusCode, fresh.Header.Get("Cache-Control"))
	}
	if fresh.Header.Get("ETag") != tag {
		t.Errorf("versioned font tag = %q, want %q", fresh.Header.Get("ETag"), tag)
	}

	again := get(t, app, "/font/NotoSansSC-Regular.otf?v="+version, map[string]string{"If-None-Match": tag})
	if again.StatusCode != http.StatusNotModified || again.Header.Get("Cache-Control") != immutableCacheControl {
		t.Errorf("revalidating a versioned font: %d %q", again.StatusCode, again.Header.Get("Cache-Control"))
	}

	stale := get(t, app, "/font/NotoSansSC-Regular.otf?v=0000000000000000", nil)
	if stale.StatusCode != http.StatusOK || stale.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("stale version: %d %q", stale.StatusCode, stale.Header.Get("Cache-Control"))
	}
	if bare := get(t, app, "/font/NotoSansSC-Regular.otf", nil); bare.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("bare font: %q", bare.Header.Get("Cache-Control"))
	}
}

func TestEtagMatches(t *testing.T) {
	tag := `W/"abc"`
	for header, want := range map[string]bool{
		`W/"abc"`: true, `"abc"`: true, `"x", W/"abc"`: true, "*": true, `"other"`: false, "": false,
	} {
		if got := etagMatches(header, tag); got != want {
			t.Errorf("etagMatches(%q) = %v, want %v", header, got, want)
		}
	}
}

// The loading screen is this package's own page now, not the one "fyne package"
// emits: same panel palette, a Material 3 progress indicator instead of the GIF,
// and the panel's six languages.
func TestLoaderIsThePanelsOwnPage(t *testing.T) {
	app := newApp(t)

	for _, path := range []string{"/", "/index.html"} {
		resp := get(t, app, path, nil)
		body, _ := io.ReadAll(resp.Body)
		page := string(body)

		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			t.Errorf("%s: %d %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		// The panel's page colour, not the packaged page's #141415 or a white
		// one: this is the last thing on screen before the canvas takes over.
		if !strings.Contains(page, "#131314") {
			t.Errorf("%s: the loader does not use the panel background (see #131314):\n%s", path, page)
		}
		// A tap on a phone used to flash the browser's own highlight over the
		// whole page; the canvas covers the viewport, so the panel turns it off
		// here and again in the stylesheet the wasm side injects.
		if !strings.Contains(page, "-webkit-tap-highlight-color: transparent") {
			t.Errorf("%s: a tap would still flash the browser highlight", path)
		}
		// The GIF is gone, and so is the browser's choice of variant.
		for _, gone := range []string{"spinner_dark.gif", "spinner_light.gif", "prefers-color-scheme"} {
			if strings.Contains(page, gone) {
				t.Errorf("%s: the page still carries %q:\n%s", path, gone, page)
			}
		}
		// A Material 3 indeterminate indicator: an arc on a track, animated by
		// the stylesheet rather than by a picture.
		for _, want := range []string{`class="track"`, `class="arc"`, "@keyframes dash"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the progress indicator has no %q", path, want)
			}
		}
		// The bootstrap contract with fyne: the exec shim, the bundle, the
		// hidden input the driver focuses, and the canvas watch.
		for _, want := range []string{"wasm_exec.js", "bundle.wasm", `id="dummyEntry"`, `querySelector("canvas")`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the page does not contain %q", path, want)
			}
		}
		// The six translations are in the page and picked in the browser, so
		// the loader speaks the same language as the panel behind it.
		for _, want := range []string{"Loading the interface…", "正在載入介面…", "正在加载界面…", "Загрузка интерфейса…", "Chargement de l'interface…", "Cargando la interfaz…"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the page has no %q", path, want)
			}
		}
	}
}

// The page runs under a policy that allows exactly one inline script, so that
// script has to carry the nonce - and the wasm side reads the same value back
// out of the meta tag for the elements it injects itself.
func TestLoaderCarriesTheNonce(t *testing.T) {
	const nonce = "dGVzdC1ub25jZQ"

	app := fiber.New()
	assets := Open(fstest.MapFS{}, nonce)
	assets.Mount(app)

	if got := assets.Nonce(); got != nonce {
		t.Fatalf("Nonce() = %q, want %q", got, nonce)
	}

	body, _ := io.ReadAll(get(t, app, "/", nil).Body)
	page := string(body)
	for _, want := range []string{
		`<script nonce="` + nonce + `">`,
		`<meta name="csp-nonce" content="` + nonce + `">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the served page does not contain %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "{{") {
		t.Errorf("a placeholder was left in the page:\n%s", page)
	}

	// Without a nonce the attribute is gone and the policy falls back to
	// allowing inline scripts; the page has to stay usable that way too.
	plain := Open(fstest.MapFS{}, "")
	if plain.Nonce() != "" {
		t.Errorf("Nonce() = %q without one", plain.Nonce())
	}
	plainPage := string(plain.loader)
	if strings.Contains(plainPage, "csp-nonce") || strings.Contains(plainPage, "nonce=") {
		t.Errorf("a page without a nonce still names one:\n%s", plainPage)
	}
}

// The nonce lives in the page, so the page's validator has to change when the
// nonce does. A tag taken from the file on disk would survive a restart, the
// browser would revalidate against it, and the 304 would hand back a page whose
// script the new policy refuses - the panel would load nothing until someone
// forced a reload.
func TestLoaderETagFollowsTheNonce(t *testing.T) {
	etagOf := func(nonce string) string {
		t.Helper()
		app := fiber.New()
		Open(fstest.MapFS{}, nonce).Mount(app)
		return get(t, app, "/", nil).Header.Get("ETag")
	}

	first, second := etagOf("bm9uY2Utb25l"), etagOf("bm9uY2UtdHdv")
	if first == second {
		t.Fatalf("the tag did not change with the nonce: %s", first)
	}

	// Both names answer with the page that is served, so both carry its tag.
	app := fiber.New()
	Open(fstest.MapFS{}, "bm9uY2U").Mount(app)
	slash := get(t, app, "/", nil).Header.Get("ETag")
	index := get(t, app, "/index.html", nil).Header.Get("ETag")
	if slash != index {
		t.Errorf("/ and /index.html disagree on the tag: %q vs %q", slash, index)
	}

	// A stale tag from a previous process - the same one the file would have
	// produced - must not be answered 304.
	if resp := get(t, app, "/", map[string]string{"If-None-Match": `W/"0000000000000000"`}); resp.StatusCode != 200 {
		t.Errorf("an unknown tag = %d, want 200", resp.StatusCode)
	}
	if resp := get(t, app, "/", map[string]string{"If-None-Match": slash}); resp.StatusCode != 304 {
		t.Errorf("the current tag = %d, want 304", resp.StatusCode)
	}
}

// The packed bundle only exists gzipped; a HEAD request has to find it the
// same way a GET does, because a client that asks what it would get is not
// asking for a 404.
func TestPackedAssetAnswersHead(t *testing.T) {
	app := newApp(t)
	req := httptest.NewRequest(http.MethodHead, "/bundle.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD /bundle.wasm = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "wasm") {
		t.Errorf("Content-Type = %q, want the wasm type", got)
	}
}

// The CJK faces ship the way the bundle does - gzipped on disk, served under
// the unpacked name - but from a subdirectory, which the packing and the
// validator both have to walk into or a Chinese panel would fetch a 404 and
// draw boxes. They are also worth caching: the browser has to keep them across
// an upgrade, so they revalidate by the same content hash.
func TestPackedFontIsServedFromSubdirectory(t *testing.T) {
	app := newApp(t)

	resp := get(t, app, "/font/NotoSansSC-Regular.otf", map[string]string{"Accept-Encoding": "gzip"})
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip client: %d %v", resp.StatusCode, resp.Header)
	}
	// Go's mime table calls .otf an OpenDocument formula template; Open fixes
	// that, or a browser would be told the panel's face is a spreadsheet.
	if ct := resp.Header.Get("Content-Type"); ct != "font/otf" {
		t.Errorf("Content-Type = %q, want font/otf", ct)
	}
	raw, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	if inflated, _ := io.ReadAll(raw); string(inflated) != "\x00otf" {
		t.Errorf("inflated body = %q", inflated)
	}

	tag := resp.Header.Get("ETag")
	if tag == "" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("no validator for a packed font: %q %q", tag, resp.Header.Get("Cache-Control"))
	}
	if again := get(t, app, "/font/NotoSansSC-Regular.otf", map[string]string{"If-None-Match": tag}); again.StatusCode != http.StatusNotModified {
		t.Errorf("a matching tag = %d, want 304", again.StatusCode)
	}
}
