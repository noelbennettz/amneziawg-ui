# AmneziaWG Web UI — a self-hosted admin panel for AmneziaWG VPN in Docker

> Applies to the current `main`, published under the dated image tags
> (`ghcr.io/noelbennettz/amneziawg-ui:2026.11.18` at the time of writing). An
> older tag keeps its documentation under `docs/versions/<tag>/`.

[![Container image](https://img.shields.io/badge/ghcr.io-amneziawg--ui-blue?logo=github)](https://github.com/noelbennettz/amneziawg-ui/pkgs/container/amneziawg-ui)
[![Website](https://img.shields.io/badge/website-noelbennettz.github.io%2Famneziawg--ui-5b7cff)](https://noelbennettz.github.io/amneziawg-ui/)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![AmneziaWG](https://img.shields.io/badge/AmneziaWG-3.1-blue)](DOCS.md#amneziawg-31--the-essentials)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)

**AmneziaWG Web UI** is a web panel for **AmneziaWG**, the obfuscated fork of
WireGuard that gets past DPI blocks on VPN traffic. One container holds the
panel and the VPN servers it manages: create servers, add clients, hand out a
`.conf` file, a QR code or a `vpn://` link for the AmneziaVPN app, and watch
traffic and handshakes in the browser. It is a single Go binary with a
WebAssembly front end, about 35 MB, no Node.js, nginx or supervisor, licensed
AGPL-3.0. AmneziaWG 3.1 (header protection, random trailers, cookies disabled)
is on for every server, with nothing to configure by hand.

<p align="center">
  <img src="screenshot.png" alt="AmneziaWG Web UI: monitoring charts for VPN traffic, CPU, RAM and storage, and the create-server form with its IPv4 and IPv6 subnet fields"/>
</p>

## Requirements

- A Linux host with Docker Engine and Docker Compose **2.24 or newer** (the compose files use `env_file.required` and `!reset`).
- A public IPv4 and/or IPv6 address. IPv6 per server is optional, but a bridge network gives clients an IPv6 tunnel without IPv6 egress — see [DOCS.md](DOCS.md#ipv6-and-nat6).
- Host ports `54845/tcp` (panel) and `54844/udp` (the first VPN server), or your own values.
- `/dev/net/tun` on the host. The tunnel runs in user space (`amneziawg-go`), so no kernel module and no `SYS_MODULE`.

## Quick start

```bash
git clone https://github.com/noelbennettz/amneziawg-ui.git
cd amneziawg-ui
cp .env.example .env
printf 'your-password' | openssl dgst -binary -sha256 | base64   # paste into WEB_UI_PASSWORD
docker compose up -d
```

`docker compose up -d` pulls `ghcr.io/noelbennettz/amneziawg-ui:latest` and
starts it. `.env` is optional, but without it the panel runs with the image's
default password (`changeme`) — see the warning below. Open
`http://<server-ip>:54845`, sign in as `admin` with the password you hashed,
then **Create server → Add client** and take the `.conf`, the QR code or the
`vpn://` link. Update with `docker compose pull && docker compose up -d`.

Check it came up:

- `docker compose ps` shows `Up … (healthy)` within a minute.
- `docker compose logs app` ends with `AmneziaWG Web UI (Go/Fiber) starting...`,
  the environment block and the listening address.
- `curl -s http://127.0.0.1:54845/status` answers `Container Uptime: 0d 0h 0m 3s`
  without a login.
- `http://<server-ip>:54845/` shows the login page; an empty server list, then
  **Create server → Add client** gives a `.conf`, a QR code and a `vpn://` link.
  A client that imports one shows a handshake and traffic in the panel within
  a few seconds.

> [!IMPORTANT]
> The image's default login is `admin` / `changeme`. Anyone who reaches the port
> before `WEB_UI_PASSWORD` holds your own hash can sign in. Change it first, or
> publish the port on `127.0.0.1` until you have.

Removing it later: `docker compose down` stops the container and keeps the
`/etc/amnezia` volume; `docker compose down -v` deletes that volume — every
server, client and key — and cannot be undone. [DOCS.md](DOCS.md#removing-it)
lists what each command leaves behind.

The container does not terminate TLS. Restrict port `54845` with a firewall or
put a TLS proxy in front of it before exposing it — the
[FAQ](DOCS.md#do-i-need-https-and-how-do-i-enable-it) has the two settings that
matter behind a proxy.

## Documentation

| Document | Contents |
| --- | --- |
| [DOCS.md](DOCS.md) | Features, AmneziaWG 3.1 parameters, IPv6 and NAT6, REST API, environment variables, compose variants, building from source, profiling, logs, backup and restore, security details, FAQ |
| [SECURITY.md](SECURITY.md) | What the panel assumes, what it does not protect against |
| [`docs/`](docs/) | Project site in English and Traditional Chinese, published at <https://noelbennettz.github.io/amneziawg-ui/> |
| [`.env.example`](.env.example) | Every environment variable with its default |
| [`docker-compose.example.yml`](docker-compose.example.yml) | The same service with every compose key and variable commented |

Dated image tags, one per build, are listed under
[Releases](https://github.com/noelbennettz/amneziawg-ui/releases); pinning one
turns an upgrade into a decision.

## Licence

**[AGPL-3.0](LICENSE)**, and section 13 is the operative part: run a modified
panel as a network service and its users get the source of the version you run.
Self-hosting, forking and selling support are fine; withholding your changes
from the people using them is not.
