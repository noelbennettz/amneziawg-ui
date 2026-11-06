package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
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
func newTestApp(t *testing.T) *fiber.App {
	t.Helper()

	guard := newTestAuth()
	app := fiber.New(fiber.Config{Immutable: true})
	app.Use(guard.Middleware())
	app.Get(StatusPath, func(c fiber.Ctx) error { return c.SendString("uptime") })
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("<html>panel</html>") })
	app.Get("/api/system/status", func(c fiber.Ctx) error {
		return c.JSON(map[string]string{"ok": "yes"})
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

// login posts the form and returns the response.
func login(t *testing.T, app *fiber.App, user, password, next string) *http.Response {
	t.Helper()

	form := url.Values{"username": {user}, "password": {password}, "next": {next}}
	req := httptest.NewRequest(http.MethodPost, LoginPath, strings.NewReader(form.Encode()))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	return do(t, app, req)
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
	for _, want := range []string{"AmneziaWG UI", `name="username"`, `name="password"`, "Sign in"} {
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
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
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
