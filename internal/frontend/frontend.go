// Package frontend serves the web UI straight off disk: the wasm bundle and its
// assets as "fyne package -os wasm" wrote them, the CJK faces beside it (see
// web-ui/internal/ui/style) and a loading page this repository writes itself
// (loader.go) - except that the Docker build leaves the bundle and the faces
// gzipped. Nothing is baked into the binary, so the frontend can be swapped
// without relinking the server.
package frontend

import (
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
	// loader is the page served for "/" and "/index.html", with the process's
	// nonce written into it. This package writes it; see loader.go.
	loader []byte
	// nonce is the value that page carries, and empty when the server was
	// started without one - see Nonce.
	nonce string
}

// Open indexes the directory. A tree without any .gz - a development build -
// simply has nothing packed, and every asset takes the ordinary static path.
//
// nonce is the script nonce the loader page carries. An empty one leaves the
// attribute off, and the policy allows inline scripts instead.
func Open(files fs.FS, nonce string) *Assets {
	// Go's mime table maps .otf to the OpenDocument formula template type, which
	// is what a browser would be told the panel's CJK face is. "font/otf" is
	// what the extension means and what both the packed and the plain static
	// handler answer with after this.
	mime.AddExtensionType(".otf", "font/otf") //nolint:errcheck // the extension is valid

	return &Assets{
		files:  files,
		packed: packedAssets(files, "."),
		loader: loaderPage(nonce),
		nonce:  nonce,
	}
}

// Nonce is what the security headers have to name in the page's policy. Empty
// when the server was started without one - a development build, or a test -
// in which case the policy falls back to allowing inline scripts.
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

// The loader page itself lives in loader.go: this package writes it rather
// than repainting the one "fyne package" emits, so the palette, the progress
// indicator and the language are the panel's own.

// sendLoader answers / and /index.html with that page. It goes before the
// static handler, which would serve the file as it sits on disk.
func (a *Assets) sendLoader() fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Path() != "/" && c.Path() != "/index.html" {
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

	// The walk goes into subdirectories: the CJK faces sit in font/ next to the
	// bundle. A request always names the asset by the path it has once
	// unpacked - "/font/NotoSansSC-Regular.otf" for "font/NotoSansSC-Regular.otf.gz"
	// - whichever directory it lives in.
	err := fs.WalkDir(files, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Printf("frontend assets: %s: %v", name, walkErr)
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".gz") {
			return nil
		}
		packed["/"+strings.TrimSuffix(name, ".gz")] = name
		return nil
	})
	if err != nil {
		log.Printf("frontend assets: %v", err)
	}
	return packed
}

// sendPacked answers a request for an asset that only exists gzipped on disk.
// The static handler cannot: it looks for a file named exactly like the path
// and would return a 404 for /bundle.wasm.
func (a *Assets) sendPacked() fiber.Handler {
	return func(c fiber.Ctx) error {
		name, ok := a.packed[c.Path()]
		if !ok || (c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead) {
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

	// Every file, in every directory: the faces under font/ are revalidated
	// like the bundle is, so a browser that has one keeps it across an upgrade
	// unless the bytes really changed.
	err := fs.WalkDir(a.files, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Printf("frontend cache: %s: %v", name, walkErr)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		tag, err := fileETag(a.files, name)
		if err != nil {
			log.Printf("frontend cache: %s: %v", name, err)
			return nil
		}
		tags["/"+name] = tag
		return nil
	})
	if err != nil {
		log.Printf("frontend cache: %v", err)
		return func(c fiber.Ctx) error { return c.Next() }
	}

	// A packed asset is requested under the name it will have once unpacked,
	// so its tag has to answer to that path too. The tag is weak, which is
	// exactly what lets one identifier stand for both encodings of the bundle.
	for requested := range a.packed {
		tags[requested] = tags[requested+".gz"]
	}
	tags["/"] = tags["/index.html"]

	// The loader page is not served as it sits on disk: it is rewritten at
	// start-up around a nonce, and that nonce changes when the process
	// restarts. Its validator therefore has to be the hash of the bytes that
	// go on the wire. A tag taken from the file would survive a restart, a
	// browser would revalidate against it, and the 304 would hand back a page
	// whose nonce the new policy does not name - so the panel would load
	// nothing at all until someone forced a reload.
	if a.loader != nil {
		sum := sha256.Sum256(a.loader)
		tag := weakTag(sum[:])
		tags["/index.html"] = tag
		tags["/"] = tag
	}

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
	return weakTag(sum.Sum(nil)), nil
}

// weakTag turns a digest - of a file, or of the bytes about to be served - into
// the validator that answers for it.
//
// Weak on purpose. The tag hashes the content as it sits on disk or in memory,
// while what goes on the wire is usually gzipped - a difference a strong tag is
// not allowed to gloss over. It also keeps the compression middleware from
// replacing the tag with a hash of the compressed body: that one changes with
// the compression settings, and it is the wrong thing to compare a cached copy
// against.
func weakTag(digest []byte) string {
	if len(digest) > 8 {
		digest = digest[:8]
	}
	return `W/"` + hex.EncodeToString(digest) + `"`
}
