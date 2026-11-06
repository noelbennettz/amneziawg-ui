package e2e

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

// TestLogin exercises the login page that guards the panel: the browser gets no
// WWW-Authenticate challenge any more, so the contexts carry no credentials and
// this form is the only way in.
//
// It runs first (the file sorts before 01) and asserts the three states a login
// form has: anonymous is redirected here, a wrong password keeps you here with
// an error, a right one puts you on the panel. Every step uses a context of its
// own - each has its own cookie jar and is closed on cleanup - so this test
// leaves no session behind and no state the later tests could trip over.
func TestLogin(t *testing.T) {
	// Anonymous: a page of the panel bounces to the form and remembers where
	// it was going, so the login can send the browser back there.
	anonymous := newApp(t)
	_, err := anonymous.page.Goto("/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	must(t, err)

	landed := parseURL(t, anonymous.page.URL())
	if landed.Path != "/login" {
		t.Errorf("GET / without a session landed on %q, want the login page", anonymous.page.URL())
	}
	if next := landed.Query().Get("next"); next != "/" {
		t.Errorf("the login page was given next=%q, want %q", next, "/")
	}
	anonymous.shot("00-login")

	// A wrong password is answered with the form again and a 401: the page
	// has to say what went wrong in the DOM, since there is no page to go to.
	anonymous.submitLogin(user, password+"-wrong")
	alert := anonymous.page.Locator("p.error")
	must(t, alert.WaitFor(playwright.LocatorWaitForOptions{Timeout: new(float64(bootTimeout.Milliseconds()))}))
	text, err := alert.TextContent()
	must(t, err)
	if want := "Incorrect username or password."; text != want {
		t.Errorf("the error reads %q, want %q", text, want)
	}
	if still := parseURL(t, anonymous.page.URL()); still.Path != "/login" {
		t.Errorf("a wrong password left the browser on %q, want the login page", anonymous.page.URL())
	}
	anonymous.shot("00-login-error")

	// The right pair on the same, still anonymous, context: the login helper
	// waits for the redirect to "/", and the canvas proves it is the panel's
	// loader page that answered, not the form again.
	anonymous.login()
	must(t, anonymous.page.Locator("canvas").WaitFor(playwright.LocatorWaitForOptions{
		Timeout: new(float64(bootTimeout.Milliseconds())),
	}))

	// The panel's pages are behind the same session, and this request carries
	// it: the API answers 200 only if the cookie the form set is a real one.
	resp, err := anonymous.page.Context().Request().Get("/api/servers")
	must(t, err)
	if resp.Status() != http.StatusOK {
		t.Errorf("/api/servers with the session cookie -> %d, want %d", resp.Status(), http.StatusOK)
	}
	must(t, resp.Dispose())
}

// parseURL reads a URL the browser reports. Failure is the driver handing back
// something that is not a URL at all, which no assertion here can improve on.
func parseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	must(t, err)
	return u
}
