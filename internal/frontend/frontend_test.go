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
		"index.html":     {Data: []byte("<html>loader</html>")},
		"bundle.wasm.gz": {Data: gzipped(t, "\x00asm")},
	}
	app := fiber.New()
	assets := Open(files)
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

// loaderFixture is the loader page "fyne package" writes, trimmed to the parts
// this package cares about: the two stylesheets and the two spinners, each
// chosen by prefers-color-scheme.
const loaderFixture = `<!DOCTYPE html>
<html>
	<head>
		<style>
			body { text-align:center; }
		</style>
		<link rel="stylesheet" href="dark.css" media="(prefers-color-scheme: dark)" />
		<link rel="stylesheet" href="light.css" media="(prefers-color-scheme: light)" />
		<meta charset="utf-8">
		<link rel="icon" type="image/png" href="icon.png">
	</head>
	<body scroll="no" style="overflow: hidden">
		<div id="main">
			<picture>
				<source srcset="spinner_dark.gif" media="(prefers-color-scheme: dark)" />
				<source srcset="spinner_light.gif" media="(prefers-color-scheme: light)" />
				<img class="spinner" src="spinner_light.gif" />
			</picture>
		</div>
	</body>
</html>
`

func newLoaderApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	assets := Open(fstest.MapFS{
		"index.html": {Data: []byte(loaderFixture)},
		"dark.css":   {Data: []byte("body { background-color: #141415; }")},
		"light.css":  {Data: []byte("body { background-color: white; }")},
	})
	assets.Mount(app)
	return app
}

// The panel is always dark, so the loading screen in front of it has to be too:
// a browser set to light would otherwise be shown a white page (and a spinner
// drawn for one) between the login and the panel.
func TestLoaderIsServedInTheDarkVariant(t *testing.T) {
	app := newLoaderApp(t)

	for _, path := range []string{"/", "/index.html"} {
		resp := get(t, app, path, nil)
		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			t.Errorf("%s: %d %q", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		// No choice is left to the browser: the dark stylesheet and the dark
		// spinner are unconditional, and the light pair can never match.
		if strings.Contains(string(body), "prefers-color-scheme") {
			t.Errorf("%s: the page still asks the browser which variant it wants:\n%s", path, body)
		}
		if !strings.Contains(string(body), `href="dark.css" media="all"`) ||
			!strings.Contains(string(body), `href="light.css" media="not all"`) ||
			!strings.Contains(string(body), `srcset="spinner_dark.gif" media="all"`) {
			t.Errorf("%s: the dark branch is not pinned:\n%s", path, body)
		}
		// The page colour is the panel's, and it is the last rule in the head so
		// it wins against the dark stylesheet's own #141415.
		if !strings.Contains(string(body), `<style>html{background:#131314;color-scheme:dark}body{background:#131314}</style></head>`) {
			t.Errorf("%s: page colour not injected before </head>:\n%s", path, body)
		}
		if at, head := strings.Index(string(body), "</head>"), strings.Index(string(body), "<style>html{"); head < 0 || head > at {
			t.Errorf("%s: injected style is not inside the head", path)
		}
	}
}

// A loader page without those markers is somebody else's page - a future "fyne
// package" - and is served exactly as it is instead of being half-rewritten.
func TestLoaderWithoutTheMarkersIsServedUntouched(t *testing.T) {
	app := newApp(t) // its fixture is "<html>loader</html>"

	for _, path := range []string{"/", "/index.html"} {
		resp := get(t, app, path, nil)
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || string(body) != "<html>loader</html>" {
			t.Errorf("%s: %d %q", path, resp.StatusCode, body)
		}
	}
}
