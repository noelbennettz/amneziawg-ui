// Package auth guards the panel. The credential the panel checks is the pair
// from WEB_UI_USER and the admin password owned by internal/adminpw - an
// argon2id hash, in the file on the config volume or in WEB_UI_PASSWORD -
// and nothing else is stored: there is no user table to keep.
//
// A browser gets the login page and, once the form is posted, a signed session
// cookie. The three public paths are the health check and the login page
// itself; everything else - the wasm bundle above all - needs the cookie. A
// script never sees the page: an Authorization: Basic header is still
// accepted, which is what keeps "curl -u admin:<password>" working.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/adminpw"
	"amneziawg-web-ui/internal/audit"
	"amneziawg-web-ui/internal/config"
	"amneziawg-web-ui/internal/totp"
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
	// TOTPHeader carries the second factor of a Basic credential. A script
	// keeps sending its header on every request; a six-digit code that has
	// to be regenerated per request would make "curl -u" unusable, so the
	// code is a header of its own, and the panel accepts it in the ±1 step
	// window internal/totp defines.
	TOTPHeader = "X-TOTP-Code"
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
	// store is the admin password, and the source of the session key: the
	// key is derived from the hash in force, so a restart keeps the sessions
	// and a password change ends every one of them.
	store *adminpw.Store
	// now is the clock the token is checked against, swapped in the tests.
	now func() time.Time
	// secure is the policy for the Secure flag on both cookies.
	secure config.SecureMode
	// audit is the security log, nil when auditing is off.
	audit *audit.Log
	// limiter counts failed logins per address, nil when the limit is off.
	limiter *limiter
	// trustProxy makes the client address come from X-Forwarded-For.
	trustProxy bool
	// firstRunPassword is the file holding the first-start password in the
	// clear; the first accepted credential deletes it.
	firstRunPassword string
	dropOnce         sync.Once
	// totp is the optional second factor, nil when none is configured.
	totp *totp.Key

	// sem bounds how many verifications run at once. argon2id is deliberate
	// about memory - 19 MiB per hash - and a script that polls the REST API
	// sends its Authorization header with every request: without a bound,
	// two dozen concurrent requests are half a gigabyte of allocations.
	sem chan struct{}
	// verified caches Basic credentials that have already been checked.
	verified *credentialCache
}

// Verification cost control. Both numbers are about the same thing: argon2id
// is expensive on purpose, and a correct password is re-sent on every request
// of every script.
const (
	// verifyConcurrency is how many hashes may be computed at the same time.
	verifyConcurrency = 2
	// basicCacheTTL is how long a verified Basic credential is believed for.
	basicCacheTTL = 5 * time.Minute
)

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

// WithLoginRateLimit turns the per-address limiter on: five failures in a
// quarter of an hour block that address, and every block after the first lasts
// twice as long as the one before it.
//
// Off by default. A panel behind a proxy that does not forward the client
// address sees the proxy's address for everybody, and a limiter there would
// lock the operator out together with whoever is guessing.
func WithLoginRateLimit(enabled bool) Option {
	return func(a *Auth) {
		if enabled {
			a.limiter = newLimiter()
		}
	}
}

// WithTrustProxy reads the client address from X-Forwarded-For.
//
// Off by default and worth thinking about before turning it on: the header is
// whatever the client sent unless a proxy in front of the panel overwrote or
// appended to it. Believed blindly, it lets a guesser pick a new address per
// attempt and walk straight past the limiter.
func WithTrustProxy(enabled bool) Option {
	return func(a *Auth) { a.trustProxy = enabled }
}

// WithFirstRunPassword names the file the first-start password was written to,
// so that the first accepted credential deletes it. The plaintext is on the
// config volume only until somebody has proved they have read it - a file that
// stayed there would be a second copy of the password, in the clear, for as
// long as the volume lives.
func WithFirstRunPassword(path string) Option {
	return func(a *Auth) { a.firstRunPassword = path }
}

// WithTOTP adds a second factor: the six digits an authenticator app holding
// the same secret shows, required by the login form and, for a Basic
// credential, in the X-TOTP-Code header. A nil key leaves the panel
// password-only.
func WithTOTP(key *totp.Key) Option {
	return func(a *Auth) { a.totp = key }
}

// clientIP is the address the limiter counts and the audit log records.
//
// Behind a proxy every request arrives from the proxy, so the forwarded
// address is the only one that means anything - and it is read only when the
// operator said a proxy is in front. The left-most entry is the client: a
// proxy appends what it saw, so anything further right was written by
// somebody else.
func (a *Auth) clientIP(c fiber.Ctx) string {
	if a.trustProxy {
		if first, _, _ := strings.Cut(c.Get(fiber.HeaderXForwardedFor), ","); strings.TrimSpace(first) != "" {
			if addr := strings.TrimSpace(first); net.ParseIP(addr) != nil {
				return addr
			}
		}
	}
	return c.IP()
}

// New builds the guard for the configured user and the admin password store.
func New(user string, store *adminpw.Store, opts ...Option) *Auth {
	a := &Auth{
		user:     user,
		store:    store,
		now:      time.Now,
		sem:      make(chan struct{}, verifyConcurrency),
		verified: newCredentialCache(basicCacheTTL),
	}
	for _, opt := range opts {
		opt(a)
	}

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

// key is the signing key for the session cookie, derived from the hash in
// force: nothing random is generated and no extra environment variable is
// needed, so a restart keeps the sessions and a password change ends every one
// of them.
func (a *Auth) key() [sha256.Size]byte {
	hash, _ := a.store.Hash()

	mac := hmac.New(sha256.New, []byte(hash.String()))
	mac.Write([]byte(keyContext))
	mac.Write([]byte(a.user))

	var key [sha256.Size]byte
	copy(key[:], mac.Sum(nil))

	return key
}

// Check reports whether the pair matches the credentials. Both comparisons are
// constant time: the username field by field, and the password through the
// hash, which is fixed length whatever the password's own length is.
//
// A correct password that has already been checked recently is answered from
// the in-memory cache instead of a fresh argon2id derivation, and no more than
// verifyConcurrency derivations run at once.
func (a *Auth) Check(user, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.user)) == 1

	hash, generation := a.store.Hash()
	// The cache is keyed by an HMAC of the pair under a random per-process
	// key, so what it holds cannot be turned back into the password, and it
	// is memory only: nothing is written anywhere.
	cacheKey := a.verified.key(user, password)
	if a.verified.get(cacheKey, generation, a.now()) {
		if userOK {
			a.dropFirstRunPassword()
		}
		return userOK
	}

	a.sem <- struct{}{}
	passOK := hash.Verify(password)
	<-a.sem

	if userOK && passOK {
		a.verified.put(cacheKey, generation, a.now())
		a.dropFirstRunPassword()
	}

	return userOK && passOK
}

// dropFirstRunPassword deletes the file the first start left the generated
// password in, once and only after a credential was accepted: a wrong password
// must not be able to remove the operator's only way in. A failed delete is not
// worth failing a login over - the password that just verified is proof enough
// that the file has been read - so it is reported and the flag stays set.
func (a *Auth) dropFirstRunPassword() {
	a.dropOnce.Do(func() {
		if err := adminpw.RemovePasswordFile(a.firstRunPassword); err != nil {
			log.Printf("warning: could not remove the first-run password file %s: %v", a.firstRunPassword, err)
			return
		}
		if a.firstRunPassword != "" {
			log.Printf("first-run password file %s deleted: the login it held is now the one in admin.hash", a.firstRunPassword)
		}
	})
}

// Middleware rejects everything that carries neither a session cookie nor a
// basic-auth header. It is registered before the REST routes and the assets,
// and lets the public paths through untouched.
func (a *Auth) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if public(c.Path()) {
			// The health check and the login page have nothing to write
			// and nobody to write as: no session, no token, no cookie in
			// the response.
			return c.Next()
		}
		if a.authenticated(c) {
			// A page that is about to write needs the token that goes with
			// it, and the panel's page is a wasm bundle that cannot be
			// handed one in its markup: the cookie is where it reads it,
			// so it is issued here, on the way in.
			if c.Method() == fiber.MethodGet && c.Cookies(CSRFCookieName) == "" {
				a.csrfToken(c)
			}
			c.Locals(UserLocalKey, a.user)
			c.Locals(audit.IPLocalKey, a.clientIP(c))
			return c.Next()
		}

		// A request for data gets data back: the frontend shows the error,
		// and curl has something to parse. Only a navigation is worth a
		// redirect, and it is the only kind that ends up with a login form.
		//
		// Before that: somebody who has just been guessing passwords gets
		// told to wait, whatever they asked for. The limiter is only
		// consulted here - never for a request that is already
		// authenticated - so a working session is never locked out by an
		// attacker sharing its address.
		if wait := a.limiter.retryAfter(a.clientIP(c)); wait > 0 {
			c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(wait.Seconds())+1))
			if wantsJSON(c) {
				return c.Status(fiber.StatusTooManyRequests).JSON(api.ErrorResponse{Error: "too many failed logins, try again later"})
			}
			return c.Status(fiber.StatusTooManyRequests).SendString("Too many failed logins. Try again later.\n")
		}

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
	if a.basicValid(c) {
		return true
	}
	// A header that was offered and refused is a wrong password, and the
	// limiter is the only thing that makes guessing one expensive.
	if c.Get(fiber.HeaderAuthorization) != "" {
		a.limiter.fail(a.clientIP(c))
	}
	return false
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
	if !a.Check(user, password) {
		return false
	}

	// The password was right; the second factor is checked after it, so a
	// wrong code costs nothing to refuse and does not tell an attacker that
	// the password they tried was the right one.
	return a.secondFactorValid(c.Get(TOTPHeader))
}

// secondFactorValid reports whether code passes the second factor. With no
// secret configured there is nothing to pass, and the check is a nil test: the
// password is the whole of the credential, as it has always been.
func (a *Auth) secondFactorValid(code string) bool {
	if a.totp == nil {
		return true
	}

	return a.totp.Verify(code, a.now())
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
