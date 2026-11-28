# Security policy

## Reporting a vulnerability

Open a private report with GitHub's *Report a vulnerability* button on the
repository's Security tab. That is a draft security advisory: it reaches the
maintainer, stays private until a fix is out, and gives you a place to attach a
proof of concept. If that form is not available to you, open a normal issue whose
title is exactly `Security contact request` and whose body says nothing about the
technical details — a maintainer will open a private channel there.

Please do not open a public issue for anything that lets someone into a panel
without the password, or lets a client reach something over the tunnel that it
should not.

There is no response-time guarantee: this is a one-person project maintained
alongside a full-time job. Reports are read and answered in the order they
arrive, and the advisory is where the timetable lives.

## Supported versions

The image is published as `ghcr.io/noelbennettz/amneziawg-ui` with two tags:
`latest`, which is the newest commit on `main`, and the date the build was made,
e.g. `2026.11.11`. Only the newest date is supported — there are no release
branches, and a fix lands on `main` and in the next build. The commit a build
came from is in `/api/version` and in the image labels, so a running panel can
say which build it is.

## Threat model

The panel is assumed to be reachable from the internet. Port `54845` offers a
`/status` page and a login form to anyone who scans it, and the tunnel port
answers with an AmneziaWG handshake. An attacker is assumed to be able to:

* guess and stuff passwords at the login form and at `curl -u` — each attempt
  makes the server run an argon2id derivation, so the guesser pays in time and
  the panel pays in memory;
* send requests with any header, cookie or body they like;
* make a victim's browser send requests to the panel from another site;
* read every response the panel gives them, including the client configs and
  `vpn://` links, once they are logged in.

An attacker is **not** assumed to have root or docker access on the host, to be
able to read the container's environment or its log, or to be on the path
between the browser and the panel after TLS is terminated. If they have any of
those, the password's hash is readable in `docker inspect` and in
`/etc/amnezia/admin.hash`, the generated password may still be in the log, and
`WEB_UI_PASSWORD` in the compose file is the least of the problems.

How the panel addresses that model:

* One credential pair, no user table: `WEB_UI_USER` and one password. It is
  kept as an argon2id hash (19 MiB, `t=2`, `p=1`) — in `admin.hash` on the
  config volume, or in `WEB_UI_PASSWORD` — and compared in constant time. The
  hash is never the password: with no variable and no file the panel generates
  one on the first start, writes it to `/etc/amnezia/first-run-password`
  (mode `0600`, not the log) and stores only its hash; the first successful
  login deletes the file. The base64 of a SHA-256 that earlier releases
  accepted is refused at startup — the format is fast to compute, and therefore
  fast to attack — and so is a hash that verifies `changeme`.
* Verifying a password is expensive by design, so it is bounded: no more than
  two derivations run at a time, and a Basic credential that verified is cached
  in memory for five minutes — an HMAC of the pair under a random per-process
  key, never the password, and never on disk. A password change empties that
  cache, because every entry carries the generation it was written under.
  A blocked address is refused before any derivation runs, so guessing cannot
  spend the server's CPU once the limiter has blocked it.
* A signed session cookie (`HttpOnly`, `SameSite=Strict`, 12 hours) whose key is
  derived from the password hash and, when one is configured, the TOTP secret:
  a restart keeps sessions, and changing either credential — in
  `WEB_UI_PASSWORD`, with `reset-password`, or by replacing
  `WEB_UI_TOTP_SECRET` — ends all of them. `Secure` follows
  `WEB_UI_SECURE_COOKIE`, the request, or `X-Forwarded-Proto`.
* A double-submit token on every state-changing request. A request that carries
  an `Authorization` header is exempt — that is not an ambient credential, and
  it is what keeps `curl -u` working.
* A CSP that allows exactly one inline script, by nonce, plus
  `'wasm-unsafe-eval'` for the wasm bundle; `frame-ancestors`/`X-Frame-Options:
  DENY`, `nosniff`, `no-referrer`.
* Request bodies are never logged. The API's own log line carries the method,
  the route, the status and the address; the URIs and private keys travel in
  response bodies only, and a list, a status answer or the result of an edit
  blanks the private-key fields — the only JSON carrying one is the config
  endpoint that exists to hand a config out.
* A suspended peer block is only ever written back into a `.conf` after its
  public key has been compared with the client it is being restored for. The
  `suspended/` directory is shared by every server and keyed by the client ID
  — six hex characters — so a block that is not that client's is refused, not
  relabelled onto its name and address; its preshared key and `AllowedIPs` are
  taken from the config, never from the file.
* Deleting a server deletes its clients' parked blocks with it, so a key that
  belongs to nothing does not stay on the volume.

## What is not there yet

Written down rather than left to be discovered:

* **Rate limiting is per address, and on by default.** Five failed logins block
  the address for 15 minutes, doubling on each further block up to a day.
  `WEB_UI_LOGIN_RATE_LIMIT=false` turns it off, which is the right setting behind
  a proxy that does not forward the client address — every request there looks
  like the proxy's address, and the limiter would lock the operator out along
  with whoever is guessing. `WEB_UI_TRUST_PROXY=true` is the other way out: the
  client address is then read from `X-Forwarded-For`, which is only safe when
  every request really does arrive through a proxy.
  It is per-address only: five guesses from five addresses are not slowed down
  at all.
* **The second factor is optional and time-based.** `WEB_UI_TOTP_SECRET` turns on
  a six-digit TOTP (RFC 6238, HMAC-SHA1, 30-second step, code accepted one step
  either side) in the login form and in an `X-TOTP-Code` header for Basic
  credentials. A code is single use: the last accepted step is remembered and a
  code from that step or an earlier one is refused, so a replayed code — the
  same digits sent twice, or one observed in transit — does not get in. That
  makes a code per request the cost of Basic auth with a second factor; a
  session cookie is the alternative for a long-running client. The counter is
  in memory, so a restart forgets it and the first code afterwards is accepted
  whatever step it falls in. There is no WebAuthn and no recovery codes, and one
  account, so the secret in the environment (or in the compose file) is the
  whole of the second factor's storage.
* **The generated password is readable on the volume until the first login.**
  The first start writes it to `/etc/amnezia/first-run-password` (mode `0600`)
  and does not print it, so the container log is not a copy; the file is deleted
  by the first successful login, so the plaintext lives only until somebody has
  read it. Anyone who can read the volume before that login can read it too —
  which is the same access that can read every client private key there.
  `api reset-password` replaces it and ends every session, which is the right
  answer if the volume has been copied.
* **No CSRF token on `GET /logout`.** It is a state change, it is reachable
  from another site, and `SameSite=Strict` is what keeps that from mattering.
* **The audit log is opt-in.** `WEB_UI_AUDIT_LOG=true` appends to
  `/etc/amnezia/audit.log`; entries are kept for `WEB_UI_AUDIT_RETENTION_HOURS`
  (24 hours by default, 30 days at most). The file is swept on startup and then
  hourly, so an entry can outlive the window by up to an hour, and the panel
  holds no lock on it — an operator who deletes the file just loses the record.

## The tunnel

The container runs the userspace implementation (amneziawg-go) and needs
`NET_ADMIN` alone — no `SYS_MODULE`, no host network by default. A client that
is connected to a server can reach that server's subnet and, through NAT, the
internet. It cannot reach another server's subnet unless you route it there.
