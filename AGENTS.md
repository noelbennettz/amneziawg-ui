## Backend
- Go 1.26
- go-fiber 3

Backend packages live under `internal`, one responsibility each, with a
one-way dependency graph `httpapi → manager → {store, awg, wgconf,
amnezialink, publicip, config, sysinfo}`:

- `httpapi` — REST handlers; no `os`/`exec`, everything goes through the
  exported `manager` API and errors map to status codes in `fail`
- `auth` — the way into the panel: the credential check against
  `WEB_UI_USER`/`WEB_UI_PASSWORD` (constant time), the signed `awg_session`
  cookie, the double-submit token, the login page embedded from
  `internal/auth/login.html` with its four strings in `i18n.go` (picked per
  request from `Accept-Language`), and the middleware that guards every route
  but `/status`, `/login` and `/logout`
- `security` — the response headers and the CSP nonce, which is generated once
  per process and written into the loader page
- `audit` — logins, logouts, rejected writes and API changes, as JSON lines in
  `/etc/amnezia/audit.log`; off unless `WEB_UI_AUDIT_LOG` is set
- `manager` — the state and the operations on it; the only package that
  takes the config lock. Split by file (`servers.go`, `clients.go`,
  `suspend.go`, `status.go`, `traffic.go`, `system.go`, `reconcile.go`)
- `awg` — the only package that shells out, behind the `Runner` interface;
  tests use `awg/awgtest` instead of the host
- `sysinfo` — CPU/RAM/disk readings of the host via `gopsutil`, carried in
  `/api/traffic` as instantaneous, cumulative figures; the dashboard keeps the
  history and computes rates, the backend stores nothing between polls
- `wgconf` — `.conf` text: rendering, peer markers, suspended blocks
- `amnezialink` — `vpn://` export; `store` — `web_config.json` and schema
  migrations; `config` — env → `Settings`; `frontend` — static assets

## Frontend
- Fyne v2.8 compiled to WebAssembly (GOOS=js GOARCH=wasm)
- Own Go module in `web-ui`, built with `go tool fyne package -os wasm`

Placement of frontend files in `web-ui` directory: `main.go` is the entry
point and the UI lives in the `web-ui/internal/ui`; the page top to bottom is
`dashboard` (charts), `newserver` (create form), `serverlist` (cards)

`web-ui/api` is what the two sides share, pulled into the root module via a
`replace` directive: the wire structs, and the AmneziaWG rules that go with
them - limits, `Validate`, `ValidateCPS`, `GenerateObfuscation`. Anything both
the create form and the backend need goes there, never twice; a rule stated in
two places is a rule that drifts, and the user sees it as "the form let me,
the server did not". The backend's `validateObfuscationParams` and
`validateISettings` are thin wrappers that turn the shared problem lists into
one error.

The one other frontend package is `web-ui/internal/fixes`: workarounds for
upstream bugs in fyne and its wasm driver, one per file, each documenting the
bug and when it can be dropped. `main.go` calls `fixes.Install()` before the
window starts; keep such patches out of the UI code.

Build the bundle with `make web-ui`; it lands in `web-ui/wasm`, which the
server serves straight off disk under the relative path `./web-ui/wasm`

## Frontend translations

The UI is in English, Russian, French, Spanish and Chinese (Simplified and
Traditional), through Fyne's `lang` package
(https://docs.fyne.io/explore/translations/). `web-ui/translation/*.json` are
embedded by `main.go` and loaded with `lang.AddTranslationsFS`; the language
comes from the browser's `navigator.languages`, `en` is the fallback.

Two things about that are not obvious:

- **The Traditional Chinese file is called `zh-TW.json`, not `zh-Hant.json`.**
  fyne picks a language in `lang.closestSupportedLocale` and hands the result to
  go-i18n through `lang.localeFromTag`, which returns `base[-region[-script]]`
  and drops the script whenever there is no region. A `zh-Hant.json` therefore
  comes back as plain `zh`, which go-i18n resolves to Simplified; a region
  survives the trip, so `TW` is what makes the file reachable. A `zh-TW`,
  `zh-HK`, `zh-MO`, `zh-Hant` or `zh-Hant-TW` browser all land on it, and
  `zh_CN`/`zh_SG`/`zh` on `zh-Hans.json`. `web-ui/main_test.go` pins this - see
  `TestTranslationsPickTheMatchingLanguage`; if a future fyne fixes
  `localeFromTag`, both files can go back to script subtags.
- **A plural entry lists the forms its own language has.** Chinese has one
  (`other`), Russian four, so `{{.Count}} clients` is an object in some files
  and a string in others. A form the language needs and the file lacks falls
  back to the English key with its template unexpanded.

### The text face

`web-ui/internal/ui/style/font.go` embeds Noto Sans SC (regular and bold, OFL)
and `darkTheme.Font` hands it out. It is there for the glyphs: fyne's own faces
are Latin/Greek/Cyrillic plus an emoji and a symbol font, the wasm build has no
system fonts behind them, and a Chinese string would otherwise draw as empty
boxes. Consequences worth knowing:

- italic text uses the regular face - Noto Sans SC has no italic cut, and the
  alternative was slanted English hints next to unreadable Chinese ones
- monospace stays on fyne's DejaVu face for the config view's column
  alignment, so Chinese in a `.conf` comment is boxes; keep that view ASCII
- the two files are ~17 MB of the wasm bundle, which is why the image grew

### The login page

`internal/auth/login.html` is a plain HTML form, not part of the wasm bundle, so
its four strings are translated in `internal/auth/i18n.go` from the request's
`Accept-Language` header (highest q wins, English the fallback, `zh-*` split
into the two scripts). The page also declares `<html lang>`. Keep the two lists
- `internal/auth/i18n.go` and `web-ui/translation/` - in step: they are the same
six languages.

`web-ui/main_test.go` checks the files against each other and against the
locale a browser would report. It lives in `package main`, which imports fyne's
GL driver, so it needs GL headers (or CI with them) to build.

# Running the project, and check docker build

In the project root directory, run command:
```sh
  make run
```

# Linting the project (after changes backend code)
In the project root directory, run command:
```sh
  make vet
```

# Building the project
In the project root directory, run command:
```sh
  make build
```

# Browser tests
The frontend is one WebAssembly canvas, so the browser tests in `e2e` click by
coordinate and assert against the REST API. They are Go tests on
[playwright-go](https://github.com/mxschmitt/playwright-go) in their own
module (`e2e/go.mod`). They expect an instance with no servers configured and build state across the
tests, so `make e2e` provisions one itself: it recreates the `awgui-test`
container from an empty volume on port 51836 first (see `e2e/README.md`).
That instance is deliberately separate from the stack `make run` leaves
behind - the reset must never discard the servers you are working on. `make e2e-down` removes it.
```sh
  make e2e
```

For manual testing possible to use `playwright-cli`

# Authentication

`internal/auth` guards the panel. The credentials are the pair the container was
started with — `WEB_UI_USER` (default `admin`) and `WEB_UI_PASSWORD`, which holds
the **base64 of the SHA-256 of the password** (default `changeme`, i.e.
`printf 'changeme' | openssl dgst -binary -sha256 | base64`). Nothing else is
stored: there is no user table and no extra environment variable.

Three paths are public: `/status` (the docker health check), `/login` (GET renders
the page, POST checks the form) and `/logout` (GET or POST, drops the cookie and
redirects to `/login`). Everything else — the wasm bundle and `/debug/pprof`
included — needs either the `awg_session` cookie or an `Authorization: Basic`
header. The cookie is `HttpOnly`, `SameSite=Strict`, `Secure` according to
`WEB_UI_SECURE_COOKIE` (unset: the request and `X-Forwarded-Proto`), lives 12
hours, and is signed with an HMAC keyed from the credentials: a restart keeps
sessions, changing `WEB_UI_PASSWORD` ends them all.

Every `POST`, `PUT` and `DELETE` also has to carry the CSRF token: the `awg_csrf`
cookie (not `HttpOnly` — the page has to read it) echoed in `X-CSRF-Token`, or
the hidden `csrf` field on the login form, which the server rotates when the
session starts. A request that authenticates with an `Authorization` header is
exempt, because no browser attaches one by itself. Two more things the panel
answers with, both in `internal/security`: a `Content-Security-Policy` that
allows the panel's own origin, `'wasm-unsafe-eval'` and one inline script by
nonce, and `X-Frame-Options: DENY`, `nosniff`, `no-referrer`.

`internal/audit` writes what an operator needs after the fact — login, logout,
a rejected write, a change through the API — to `/etc/amnezia/audit.log` as one
JSON object per line: time, address, user, method, route, status. It is off
unless `WEB_UI_AUDIT_LOG` is set, keeps `WEB_UI_AUDIT_RETENTION_HOURS` (24 by
default, 720 at most), and never records a body or a query string.

A request carrying neither proof is answered according to what it asked for:
anything under `/api` or `/debug`, or any request with `Accept:
application/json`, gets `401` and `{"error":"authentication required"}`; every
other request is a navigation and is redirected (`303`) to
`/login?next=<escaped original path>`.

## In a browser

Log in once per browser context through the form; from then on the cookie is
sent with the page, the wasm bundle and every `fetch` from the UI, since all of
them are same-origin. `web-ui` needs no auth code of its own.

```sh
make run
```

Then open `http://localhost:54845/`, type `admin` / `changeme` into the form and
submit. The form is the only thing in the panel with CSS selectors — the panel
itself is a canvas: `#username`, `#password`, `button[type=submit]` (the one
labelled `Sign in`).
A `playwright-cli` session is therefore `goto /login`, fill those two fields,
press the button, and continue with `goto /`; the context keeps the cookie.
Credentials in the URL (`http://admin:changeme@host/`) are **not** a substitute
any more: Chrome refuses `fetch()` on a URL that carries them, and no
`WWW-Authenticate` header is sent, so there is no prompt and no realm to reuse.

## From a script, and in `e2e`

`curl -u admin:changeme` still works: an `Authorization: Basic` header is
accepted on every route, which is why existing automation and the profile
targets in the `Makefile` needed no change. The e2e suite shows both halves —
`startApp` logs a fresh context in through the form (`submitLogin` in
`e2e/main_test.go`), and the raw HTTP helpers in the same file keep
`req.SetBasicAuth(user, password)`.

The panel's app bar has a **Log out** action at the right-hand end, next to the
refresh button; on a phone the bar drops both labels and leaves their icons. It
follows `/logout` in the browser (`browser.Navigate`, which replaces the
document) rather than calling the API, so the login page is what renders next.
