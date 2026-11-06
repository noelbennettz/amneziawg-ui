// Package e2e drives the real WebAssembly UI in Chrome through playwright-go
// and asserts against the backend's REST API.
//
// The UI is a single Fyne canvas: nothing in it can be selected by CSS, so
// the tests click by coordinate at a fixed viewport and check the result
// through /api/.... The tests share one backend and build on each other's
// state, so they run in file order, one at a time - never add t.Parallel.
package e2e

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
)

const (
	user     = "admin"
	password = "changeme"

	// Every coordinate in the tests assumes this viewport.
	viewportWidth  = 1500
	viewportHeight = 950

	// narrowWidth is the phone the compact layout is tested at: a small phone
	// held upright, which is what is left of a 6-inch screen once the
	// browser's own chrome takes its share.
	narrowWidth = 390

	// How long the loader page may take to fetch and start bundle.wasm.
	bootTimeout = 120 * time.Second
)

var (
	baseURL string
	browser playwright.Browser
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	baseURL = strings.TrimRight(cmp.Or(os.Getenv("AWG_URL"), "http://localhost:51836"), "/")

	// Only the driver: the tests reuse the Chrome installed on the machine
	// instead of Playwright's own download; the app needs WebGL either way.
	if err := playwright.Install(&playwright.RunOptions{SkipInstallBrowsers: true}); err != nil {
		fmt.Fprintln(os.Stderr, "install the playwright driver:", err)
		return 1
	}
	pw, err := playwright.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "start playwright:", err)
		return 1
	}
	defer pw.Stop()

	browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Channel: new("chrome"),
		Args:    []string{"--enable-unsafe-swiftshader"},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "launch chrome:", err)
		return 1
	}
	defer browser.Close()

	if err := os.MkdirAll("shots", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

// app is one test's browser page with the UI loaded in it.
type app struct {
	t    *testing.T
	page playwright.Page

	// touch is set for the phone-sized context: a tap there is a tap, not a
	// pointer being moved and clicked.
	touch bool

	mu     sync.Mutex
	errors []string
}

// newApp opens a fresh browser context - its own cookies and viewport, as
// Playwright Test gives every test - without loading the UI yet, so a test can
// hook page events before the first request goes out.
//
// The context starts out anonymous: the panel is behind a session cookie now,
// not behind HTTP basic auth, so there is nothing to hand the browser up front
// and a test that wants to see the panel has to log in through the form first.
func newApp(t *testing.T) *app {
	t.Helper()
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		BaseURL:  new(baseURL),
		Viewport: &playwright.Size{Width: viewportWidth, Height: viewportHeight},
	})
	must(t, err)
	t.Cleanup(func() { _ = ctx.Close() })

	page, err := ctx.NewPage()
	must(t, err)

	a := &app{t: t, page: page}
	page.OnPageError(func(err error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.errors = append(a.errors, err.Error())
	})
	return a
}

// newNarrowApp opens a phone-sized, touch-enabled context. Chrome treats it as
// a phone in the ways that matter here: the viewport meta is honoured, a touch
// synthesises the mouse events the driver reads, and window.maxTouchPoints is
// not zero - which is what makes the page install its on-screen-keyboard shim.
func newNarrowApp(t *testing.T, width, height int) *app {
	t.Helper()
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		BaseURL:  new(baseURL),
		Viewport: &playwright.Size{Width: width, Height: height},
		HasTouch: new(true),
		IsMobile: new(true),
	})
	must(t, err)
	t.Cleanup(func() { _ = ctx.Close() })

	page, err := ctx.NewPage()
	must(t, err)

	a := &app{t: t, page: page, touch: true}
	page.OnPageError(func(err error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.errors = append(a.errors, err.Error())
	})
	return a
}

// login signs the page in through the form at /login. The panel sits behind a
// session cookie rather than HTTP basic auth, so a fresh context gets a redirect
// to this form instead of the page; the form is plain server-rendered HTML, and
// therefore the only thing in the suite that can be driven by a selector.
func (a *app) login() {
	a.t.Helper()
	a.submitLogin(user, password)

	// A right pair answers 303 with the cookie and sends the browser to "/".
	// Waiting for that URL rather than for the click to return is what proves
	// the cookie was accepted and the request was not bounced again.
	err := a.page.WaitForURL(baseURL+"/", playwright.PageWaitForURLOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   new(float64((30 * time.Second).Milliseconds())),
	})
	if err != nil {
		a.t.Fatalf("logging in as %q did not reach the panel: %v", user, err)
	}
}

// submitLogin loads the form, fills it in and presses the button, and leaves
// the outcome to the caller: a wrong pair answers 401 and renders the form
// again, which only TestLogin has anything to say about.
func (a *app) submitLogin(name, secret string) {
	a.t.Helper()
	_, err := a.page.Goto("/login", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	must(a.t, err)
	must(a.t, a.page.Locator("#username").Fill(name))
	must(a.t, a.page.Locator("#password").Fill(secret))
	must(a.t, a.page.Locator("button[type=submit]").Click())
}

// startApp opens a page, logs it in and loads the UI in it.
func startApp(t *testing.T) *app {
	t.Helper()
	a := newApp(t)
	// The session has to exist before the first GET /: without it that
	// request is a redirect to the login form, not the panel.
	a.login()
	a.start()
	return a
}

// start loads the page and waits until the Fyne canvas is up and the app has
// fetched its first data from the backend.
func (a *app) start() {
	a.t.Helper()
	_, err := a.page.Goto("/", playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded})
	must(a.t, err)
	must(a.t, a.page.Locator("canvas").WaitFor(playwright.LocatorWaitForOptions{
		Timeout: new(float64(bootTimeout.Milliseconds())),
	}))
	// "fyne package" generates the loader page: it hides #main as soon as
	// Fyne puts its canvas up.
	_, err = a.page.WaitForFunction(`() => document.getElementById('main')?.style.display === 'none'`, nil,
		playwright.PageWaitForFunctionOptions{Timeout: new(float64(bootTimeout.Milliseconds()))})
	must(a.t, err)
	time.Sleep(3 * time.Second)

	// Fold the monitoring panel away. It opens by default and is a quarter of
	// the viewport tall, which would push the form and the cards the tests
	// click on below the fold; collapsed, the dashboard is one row.
	a.tap("monitoring.toggle")
}

// click moves the pointer first: the Fyne web driver taps wherever its last
// known cursor position is, so a bare click would land in the wrong place.
func (a *app) click(x, y float64) {
	a.t.Helper()
	if a.touch {
		must(a.t, a.page.Touchscreen().Tap(int(x), int(y)))
		time.Sleep(600 * time.Millisecond)
		return
	}

	mouse := a.page.Mouse()
	must(a.t, mouse.Move(x, y))
	time.Sleep(150 * time.Millisecond)
	must(a.t, mouse.Click(x, y))
	time.Sleep(600 * time.Millisecond)
}

// tap clicks the centre of a named widget. Every interactive part of the page
// registers one, so no test carries a coordinate: a card that grows a row, a
// button that moves into an overflow menu or a form that changes its column
// count cannot silently send the click somewhere else.
func (a *app) tap(id string) {
	a.t.Helper()
	rect, ok := a.rect(id)
	if !ok {
		a.t.Fatalf("no widget %q on the page", id)
	}
	a.click(rect["x"]+rect["w"]/2, rect["y"]+rect["h"]/2)
}

// tapAndType focuses a named field and types into it.
func (a *app) tapAndType(id, text string) {
	a.t.Helper()
	a.tap(id)
	a.typeText(text)
}

// act runs a named action: switching a dialog's tab, choosing an entry of a
// menu that the test does not need to open. These have no widget of their own
// to click while the suite runs.
func (a *app) act(id string) {
	a.t.Helper()
	known, err := a.page.Evaluate(`(id) => String(window.awgHas(id))`, id)
	if err != nil {
		a.t.Fatalf("action %q: %v", id, err)
	}
	if ok, _ := known.(string); ok != "true" {
		a.t.Fatalf("no action %q on the page", id)
	}
	if _, err := a.page.Evaluate(`(id) => window.awgAction(id)`, id); err != nil {
		a.t.Fatalf("action %q: %v", id, err)
	}
	time.Sleep(600 * time.Millisecond)
}

// rect asks the page where a named widget is, in points. The answer travels as
// a JSON string rather than as an object: playwright hands a returned object
// back as a map whose numbers do not all arrive as float64, which silently
// dropped every field but one.
func (a *app) rect(id string) (map[string]float64, bool) {
	a.t.Helper()
	value, err := a.page.Evaluate(`(id) => JSON.stringify(window.awgTarget(id))`, id)
	if err != nil {
		a.t.Fatalf("target %q: %v", id, err)
	}
	text, _ := value.(string)
	if text == "" || text == "null" {
		return nil, false
	}
	var rect map[string]float64
	if err := json.Unmarshal([]byte(text), &rect); err != nil {
		a.t.Fatalf("target %q: %v", id, err)
	}
	return rect, true
}

func (a *app) typeText(text string) {
	a.t.Helper()
	must(a.t, a.page.Keyboard().Type(text, playwright.KeyboardTypeOptions{Delay: new(25.0)}))
	time.Sleep(300 * time.Millisecond)
}

func (a *app) press(key string) {
	a.t.Helper()
	must(a.t, a.page.Keyboard().Press(key))
}

func (a *app) scroll(x, y, delta float64) {
	a.t.Helper()
	mouse := a.page.Mouse()
	must(a.t, mouse.Move(x, y))
	must(a.t, mouse.Wheel(0, delta))
	time.Sleep(800 * time.Millisecond)
}

// shot saves a screenshot of the page to shots/<name>.png.
func (a *app) shot(name string) {
	a.t.Helper()
	_, err := a.page.Screenshot(playwright.PageScreenshotOptions{Path: new("shots/" + name + ".png")})
	must(a.t, err)
}

// noPageErrors fails the test if the page threw anything uncaught - a Go
// panic in the wasm bundle surfaces this way.
func (a *app) noPageErrors() {
	a.t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.errors) > 0 {
		a.t.Errorf("uncaught page errors:\n%s", strings.Join(a.errors, "\n"))
	}
}

// apiGet returns the body of a GET to the backend, with the same credentials
// the browser uses, failing the test on anything but 200.
func apiGet(t *testing.T, path string) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	must(t, err)
	req.SetBasicAuth(user, password)
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	must(t, err)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s -> %d: %s", path, resp.StatusCode, body)
	}
	return body
}

// getJSON decodes the JSON the backend returns for path into a T.
func getJSON[T any](t *testing.T, path string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(apiGet(t, path), &v); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return v
}

// eventually retries cond once a second until it holds, and fails the test
// with what if it still does not after timeout.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s: %s", timeout, what)
		}
		time.Sleep(time.Second)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
