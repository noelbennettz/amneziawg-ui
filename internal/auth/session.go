package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

// A session is a token the server can check without keeping anything: the
// payload is the user and the expiry, and the signature is an HMAC over both.
// The value is "<payload>.<signature>", both base64url, so it survives a cookie
// header as it is.

// issue returns the token a login at now is worth.
func (a *Auth) issue(now time.Time) string {
	payload := a.user + "|" + strconv.FormatInt(now.Add(SessionTTL).Unix(), 10)

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + a.sign(payload)
}

// sessionValid reports whether token was signed by this guard, names the
// configured user, and has not expired at now. Anything malformed is simply
// not a session: every early return is a rejection, so a truncated or
// hand-written cookie costs no more than an absent one.
func (a *Auth) sessionValid(token string, now time.Time) bool {
	if token == "" {
		return false
	}

	encoded, signature, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	// The comparison is the one thing that must not leak: a token is
	// accepted only when its signature is byte for byte the one this key
	// produces over this payload.
	if !hmac.Equal([]byte(signature), []byte(a.sign(string(payload)))) {
		return false
	}

	user, deadline, ok := strings.Cut(string(payload), "|")
	if !ok || user != a.user {
		return false
	}
	expires, err := strconv.ParseInt(deadline, 10, 64)
	if err != nil {
		return false
	}

	return now.Before(time.Unix(expires, 0))
}

// sign is the MAC over a payload, in the encoding the token carries it.
func (a *Auth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.key[:])
	mac.Write([]byte(payload))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// setSession hands the browser the cookie a successful login is worth.
func (a *Auth) setSession(c fiber.Ctx) {
	now := a.now()
	c.Cookie(&fiber.Cookie{
		Name:  CookieName,
		Value: a.issue(now),
		Path:  "/",
		// Expires matches the expiry inside the token, so the browser and
		// the server let go of the session at the same moment.
		Expires:  now.Add(SessionTTL),
		HTTPOnly: true,
		// Secure follows WEB_UI_SECURE_COOKIE and, when that is left alone,
		// the request: the panel is normally reached over plain HTTP with a
		// TLS proxy in front of it, and a Secure cookie there is one the
		// browser silently drops.
		Secure: a.cookieSecure(c),
		// Strict: no other site gets to send this cookie, not even by
		// navigating the browser here. The cost is a redirect through
		// /login when someone follows a link from elsewhere, which the
		// login page turns straight back around.
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}

// clearSession deletes the cookie.
func (a *Auth) clearSession(c fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   a.cookieSecure(c),
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}
