# Security policy

## Reporting a vulnerability

Open a private report with GitHub's *Report a vulnerability* button on the
repository's Security tab. That is a draft security advisory: it reaches the
maintainer, stays private until a fix is out, and gives you a place to attach a
proof of concept. If the form is not available to you, mail the address on the
maintainer's GitHub profile, with the same contents.

Please do not open a public issue for anything that lets someone into a panel
without the password, or lets a client reach something over the tunnel that it
should not.

Expect an acknowledgement within a few days. This is a one-person project
maintained alongside a full-time job, so a fix may take longer than the report;
the advisory is where the timetable lives.

## Supported versions

The image is published as `ghcr.io/noelbennettz/amneziawg-ui` with two tags:
`latest`, which is the newest commit on `main`, and the date the build was made,
e.g. `2026.11.11`. Only the newest date is supported — there are no release
branches, and a fix lands on `main` and in the next build. The panel reports
nothing about its own version, so "which one am I running" is answered by the
tag you pulled.

## Threat model

The panel is assumed to be reachable from the internet. Port `54845` offers a
`/status` page and a login form to anyone who scans it, and the tunnel port
answers with an AmneziaWG handshake. An attacker is assumed to be able to:

* guess and stuff passwords at the login form and at `curl -u` — each attempt
  costs the server an argon2id derivation, so the cost is theirs to pay but the
  memory is the panel's;
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
  one on the first start, prints it once and stores only its hash. The base64
  of a SHA-256 that earlier releases accepted is refused at startup — the
  format is fast to compute, and therefore fast to attack — and so is a hash
  that verifies `changeme`.
* Verifying a password is expensive by design, so it is bounded: no more than
  two derivations run at a time, and a Basic credential that verified is cached
  in memory for five minutes — an HMAC of the pair under a random per-process
  key, never the password, and never on disk. A password change empties that
  cache, because every entry carries the generation it was written under.
* A signed session cookie (`HttpOnly`, `SameSite=Strict`, 12 hours) whose key is
  derived from the password hash: a restart keeps sessions, changing the password
  — in `WEB_UI_PASSWORD` or with `reset-password` — ends all of them. `Secure` follows `WEB_UI_SECURE_COOKIE`, the request, or
  `X-Forwarded-Proto`.
* A double-submit token on every state-changing request. A request that carries
  an `Authorization` header is exempt — that is not an ambient credential, and
  it is what keeps `curl -u` working.
* A CSP that allows exactly one inline script, by nonce, plus
  `'wasm-unsafe-eval'` for the wasm bundle; `frame-ancestors`/`X-Frame-Options:
  DENY`, `nosniff`, `no-referrer`.
* Request bodies are never logged. The API's own log line carries the method,
  the route, the status and the address; the URIs and private keys travel in
  response bodies only.
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
  credentials. There is no WebAuthn, no recovery codes and one account, so the
  secret in the environment (or in the compose file) is the whole of the second
  factor's storage; a code is not single-use across the ±1 step window, which is
  the compromise that keeps `curl -u` usable.
* **The generated password is printed to the container log.** It is the only
  copy — the volume holds just the hash — so anyone who can read `docker logs`
  during or after the first start can read it, and it stays there until the log
  rotates. `api reset-password` replaces it and ends every session, which is the
  right answer if the log has been shipped somewhere.
* **No CSRF token on `GET /logout`.** It is a state change, it is reachable
  from another site, and `SameSite=Strict` is what keeps that from mattering.
* **The audit log is opt-in.** `WEB_UI_AUDIT_LOG=true` appends to
  `/etc/amnezia/audit.log`; entries are kept for `WEB_UI_AUDIT_RETENTION_HOURS`
  (24 hours by default, 30 days at most). The file is swept at most once an
  hour, so an entry can outlive the window by up to an hour, and the panel holds
  no lock on it — an operator who deletes the file just loses the record.

## The tunnel

The container runs the userspace implementation (amneziawg-go) and needs
`NET_ADMIN` alone — no `SYS_MODULE`, no host network by default. A client that
is connected to a server can reach that server's subnet and, through NAT, the
internet. It cannot reach another server's subnet unless you route it there.
