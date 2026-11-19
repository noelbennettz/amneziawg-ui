#!/usr/bin/env bash
# check-licences.sh - the licence gate for every Go dependency the panel and
# its frontend are compiled from.
#
# The allowlist is strict: Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, MIT.
# One written exception sits in front of it rather than inside it. fyne.io/fyne/v2
# - the GUI toolkit the WebAssembly frontend is built with - pulls in two
# packages that are public-domain dedications under Unlicense, and dropping them
# would mean dropping Fyne's text stack. So the strict check runs with exactly
# those two modules ignored, and then the packages that carry Unlicense are
# listed and matched one by one against the pair and the versions below.
#
# Anything else under Unlicense fails, either at the strict check (a package
# nobody excused) or at the audit (one of the two at a version this file does
# not name). An exception that has gone stale - the dependency moved or the
# versions bumped without this file - fails too, so the list cannot rot.
#
# Usage: scripts/check-licences.sh
#   GO_LICENSES  path to the binary (default: go-licenses on PATH, then
#                "$(go env GOPATH)/bin/go-licenses")
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

# The project's own packages are ignored: the panel is AGPL-3.0, which is not on
# the list because the list is about what the panel links, not what it is.
own_modules=(amneziawg-web-ui amneziawg-web-ui/web-ui)

allowed_licences='Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT'

# The exception, as (module, version) pairs. Both are dependencies of the
# frontend module only. Bump a version here in the same commit that bumps it in
# web-ui/go.mod; the audit below compares the two.
exception_modules=(github.com/go-text/render github.com/go-text/typesetting)
exception_versions=(v0.2.1 v0.3.4)

bin=${GO_LICENSES:-}
if [ -z "$bin" ]; then
    if command -v go-licenses >/dev/null 2>&1; then
        bin=$(command -v go-licenses)
    else
        bin="$(go env GOPATH)/bin/go-licenses"
    fi
fi
if [ ! -x "$bin" ]; then
    echo "check-licences: go-licenses not found at $bin" >&2
    echo "  install it with: go install github.com/google/go-licenses/v2@${GO_LICENSES_VERSION:-v2.0.1}" >&2
    exit 1
fi

fail=0
note() { printf '%s\n' "$*" >&2; }

# strict_check <module dir> <own package prefix>: the allowlist, with the two
# excepted modules taken out so their licence is judged by the audit below
# rather than by the list.
strict_check() {
    local dir=$1 prefix=$2
    local args=(check ./... --allowed_licenses="$allowed_licences" --ignore "$prefix")
    local module
    for module in "${exception_modules[@]}"; do
        args+=(--ignore "$module")
    done
    if ! ( cd "$dir" && "$bin" "${args[@]}" ); then
        note "check-licences: $dir links a dependency outside $allowed_licences"
        fail=1
    fi
}

strict_check . "${own_modules[0]}"
strict_check web-ui "${own_modules[1]}"

# audit_unlicense <module dir>: every package the report marks Unlicense has to
# be one of the excepted pair, at the version named above.
audit_unlicense() {
    local dir=$1
    local report module licence version want index found=0
    if ! report=$( cd "$dir" && "$bin" report ./... 2>/dev/null ); then
        note "check-licences: could not read the licence report of $dir"
        fail=1
        return
    fi

    while IFS=, read -r module _ licence; do
        [ "$licence" = "Unlicense" ] || continue
        index=-1
        for i in "${!exception_modules[@]}"; do
            if [ "${exception_modules[$i]}" = "$module" ]; then
                index=$i
                break
            fi
        done
        if [ "$index" -lt 0 ]; then
            note "check-licences: $module is Unlicense, and only ${exception_modules[*]} may be"
            fail=1
            continue
        fi
        want=${exception_versions[$index]}
        version=$( cd "$dir" && go list -m -f '{{.Version}}' "$module" )
        if [ "$version" != "$want" ]; then
            note "check-licences: $module is Unlicense at $version; the exception in this script names $want"
            note "  bump the pair here and the entry in THIRD_PARTY_NOTICES.md together"
            fail=1
            continue
        fi
        found=$((found + 1))
    done <<<"$report"

    if [ "$found" -ne "${#exception_modules[@]}" ]; then
        note "check-licences: only $found of the ${#exception_modules[@]} excepted Unlicense packages are still dependencies of $dir"
        note "  the exception is stale: remove the entry from this script and from THIRD_PARTY_NOTICES.md"
        fail=1
    fi
}

audit_unlicense web-ui

# The inventory, so a reviewer can see what changed without reading go.sum.
echo "== server dependencies =="
"$bin" report ./... 2>/dev/null | grep -v -E "^(${own_modules[0]}),"
echo "== frontend dependencies =="
( cd web-ui && "$bin" report ./... 2>/dev/null ) | grep -v -E "^(${own_modules[1]}),"

exit "$fail"
