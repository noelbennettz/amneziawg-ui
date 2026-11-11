// Package auth guards the panel. The credentials are the pair the container is
// started with (WEB_UI_USER, WEB_UI_PASSWORD - the password as base64 of its
// SHA-256, the format fiber's basic-auth middleware used to expect), and
// nothing else is stored: there is no user table to keep.
//
// A browser gets the login page and, once the form is posted, a signed session
// cookie. The four public paths are the health check and the login page itself;
// everything else - the wasm bundle above all - needs the cookie. A script
// never sees the page: an Authorization: Basic header is still accepted, which
// is what keeps "curl -u admin:changeme" working.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/audit"
	"amneziawg-web-ui/internal/config"
	"amneziawg-web-ui/web-ui/api"
)

const (
	// LoginPath serves the login page (GET) and checks the form (POST).
	LoginPath = "/login"
	// LogoutPath drops the session cookie and sends the browser back to the
	// login page.
	LogoutPath = "/logout"
	// StatusPath is the container health check, which has to answer without
	// credentials: docker has none to offer.
	StatusPath = "/status"
	// CookieName is the session cookie. Nothing else in the panel sets a
	// cookie, so the name needs no prefix.
	CookieName = "awg_session"
	// SessionTTL is how long a login lasts. It is deliberately short enough
	// that a stolen cookie ages out on its own, and long enough not to
	// interrupt a working day.
	SessionTTL = 12 * time.Hour
)

// keyContext separates the session key from any other HMAC this password hash
// might be used for.
const keyContext = "amneziawg-web-ui session key v1"

// UserLocalKey is where the guard puts the name it authenticated, for the
// audit middleware to pick up when a request turns out to have changed
// something.
const UserLocalKey = audit.UserLocalKey

// Auth answers "is this request allowed" and owns the login page.
type Auth struct {
	user string
	// passwordHash is base64 of the SHA-256 of the password.
	passwordHash string
	// key signs the session cookie. It is derived from the credentials, so
	// nothing random is generated and no extra environment variable is
	// needed: a restart keeps the sessions, and changing the password ends
	// every one of them.
	key [sha256.Size]byte
	// now is the clock the token is checked against, swapped in the tests.
	now func() time.Time
	// secure is the policy for the Secure flag on both cookies.
	secure config.SecureMode
	// audit is the security log, nil when auditing is off.
	audit *audit.Log
}

// Option configures the guard.
type Option func(*Auth)

// WithSecureCookies sets the policy for the Secure flag. The default, zero,
// leaves the decision to the request: the panel is normally reached over
// plain HTTP with a TLS proxy in front of it, and a Secure cookie there is
// one the browser silently drops.
func WithSecureCookies(mode config.SecureMode) Option {
	return func(a *Auth) { a.secure = mode }
}

// WithAudit records logins, logouts and rejected writes.
func WithAudit(log *audit.Log) Option {
	return func(a *Auth) { a.audit = log }
}

// New builds the guard for the credentials the container was started with.
func New(user, passwordHash string, opts ...Option) *Auth {
	a := &Auth{user: user, passwordHash: strings.TrimSpace(passwordHash), now: time.Now}
	for _, opt := range opts {
		opt(a)
	}

	mac := hmac.New(sha256.New, []byte(a.passwordHash))
	mac.Write([]byte(keyContext))
	mac.Write([]byte(user))
	copy(a.key[:], mac.Sum(nil))

	return a
}

// cookieSecure decides the Secure flag for one response.
func (a *Auth) cookieSecure(c fiber.Ctx) bool {
	switch a.secure {
	case config.SecureOn:
		return true
	case config.SecureOff:
		return false
	default:
		return c.Secure() || forwardedHTTPS(c)
	}
}

// Check reports whether the pair matches the credentials. The comparison is
// constant time: the hash is fixed length, so mixing user and password can
// leak nothing about where they differ.
func (a *Auth) Check(user, password string) bool {
	sum := sha256.Sum256([]byte(password))
	got := base64.StdEncoding.EncodeToString(sum[:])

	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.user)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(got), []byte(a.passwordHash)) == 1

	return userOK && passOK
}

// Middleware rejects everything that carries neither a session cookie nor a
// basic-auth header. It is registered before the REST routes and the assets,
// and lets the public paths through untouched.
func (a *Auth) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if public(c.Path()) || a.authenticated(c) {
			// A page that is about to write needs the token that goes with
			// it, and the panel's page is a wasm bundle that cannot be
			// handed one in its markup: the cookie is where it reads it,
			// so it is issued here, on the way in.
			if c.Method() == fiber.MethodGet && c.Cookies(CSRFCookieName) == "" {
				a.csrfToken(c)
			}
			if !public(c.Path()) {
				c.Locals(UserLocalKey, a.user)
			}
			return c.Next()
		}

		// A request for data gets data back: the frontend shows the error,
		// and curl has something to parse. Only a navigation is worth a
		// redirect, and it is the only kind that ends up with a login form.
		if wantsJSON(c) {
			return c.Status(fiber.StatusUnauthorized).JSON(api.ErrorResponse{Error: "authentication required"})
		}

		// A navigation is answered with a redirect, and only a navigation
		// ends up with a login form: 303 makes the browser follow with a GET,
		// so a POST that was refused is never replayed against /login.
		return c.Redirect().To(LoginPath + "?next=" + url.QueryEscape(c.OriginalURL()))
	}
}

// Register adds the three public handlers. It has to be called after
// Middleware, which skips exactly these paths.
func (a *Auth) Register(app *fiber.App) {
	app.Get(LoginPath, a.showLogin)
	app.Post(LoginPath, a.submitLogin)
	app.Get(LogoutPath, a.logout)
	app.Post(LogoutPath, a.logout)
}

// authenticated reports whether the request carries proof of identity: a valid
// session cookie, or basic-auth credentials for the scripts and the health of
// existing automation.
func (a *Auth) authenticated(c fiber.Ctx) bool {
	if a.sessionValid(c.Cookies(CookieName), a.now()) {
		return true
	}
	return a.basicValid(c)
}

func (a *Auth) basicValid(c fiber.Ctx) bool {
	const prefix = "basic "

	header := c.Get(fiber.HeaderAuthorization)
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return false
	}
	user, password, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	return a.Check(user, password)
}

// public lists the paths that need no credentials.
func public(path string) bool {
	switch path {
	case StatusPath, LoginPath, LogoutPath:
		return true
	}
	return false
}

// wantsJSON separates a request for data from a navigation. The API and the
// profiler are asked by clients that parse a body, everything else by a
// browser that renders whatever it gets.
func wantsJSON(c fiber.Ctx) bool {
	path := c.Path()
	if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/debug") {
		return true
	}
	return strings.Contains(c.Get(fiber.HeaderAccept), fiber.MIMEApplicationJSON)
}
