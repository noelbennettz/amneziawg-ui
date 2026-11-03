package frontend

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
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
