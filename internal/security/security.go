// Package security adds the response headers that keep the panel out of a
// foreign frame, out of a MIME-sniffed script tag, and out of a page that
// would otherwise let an injected script run.
package security

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/gofiber/fiber/v3"
)

// NewNonce returns the per-process nonce the loader page and the wasm bundle
// use. Sixteen bytes of randomness: long enough that it cannot be guessed,
// short enough to sit in a header.
//
// The value is stable for the life of the process on purpose - the loader
// page is cached in memory and carries the nonce in its markup, so a nonce
// that changed per response would have to break that cache. It still stops
// every blind "<script>" or "<img onerror>" an attacker can inject, which is
// the whole of the attack CSP is here for.
func NewNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Without randomness there is no nonce worth having, and a
		// process that cannot read /dev/urandom will fail elsewhere.
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// Headers sets the security headers on every response, including the login
// page, the redirects and the asset tree.
//
// nonce is the value the loader page carries. An empty one - a frontend that
// could not be prepared, or a development build with no loader - falls back
// to 'unsafe-inline' so that a page never breaks; the panel's own script is
// the only thing this policy has to allow.
func Headers(nonce string) fiber.Handler {
	policy := ContentSecurityPolicy(nonce)

	return func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentSecurityPolicy, policy)
		// frame-ancestors above already covers modern browsers; this is
		// the header every older one reads.
		c.Set(fiber.HeaderXFrameOptions, "DENY")
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		c.Set(fiber.HeaderReferrerPolicy, "no-referrer")
		return c.Next()
	}
}

// ContentSecurityPolicy is the policy the panel is served with.
//
// Every source is the panel's own origin: the bundle, the stylesheets, the
// spinner, the icons and the API all come from there, and a panel on a host
// with no route to the internet keeps working. 'wasm-unsafe-eval' is what
// lets WebAssembly.instantiate run without opening the whole of eval; the
// canvas driver evaluates nothing else.
//
// style-src keeps 'unsafe-inline': fyne's wasm driver injects a <style>
// element for the viewport fix and sets cssText on the hidden input, and
// neither can be nonce'd from here. Styles are the one thing an HTML
// injection cannot turn into code.
func ContentSecurityPolicy(nonce string) string {
	script := "'self' 'wasm-unsafe-eval'"
	if nonce != "" {
		script += " 'nonce-" + nonce + "'"
	} else {
		script += " 'unsafe-inline'"
	}

	return "default-src 'self'; " +
		"script-src " + script + "; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; " +
		"font-src 'self'; " +
		"connect-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'none'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'"
}
