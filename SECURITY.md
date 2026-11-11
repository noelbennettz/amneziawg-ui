# Security policy

## Reporting a vulnerability

Open a private report with GitHub's *Report a vulnerability* button on the
repository's Security tab. That is a draft security advisory: it reaches the
maintainer, stays private until a fix is out, and gives you a place to attach a
proof of concept. If the form is not available to you, mail the address on the
maintainer's GitHub profile, with the same contents.

Please do not open a public issue for anything that lets someone into a panel
they were not given the password for, or that lets a client on the tunnel reach
something it should not.

Expect an acknowledgement within a few days. This is a one-person project run
next to a job, so a fix may take longer than the report does; the advisory is
where the timetable lives.

## Supported versions

The image is published as `ghcr.io/noelbennettz/amneziawg-ui` with two tags:
`latest`, which is the newest commit on `main`, and the date the build was made,
e.g. `2026.11.11`. Only the newest date is supported — there are no release
branches, and a fix lands on `main` and in the next build. The panel reports
nothing about its own version, so "which one am I running" is answered by the
tag you pulled.

## Threat model

The panel is assumed to be reachable from the internet. Port `54845` is a
`/status` page and a login form to anyone who scans it, and the tunnel port
answers with an AmneziaWG handshake. An attacker is assumed to be able to:

* guess and stuff passwords at the login form and at `curl -u`;
* send requests with any header, cookie or body they like;
* make a victim's browser send requests to the panel from another site;
* read every response the panel gives them, including the client configs and
  `vpn://` links, once they are logged in.

An attacker is **not** assumed to have root or docker access on the host, to be
able to read the container's environment, or to be on the path between the
browser and the panel after TLS is terminated. If they have any of those, the
credentials are readable in `docker inspect` and `WEB_UI_PASSWORD` in the
compose file is the least of the problems.

What the panel does about the model above:

* One credential pair, no user table: `WEB_UI_USER` / `WEB_UI_PASSWORD`. The
  password is stored as base64 of its SHA-256 (the format fiber's basic-auth
  middleware used), compared in constant time.
* A signed session cookie (`HttpOnly`, `SameSite=Strict`, 12 hours) whose key is
  derived from the credentials: a restart keeps sessions, changing the password
  ends all of them. `Secure` follows `WEB_UI_SECURE_COOKIE`, the request, or
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

## What is not there yet

Written down rather than left to be discovered:

* **No rate limiting on the login form.** A password can be guessed at line
  speed. Use a long one, and put the panel behind a proxy that can rate-limit
  if the port is on the internet.
* **No second factor.** There is no TOTP, no WebAuthn.
* **The password is SHA-256 in base64, not argon2id.** It is a hash of a
  password the operator chooses, in an environment variable they can read.
* **No CSRF token on `GET /logout`.** It is a state change, it is reachable
  from another site, and `SameSite=Strict` is what keeps that from mattering.
* **The audit log is opt-in and unbounded.** `WEB_UI_AUDIT_LOG=true` appends to
  `/etc/amnezia/audit.log` and never rotates it.

## The tunnel

The container runs the userspace implementation (amneziawg-go) and needs
`NET_ADMIN` alone — no `SYS_MODULE`, no host network by default. A client that
is connected to a server can reach that server's subnet and, through NAT, the
internet. It cannot reach another server's subnet unless you route it there.
