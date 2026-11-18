package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/audit"
	"amneziawg-web-ui/web-ui/api"
)

const (
	// CSRFCookieName holds the double-submit token. It is readable by the
	// page on purpose: the wasm client has to echo it in a header, and a
	// cookie another site cannot read is a cookie another site cannot echo.
	CSRFCookieName = "awg_csrf"
	// CSRFHeader is where the page echoes it back.
	CSRFHeader = "X-CSRF-Token"
	// csrfField is the form field the login page carries the token in.
	csrfField = "csrf"
)

// CSRF guards every state-changing request.
//
// The session cookie is an ambient credential: the browser attaches it to
// anything that asks, which is exactly what a request forged on another site
// relies on. So a write is only accepted when it also carries a token that
// matches the one in a second cookie - the one cookie an attacker's page can
// cause to be sent but cannot read, and cannot put into a header.
//
// Two kinds of request are exempt. One that proves itself with an
// Authorization header is not ambient - no browser attaches one by itself -
// which keeps "curl -u admin:changeme -X DELETE ..." working. The login form
// is the other: it carries the token as a field, checked where the
// credentials are, in submitLogin.
func (a *Auth) CSRF() fiber.Handler {
	return func(c fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return c.Next()
		}
		if c.Get(fiber.HeaderAuthorization) != "" {
			return c.Next()
		}
		if p := c.Path(); p == LoginPath || p == LogoutPath {
			return c.Next()
		}
		if a.csrfValid(c) {
			return c.Next()
		}

		a.audit.Record(audit.Event{
			Event:  audit.EventCSRFRejected,
			IP:     a.clientIP(c),
			Method: c.Method(),
			Path:   c.Path(),
			Status: fiber.StatusForbidden,
		})
		return c.Status(fiber.StatusForbidden).JSON(api.ErrorResponse{
			Error: "missing or invalid CSRF token",
		})
	}
}

// csrfValid reports whether the request echoes the token from the cookie.
func (a *Auth) csrfValid(c fiber.Ctx) bool {
	want := c.Cookies(CSRFCookieName)
	got := c.Get(CSRFHeader)
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// formTokenValid is the login form's version: the same comparison, with the
// token in the body rather than in a header.
func (a *Auth) formTokenValid(c fiber.Ctx) bool {
	want := c.Cookies(CSRFCookieName)
	got := c.FormValue(csrfField)
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// csrfToken is the token the browser holds, issued if it holds none.
func (a *Auth) csrfToken(c fiber.Ctx) string {
	if token := c.Cookies(CSRFCookieName); token != "" {
		return token
	}
	token := newToken()
	a.setCSRFCookie(c, token)
	return token
}

// rotateCSRF gives the browser a fresh token. It runs on a successful login:
// a token that was handed out before the session existed is one that had a
// chance to be fixed, and the page that loads after the redirect picks the
// new value up from the cookie.
func (a *Auth) rotateCSRF(c fiber.Ctx) {
	a.setCSRFCookie(c, newToken())
}

func (a *Auth) setCSRFCookie(c fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:  CSRFCookieName,
		Value: token,
		Path:  "/",
		// Session cookie: it has no life of its own to outlive the
		// session it is protecting, and the browser is done with it when
		// the tab is.
		HTTPOnly: false,
		Secure:   a.cookieSecure(c),
		// Lax, not Strict: this cookie has to survive the trip back from
		// the login form, and Strict is on the credential itself.
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

// newToken is 32 bytes of randomness, in a form a cookie and a header both
// carry unchanged.
func newToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// forwardedHTTPS reports whether a proxy in front of the panel terminated
// TLS for this request. It is read only when the Secure flag is left on auto,
// and the worst a forged value does is mark a cookie Secure on a connection
// that is not - which costs that client its own session and nothing else.
func forwardedHTTPS(c fiber.Ctx) bool {
	proto, _, _ := strings.Cut(c.Get(fiber.HeaderXForwardedProto), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}
