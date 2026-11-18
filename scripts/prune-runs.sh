#!/usr/bin/env bash
# Keep only the newest runs of the workflow that is currently running.
#
# Deleting a run also deletes its logs and artifacts, which is the point: the
# Actions list and the artifact storage quota stay small without a cron job.
# Deleting a *release* run does not touch the GitHub Release itself — releases
# are separate objects — only that run's page and logs.
#
# Needs `actions: write` on the token; the workflows grant it explicitly.
#
# Usage: prune-runs.sh [keep]      default keep=3
set -euo pipefail

keep="${1:-3}"
[[ "$keep" =~ ^[0-9]+$ ]] && [ "$keep" -ge 1 ] || {
  echo "::error::keep must be a positive integer (got '$keep')"
  exit 1
}

if ! command -v gh >/dev/null 2>&1; then
  echo "::warning::gh is not available; skipping the run cleanup"
  exit 0
fi

# Inside a run: owner/repo/.github/workflows/build.yml@refs/heads/main
workflow_ref="${GITHUB_WORKFLOW_REF:-}"
workflow="${workflow_ref%%@*}"
workflow="$(basename "$workflow")"
repo="${GITHUB_REPOSITORY:-}"

if [ -z "$workflow" ] || [ -z "$repo" ]; then
  echo "::warning::GITHUB_WORKFLOW_REF/GITHUB_REPOSITORY are not set; skipping"
  exit 0
fi

# Let gh find the repository without repeating --repo on every call.
export GH_REPO="$repo"

echo "Pruning $repo · $workflow (keeping the newest $keep)"
deleted=0

# `gh run list` is paged, so repeat until a pass frees nothing: either every
# older run is gone, or the token may not delete them (then stop, don't spin).
while :; do
  if ! listing="$(gh run list --workflow "$workflow" --limit 100 \
      --json databaseId --jq '.[].databaseId')"; then
    echo "::warning::could not list runs of $workflow; nothing pruned"
    exit 0
  fi

  mapfile -t stale < <(printf '%s\n' "$listing" | sed '/^$/d' | tail -n "+$((keep + 1))")
  [ "${#stale[@]}" -eq 0 ] && break

  progress=0
  for id in "${stale[@]}"; do
    if gh run delete "$id" >/dev/null 2>&1; then
      progress=$((progress + 1))
    else
      # In progress, or the token lacks actions: write.
      echo "::warning::could not delete run $id"
    fi
  done
  deleted=$((deleted + progress))
  [ "$progress" -eq 0 ] && break
done

echo "Pruned $deleted run(s) of $workflow; kept the newest $keep."
