// Package frontend serves the packaged web UI straight off disk: the loader
// page, the wasm bundle and its assets, exactly as "fyne package -os wasm"
// wrote them - except that the Docker build leaves the bundle gzipped.
// Nothing is baked into the binary, so the bundle can be swapped without
// relinking the server.
package frontend

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"log"
	"mime"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// Assets is one directory of frontend files and the index of those that sit
// on disk pre-compressed.
type Assets struct {
	files  fs.FS
	packed map[string]string // request path -> .gz file answering it
	// loader is the page served for "/" and "/index.html": the packaged one,
	// repainted. Nil when the tree has no index.html at all.
	loader []byte
	// nonce is what the loader page carries when it could be prepared for a
	// strict CSP, and empty when it could not - see Nonce.
	nonce string
}

// Open indexes the directory. A tree without any .gz - a development build -
// simply has nothing packed, and every asset takes the ordinary static path.
//
// nonce is the script nonce the loader page is rewritten around. An empty one
// leaves the page exactly as packaged.
func Open(files fs.FS, nonce string) *Assets {
	page, ready := loaderPage(files, nonce)
	a := &Assets{files: files, packed: packedAssets(files, "."), loader: page}
	if ready {
		a.nonce = nonce
	}
	return a
}

// Nonce is what the security headers have to name in the page's policy, or
// empty when the packaged loader no longer has the markers the rewrite needs.
// The headers then fall back to allowing inline scripts rather than breaking a
// page they cannot see into.
func (a *Assets) Nonce() string {
	return a.nonce
}

// Compression is the compress middleware configured for these assets. The
// wasm bundle is tens of megabytes uncompressed, so compression is not
// optional; the assets that already sit on disk gzipped are excluded, since
// the packed handler hands those bytes over verbatim and re-encoding them
// would be wasted work.
func (a *Assets) Compression() fiber.Handler {
	return compress.New(compress.Config{
		Next: func(c fiber.Ctx) bool {
			_, ok := a.packed[c.Path()]
			return ok
		},
	})
}

// Mount registers the asset handlers. It goes last: everything the REST
// routes did not claim is a frontend asset.
func (a *Assets) Mount(app *fiber.App) {
	app.Use(a.cache())
	app.Use(a.sendPacked())
	app.Use(a.sendLoader())
	app.Get("/*", static.New("", static.Config{
		FS:         a.files,
		IndexNames: []string{"index.html"},
		Browse:     false,
	}))
}

// The loader page "fyne package" writes chooses between its two stylesheets and
// its two spinners with prefers-color-scheme, and the panel behind it forces the
// dark variant whatever the browser asks for. So the choice is made here
// instead of in the browser: the dark branch is pinned to every colour scheme,
// or a browser set to light shows a white page - and then a spinner drawn for a
// white page on a white page - between the login and the panel.
const (
	darkMedia  = `media="(prefers-color-scheme: dark)"`
	lightMedia = `media="(prefers-color-scheme: light)"`
	anyMedia   = `media="all"`
	noMedia    = `media="not all"`

	// loaderBackground is the page colour of the panel
	// (web-ui/internal/ui/style/style.go) and of the login page in front of it
	// (internal/auth/login.html). The loader's own dark stylesheet says
	// #141415, close but not the same, and it is the last thing on screen
	// before the canvas takes over.
	loaderBackground = "#131314"
)

// loaderPage reads the packaged index.html, or nil when the tree has none - a
// test fixture, or a bundle that was not built yet; the static handler then
// answers the request with whatever the tree does have. ready reports whether
// the page came back prepared for a policy that allows no inline script but
// its own.
func loaderPage(files fs.FS, nonce string) (page []byte, ready bool) {
	raw, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, false
	}
	return darkLoader(raw, nonce)
}

// darkLoader pins the loading screen to the dark variant and to the page colour
// of everything around it, and - when a nonce is given - turns the page into
// one that runs under the panel's CSP.
//
// The palette of the panel is decided inside the wasm bundle, which has not
// started yet while this page is on screen, so nothing in the bundle can be
// asked to recolour it - the page has to be served that way. A page without the
// markers (a loader from a future "fyne package") is returned untouched rather
// than broken by a rewrite that no longer applies.
func darkLoader(page []byte, nonce string) ([]byte, bool) {
	if !bytes.Contains(page, []byte(darkMedia)) || !bytes.Contains(page, []byte(lightMedia)) {
		return page, false
	}

	pinned := bytes.ReplaceAll(page, []byte(darkMedia), []byte(anyMedia))
	pinned = bytes.ReplaceAll(pinned, []byte(lightMedia), []byte(noMedia))

	ready := false
	if nonce != "" {
		pinned, ready = nonceLoader(pinned, nonce)
	}

	// Last rule in the head: the dark stylesheet sets the body to its own
	// #141415, and the injection has to win that tie on specificity.
	injected := `<style>html{background:` + loaderBackground + `;color-scheme:dark}body{background:` + loaderBackground + `}</style>`
	if ready {
		// The wasm bundle injects a <script> of its own once it starts (the
		// phone input and drag handling); it reads this to give that script
		// the nonce the policy names.
		injected = `<meta name="csp-nonce" content="` + nonce + `">` + injected
	}
	if at := bytes.Index(pinned, []byte("</head>")); at >= 0 {
		pinned = append(pinned[:at:at], append([]byte(injected), pinned[at:]...)...)
	}
	return pinned, ready
}

// nonceLoader makes the packaged loader run under a policy that allows exactly
// one inline script: the page's own.
//
// "fyne package" writes the startup code into an inline <script> and calls it
// from an onload attribute; CSP allows neither without help, and an attribute
// cannot be rescued by a nonce at all. So the attribute comes out - the call
// moves into the script as a load listener - and the element gets the nonce the
// panel's policy names.
func nonceLoader(page []byte, nonce string) ([]byte, bool) {
	const (
		openTag  = "<script>"
		closeTag = "</script>"
		onload   = ` onload="download_application()"`
	)
	if !bytes.Contains(page, []byte(onload)) {
		return page, false
	}

	open := bytes.Index(page, []byte(openTag))
	if open < 0 {
		return page, false
	}
	body := open + len(openTag)
	closing := bytes.Index(page[body:], []byte(closeTag))
	if closing < 0 {
		return page, false
	}
	closing += body

	code := append([]byte("\nwindow.addEventListener('load', download_application);\n"), page[body:closing]...)

	out := make([]byte, 0, len(page)+len(nonce)+len(code))
	out = append(out, page[:open]...)
	out = append(out, `<script nonce="`+nonce+`">`...)
	out = append(out, code...)
	out = append(out, page[closing:]...)

	return bytes.Replace(out, []byte(onload), nil, 1), true
}

// sendLoader answers / and /index.html with that page. It goes before the
// static handler, which would serve the file as it sits on disk.
func (a *Assets) sendLoader() fiber.Handler {
	return func(c fiber.Ctx) error {
		if a.loader == nil || (c.Path() != "/" && c.Path() != "/index.html") {
			return c.Next()
		}
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return c.Next()
		}

		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.Send(a.loader)
	}
}

// packedAssets maps a request path to the pre-compressed file that answers it.
// "make web-ui" leaves the bundle on disk uncompressed, but the Docker build
// gzips it and the image ships bundle.wasm.gz alone: that is 14 MB instead of
// 49 MB of layer, and it spares the server from gzipping the same 49 MB on
// every cold request.
func packedAssets(files fs.FS, root string) map[string]string {
	packed := map[string]string{}

	entries, err := fs.ReadDir(files, root)
	if err != nil {
		log.Printf("frontend assets: %v", err)
		return packed
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".gz") {
			continue
		}
		packed["/"+strings.TrimSuffix(name, ".gz")] = path.Join(root, name)
	}
	return packed
}

// sendPacked answers a request for an asset that only exists gzipped on disk.
// The static handler cannot: it looks for a file named exactly like the path
// and would return a 404 for /bundle.wasm.
func (a *Assets) sendPacked() fiber.Handler {
	return func(c fiber.Ctx) error {
		name, ok := a.packed[c.Path()]
		if !ok || c.Method() != fiber.MethodGet {
			return c.Next()
		}

		file, err := a.files.Open(name)
		if err != nil {
			return c.Next()
		}

		// The content type is the one of the asset, not of the envelope:
		// instantiateStreaming in the loader page refuses a bundle that does
		// not arrive as application/wasm.
		if ct := mime.TypeByExtension(path.Ext(strings.TrimSuffix(name, ".gz"))); ct != "" {
			c.Set(fiber.HeaderContentType, ct)
		}
		c.Vary(fiber.HeaderAcceptEncoding)

		// Browsers all advertise gzip, but curl sends no Accept-Encoding at
		// all unless asked to, and the bundle has to stay readable by a client
		// that will not decompress it itself.
		if !strings.Contains(c.Get(fiber.HeaderAcceptEncoding), "gzip") {
			unpacked, err := gzip.NewReader(file)
			if err != nil {
				file.Close() //nolint:errcheck
				return err
			}
			return c.SendStream(gzipFile{Reader: unpacked, file: file})
		}

		c.Set(fiber.HeaderContentEncoding, "gzip")

		info, err := fs.Stat(a.files, name)
		if err != nil {
			return c.SendStream(file)
		}
		return c.SendStream(file, int(info.Size()))
	}
}

// gzipFile ties the decompressor to the file underneath it. fasthttp closes
// the body stream it is handed if it is an io.Closer, and closing a gzip
// reader on its own leaks the descriptor it was reading from.
type gzipFile struct {
	*gzip.Reader
	file fs.File
}

func (g gzipFile) Close() error {
	err := g.Reader.Close()
	if cerr := g.file.Close(); err == nil {
		err = cerr
	}
	return err
}

// cache makes the browser revalidate the frontend instead of caching it
// blindly. The file names are the same in every release, so without a
// content-derived validator a browser would happily keep running the
// previous bundle after an upgrade - and re-downloading ~50 MB of wasm on
// every page load is not an acceptable alternative.
func (a *Assets) cache() fiber.Handler {
	tags := map[string]string{}

	entries, err := fs.ReadDir(a.files, ".")
	if err != nil {
		log.Printf("frontend cache: %v", err)
		return func(c fiber.Ctx) error { return c.Next() }
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		tag, err := fileETag(a.files, entry.Name())
		if err != nil {
			log.Printf("frontend cache: %v", err)
			continue
		}
		tags["/"+entry.Name()] = tag
	}

	// A packed asset is requested under the name it will have once unpacked,
	// so its tag has to answer to that path too. The tag is weak, which is
	// exactly what lets one identifier stand for both encodings of the bundle.
	for requested, name := range a.packed {
		tags[requested] = tags["/"+path.Base(name)]
	}
	tags["/"] = tags["/index.html"]

	return func(c fiber.Ctx) error {
		tag, ok := tags[c.Path()]
		if !ok || tag == "" || c.Method() != fiber.MethodGet {
			return c.Next()
		}

		c.Set(fiber.HeaderCacheControl, "no-cache")
		c.Set(fiber.HeaderETag, tag)
		if etagMatches(c.Get(fiber.HeaderIfNoneMatch), tag) {
			return c.SendStatus(fiber.StatusNotModified)
		}

		// The content hash is the only validator worth trusting here: a
		// modification time survives a copy (docker COPY, rsync -a) that
		// replaces the file contents, and the static handler below would
		// answer such an If-Modified-Since with a 304 over a bundle the
		// browser has never seen. So the timestamp is dropped from both ends
		// of the exchange rather than left to contradict the hash.
		c.Request().Header.Del(fiber.HeaderIfModifiedSince)
		err := c.Next()
		c.Response().Header.Del(fiber.HeaderLastModified)
		return err
	}
}

// etagMatches reports whether an If-None-Match header covers tag. Browsers
// echo back what they were sent, but the header is a list and the weak marker
// is not part of the comparison, so both are handled.
func etagMatches(header, tag string) bool {
	tag = strings.TrimPrefix(tag, "W/")
	for candidate := range strings.SplitSeq(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == tag {
			return true
		}
	}
	return false
}

func fileETag(files fs.FS, name string) (string, error) {
	file, err := files.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}

	// Weak on purpose. The tag hashes the file as it sits on disk, while what goes
	// on the wire is usually gzipped - a difference a strong tag is not
	// allowed to gloss over. It also keeps the compression middleware from
	// replacing the tag with a hash of the compressed body: that one changes
	// with the compression settings, and it is the wrong thing to compare a
	// cached copy against.
	return `W/"` + hex.EncodeToString(sum.Sum(nil)[:8]) + `"`, nil
}
