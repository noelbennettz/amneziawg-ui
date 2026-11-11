# Changelog

## Unreleased

- **A write now has to prove it came from the panel.** The session cookie is an
  ambient credential: the browser attaches it to anything that asks, so a form
  on another site could make it delete a server. Every `POST`, `PUT` and
  `DELETE` therefore also has to carry a token that matches the one in a second
  cookie, `awg_csrf` — the cookie an attacker's page can cause to be sent but
  cannot read, and cannot put into a header. The wasm client reads it and sends
  `X-CSRF-Token`; the login form carries it in a hidden field and the server
  rotates it when the session starts. A request that authenticates with an
  `Authorization` header is exempt, because no browser attaches one by itself —
  which is what keeps `curl -u` and the existing automation working. The session
  cookie moves from `SameSite=Lax` to `Strict` with it. `TestWriteNeedsTheToken`,
  `TestWriteWithBasicAuthNeedsNoToken`, `TestLoginFormNeedsTheToken`,
  `TestLoginRotatesTheToken`.

- **The panel is served under a Content-Security-Policy**, along with
  `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff` and
  `Referrer-Policy: no-referrer`. The policy allows the panel's own origin and
  `'wasm-unsafe-eval'`, and exactly one inline script — by nonce, generated when
  the process starts and written into the loader page the server already
  rewrites. `fyne package` calls that script from an `onload` attribute, which
  no nonce can rescue, so the call moves into the script as a load listener.
  The two scripts the bundle injects at runtime (phone input, wheel handling)
  read the same nonce from a `<meta name="csp-nonce">` the page now carries.
  A loader page without the markers is served untouched and the policy falls
  back to `'unsafe-inline'` rather than breaking a page the server cannot see
  into. Verified in Chrome 157: the canvas boots, the injected scripts run under
  touch emulation, and the console reports no violation.
  `TestLoaderIsPreparedForTheNoncePolicy`, `TestContentSecurityPolicy`.

- **`WEB_UI_SECURE_COOKIE`** pins the `Secure` flag on the cookies (`true`), or
  off (`false`), or leaves it to the request and `X-Forwarded-Proto` when it is
  not set. Behind a TLS-terminating proxy the container sees plain HTTP, so the
  flag used to be absent exactly where it mattered. **`WEB_UI_BIND`** sets the
  address the panel binds inside the container: unset is still the dual-stack
  listener, and `127.0.0.1` is for `network_mode: host` — with published ports
  the restriction belongs on the host side of the mapping
  (`"127.0.0.1:54845:54845/tcp"`), because Docker forwards to the container's
  address and not to its loopback.

- **`WEB_UI_AUDIT_LOG`** (off by default) appends logins, logouts, rejected
  writes and every change made through the API to `/etc/amnezia/audit.log`:
  time, address, user, method, route and status, one JSON object per line.
  Bodies are never recorded, and neither is the query string, so no key and no
  `.conf` can end up there. A nil log takes every event and creates nothing,
  so the default costs a branch and no file.
  `TestAuditRecordsLoginsAndChanges`, `TestAuditIsOffByDefault`.

- **`SYS_MODULE` is gone from the compose file.** The tunnel runs as a
  userspace process and nothing in the image loads a kernel module, so the
  capability was a door with no reason to be unlocked. `NET_ADMIN`, the TUN
  device and the four `ip_forward`/`forwarding` sysctls are the whole set.

- **The image is published under two tags**: `latest`, and the date the build
  was made (`2026.11.11`). Pinning the dated tag turns an upgrade into a
  decision; a second push on the same day moves that day's tag, because the
  panel carries no version of its own and the registry holds one version per
  tag. `SECURITY.md` documents the threat model, the reporting channel and what
  is deliberately not implemented yet.

- **The loading screen is dark whatever the browser prefers.** `fyne package`
  writes a loader page that picks between a light and a dark stylesheet *and* a
  light and a dark spinner with `prefers-color-scheme`, while the panel behind it
  forces the dark variant regardless. A browser set to light therefore showed a
  white page - with a white-page spinner on it - between the login and the
  panel, and a dark browser got the loader's `#141415` rather than the `#131314`
  the rest of the product is painted in. The server now pins the page to the dark
  branch (dark stylesheet and dark spinner unconditional, the light pair never
  matching) and serves it with the panel's page colour. It is done in
  `internal/frontend`, the one place that hands the page out, because
  `web-ui/wasm/` is generated on every build; a loader page from a future
  `fyne package` that no longer carries those markers is served untouched.
  `TestLoaderIsServedInTheDarkVariant`.

- **The login page leads with the panel's blue.** Its *Sign in* pill was the
  pastel primary (`#a8c7fa`), while the one large accent the panel itself shows
  is the traffic card in the primary *container* tone (`#0842a0` with
  `#d3e3fd`): two different blues, one on each side of the login, so the form
  and the page behind it did not read as the same product. The pill now uses the
  container pair; the pastel stays for the panel's small buttons and focus ring,
  where it belongs.

- **The login page paints the browser's own surfaces too.** The page colour was
  on `body` alone, so whatever the browser draws outside it - the strip under a
  phone viewport sized to the *large* height, the overscroll area, the area
  behind the URL bar - came out light, while the panel after login sets `html`,
  `body` and `overscroll-behavior` itself. `html` now carries `#131314`,
  `:root` declares `color-scheme: dark` (scrollbars and native controls follow),
  and the page is sized with `100dvh` where it is known so the card is centred
  in the viewport a phone is actually showing.

- **The panel answers on IPv6.** Fiber's listener defaults to `tcp4`, so
  `Listening on :54845` was really `0.0.0.0:54845`: the container published the
  port on both stacks and the process behind it only accepted IPv4, which made
  every `http://[2001:db8::1]:54845` attempt fail. The app now listens on the
  dual-stack `tcp` network (`httpapi.ListenConfig`), one listener for both
  families, falling back to `0.0.0.0` where the kernel has no IPv6 at all.
  `TestFiberConfigListensOnBothStacks` connects from `127.0.0.1` and `::1` to
  the listener that config builds.

- **A client config can carry an IPv6 endpoint.** `Endpoint = %s:%d` wrote
  `Endpoint = 2001:db8::1:54844`, which is not that address with a port - it is
  a long address that does not exist - so a server whose endpoint was an IPv6
  literal produced configs that never connected. The line (and the
  `# Server Endpoint` comment above it) is built by `api.EndpointAddress`,
  which brackets IPv6 and leaves names and IPv4 addresses alone.

- **The endpoint field is validated instead of guessed at.** It took any text:
  `vpn.example.com:54844` came out as `vpn.example.com:54844:54844`, and a
  bracketed literal only worked if the operator happened to type the brackets.
  `api.ParseEndpoint` now reads it as a host - name, IPv4, or IPv6 with or
  without brackets, stored canonically - and the create request is refused with
  a `400` for anything else. The form runs the same check.

- **NAT66 and the forwarding rules match the real uplink.** Both iptables
  scripts matched `-o eth+`, which is the container's bridge interface: on a
  host-network deployment, or any host whose uplink is not `eth*`, the
  masquerade and forwarding rules matched nothing - the tunnel came up and no
  client reached the internet through it, on either stack. The scripts read the
  uplink from `ip route show default` / `ip -6 route show default` per family,
  warn when there is no default route *or* no global IPv6 address behind it
  (a default route onto a ULA-only interface masquerades to nothing), and
  cleanup tries every interface that exists, so an uplink that was renamed
  between setup and teardown does not leave a rule behind. `iptables-test` now
  reports the routing side too: an IPv4 default route for every server, and for
  a server with an IPv6 subnet also an IPv6 default route and a global IPv6
  address.

- **Detection no longer gives an IPv6-only host a broken endpoint.** The public
  address was only ever accepted if it parsed as IPv4, so a v6-only machine got
  `YOUR_SERVER_IP` and every config generated from it pointed at nothing.
  Detection now asks the v6 services when no v4 service answers, and falls back
  to the v6 route source (`ip -6 route get`) after the v4 one. IPv4 still wins
  on a dual-stack host: it is what every client can reach, and a name endpoint
  is the way to hand out both.

- **DNS resolvers are accepted on either stack.** The backend validated them
  with `IsIPv4` and the form with an IPv4 regex, so
  `2606:4700:4700::1111` was refused in both places - and a client on an
  IPv6-only network has no way to reach `8.8.8.8` at all. Both now take any IP
  literal (`api.IsIPAddress`) and store it canonically.

- **The server dialog shows the endpoint clients really get.** It showed the
  detected public IP only, so a server created with `vpn.example.com` looked as
  if the next *Refresh IP* had overwritten it, while its configs still carried
  the name. `ServerInfo` now carries `endpoint`, and the dialog shows it
  whenever it differs from the detected address.

- **The create form keeps following `DEFAULT_PORT`, `DEFAULT_SUBNET` and
  `DEFAULT_DNS`.** Two things broke that, and between them they made an
  operator's settings look like they had been ignored. The first is that the
  form moves the port and subnet fields itself when the operator's values are
  already taken - a second server cannot share the first one's port - and it
  wrote those auto-picked values without recording them as its own, so
  `SeedDefaults` read them as something the operator had typed and left them
  alone: after one server existed, changing `DEFAULT_PORT` reached the page no
  more. The second is that the status call, which is where those defaults come
  from, ran once per page load; a page left open while the container was
  recreated kept the old numbers for good. It now repeats on the server-list
  beat (30 s), and `SeedDefaults` still leaves a value the operator typed over.
  Both are pinned down by `TestTheFormKeepsFollowingTheBackendDefaults`.

- **A second server stays in the operator's block.** `nextFreeSubnet` used to
  fall back to walking `10.x.0.0/24` from the beginning as soon as
  `DEFAULT_SUBNET` was taken, so with `10.9.0.0/24` configured the second server
  landed on `10.0.0.0/24` - the built-in default, which reads as the setting
  having done nothing. It now walks the /24s that share the configured first two
  octets (10.9.1.0/24, 10.9.2.0/24, …) and only falls back to the 10.x search
  for a `DEFAULT_SUBNET` that is not a /24 or whose neighbours are all taken.

- **`AUTO_START_SERVERS` reaches the create form.** It was the one setting the
  panel never asked the backend about: `/api/system/status` has been reporting
  `auto_start_servers` all along, but the form ignored it and sent
  `auto_start: true` for every server created in it - the switch in the advanced
  mode was wired on at build time and the simple mode hardcoded the same value -
  so an operator who set `AUTO_START_SERVERS=false` got their servers started
  anyway. The switch now starts from the setting, the simple mode sends what it
  says, and, like the port, subnet and DNS fields, a value the operator flips
  themselves is left alone when the status call comes round again. The state
  remembers whether the backend ever answered, so a page that has not had its
  first status call does not read the zero value as "start nothing".
  `DEFAULT_PORT`, `DEFAULT_SUBNET`, `DEFAULT_DNS` and `DEFAULT_MTU` were already
  reaching the form, and the tests that pin them are now joined by one for this
  (`TestAutoStartFollowsTheBackendSetting`) plus a pair for the state's
  fallbacks.

- **The panel speaks six languages, and Chinese at all.** English and Russian
  are joined by French, Spanish and Chinese in both its scripts, so
  `web-ui/translation/` now holds `en`, `ru`, `fr`, `es`, `zh-Hans` and
  `zh-TW` - 213 keys each, plurals included, and `main_test.go` fails if one
  file drifts from another. Two things had to be solved before any of it
  showed up on screen. The first is the script: fyne's `closestSupportedLocale`
  returns a tag and `localeFromTag` turns it into `base[-region[-script]]`,
  dropping the script whenever there is no region - so a `zh-Hant.json` came
  back as plain `zh` and go-i18n read it as Simplified. A region survives, so
  the Traditional file is `zh-TW.json`, and a `zh-TW`, `zh-HK`, `zh-MO`,
  `zh-Hant` or `zh-Hant-TW` browser all reach it while `zh`/`zh-CN`/`zh-SG`
  reach the Simplified one; `TestTranslationsPickTheMatchingLanguage` pins
  every one of those cases. The second is the glyphs: fyne's faces stop at
  Latin, Greek and Cyrillic, and the wasm build has no system fonts behind
  them, so Chinese drew as empty boxes. `style/font.go` now embeds Noto Sans SC
  regular and bold (OFL, 30 890 glyphs - it serves both scripts and any Chinese
  typed into a server or client name) and `darkTheme.Font` hands it out.
  Italic text uses the regular face because the font has no italic cut, and
  monospace stays on DejaVu so the config view's columns line up.
  The consequence is size: the wasm bundle goes from 14 MB to 26 MB gzipped,
  and the image from ~23 MB to ~35 MB. The login page in front of the panel is
  plain HTML rather than part of the bundle, so its four strings are translated
  separately, in `internal/auth/i18n.go`, from the request's `Accept-Language`
  header (`zh-*` split into its two scripts, English the fallback), and the
  page declares `<html lang>`.

- **Redundant code out.** The two copies of `roundedRect` - the tonal disc
  behind an avatar, one in the server card and one in the client row - are now
  `widgets.RoundedRect`, next to the `surface` helper that wants the same
  rectangle. `stringsUpper` in the server card was a function that returned its
  argument, kept only for the comment explaining why a status chip is not
  shouted in upper case; the comment moved to the call. The two translation
  files that were already there lost the blank line in the middle of their JSON.

- **Dead code removed.** Three functions nothing reached any more: the manager's
  `Settings()` accessor, `backend.Client.Clients` (the frontend takes its clients
  from the server list payload, never from that endpoint), and a one-line
  `MonoText` re-export in the client row that only wrapped `widgets.MonoText`.
  Found with `golang.org/x/tools/cmd/deadcode` over both binaries - the server
  with `-test`, and the wasm bundle under `GOOS=js GOARCH=wasm`. What it reports
  for the wasm build alone and not for the server is *not* dead: `api.
  ValidateISettings`, `NextFreeSubnetV6` and `seedGroup` are called by the
  backend, which is a separate `main`.

- **One test no longer fails one run in sixteen.** The test that checks the ULA
  /64 a new server gets expected the ID's first four hex digits to be spelled
  verbatim, so an ID starting `07cc` had to produce `fd42:07cc::/64` while the
  code produced `fd42:7cc::/64` - the canonical spelling, and the one the
  server's own address is built from. IPv6 text drops a group's leading zeros,
  so the test was wrong and the code was right; it now states the rule, and a new
  case in `web-ui/api` pins it down without depending on the random ID.

- **A field that has focus looks like it, and a menu item reacts to the pointer.**
  Three separate reasons why nothing on the page acknowledged a click, all of
  them fixed here. The first is the theme: fyne builds the text caret out of
  `SizeNameInputBorder` (the caret rectangle is that many points wide), and this
  theme answered `0` there because zero is what removes the outline from a
  filled M3 field - so the field took focus, swallowed the keystrokes and showed
  no cursor and no focus ring, which fyne draws in the same stroke. The size is
  now `2` and the stroke colour is transparent, so the field keeps its filled
  look while a focused one gets the two-point primary ring and the caret M3
  draws. The second is the overflow menu's rows: their hover handlers were
  declared as `MouseIn(*fyne.PointEvent)`, and `desktop.Hoverable` is defined
  over `*desktop.MouseEvent`, so the driver never treated a row as hoverable at
  all and the highlight in the renderer was dead code - a compile-time assertion
  now holds the two interfaces to the row. The third is that a row had no pressed
  state to begin with: hover is a mouse, and a finger never hovers, so on a phone
  an item vanished under the dialog it opened with nothing on screen saying the
  tap had landed. It now takes the accent as a state layer while it is held
  down.

- **The monitoring header is a block like everything under it.** The section
  title used to sit straight on the page background while the create form and
  every server below it was a surface, which made the dashboard read as a
  heading with cards under it rather than as the first of the page's blocks. It
  is a card now, with the same surface and the same corner radius.

- **The panel has a login page instead of the browser's password prompt.** Every
  route but `/status`, `/login` and `/logout` now needs a session cookie called
  `awg_session`, handed out when the form at `/login` is posted with the pair
  from `WEB_UI_USER` and `WEB_UI_PASSWORD` - the same two variables, the same
  base64-of-SHA-256 format, and nothing else stored anywhere. The cookie is
  `HttpOnly` and `SameSite=Lax`, is marked `Secure` only when the request itself
  arrived over TLS, lasts 12 hours, and is signed with an HMAC whose key is
  derived from the credentials: a container restart keeps you logged in, changing
  `WEB_UI_PASSWORD` ends every session at once. The page itself is one
  server-rendered HTML document embedded in the binary (`internal/auth`), with no
  JavaScript and no external assets, drawn in the panel's Material 3 dark palette
  so the form and the panel after it look like one thing. A navigation that asks
  for a guarded path is redirected to `/login?next=...` and lands back on it
  afterwards; anything under `/api` or `/debug` - or any request that sends
  `Accept: application/json` - gets `401` with
  `{"error":"authentication required"}` instead. `curl -u admin:changeme` and
  every other script keep working unchanged: an `Authorization: Basic` header is
  still accepted on every route, and no `WWW-Authenticate` header is sent any
  more, which is why the browser no longer opens a credential dialog. The app bar
  carries a **Log out** action at its right-hand end - the label drops to the icon
  alone on a phone, like the refresh action beside it - which follows `/logout`
  and leaves the login page on screen.

- **Nothing on the page is drawn on top of anything else any more.** Two things
  behind the overlapping text in the create form's parameter block. A wrapped
  paragraph reports its height as one line height per line, but the lines were
  stacked in a `container.NewVBox`, which puts the theme's 4 points between
  every pair - so every note was given four points per line less than it drew,
  and its last lines landed on whatever came after it. And a fyne layout asks
  every child for its minimum size before it gives any of them a width: on the
  pass that narrows the page the notes were stacked at the height they had a
  moment ago and then wrapped into two or three more lines, which put one note
  on top of the next until something else laid the page out again. The lines
  are now stacked with no padding, and the page and the dialogs lay their
  content out twice per pass - once to learn the width, once to stack at the
  heights that follow from it.

- **Scrolling moves by what the wheel says.** The wasm driver hands fyne the
  browser's pixel delta as if it were a wheel notch, and fyne then doubles
  anything from 5 up: one notch of 100 pixels asked the page to move 200
  points, eight times what the same notch moves a desktop fyne window, while
  the one or two pixel events a trackpad sends fell under that cutoff and
  moved a fraction of a point each - so scrolling jumped in huge steps with a
  crawl in between. The wheel is now coalesced into at most one event per
  frame, converted from pixels (or lines, or pages) to exactly the points the
  page should move. The driver's wheel listener is also registered on the
  document without `{passive: false}`, which every browser since Chrome 73
  treats as passive: its `preventDefault` was discarded and each wheel event
  printed an `[Intervention] Unable to preventDefault inside passive event
  listener` line in the console.

- CI publishes a single image tag, `latest`, and no longer runs on `v*.*.*`
  tags: every push to `main` replaces `latest` in place, so the package keeps
  one version instead of one per release. Version tags no longer produce
  `3.1.10`, `3.1` or `3` images, and the `main` tag is gone.
- The README screenshot carries the current monitoring panel with the public IP
  in the header blacked out.

## Version 3.1.10

- **IPv6 and NAT6 are now on for every server and every client.** A client's
  default `AllowedIPs` is `0.0.0.0/0, ::/0`, but the tunnel had no IPv6
  address, so `::/0` went into the tunnel and vanished there: IPv6 on the
  internet never worked for any client. Every server now gets a free ULA /64
  (`fd42:<server ID>::/64`, only `fc00::/7`), the server takes the first
  address in it, clients the ones after it, and a deleted client's address
  returns to the pool. The `Address` line in the configs carries both
  addresses (`Address = 10.0.0.1/24, fd42:ab12::1/64`) and a peer in the
  server config gets `AllowedIPs = 10.0.0.2/32, fd42:ab12::2/128`.
- `scripts/setup_iptables.sh` writes the same rule set into `ip6tables`,
  including `POSTROUTING -j MASQUERADE` for the tunnel subnet — that is NAT6,
  through which clients reach IPv6 over the host's uplink. `GET
  /api/system/iptables-test` shows three extra checks for a server with an
  IPv6 subnet; a missing `ip6tables` or `ip6table_nat` module shows up there
  too, while the server still comes up.
- A custom IPv6 subnet is set with the `subnet_v6` field of `POST
  /api/servers` or the "IPv6 subnet" row in the advanced form; an empty value
  picks a free one. The server card and the client row show the addresses of
  both stacks.
- The config is brought to schema version 5: existing servers and clients get
  IPv6 addresses, and the panel appends them to their `.conf` files on startup
  (the server's `Address` and every peer's `AllowedIPs`, suspended ones
  included), so servers do not have to be recreated after the update.
- The page is redrawn in the Material 3 dark language: neutral surfaces with
  one blue accent, 28px cards without borders, a regular-weight type scale,
  chips instead of badges, switches instead of checkboxes, and the actions of a
  server or client row folded into an overflow menu. The dashboard is one
  accent card with the traffic on a two-series chart plus four gauges, and
  outcomes appear in a snackbar rather than in the footer line.
- Dialogs and the row menus are drawn by the page itself: fyne's dialog and
  pop-up menus brought their own chrome - a small title, a hairline border, an
  almost-square card, no elevation - so they now sit on the same rounded
  surfaces as everything else, with the actions as text and one filled button.
- **The page works on a phone.** Three separate things had to change. The
  loader page carried no viewport meta at all, so a phone laid the canvas out
  at the browser's 980-point default and scaled the whole thing down to fit the
  screen; it is now pinned to the device width. The layout has a breakpoint
  below 720 points: the app bar puts its facts under the title, the gauges go
  under the traffic card, the create form folds from two columns into one, and
  a row's actions drop under its name instead of being pushed off the edge.
  And the page no longer demands more width than a phone has - the widest
  offenders were a server's monospace meta line (430 points on its own) and a
  wrapped label, which reports its minimum as the height it takes at one word
  per line.
- Typing works on a phone: tapping a field used to focus nothing that could
  raise the browser's keyboard - fyne's wasm driver ships that hook but never
  calls it - so the page now keeps the loader's input element focused while a
  field has focus, and feeds what the soft keyboard types back into the page.
  Dragging scrolls the page, which the wasm driver does not handle either.
- A browser window narrower than the two-column layout folds into the compact
  one, and back, as it is dragged - no reload. Two things were wrong there.
  fyne sizes its canvas to at least the content's minimum, so a page carrying
  the 700 points of its wide layout was laid out 700 points wide inside a
  588-point window and could never see that the window was narrow: the cards on
  the right were simply clipped. The page now reports only what a window can
  hold, and the breakpoint moved to 1024 - the width the wide layout actually
  needs, rather than the 720 that merely looked like "a phone".
- `DEFAULT_PORT`, `DEFAULT_SUBNET` and `DEFAULT_DNS` now reach a server created
  in the panel. The form carried built-in numbers of its own (54844,
  10.0.0.0/24, 8.8.8.8/1.1.1.1) and only `DEFAULT_MTU` was honoured - that one
  is read back from `/api/system/status` - so setting `DEFAULT_DNS` changed
  nothing anybody could see. The three are now read back the same way, into the
  fields nobody has typed over yet, and the simple mode sends them too.
- Dialogs keep their actions on screen. A card sized itself to its content, so
  on a phone - where the client's config page is a QR code above a column of
  config - it grew past the window and took its only button with it: there was
  no way to close it. The card now holds the room the window allows, scrolls
  what does not fit above a pinned action row, and the two dialogs that used to
  wrap their content in a scroll of their own no longer do.
- A server card's name and meta take the width the card has instead of their
  own minimum, so on a wide window the meta stays one line instead of breaking
  into four fragments in the middle of the card, and the client rows show both
  of a peer's addresses on one line again.
- The page lays its content out again whenever something in it changes height -
  a traffic counter growing, a wrapped line appearing, the breakpoint flipping -
  instead of only repainting. Fyne lays a container out only when it is
  resized, so a text that grew kept the geometry measured before it: the
  separator and the client list of a card were drawn over the meta it had just
  re-wrapped.
- The app bar's pills no longer draw over each other when the public IP
  arrives: a text that grows does not resize the row it sits in, so the row is
  laid out again when one of its figures changes (the same for a server's and a
  client's traffic counters).
- The browser tests address the page by name (`window.awgTarget` /
  `window.awgAction`) instead of by coordinate, so a redesign no longer leaves
  a page of numbers to re-measure. They also run once at phone size with touch
  input, and fail if anything reaches past the right edge.
- CI publishes the image to GitHub Container Registry
  (`ghcr.io/<owner>/<repository>`) with the workflow's built-in
  `GITHUB_TOKEN` — on every push to `main` (tags `main` and `latest`) and on
  semver tags (`3.1.10`, `3.1`, `3`, `latest`). Docker Hub is no longer used,
  so no `DOCKERHUB_*` secret is needed.

## Version 3.1.9

- The `vpn://` link is assembled the same way as the AmneziaVPN app's own
  client export: without `isThirdPartyConfig` and with the `.conf` text in
  `last_config.config`. The flag used to send the app to an empty raw-config
  page; now it opens the AmneziaWG connection settings with an editable MTU.
- When importing any `vpn://` link the AmneziaVPN app replaces the MTU with its
  own 1376, whatever the link says; with a server MTU of 1280 some sites stop
  loading. The link tab in the client config dialog now warns about this when
  the server MTU is not 1376 and suggests setting it in the app by hand or
  importing the `.conf`, which keeps the MTU.

## Version 3.1.8

- Every client row has an "online" light to the left of the rx/tx counters:
  green when the last handshake was less than three minutes ago (the lifetime
  of a WireGuard session), grey otherwise. The time comes from
  `awg show <iface> latest-handshakes` and arrives in `/api/traffic` as the
  `online` field.
- A client's auto-suspend time is picked in a calendar
  ([fyne-datepicker](https://github.com/sdassow/fyne-datepicker)) behind a
  button next to the field; typing it by hand in the `2006-01-02 15:04` format
  still works.
- The auto-suspend time fires once: suspending clears it, so a client switched
  back on by hand afterwards is not suspended again on the next minute.
- The browser tests were rewritten from `@playwright/test` to Go via
  [playwright-go](https://github.com/mxschmitt/playwright-go): a separate
  `e2e` module, plain `go test`, no Node.js and no `npm install`. The REST API
  checks decode responses into the shared `web-ui/api` structs; `make vet`
  covers that module too.
- Creating a server checks the port and the subnet before saving rather than at
  start: a port outside 1–65535 and a subnet that is not an IPv4 CIDR with room
  for at least one client (no longer than `/30`) are rejected with `400`, and a
  subnet overlapping another server's (the same one, containing it or sitting
  inside it) with `409` and that server's name. The rules are shared by the
  form and the backend (`api.ParseSubnet`, `api.SubnetsOverlap`); the form
  fills in a free subnet when it opens.
- A failed server start shows the reason: the stderr of the failing step (for
  example `Line unrecognized: …`) is taken from `awg-quick`'s output instead of
  a bare `exit status 1`, and without the trace, the userspace notice and the
  banner.

## Version 3.1.7

- Server and client traffic counters come from one source —
  `awg show <iface> transfer`. The server used to take its bytes from
  `ifconfig` (the decrypted payload inside the tunnel) while clients came from
  `awg show` (the encrypted UDP outside, headers, handshake and junk packets
  included), and the clients' sum was 4–5 % above the server's figure. The
  server is now the sum of its peers.
- A collapsible "Monitoring" panel sits above the create-server form (open by
  default): five charts for the last 5 minutes — outgoing and incoming VPN
  traffic summed over all servers (current rate on top, total transferred
  below), CPU load (core count below), RAM and disk usage (used / total
  below). The history lives in the browser only and starts over on a page
  reload; the backend stores nothing.
- `GET /api/traffic` was extended: next to the rounded `rx`/`tx` in
  `server_traffic` there are exact `rx_bytes`/`tx_bytes` (the same `ifconfig`
  numbers the frontend derives the rate from), and the new `system` field
  carries instantaneous host readings through `gopsutil`: cumulative CPU time
  (`cpu`), memory and the disk under `/etc/amnezia` (`memory`, `storage`). New
  `internal/sysinfo` package.
- The browser tests collapse the panel before clicking (`startApp`); the
  coordinates in the specs are shifted by its collapsed height.
- The traffic counters in the server card and the client rows are split into
  two values with download/upload icons (the same as on the monitoring tiles)
  instead of a textual `RX … · TX …`.
- Uptime: the server's interface line shows how long it has been up
  (`up 4h 12m`), the header shows how long the backend itself has been running.
  The interface counts from the moment the backend first saw it up (a start
  from the panel or the first traffic poll); stopping resets the counter. The
  API exposes it as the `uptime_seconds` field in `server_traffic` of
  `GET /api/traffic` and in `GET /api/system/status`.
- The client configuration dialog moved to tabs with icons: the `.conf` with
  the QR code opens first, the AmneziaVPN link second; each tab is built once,
  with no state switching between views.
- Fixed the page "freezing" after the tab spent more than a minute in the
  background (another tab, a minimised window): on return nothing refreshed and
  clicks did nothing until the window was resized or the page reloaded.

## Version 3.1.6

- The backend was split into packages by area of responsibility under
  `internal`
- Socket.IO is gone from both sides: the panel already kept REST polling as a
  fallback, and the "live" channel in wasm could only work over long-polling —
  the same HTTP, just with an extra library, its own protocol and exceptions in
  the middleware. Counters now refresh every 5 seconds with one request to the
  new `GET /api/traffic`, which replaced `GET /api/servers/traffic` and
  `GET /api/servers/{id}/traffic` (the fallback used to fetch each server's
  traffic separately), and the server list every 30 seconds and after every
  action. The header indicator shows `online`/`offline` instead of
  `live`/`polling`.
- The frontend was split into packages along the page structure (`newserver`,
  `serverlist` → `serverentry` → `clientlist` → `cliententry` →
  `clientdialogs`); components get their dependencies through `env.Env` rather
  than through a shared `UI` object.
- The interface was translated into Russian: the language comes from the
  browser settings (`navigator.languages`), English for everyone else. The
  translations live in `web-ui/translation/{en,ru}.json` and are embedded into
  the bundle through Fyne's `lang`; strings from the shared `web-ui/api`
  validation and backend errors stay in English.

## Version 3.1.5

- The captions of the numeric fields carry the recommended value
  (`Jc (recommended 4-12)`, `S1 (recommended 15-150)`, `H1 (recommended 1)`)
  while the placeholder shows the range the engine accepts. Sources: the
  AmneziaWG 3.1 parameter reference and the amneziawg-linux-kernel-module
  README.
- The parameter generator now follows what the documentation recommends for the
  header-protection plus `RandomTrailers` mode: all four paddings `S1`-`S4`
  equal, `H1`-`H4` = 1/2/3/4. Servers that already exist are not affected.
- The simple mode of the form no longer ignores `DEFAULT_MTU`: it used to send
  a hardcoded 1420, so the backend setting had no effect on servers created
  through the UI. The advanced mode seeds the MTU field with the same value
  when the form opens.
- The advanced mode has a "Roll S1-S4 separately" checkbox: equal paddings, as
  the vendor recommends, leave the difference between the message-type sizes at
  WireGuard's own 148/92/64/32, which can be recovered from the minimums.
  Separate values mask that at the cost of the receiver's margin in telling the
  types apart. The default is what the documentation says.
- The padding size is chosen so that a full-size transport packet fits the
  usual 1500 bytes: `S4` rides on every packet, so `1440 − MTU` is left for it.
  The hint on the MTU field was rewritten — above about 1425 fragmentation is
  unavoidable.
- Validation now rejects only what the engine itself rejects: `H1`-`H4` unique
  in `1-4294967295` (the old lower bound of 5 rejected exactly the recommended
  configuration), `S1`-`S4` in `12-65535` instead of homegrown 256/32 limits,
  and timing-parameter ranges in `0-65535` and ascending. Checked in the form
  and on the server.
- Fixed: pasting into any field crashed the whole application when the panel is
  not served over HTTPS and not from `localhost`. `glfw-js` takes
  `navigator.clipboard` once, at package initialisation, and outside a secure
  context it is not there — the first read panics and takes the program with
  it: the canvas freezes on the last frame, the console then only says "Go
  program has already exited", and only a page reload helps. Typing works until
  then, which makes it look like a paste bug rather than a crash. Copying,
  cutting and pasting now go through the browser's `copy`/`cut`/`paste`
  events, which need neither a secure context nor permissions, and `Paste` in a
  field's context menu goes through the panel's own buffer. Patch in
  `web-ui/internal/fixes/clipboard_wasm.go`.
- Fixed: the mouse wheel over a single-line input scrolls the page again.
- Fixed: the `Close`, `Cancel`, `Save`, `Yes`, `No` and `OK` buttons in dialogs
  show a hand cursor.
- Fixed: a client's `server_id` got corrupted — fiber's `c.Params()` points into
  the request buffer, which is reused, and the garbage reached the config.
  Cured with `Immutable: true`; configs already damaged are repaired by
  schema-4 migration. The VPN was not affected, only API responses.
- Fixed: auto-generation could produce identical `H1`-`H4` — the ranges shared
  their boundary values.
- The AmneziaWG rules moved into the shared `web-ui/api` module, where the wire
  structs live: the limits, `Validate`, `ValidateCPS`, `GenerateObfuscation`.
  The checks and the generator used to exist as two almost identical copies —
  in the frontend and in the backend — with different wording for the errors
  and a different set of checks (the backend one, for instance, never checked
  the MTU-dependent `S1`/`S2` bounds at all). There is one copy now, and one
  set of tests for the rules, while each side tests only its own plumbing.
- The `I1`-`I5` signature packets are parsed by the `newObfChain` grammar from
  `amneziawg-go` — in the form and on the server. The string used to go into
  the config as typed, and a typo surfaced at the client: the tunnel would not
  come up. The client dialog no longer closes on an error and loses the input,
  and the hint describes the tags themselves.
- `make e2e` brings a clean instance up on its own: `make e2e-reset` removes the
  container with its volume and rebuilds the image. The instance is separate
  from the one `make run` leaves behind; `make e2e-down` removes it.
- The header next to the title shows which build is open: the release tag and
  the Fyne build number — `v3.1.5 · build 44`. `fyne package` embeds both into
  the bundle as application metadata: it reads the tag from
  `web-ui/FyneApp.toml` and bumps the build number there on every packaging. A
  bundle built with a plain `go build` carries no metadata and says `dev`
  instead of inventing a version.

## Version 3.1.4

- The create-server form is split into two modes: name and port only by
  default, everything else generated; the "Advanced settings" checkbox reveals
  every field.
- A port already taken by another server or by the panel itself is rejected
  right away instead of surfacing later, when `awg-quick` cannot bind the
  socket. The form fills in the first free one.
- `CreateServer` errors are split by kind, and a port conflict answers `409`.
- Fixed typing non-ASCII characters (Cyrillic and the like): `glfw-js` measured
  the length of `KeyboardEvent.key` in bytes. Patches for upstream bugs moved
  into the `web-ui/internal/fixes` package.
- The image size was added to the README and DOCS (~26 MB against ~80 MB for
  `amnezia-wg-easy`).

## Version 3.1.3

- The same commit as 3.1.2, released again under a new tag. No code changes.

## Version 3.1.2

- The publish workflow reacts to semver tags only and builds the image tags from
  the parsed version: `v3.1.1` → `3.1.1`, `3.1`, `3`, `latest`.

## Version 3.1.1

- Multi-platform image build: the Go stages cross-compile from the build
  platform instead of emulating the target under QEMU.

## Version 3.1

- AmneziaWG 3.1 support: two new switches in the `[Interface]` section —
  `RandomTrailers` (a random-length "trailer" on every packet, so handshakes
  lose their fixed on-the-wire size) and `DisableCookies` (never send a cookie
  reply, skip the MAC2 check under load). Both are on by default for new
  servers, available as checkboxes in the create-server form, and written from
  the same stored values into the server `.conf`, into every client `.conf` and
  into the `vpn://` link. `RandomTrailers` must match on both sides — a
  receiver without it silently drops the oversized handshake — while
  `DisableCookies` is purely local. A client older than AmneziaWG 3.1
  (AmneziaVPN < 5.0.1.5) knows neither key, so clear the checkboxes if such
  clients are to connect. A switch that is off is written as the absence of the
  key rather than `= off`, because parsers before 3.1 reject an unknown key.
  Existing servers are unaffected: their stored parameters have no flags, and
  absence means "off".
- Fixed the `protocol_version` value in the native config export for
  AmneziaVPN: the app compares it against its `awgV3` constant, which is
  literally the string `"3.1"` in every version that knows about AmneziaWG 3
  (from 5.0.1.5 on). The previous `"3"` matched nothing, so the app showed the
  server with a pre-3.0 settings page and marked the container as outdated.
- **The upstream AmneziaWG versions are pinned in the `Dockerfile`**
  (`AWG_GO_VERSION=v3.1.20260828`, `AWG_TOOLS_VERSION=v3.1.20260812`) instead of
  cloning `master`, so a rebuild can no longer pick up an incompatible or
  broken commit unnoticed. A different tag is set with `--build-arg`.

## Version 3.0

- AmneziaWG 3.0 (header protection) support: the `HeaderProtectionKey`
  obfuscation parameter, enforcement of S1-S4 ≥ 12 and the optional
  "client-side" timing parameters (`ContentPaddingAddition`, `RekeyAfterTime`,
  `RekeyTimeout`, `RejectAfterTime`, `KeepaliveTimeout`, `MaxHandshakeAttempts`,
  `PersistentKeepalive`).
- **Breaking change**: support for AmneziaWG-1.0/1.5/2.0-only modes and the
  `awg2`/`awg3` request flags was removed. Enabling obfuscation now always means
  the full AmneziaWG 3.0 parameter set (header protection mandatory, S1-S4
  always ≥ 12). Servers already running are unaffected — the change only
  touches newly created ones.
- Native AmneziaVPN config export: the `vpn://…` link, the new
  `GET /api/servers/:id/clients/:clientId/link` endpoint, and a third tab in the
  client QR-code dialog. The plain-`.conf` importer in the official
  `amnezia-client` app always marks an imported AWG config as the outdated
  `amnezia-awg` container (shown as "AmneziaWG 2.0", without header
  protection) instead of the current `amnezia-awg2` — this native link carries
  the `container` and `protocol_version` fields the app uses to recognise the
  server as AmneziaWG 3.0.

## Version 2.0

First release.
