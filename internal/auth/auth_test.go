package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"amneziawg-web-ui/internal/audit"
	"amneziawg-web-ui/internal/config"
)

// The defaults the image ships and the e2e suite logs in with.
const (
	testUser     = "admin"
	testPassword = "changeme"
	// base64(SHA-256("changeme")), the format WEB_UI_PASSWORD holds.
	testHash = "BXugPWxEEEhj3HNh/kV4ll0YhzYPkKCJWILlimJI/IY="

	// testNow is the clock every test starts with, so a token's expiry is a
	// number and not a race with the wall.
	testNow = 1_700_000_000
)

// base64Std and base64url are the two encodings the package and its tokens use.
func base64Std(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func base64url(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func newTestAuth() *Auth {
	a := New(testUser, testHash)
	a.now = func() time.Time { return time.Unix(testNow, 0) }
	return a
}

func TestCheck(t *testing.T) {
	a := newTestAuth()

	cases := []struct {
		user, password string
		want           bool
	}{
		{testUser, testPassword, true},
		{testUser, "wrong", false},
		{"root", testPassword, false},
		{"admin", "", false},
		{"", "", false},
		// The hash of the password is not the password.
		{testUser, testHash, false},
	}

	for _, tc := range cases {
		if got := a.Check(tc.user, tc.password); got != tc.want {
			t.Errorf("Check(%q, %q) = %v, want %v", tc.user, tc.password, got, tc.want)
		}
	}
}

func TestSessionRoundTrip(t *testing.T) {
	a := newTestAuth()
	now := time.Unix(testNow, 0)
	token := a.issue(now)

	if !a.sessionValid(token, now) {
		t.Fatal("a token just issued is not valid")
	}
	if !a.sessionValid(token, now.Add(SessionTTL-1)) {
		t.Fatal("the token is invalid one second before it expires")
	}
	if a.sessionValid(token, now.Add(SessionTTL)) {
		t.Fatal("the token is valid at its expiry")
	}
}

func TestSessionRejectsForgery(t *testing.T) {
	a := newTestAuth()
	now := time.Unix(testNow, 0)
	token := a.issue(now)
	payload, signature, _ := strings.Cut(token, ".")

	// A later expiry under the old signature: the MAC is what stops this.
	later := a.user + fmt.Sprintf("|%d", now.Add(100*SessionTTL).Unix())
	forged := base64url(later) + "." + signature

	cases := map[string]string{
		"empty":           "",
		"no separator":    payload,
		"empty signature": payload + ".",
		"forged payload":  forged,
		"forged sig":      payload + "." + signature[:len(signature)-2] + "AA",
		"garbage":         "not-a-token",
		"not base64":      "!!!.???",
	}

	for name, candidate := range cases {
		if a.sessionValid(candidate, now) {
			t.Errorf("%s: accepted %q", name, candidate)
		}
	}
}

func TestSessionEndsWithThePassword(t *testing.T) {
	now := time.Unix(testNow, 0)
	token := newTestAuth().issue(now)

	// Another password is another signing key, so every session minted under
	// the old one stops being accepted the moment it is changed.
	other := New(testUser, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if other.sessionValid(token, now) {
		t.Fatal("a token signed with the old password is still valid")
	}
}

func TestSessionIsBoundToTheUser(t *testing.T) {
	now := time.Unix(testNow, 0)
	token := newTestAuth().issue(now)

	renamed := New("someone-else", testHash)
	if renamed.sessionValid(token, now) {
		t.Fatal("the token of another user name is valid")
	}
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"":                       "/",
		"/":                      "/",
		"/servers?tab=clients":   "/servers?tab=clients",
		"//evil.example.com":     "/",
		"https://evil.example":   "/",
		"/\\evil.example":        "/",
		"/login":                 "/",
		"/login?next=%2Fservers": "/",
		"servers":                "/",
	}

	for next, want := range cases {
		if got := safeNext(next); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", next, got, want)
		}
	}
}

// newTestApp is the panel as main.go wires it, minus the manager and the
// assets: the guard in front of one page, one API route and the health check.
// The options are what a test varies - the cookie policy and the audit log.
func newTestApp(t *testing.T, opts ...Option) *fiber.App {
	t.Helper()

	guard := New(testUser, testHash, opts...)
	guard.now = func() time.Time { return time.Unix(testNow, 0) }
	app := fiber.New(fiber.Config{Immutable: true})
	app.Use(guard.Middleware())
	app.Use(guard.CSRF())
	app.Use(audit.Middleware(guard.audit))
	app.Get(StatusPath, func(c fiber.Ctx) error { return c.SendString("uptime") })
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("<html>panel</html>") })
	app.Get("/api/system/status", func(c fiber.Ctx) error {
		return c.JSON(map[string]string{"ok": "yes"})
	})
	app.Post("/api/servers", func(c fiber.Ctx) error {
		return c.JSON(map[string]string{"created": "yes"})
	})
	guard.Register(app)

	return app
}

// do runs one request against the app. app.Test does not follow redirects, so
// a 302 is visible as one.
func do(t *testing.T, app *fiber.App, req *http.Request) *http.Response {
	t.Helper()

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// get asks for path with the cookie and headers given.
func get(t *testing.T, app *fiber.App, path string, cookie *http.Cookie, header map[string]string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return do(t, app, req)
}

// csrfToken loads the login page and returns the double-submit token it hands
// out, along with the cookie that holds it.
func csrfToken(t *testing.T, app *fiber.App) (string, *http.Cookie) {
	t.Helper()

	resp := get(t, app, LoginPath, nil, nil)
	for _, cookie := range resp.Cookies() {
		if cookie.Name == CSRFCookieName && cookie.Value != "" {
			return cookie.Value, cookie
		}
	}
	t.Fatal("the login page sets no CSRF cookie")
	return "", nil
}

// login posts the form the way the browser would: with the token the page gave
// it, in the body and in the cookie.
func login(t *testing.T, app *fiber.App, user, password, next string) *http.Response {
	t.Helper()

	token, cookie := csrfToken(t, app)
	return postLogin(t, app, user, password, next, token, cookie)
}

// postLogin posts the form with the token and cookie given, so a test can leave
// one of them out.
func postLogin(t *testing.T, app *fiber.App, user, password, next, token string, cookie *http.Cookie) *http.Response {
	t.Helper()

	form := url.Values{"username": {user}, "password": {password}, "next": {next}, "csrf": {token}}
	req := httptest.NewRequest(http.MethodPost, LoginPath, strings.NewReader(form.Encode()))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return do(t, app, req)
}

// write posts to the API with the cookies and headers given.
func write(t *testing.T, app *fiber.App, cookies []*http.Cookie, header map[string]string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/servers", nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return do(t, app, req)
}

// loginCookies logs in and returns the session and the rotated CSRF cookie: the
// pair a browser holds once it is past the form.
func loginCookies(t *testing.T, app *fiber.App) (*http.Cookie, *http.Cookie) {
	t.Helper()

	resp := login(t, app, testUser, testPassword, "/")
	var session, csrf *http.Cookie
	for _, cookie := range resp.Cookies() {
		switch {
		case cookie.Name == CookieName && cookie.Value != "":
			session = cookie
		case cookie.Name == CSRFCookieName && cookie.Value != "":
			csrf = cookie
		}
	}
	if session == nil || csrf == nil {
		t.Fatalf("a successful login set session=%v csrf=%v", session, csrf)
	}
	return session, csrf
}

// sessionCookie returns the session cookie a response sets, if any.
func sessionCookie(resp *http.Response) *http.Cookie {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == CookieName && cookie.Value != "" {
			return cookie
		}
	}
	return nil
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPublicPathsNeedNoCredentials(t *testing.T) {
	app := newTestApp(t)

	if resp := get(t, app, StatusPath, nil, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("%s = %d, want 200", StatusPath, resp.StatusCode)
	}

	page := get(t, app, LoginPath, nil, nil)
	if page.StatusCode != http.StatusOK {
		t.Errorf("%s = %d, want 200", LoginPath, page.StatusCode)
	}
	html := body(t, page)
	for _, want := range []string{"AmneziaWG Web UI", `name="username"`, `name="password"`, "Sign in"} {
		if !strings.Contains(html, want) {
			t.Errorf("the login page does not contain %q", want)
		}
	}
	if got := page.Header.Get(fiber.HeaderCacheControl); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestUnloggedInNavigationIsRedirected(t *testing.T) {
	app := newTestApp(t)

	resp := get(t, app, "/", nil, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("GET / = %d, want 303", resp.StatusCode)
	}
	if got, want := resp.Header.Get(fiber.HeaderLocation), "/login?next=%2F"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	// The browser must not be offered a native credential prompt.
	if got := resp.Header.Get(fiber.HeaderWWWAuthenticate); got != "" {
		t.Errorf("WWW-Authenticate = %q, want none", got)
	}

	// A deep link survives the trip through the login page.
	resp = get(t, app, "/servers?tab=clients", nil, nil)
	if got, want := resp.Header.Get(fiber.HeaderLocation), "/login?next=%2Fservers%3Ftab%3Dclients"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestUnloggedInAPIIsUnauthorized(t *testing.T) {
	app := newTestApp(t)

	resp := get(t, app, "/api/system/status", nil, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("GET /api/system/status = %d, want 401", resp.StatusCode)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(body(t, resp)), &payload); err != nil {
		t.Fatalf("the 401 is not JSON: %v", err)
	}
	if payload["error"] == "" {
		t.Error("the 401 carries no error message")
	}

	// A page asked for as data is answered as data, not with a redirect.
	resp = get(t, app, "/", nil, map[string]string{fiber.HeaderAccept: fiber.MIMEApplicationJSON})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("GET / with Accept: application/json = %d, want 401", resp.StatusCode)
	}
}

func TestLoginSetsTheSession(t *testing.T) {
	app := newTestApp(t)

	resp := login(t, app, testUser, testPassword, "/servers?tab=clients")
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("POST /login = %d, want 303", resp.StatusCode)
	}
	if got, want := resp.Header.Get(fiber.HeaderLocation), "/servers?tab=clients"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	cookie := sessionCookie(resp)
	if cookie == nil {
		t.Fatal("a successful login sets no session cookie")
	}
	if !cookie.HttpOnly {
		t.Error("the session cookie is not HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want /", cookie.Path)
	}

	// The cookie is what the panel was waiting for.
	if resp := get(t, app, "/", cookie, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET / with the session = %d, want 200", resp.StatusCode)
	}
	if resp := get(t, app, "/api/system/status", cookie, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/system/status with the session = %d, want 200", resp.StatusCode)
	}
}

func TestLoginRejectsWrongCredentials(t *testing.T) {
	app := newTestApp(t)

	for name, pair := range map[string][2]string{
		"password": {testUser, "wrong"},
		"user":     {"root", testPassword},
		"both":     {"root", "wrong"},
		"empty":    {"", ""},
	} {
		resp := login(t, app, pair[0], pair[1], "/")
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("%s: POST /login = %d, want 401", name, resp.StatusCode)
		}
		if cookie := sessionCookie(resp); cookie != nil {
			t.Errorf("%s: a rejected login set a session cookie", name)
		}
		if !strings.Contains(body(t, resp), "Incorrect username or password.") {
			t.Errorf("%s: the page does not report the failure", name)
		}
	}
}

func TestLoginDoesNotRedirectOffTheHost(t *testing.T) {
	app := newTestApp(t)

	for _, next := range []string{"//evil.example", "https://evil.example/x", "/\\evil.example"} {
		resp := login(t, app, testUser, testPassword, next)
		if got := resp.Header.Get(fiber.HeaderLocation); got != "/" {
			t.Errorf("next=%q: Location = %q, want /", next, got)
		}
	}
}

func TestLoginPageSendsTheLoggedInOn(t *testing.T) {
	app := newTestApp(t)

	cookie := sessionCookie(login(t, app, testUser, testPassword, "/"))
	if cookie == nil {
		t.Fatal("no session cookie")
	}

	resp := get(t, app, LoginPath, cookie, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Errorf("GET /login while logged in = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderLocation); got != "/" {
		t.Errorf("Location = %q, want /", got)
	}
}

func TestLogoutClearsTheSession(t *testing.T) {
	app := newTestApp(t)

	resp := get(t, app, LogoutPath, nil, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("GET /logout = %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderLocation); got != LoginPath {
		t.Errorf("Location = %q, want %s", got, LoginPath)
	}

	var cleared bool
	for _, cookie := range resp.Cookies() {
		if cookie.Name == CookieName && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout does not delete the session cookie")
	}
}

func TestBasicAuthStillWorksForScripts(t *testing.T) {
	app := newTestApp(t)

	header := map[string]string{
		fiber.HeaderAuthorization: "Basic " + base64Std(testUser+":"+testPassword),
	}
	if resp := get(t, app, "/api/system/status", nil, header); resp.StatusCode != http.StatusOK {
		t.Errorf("basic auth on the API = %d, want 200", resp.StatusCode)
	}

	// A wrong password in the header is no better than no header at all.
	header[fiber.HeaderAuthorization] = "Basic " + base64Std(testUser+":wrong")
	if resp := get(t, app, "/api/system/status", nil, header); resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("wrong basic auth on the API = %d, want 401", resp.StatusCode)
	}
	if resp := get(t, app, "/api/system/status", nil, map[string]string{
		fiber.HeaderAuthorization: "Basic !!!",
	}); resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("a malformed Authorization header = %d, want 401", resp.StatusCode)
	}
}

// The session cookie is attached to anything that asks, which is what a write
// forged on another site relies on. The token in the second cookie is what an
// attacker's page can cause to be sent but not read, so a write without it is
// not the panel's.
func TestWriteNeedsTheToken(t *testing.T) {
	app := newTestApp(t)
	session, csrf := loginCookies(t, app)

	if resp := write(t, app, []*http.Cookie{session}, nil); resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("a write with the session and no token = %d, want 403", resp.StatusCode)
	}

	// A token that is not the one in the cookie is no better than none.
	resp := write(t, app, []*http.Cookie{session, csrf}, map[string]string{CSRFHeader: "not-the-token"})
	if resp.StatusCode != fiber.StatusForbidden {
		t.Errorf("a write with a forged token = %d, want 403", resp.StatusCode)
	}

	// A token with no session is a request from nowhere: it is answered as
	// one, before the token is even looked at.
	resp = write(t, app, nil, map[string]string{CSRFHeader: csrf.Value})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("a write with a token and no session = %d, want 401", resp.StatusCode)
	}

	resp = write(t, app, []*http.Cookie{session, csrf}, map[string]string{CSRFHeader: csrf.Value})
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("a write with the right token = %d, want 200", resp.StatusCode)
	}
}

// A request that proves itself with an Authorization header is not ambient: no
// browser attaches one on its own, so "curl -u" keeps working without a token.
func TestWriteWithBasicAuthNeedsNoToken(t *testing.T) {
	app := newTestApp(t)

	header := map[string]string{
		fiber.HeaderAuthorization: "Basic " + base64Std(testUser+":"+testPassword),
	}
	if resp := write(t, app, nil, header); resp.StatusCode != http.StatusOK {
		t.Errorf("a write with basic auth = %d, want 200", resp.StatusCode)
	}
}

// A form posted from somewhere else carries no token and no cookie, and must
// not log anybody in: that is the whole of login CSRF.
func TestLoginFormNeedsTheToken(t *testing.T) {
	app := newTestApp(t)

	token, cookie := csrfToken(t, app)

	cases := map[string]struct {
		token  string
		cookie *http.Cookie
	}{
		"no token":         {"", cookie},
		"wrong token":      {"not-the-token", cookie},
		"token, no cookie": {token, nil},
		"wrong cookie":     {token, &http.Cookie{Name: CSRFCookieName, Value: "other"}},
	}

	for name, tc := range cases {
		resp := postLogin(t, app, testUser, testPassword, "/", tc.token, tc.cookie)
		if resp.StatusCode != fiber.StatusSeeOther {
			t.Errorf("%s: POST /login = %d, want 303", name, resp.StatusCode)
		}
		if cookie := sessionCookie(resp); cookie != nil {
			t.Errorf("%s: a form without a valid token set a session", name)
		}
	}

	// The form the page handed out does work, token and cookie together.
	if resp := postLogin(t, app, testUser, testPassword, "/", token, cookie); sessionCookie(resp) == nil {
		t.Error("the form the page handed out does not log in")
	}
}

// The token is replaced when the session starts: a value that was handed out
// before the login had a chance to be fixed.
func TestLoginRotatesTheToken(t *testing.T) {
	app := newTestApp(t)

	before, _ := csrfToken(t, app)
	resp := login(t, app, testUser, testPassword, "/")

	for _, cookie := range resp.Cookies() {
		if cookie.Name == CSRFCookieName {
			if cookie.Value == "" || cookie.Value == before {
				t.Errorf("the CSRF token was not rotated: %q", cookie.Value)
			}
			return
		}
	}
	t.Error("a successful login set no CSRF cookie")
}

// The token cookie is meant to be read by the page - that is the whole
// mechanism - and it is not a credential on its own.
func TestCSRFCookieIsReadableByThePage(t *testing.T) {
	app := newTestApp(t)

	resp := get(t, app, LoginPath, nil, nil)
	for _, cookie := range resp.Cookies() {
		if cookie.Name != CSRFCookieName {
			continue
		}
		if cookie.HttpOnly {
			t.Error("the CSRF cookie is HttpOnly: the page cannot echo what it cannot read")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
		}
		return
	}
	t.Fatal("no CSRF cookie")
}

// Secure is off by default behind a TLS proxy, which is why it has to be
// possible to force it on - and off again for a deployment on plain HTTP.
func TestSecureCookiePolicy(t *testing.T) {
	t.Run("forced on", func(t *testing.T) {
		app := newTestApp(t, WithSecureCookies(config.SecureOn))

		session, csrf := loginCookies(t, app)
		if !csrf.Secure {
			t.Error("the CSRF cookie is not Secure")
		}
		if !session.Secure {
			t.Error("the session cookie is not Secure")
		}
	})

	t.Run("auto reads X-Forwarded-Proto", func(t *testing.T) {
		app := newTestApp(t, WithSecureCookies(config.SecureAuto))

		req := httptest.NewRequest(http.MethodGet, LoginPath, nil)
		req.Header.Set(fiber.HeaderXForwardedProto, "https")
		resp := do(t, app, req)

		var seen bool
		for _, cookie := range resp.Cookies() {
			if cookie.Name == CSRFCookieName {
				seen = true
				if !cookie.Secure {
					t.Error("a request a proxy reported as https set a cookie without Secure")
				}
			}
		}
		if !seen {
			t.Fatal("no CSRF cookie")
		}
	})

	t.Run("forced off", func(t *testing.T) {
		app := newTestApp(t, WithSecureCookies(config.SecureOff))

		req := httptest.NewRequest(http.MethodGet, LoginPath, nil)
		req.Header.Set(fiber.HeaderXForwardedProto, "https")
		resp := do(t, app, req)

		for _, cookie := range resp.Cookies() {
			if cookie.Name == CSRFCookieName && cookie.Secure {
				t.Error("Secure was pinned off and the cookie is still Secure")
			}
		}
	})
}

// With auditing on, a login, a rejected login and a change each leave a line
// behind - and a read does not, because a panel polls every 30 seconds.
func TestAuditRecordsLoginsAndChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	app := newTestApp(t, WithAudit(audit.New(true, path, 0)))

	login(t, app, testUser, "wrong", "/")
	postLogin(t, app, testUser, testPassword, "/", "", nil) // rejected: no token
	session, csrf := loginCookies(t, app)
	write(t, app, []*http.Cookie{session, csrf}, map[string]string{CSRFHeader: csrf.Value})
	get(t, app, "/api/system/status", session, nil)
	get(t, app, LogoutPath, session, nil)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no audit log: %v", err)
	}

	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var event audit.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("audit line %q is not JSON: %v", line, err)
		}
		if event.IP == "" {
			t.Errorf("%s: no address recorded", event.Event)
		}
		if event.Time.IsZero() {
			t.Errorf("%s: no time recorded", event.Event)
		}
		counts[event.Event]++
	}

	for event, want := range map[string]int{
		audit.EventLoginFailed:  1,
		audit.EventCSRFRejected: 1,
		audit.EventLogin:        1,
		audit.EventChange:       1,
		audit.EventLogout:       1,
	} {
		if counts[event] != want {
			t.Errorf("%s: recorded %d times, want %d (all: %v)", event, counts[event], want, counts)
		}
	}
}

// Off is the default, and off has to cost nothing: a nil log takes every
// Record and does not create a file.
func TestAuditIsOffByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	app := newTestApp(t, WithAudit(audit.New(false, path, 0)))

	loginCookies(t, app)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a disabled audit log created %s", path)
	}
}

// The health check is asked by docker every 30 seconds and keeps nothing, and
// a visitor who has not logged in has nothing to echo a token with: neither
// gets a cookie. The login page is the exception - its form needs one.
func TestPublicPathsSetNoCookie(t *testing.T) {
	app := newTestApp(t)

	for _, cookie := range get(t, app, StatusPath, nil, nil).Cookies() {
		t.Errorf("%s set the cookie %q", StatusPath, cookie.Name)
	}

	cookies := get(t, app, LoginPath, nil, nil).Cookies()
	if len(cookies) != 1 || cookies[0].Name != CSRFCookieName {
		t.Errorf("GET %s set %v, want the CSRF cookie alone", LoginPath, cookies)
	}
}

// The limiter is off unless asked for, and on it answers the sixth attempt
// with 429 - from the form, from the API and from a basic-auth header alike,
// so moving to another door does not reset the count.
func TestLoginRateLimitAnswers429(t *testing.T) {
	app := newTestApp(t, WithLoginRateLimit(true))

	for i := range 5 {
		if resp := login(t, app, testUser, "wrong"+strconv.Itoa(i), "/"); resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, resp.StatusCode)
		}
	}

	resp := login(t, app, testUser, testPassword, "/")
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("the sixth attempt = %d, want 429", resp.StatusCode)
	}
	if got := resp.Header.Get(fiber.HeaderRetryAfter); got == "" {
		t.Error("the 429 carries no Retry-After")
	}
	// The page says the address is blocked, not that the password is wrong:
	// the second would send the operator looking for a typo that is not there.
	if page := body(t, resp); !strings.Contains(page, "Too many failed logins") {
		t.Errorf("the 429 does not explain itself: %.200s", page)
	}
	if page := body(t, login(t, app, testUser, testPassword, "/")); strings.Contains(page, "Incorrect username") {
		t.Error("a blocked address was told its password was wrong")
	}

	if resp := get(t, app, "/api/system/status", nil, nil); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("the API from a blocked address = %d, want 429", resp.StatusCode)
	}
	if resp := write(t, app, nil, map[string]string{
		fiber.HeaderAuthorization: "Basic " + base64Std(testUser+":wrong"),
	}); resp.StatusCode != fiber.StatusTooManyRequests {
		t.Errorf("basic auth from a blocked address = %d, want 429", resp.StatusCode)
	}
}

// Off is the default: a wrong password is 401 every time, however many.
func TestLoginRateLimitIsOffByDefault(t *testing.T) {
	app := newTestApp(t)

	for i := range 10 {
		if resp := login(t, app, testUser, "wrong"+strconv.Itoa(i), "/"); resp.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, resp.StatusCode)
		}
	}
}

// Two logins in the same second are two different sessions, and a token minted
// before the nonce existed is still accepted: an upgrade must not log anybody
// out.
func TestSessionsAreDistinctAndLegacyTokensSurvive(t *testing.T) {
	a := newTestAuth()
	now := time.Unix(testNow, 0)

	if a.issue(now) == a.issue(now) {
		t.Error("two logins in the same second produced the same token")
	}

	payload := testUser + "|" + strconv.FormatInt(now.Add(SessionTTL).Unix(), 10)
	legacy := base64url(payload) + "." + a.sign(payload)
	if !a.sessionValid(legacy, now) {
		t.Error("a token without the nonce was rejected")
	}
}
