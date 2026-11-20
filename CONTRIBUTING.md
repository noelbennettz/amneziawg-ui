# Contributing

Bug reports, documentation fixes and patches are all welcome. This is a
one-person project maintained alongside a full-time job, so a review may take a
few days — the pull request is the queue, and nothing gets lost in it.

## Before you write code

Open an issue for anything larger than a documentation fix or an obvious bug.
The panel is deliberately small: a feature that pulls in a runtime, a service or
a JavaScript build step is the wrong shape for it, and it is better to find that
out before the patch exists.

Security issues do not belong in an issue or a pull request: [SECURITY.md](SECURITY.md)
says where to send them.

## Development environment

Go (the version in `go.mod`), Docker with Compose, and nothing else. The
frontend is compiled to WebAssembly by the Go toolchain; there is no Node.js,
no npm and no bundler.

```sh
make build        # frontend bundle + server binary
make run          # build the image from source and run it (docker compose)
make vet          # go vet over every module
make web-ui       # rebuild only the WebAssembly bundle into web-ui/wasm
make e2e          # rebuild a clean test instance and run the browser tests
go test ./...     # the backend tests; there is no make target for them
make help         # every target, with its description
```

`go test ./...` in the repository root runs the backend tests; the packages
under `internal/` carry the interesting ones. `AGENTS.md` describes how the code
is laid out and which package owns what — read it before moving anything
between packages, it is the map the tests assume.

The browser tests in `e2e/` are their own module and drive the real WebAssembly
UI through playwright-go; `make e2e` provisions the container they need, so
Docker has to be running. They are the only place a UI change is verified
against the rendered canvas, and a widget that moves fails them on purpose.

## Commit messages

English, imperative subject line, and the body explains *why* the change is
worth making — the diff already says what changed. One logical change per
commit; a fix and the test that pins it belong together.

## Sign-off (DCO)

Every commit has to carry a `Signed-off-by` trailer. It is the
[Developer Certificate of Origin](https://developercertificate.org/) — the
statement that you wrote the patch, or have the right to submit it under the
project's licence ([AGPL-3.0](LICENSE)). `git commit -s` adds the trailer from
your git identity:

```
Signed-off-by: Your Name <you@example.com>
```

The name and address have to be your real ones and have to match the commit's
author or committer address. Please do not sign off somebody else's work, and do
not add the trailer to a commit whose authorship is not yours.

The `DCO` workflow (`.github/workflows/dco.yml`) checks every commit in a pull
request and skips merge commits and bot commits, which cannot sign anything.
Fixing a missing trailer is one command on the branch:

```sh
git commit -s --amend          # the newest commit
git rebase --signoff HEAD~3    # the last three, keeping their contents
git push --force-with-lease
```

## Continuous integration

Five workflows: `docker.yml` (a push to `main` or a manual run: backend and
WebAssembly tests, image build and push, signing, SBOM), `e2e.yml` (pushes and
pull requests: the browser suite), `licences.yml` (the same), `lint.yml` (changes
under `.github/` only: actionlint) and `dco.yml` (pull requests only). Every
one of them keeps only its own three newest runs — a `prune-runs-before` job at
the top of `jobs:` and a `prune-runs-after` job after the main jobs (`if:
always()`) call `scripts/prune-runs.sh 3`, which also deletes the logs and
artifacts of the removed runs. Both authenticate with the automatic
`github.token` under a job-level `permissions: actions: write` +
`contents: read`; keep it that way. A pull request from a fork gets a read-only
token, and the script is written to warn and exit successfully there rather than
turn the check red. Keep the action pins: every `uses:` is a full commit SHA,
maintained by `.github/dependabot.yml`.

## Pull requests

- Branch from `main` and keep the branch rebased on it; the project squash-merges,
  so the pull request is the unit of change and its commits arrive as one on
  `main`.
- Say what behaviour changes, which commands you ran, and what the output was.
  Tests and lint that were not run are worth naming as not run.
- New user-visible text in the UI goes into every file under
  `web-ui/translation/` and, for the login page, into `internal/auth/i18n.go` —
  the two lists are the same six languages.
- `README.md` and `DOCS.md` are English. A behaviour change that an operator can
  see belongs in `DOCS.md`, not only in the commit message.
- No telemetry, no outbound requests and no assets fetched at runtime: the panel
  has to work on a host with no route to the internet.

## Licence

By contributing you agree that your contribution is licensed under the
[GNU AGPL-3.0](LICENSE), the same licence as the rest of the project.
