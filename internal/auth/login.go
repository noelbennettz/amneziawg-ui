package auth

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/url"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/audit"
)

// loginPage is the whole of the page: one form, its stylesheet and nothing
// external, so it renders on a host with no route to the internet and without
// a byte of javascript.
//
//go:embed login.html
var loginPage string

// loginTemplate is parsed once at startup. A failure here is a programming
// error - the template is a constant - so it panics rather than leaving the
// panel with no way in.
var loginTemplate = template.Must(template.New("login").Parse(loginPage))

// pageData is what the template is given.
type pageData struct {
	// Next is where the browser goes after a successful login: the path it
	// asked for before it was sent here.
	Next string
	// CSRF is the double-submit token the form carries back.
	CSRF string
	// Failed is set after a wrong pair, and puts the error above the form.
	Failed bool
	// Text is the page's wording, in the language the browser asked for.
	Text loginText
}

// showLogin renders the form. Someone who is already logged in is sent on
// rather than offered a second login.
func (a *Auth) showLogin(c fiber.Ctx) error {
	if a.authenticated(c) {
		return c.Redirect().To(safeNext(c.Query("next")))
	}
	return a.render(c, fiber.StatusOK, pageData{
		Next: safeNext(c.Query("next")),
		CSRF: a.csrfToken(c),
	})
}

// submitLogin checks the form. A wrong pair is answered with the page again and
// a 401: the status is the truth about the request, and the page is what a
// person needs to try once more.
func (a *Auth) submitLogin(c fiber.Ctx) error {
	next := safeNext(c.FormValue("next"))

	// A form that does not carry this browser's token was not rendered by
	// this panel - it was posted from somewhere else, or replayed after
	// the tab sat open long enough for the cookie to go. Either way there
	// is nothing to answer: the login page is where a person starts over,
	// and a fresh token comes with it.
	if !a.formTokenValid(c) {
		a.audit.Record(audit.Event{
			Event:  audit.EventCSRFRejected,
			IP:     c.IP(),
			Method: c.Method(),
			Path:   c.Path(),
			Status: fiber.StatusSeeOther,
		})
		return c.Redirect().To(LoginPath + "?next=" + url.QueryEscape(next))
	}

	user := c.FormValue("username")

	// A blocked address is told to wait before the password is looked at.
	// The point of the limit is to make a guess expensive, not to answer one
	// faster.
	if wait := a.limiter.retryAfter(c.IP()); wait > 0 {
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(wait.Seconds())+1))
		return c.Status(fiber.StatusTooManyRequests).SendString("Too many failed logins. Try again later.\n")
	}

	if !a.Check(user, c.FormValue("password")) {
		a.limiter.fail(c.IP())
		a.audit.Record(audit.Event{
			Event:  audit.EventLoginFailed,
			User:   user,
			IP:     c.IP(),
			Method: c.Method(),
			Path:   c.Path(),
			Status: fiber.StatusUnauthorized,
		})
		return a.render(c, fiber.StatusUnauthorized, pageData{
			Next:   next,
			CSRF:   a.csrfToken(c),
			Failed: true,
		})
	}

	a.audit.Record(audit.Event{
		Event:  audit.EventLogin,
		User:   a.user,
		IP:     c.IP(),
		Method: c.Method(),
		Path:   c.Path(),
		Status: fiber.StatusSeeOther,
	})
	a.limiter.succeed(c.IP())

	// A token handed out before the session existed is one that had a
	// chance to be fixed; the page that loads after the redirect reads the
	// new one from the cookie.
	a.rotateCSRF(c)
	a.setSession(c)
	return c.Redirect().To(next)
}

// logout forgets the cookie. GET is allowed as well as POST: the link is only
// ever handed to the person who is logged in, and SameSite=Strict keeps
// another site from triggering it.
func (a *Auth) logout(c fiber.Ctx) error {
	if a.sessionValid(c.Cookies(CookieName), a.now()) {
		a.audit.Record(audit.Event{
			Event:  audit.EventLogout,
			User:   a.user,
			IP:     c.IP(),
			Method: c.Method(),
			Path:   c.Path(),
			Status: fiber.StatusSeeOther,
		})
	}
	a.clearSession(c)
	return c.Redirect().To(LoginPath)
}

// render writes the page with an explicit status, in the language the request
// asked for.
func (a *Auth) render(c fiber.Ctx, status int, data pageData) error {
	data.Text = loginTextFor(c.Get(fiber.HeaderAcceptLanguage))

	var page bytes.Buffer
	if err := loginTemplate.Execute(&page, data); err != nil {
		return err
	}

	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	// The page has no business in a cache: it carries the state of a login
	// attempt and is only reachable while there is none.
	c.Set(fiber.HeaderCacheControl, "no-store")

	return c.Status(status).Send(page.Bytes())
}

// safeNext turns a requested path into one the browser can be redirected to.
// Only a path on this host is accepted - "//elsewhere" and "https://elsewhere"
// are another site, and a login form that forwards to another site is an open
// redirect.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") {
		return "/"
	}
	if strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\") {
		return "/"
	}
	// A bare "/login" would put the user back where they came from.
	if next == LoginPath || strings.HasPrefix(next, LoginPath+"?") {
		return "/"
	}
	return next
}
