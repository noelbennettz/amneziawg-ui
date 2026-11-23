# Versions

This project does not publish numbered releases. There is no `v1.0.0` and no
list of past versions, because none was ever produced — before this directory
existed the answer to "which version is this?" was the image tag, and inventing
a history now would be writing down something that did not happen.

What does identify a build:

- **The commit.** The image is built from a commit on `main`; the commit is in
  the binary and in the image's labels.
- **`/api/version`**, behind the login:

  ```bash
  curl -u admin:<password> https://panel.example:54845/api/version
  ```

  ```json
  {"version":"2026.11.20","commit":"70ce4a3","build_time":"2026-11-20T10:08:00Z","go":"go1.27.x"}
  ```

- **The image labels**, for a container that is not running or whose login you
  do not have:

  ```bash
  docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}} {{index .Config.Labels "org.opencontainers.image.revision"}}' <container>
  ```

- **The tag.** `latest` moves with every push to `main`. The date tag
  (`2026.11.11`) is the day the build was made, and a second build on the same
  day replaces it - the registry holds one image per tag. To keep one image for
  good, pin its digest, not its date.
- **The signature.** Every published image is signed keylessly by the workflow
  that built it, and `cosign verify` names the commit's workflow run —
  [Verifying a published image](../../DOCS.md#verifying-a-published-image).

The three values are filled by the linker, not guessed at runtime:

- `make build` / `make build-server` derive them from `git describe`, the short
  commit and the clock (`Makefile`);
- `docker build` takes them as `VERSION`, `COMMIT` and `BUILD_TIME` build
  arguments, which `make run` fills and a plain `docker build` leaves at `dev`
  and `unknown` (`Dockerfile`, `docker-compose.build.yml`);
- the publish workflow computes them from the checkout it builds
  (`.github/workflows/docker.yml`).

A binary built without any of that reports `dev` / `unknown`, and that is the
truth for such a build: it did not record where it came from.

## If a release series starts

Then this directory grows one file per version (`1.0.0.md`, `1.1.0.md`, …) with
what changed and what it means for an upgrade, the `VERSION` build argument
carries the same number as the git tag, and the frontend's
`web-ui/FyneApp.toml` (which already takes its `Version` from the newest tag)
shows it in the header. Nothing here has to be backfilled: the fields already
exist, only the meaning of `version` would change.
