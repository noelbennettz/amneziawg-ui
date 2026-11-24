## Backend
- Go 1.26
- go-fiber 3

Backend packages live under `internal`, one responsibility each, with a
one-way dependency graph `httpapi → manager → {store, awg, wgconf,
amnezialink, publicip, config, sysinfo}`:

- `httpapi` — REST handlers; no `os`/`exec`, everything goes through the
  exported `manager` API and errors map to status codes in `fail`. Every list,
  status and edit response blanks the private-key fields (`redact.go`); only
  the config endpoints hand a key out
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

Placement of frontend files in the `web-ui` directory: `main.go` is the entry
point, the UI lives in `web-ui/internal/ui`, and the page, from top to bottom,
is `dashboard` (charts), `newserver` (create form), `serverlist` (cards).

`web-ui/api` is what the two sides share, pulled into the root module via a
`replace` directive: the wire structs, and the AmneziaWG rules that go with
them - the subnet bounds, `Validate`, `ValidateCPS`, `ValidateAllowedIPs`,
`GenerateObfuscation`. Anything both
the create form and the backend need goes there, never twice; a rule stated in
two places is a rule that drifts, and the user sees it as "the form let me,
the server did not". The backend's `validateObfuscationParams` and
`validateISettings` are thin wrappers that turn the shared problem lists into
one error.

`web-ui/internal/ui/browser` is the frontend's whole view of the page, and it
holds two kinds of thing. The DOM helpers the UI calls by name (the origin,
opening a link, handing over a file) and the page-level setup: `main.go` calls
`browser.Install()` before the window starts, which adds the viewport meta,
the on-screen keyboard, drag scrolling, a wheel event per frame, non-ASCII key
delivery, a clipboard that survives plain http, and the repaint a hidden tab
needs. Each of those is its own file and opens with the upstream behaviour it
answers and when it can be dropped, because they are the panel's half of the
browser contract - not patches kept away from the UI code.

Build the bundle with `make web-ui`; it lands in `web-ui/wasm`, which the
server serves straight off disk under the relative path `./web-ui/wasm`.

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
- **A key is the exact string the code passes to `lang.L`/`lang.N`.** Fyne
  answers an unknown key with the key itself, so a key that drifts from the
  source - a hint that was reworded, a status chip asking for `active` while the
  files carry `ACTIVE` - leaves that string in English in every language without
  any error. `TestTranslationsHaveTheSameKeys` only compares the files with each
  other; when a string moves, check it against the source too.

### The text face

Noto Sans SC (regular and bold, OFL) is the only reason a Chinese string is not
a row of empty boxes: fyne's own faces are Latin/Greek/Cyrillic plus an emoji and
a symbol font, and the wasm build has no system fonts behind them.

The two files are not in the bundle. They weigh ~17 MB, and only a browser that
reads Chinese, Japanese or Korean needs them, so `make web-ui` copies them to
`web-ui/wasm/font/` and `web-ui/font_wasm.go` fetches them from there when
`navigator.languages` says so, before the first widget is built. That is the one
place the frontend's bytes depend on the browser; everybody else keeps fyne's
faces and downloads a 12 MB bundle instead of 26. Consequences worth knowing:

- `style.NeedsCJK` decides, from the whole `navigator.languages` list - Fyne
  matches that list too and can land on Chinese from a probe that starts with
  German, and a first-entry-only rule would then draw boxes
- a failed fetch is logged and the panel runs with fyne's faces; it must never
  become a blank page
- `internal/frontend` gzips both files next to the bundle in the image and walks
  into `font/` for the packing and the ETags; `Open` also registers `.otf` as
  `font/otf`, because Go's mime table calls it an OpenDocument formula template
- italic requests get the regular face - Noto Sans SC has no italic cut, and a
  slanted English hint next to an unreadable Chinese one is worse than no slant
  at all. Nothing in the panel asks for one any more: the M3 type scale has no
  italic, so hints and notes are told apart by colour and size
- monospace stays on fyne's DejaVu face for the config view's column
  alignment, so Chinese in a `.conf` comment is boxes; keep that view ASCII

### The loading page

`internal/frontend/loader.go` holds the page a browser sees while the bundle
downloads, and the server serves it instead of the `index.html` "fyne package"
writes. That output is not used at all: it draws its progress with two animated
GIFs and speaks English, and rewriting it meant string surgery that had to fall
back to `'unsafe-inline'` whenever fyne moved a marker.

Owning the page leaves a contract with the wasm side that fyne upgrades can
break, so it is written down here:

- `wasm_exec.js` defines `Go`; `bundle.wasm` is the module; both come from the
  same directory the page is served from
- the canvas is appended to `document.body` when the app starts, and the page
  hides itself on seeing it - there is no callback from the module
- `#dummyEntry` is the input fyne's driver focuses for a soft keyboard; the
  touch handling in `web-ui/internal/ui/browser` looks it up by that id
- the `<meta name="csp-nonce">` is what the bundle reads back for the styles and
  scripts it injects (see `web-ui/internal/ui/browser`)

A fyne release that changes any of those shows up as a loader that never goes
away, which is why the frontend's browser tests wait for the canvas.

The page also picks the loader's language from `navigator.languages` with the
same rule as the Go side (`internal/auth/i18n.go` and the wasm's translation
list are the same six languages); the strings live in the page's own script,
not in the bundle, because the bundle is what it is waiting for.

### The login page

`internal/auth/login.html` is a plain HTML form, not part of the wasm bundle, so
its four strings are translated in `internal/auth/i18n.go` from the request's
`Accept-Language` header (highest q wins, English the fallback, `zh-*` split
into the two scripts). The page also declares `<html lang>`. Keep the two lists
(`internal/auth/i18n.go` and `web-ui/translation/`) in step: they are the same
six languages.

`web-ui/main_test.go` checks the files against each other and against the
locale a browser would report. It lives in `package main`, which imports fyne's
GL driver, so it needs GL headers (or CI with them) to build.

## Running the project, and checking the Docker build

From the project root, run:

```sh
  make run
```

## Linting the project (after changing backend code)

From the project root, run:

```sh
  make vet
```

`make vet` is `go vet` over the three modules. Two things it does not catch, and
both are worth a run before a change is called finished:

```sh
  # unreachable functions in the shipped binary. Anything that only a test
  # calls shows up here, which is the shape dead code takes in this repository:
  # delete it, or delete the accessor and check the behaviour instead.
  go run golang.org/x/tools/cmd/deadcode@latest ./...
  cd web-ui && GOOS=js GOARCH=wasm go run golang.org/x/tools/cmd/deadcode@latest ./...

  # unused fields, constants and functions, plus the usual simplifications
  go run honnef.co/go/tools/cmd/staticcheck@latest ./...
```

A function whose only caller is its own test is what that first command finds:
`totp.Key.Verify` was `Match` with the verdict thrown away, `Manager.ServerStatus`
was a wrapper around `serverStatus` that only the panel's tests called, and
`adminpw.Store.FromEnv` / `Store.Err` were accessors nothing but their own tests
read. All four are gone; do not add their successors back.

## Building the project

From the project root, run:

```sh
  make build
```

## Browser tests

The frontend is one WebAssembly canvas, so the browser tests in `e2e` click by
coordinate and assert against the REST API. They are Go tests on
[playwright-go](https://github.com/mxschmitt/playwright-go) in their own
module (`e2e/go.mod`). They expect an instance with no servers configured and
build on each other's state, so `make e2e` provisions one itself: it recreates
the `awgui-test` container from an empty volume on port 51836 first (see
`e2e/README.md`). That instance is deliberately separate from the stack
`make run` leaves behind — the reset must never discard the servers you are
working on. `make e2e-down` removes it.

```sh
  make e2e
```

For manual testing, you can use `playwright-cli`.

## CI

Five workflows: `docker.yml` (backend and wasm tests, image build and push to
GHCR, cosign signing, SBOM), `e2e.yml` (the browser suite against a built
image), `licences.yml`, `lint.yml` (actionlint) and `dco.yml`. Every workflow
prunes its own history: `prune-runs-before` at the top of `jobs:` and
`prune-runs-after` after the main jobs (`if: always()`) both run
`scripts/prune-runs.sh 3`, which keeps only that workflow's three newest runs.
The jobs authenticate with `github.token` and a job-level `permissions:
actions: write` + `contents: read`; do not introduce a PAT, a repository secret
or a cache key for them — a fork PR gets a read-only token, and the script is
written to warn and exit successfully in that case. Pruning is per workflow:
`prune-runs.sh` resolves the called workflow from `GITHUB_WORKFLOW_REF`, so it
never touches another workflow's runs.

## Authentication

`internal/auth` guards the panel; `internal/adminpw` owns the password itself.
The credentials are `WEB_UI_USER` (default `admin`) and one password, resolved at
startup in this order: `WEB_UI_PASSWORD`, which holds a hash — **argon2id**
(`$argon2id$v=19$m=19456,t=2,p=1$…`, what `api hash-password` prints); the
base64-of-SHA-256 format releases before it used is refused at startup; then
`/etc/amnezia/admin.hash` on the config volume (`0600`); and with neither, a
password generated on the first start, printed once in the log, with only its
hash stored. There is no user table. A hash that verifies `changeme` is refused
at startup.

Two things bound what verifying costs, because argon2id allocates 19 MiB per
attempt and a script re-sends its header with every request: at most two
verifications run at once (a semaphore), and a verified Basic credential is
cached in memory for five minutes, keyed by an HMAC under a random per-process
key. Both live in `internal/auth`. The cache and the sessions are tied to the
password: the session key is an HMAC over the hash in force, so a rewritten
`admin.hash` — which the store re-reads at most once a second, so
`reset-password` needs no restart — ends every session and empties the cache.

Three paths are public: `/status` (the docker health check), `/login` (GET renders
the page, POST checks the form) and `/logout` (GET or POST, drops the cookie and
redirects to `/login`). Everything else — the wasm bundle and `/debug/pprof`
included — needs either the `awg_session` cookie or an `Authorization: Basic`
header. The cookie is `HttpOnly`, `SameSite=Strict`, `Secure` according to
`WEB_UI_SECURE_COOKIE` (unset: the request and `X-Forwarded-Proto`), lives 12
hours, and is signed with an HMAC keyed from the password hash: a restart keeps
sessions, changing the password ends them all.

Two optional pieces of hardening sit in front of that. The per-address limiter
(`internal/auth/limiter.go`) is on by default and `WEB_UI_LOGIN_RATE_LIMIT=false`
turns it off — five failures in fifteen minutes block an address, doubling per
block up to a day. `WEB_UI_TOTP_SECRET` adds a second factor: a base32 secret,
six digits from `internal/totp` (RFC 6238, HMAC-SHA1, ±1 step) in the login
form's `totp` field, or in `X-TOTP-Code` for a Basic credential. With it set, a
password alone is not a login. A code is single use — `Auth.secondFactorValid`
consumes the step it matched under a mutex and refuses anything at or before it —
so a Basic client needs a fresh code per request (or a session cookie instead),
and the counter is in memory, forgotten on restart.

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

### In a browser

Log in once per browser context through the form; from then on the cookie is
sent with the page, the wasm bundle and every `fetch` from the UI, since all of
them are same-origin. `web-ui` needs no auth code of its own.

```sh
make run
```

Then open `http://localhost:54845/`, type `admin` and the password from
`docker compose exec app cat /etc/amnezia/first-run-password` (or the one
`WEB_UI_PASSWORD` was hashed from) into the form and submit. The form is the only thing in the panel with CSS selectors — the panel
itself is a canvas: `#username`, `#password`, `button[type=submit]` (the one
labelled `Sign in`).
A `playwright-cli` session is therefore `goto /login`, fill those two fields,
press the button, and continue with `goto /`; the context keeps the cookie.
Credentials in the URL (`http://admin:<password>@host/`) are **not** a substitute
any more: Chrome refuses `fetch()` on a URL that carries them, and no
`WWW-Authenticate` header is sent, so there is no prompt and no realm to reuse.

### From a script, and in `e2e`

`curl -u admin:<password>` still works: an `Authorization: Basic` header is
accepted on every route, which is why existing automation and the profile
targets in the `Makefile` needed no change. The e2e suite shows both halves —
`startApp` logs a fresh context in through the form (`submitLogin` in
`e2e/main_test.go`), and the raw HTTP helpers in the same file keep
`req.SetBasicAuth(user, password)`.

The panel's app bar has a **Log out** action at the right-hand end, next to the
refresh button; on a phone the bar drops both labels and leaves their icons. It
follows `/logout` in the browser (`browser.Navigate`, which replaces the
document) rather than calling the API, so the login page is what renders next.
