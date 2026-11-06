package auth

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"

	"github.com/gofiber/fiber/v3"
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
	// Error is shown above the form after a failed attempt, empty otherwise.
	Error string
}

// showLogin renders the form. Someone who is already logged in is sent on
// rather than offered a second login.
func (a *Auth) showLogin(c fiber.Ctx) error {
	if a.authenticated(c) {
		return c.Redirect().To(safeNext(c.Query("next")))
	}
	return a.render(c, fiber.StatusOK, pageData{Next: safeNext(c.Query("next"))})
}

// submitLogin checks the form. A wrong pair is answered with the page again and
// a 401: the status is the truth about the request, and the page is what a
// person needs to try once more.
func (a *Auth) submitLogin(c fiber.Ctx) error {
	next := safeNext(c.FormValue("next"))

	if !a.Check(c.FormValue("username"), c.FormValue("password")) {
		return a.render(c, fiber.StatusUnauthorized, pageData{
			Next:  next,
			Error: "Incorrect username or password.",
		})
	}

	a.setSession(c)
	return c.Redirect().To(next)
}

// logout forgets the cookie. GET is allowed as well as POST: the link is only
// ever handed to the person who is logged in, and SameSite=Lax keeps another
// site from triggering it.
func (a *Auth) logout(c fiber.Ctx) error {
	a.clearSession(c)
	return c.Redirect().To(LoginPath)
}

// render writes the page with an explicit status.
func (a *Auth) render(c fiber.Ctx, status int, data pageData) error {
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
