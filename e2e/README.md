# Browser tests

End-to-end tests that drive the real WebAssembly UI in Chrome through
[playwright-go](https://github.com/mxschmitt/playwright-go) and assert against
the backend's REST API.

The frontend renders into a single `<canvas>`, so there is nothing to select
by CSS: the tests click by coordinate (at a fixed 1500x950 viewport) and check
the result through `/api/...`, decoding the responses into the shared wire
structs of `web-ui/api`. Screenshots of every step land in `shots/`.

`startApp` collapses the monitoring panel at the top of the page before a
test gets to click: open, it would push the form and the cards below the
fold. Every coordinate in the tests assumes that collapsed 56px row above the
"Create New VPN Server" button; dialogs are centred in the window and do not
move either way.

## Authentication

The panel is in front of a login page at `/login`. It is server-rendered plain
HTML with no JavaScript, and therefore the only page in the suite that can be
selected by CSS: the helpers log a context in by filling `#username` and
`#password` and pressing the submit button. Browser contexts are created without
credentials - Playwright's `HttpCredentials` answered the old HTTP basic auth,
and there is no `WWW-Authenticate` challenge left for it to answer. The REST
helpers in `main_test.go` still send `Authorization: Basic`, which every route
keeps accepting, including `/api`.

The form is translated from the request's `Accept-Language`, so every context
in the suite states the language it works in (`Locale: en-US`) rather than
inheriting the host's; `TestLoginPageSpeaksTheBrowsersLanguage` asks for
`zh-TW` on purpose, and reads the translated labels straight out of the DOM.

## Layout

This directory is its own Go module, so playwright-go stays out of the root
`go.mod`; `web-ui` comes in through a `replace` directive, as it does for the
backend.

- `main_test.go` - `TestMain` installs the Playwright driver and launches
  Chrome once; the helpers give every test a fresh browser context with the
  viewport, log it in through the login form, and wrap clicks, typing,
  screenshots and the REST calls
- `00_login_test.go` - the login page: an anonymous `/` redirects to it, a wrong
  password shows the error and stays there, a right one lands on the panel
- `01_smoke_test.go` ... `09_dialog_test.go` - one test each
- `10_defaults_test.go` - the create form's defaults on a *second* instance,
  started with `AUTO_START_SERVERS=false`; skipped when the run has no such
  instance to talk to

## Running

From the repository root:

```sh
make e2e
```

That is the whole thing. `make e2e` depends on `make e2e-reset`, which
recreates the backends from scratch before every run: it removes both test
containers and their volumes, rebuilds the image from the working tree, starts
the main instance on port 51836 and a second one on 51837 with
`AUTO_START_SERVERS=false`, and waits for `/status` on both to answer.

The reset is why the suite gets its own container, volume and port rather than
the stack `make run` leaves behind: the tests share one backend and build on
each other's state - `02` creates the server that `03` adds a client to - so
they run in file order, one at a time (never `t.Parallel`), and expect to
start against an instance with no servers configured. Wiping state before a
test run must never discard the servers you are working on. The second
instance is separate for the same reason: its whole point is a setting the
other one does not have.

The instances stay up after the run, so a failed test can be inspected in the
browser at <http://localhost:51836> (admin / changeme) or, for the defaults
test, <http://localhost:51837>. `make e2e-down` removes the containers and the
volumes.

Override any of it with the Makefile variables `E2E_PORT`, `E2E_NAME`,
`E2E_VOLUME`, `E2E_IMAGE`, `E2E_URL`, `E2E_TIMEOUT`, `E2E_TEST_TIMEOUT`, and
their `_NOAUTO` twins (`E2E_PORT_NOAUTO`, `E2E_NAME_NOAUTO`,
`E2E_VOLUME_NOAUTO`, `E2E_URL_NOAUTO`) for the second instance.

To run the tests against a backend you started yourself, skip the Makefile:

```sh
cd e2e
AWG_URL=http://localhost:54845 go test -count=1 -v .
```

`AWG_URL` defaults to `http://localhost:51836`. Whatever it points at needs
`WEB_UI_USER=admin` and `WEB_UI_PASSWORD=BXugPWxEEEhj3HNh/kV4ll0YhzYPkKCJWILlimJI/IY=`
- the base64 SHA-256 of `changeme`, which is what the tests type into the login
form. `10_defaults_test.go` reads `AWG_URL_NOAUTOSTART` instead, and skips when
it is not set.

No Node.js is needed: on the first run `TestMain` downloads the Playwright
driver (with its own Node) into `~/.cache/ms-playwright-go`. Browsers are not
downloaded - the tests reuse the Chrome installed on the machine
(`Channel: "chrome"`); it needs WebGL, which the app requires.
