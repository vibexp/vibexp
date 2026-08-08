#!/usr/bin/env bash
#
# Guard against migration version-number collisions.
#
# golang-migrate identifies a migration by its numeric prefix alone, so two
# files sharing a version (e.g. two branches each adding their own 013_*.sql)
# merge cleanly in git yet break every startup with a duplicate-version error.
# This script fails fast instead. Called by `make backend-check-migrations`
# from backend/:
#
#   bash ../.github/scripts/check-duplicate-migrations.sh migrations false
#
# Usage: check-duplicate-migrations.sh <migrations-dir> <merge-mode> [base-branch]
#   <migrations-dir>  directory holding NNN_name.{up,down}.sql files,
#                     relative to the caller's working directory
#   <merge-mode>      "true" additionally compares version numbers against the
#                     base branch, catching a branch that reuses a number the
#                     base has since taken; "false" checks the local tree only.
#                     Merge mode legitimately fires on a deliberate renumbering
#                     (a post-release consolidation replaces the base branch's
#                     filenames on purpose).
#   [base-branch]     branch to compare against in merge mode, resolved as
#                     `origin/<base-branch>`. Defaults to "main". CI passes the
#                     PR's target branch, which is `release/X.Y.x` for a
#                     backport (#747) — comparing a backport against `main`
#                     would fail on every post-release consolidation renumbering
#                     and is not the collision we are looking for.
set -euo pipefail

dir="${1:?usage: $0 <migrations-dir> <merge-mode:true|false> [base-branch]}"
check_main="${2:-false}"
base_branch="${3:-main}"
base_ref="origin/${base_branch}"

if [ ! -d "$dir" ]; then
    echo "ERROR: migrations directory not found: $dir" >&2
    exit 1
fi

# sed strips the directory prefix instead of find's -printf '%f\n', which is
# GNU-only and errors on the BSD find that macOS contributors have.
files=$(find "$dir" -maxdepth 1 -name '*.sql' | sed 's|.*/||' | sort)
if [ -z "$files" ]; then
    echo "ERROR: no .sql files found in $dir" >&2
    exit 1
fi

status=0

# --- 1. Every file must match NNN_name.up.sql / NNN_name.down.sql -----------
while IFS= read -r f; do
    if ! printf '%s\n' "$f" | grep -Eq '^[0-9]{3}_[a-z0-9_]+\.(up|down)\.sql$'; then
        echo "ERROR: malformed migration filename: $f (expected NNN_name.up.sql / NNN_name.down.sql)" >&2
        status=1
    fi
done <<< "$files"

# --- 2. One base name per version, one up + one down per base name ----------
bases=$(printf '%s\n' "$files" | sed -E 's/\.(up|down)\.sql$//' | sort -u)

while IFS= read -r v; do
    [ -n "$v" ] || continue
    echo "ERROR: duplicate migration version $v claimed by more than one name:" >&2
    printf '%s\n' "$files" | grep "^${v}_" | sed 's/^/    /' >&2
    status=1
done <<< "$(printf '%s\n' "$bases" | cut -d_ -f1 | sort | uniq -d)"

while IFS= read -r base; do
    for side in up down; do
        if [ ! -f "$dir/${base}.${side}.sql" ]; then
            echo "ERROR: ${base} is missing its .${side}.sql counterpart" >&2
            status=1
        fi
    done
done <<< "$bases"

# --- 3. Merge mode: a version must resolve to the same name as the base ------
if [ "$check_main" = "true" ]; then
    # --full-tree makes the pathspec repo-root-relative regardless of the
    # caller's working directory (a bare cwd-relative pathspec would silently
    # match nothing when invoked from backend/).
    prefix=$(git rev-parse --show-prefix)
    base_bases=$(git ls-tree -r --name-only --full-tree "$base_ref" -- "${prefix}${dir}" \
        | sed -E "s|^${prefix}${dir}/||" \
        | { grep -E '\.sql$' || true; } \
        | sed -E 's/\.(up|down)\.sql$//' | sort -u)
    if [ -z "$base_bases" ]; then
        echo "ERROR: merge mode found no migrations at ${prefix}${dir} on ${base_ref} (is ${base_ref} fetched?)" >&2
        exit 1
    fi

    while IFS= read -r v; do
        [ -n "$v" ] || continue
        echo "ERROR: migration version $v resolves differently here and on ${base_ref}:" >&2
        echo "    local:       $(printf '%s\n' "$bases" | grep "^${v}_" || echo '(absent)')" >&2
        echo "    ${base_ref}: $(printf '%s\n' "$base_bases" | grep "^${v}_" || echo '(absent)')" >&2
        status=1
    done <<< "$(printf '%s\n%s\n' "$bases" "$base_bases" | sort -u | cut -d_ -f1 | sort | uniq -d)"
fi

if [ "$status" -eq 0 ]; then
    echo "OK: migration files in $dir are well-formed with no duplicate versions"
fi
exit "$status"
