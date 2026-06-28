#!/usr/bin/env bash
#
# sync-env.sh — reconcile a component's local .env against its .env.example.
#
#   - If .env does not exist, copy .env.example -> .env (dev defaults).
#   - If .env exists, append any keys present in .env.example but missing from
#     .env, using the example's default line verbatim. This lets existing
#     clones pick up newly-introduced variables after a pull, not just fresh
#     clones.
#
# Append-only: keys already present in .env (including user-customized values)
# are never modified, reordered, or removed. Comparison is by key name only,
# and values are copied verbatim, so quoting, '=' inside values, and inline
# comments are preserved untouched.
#
# Usage: scripts/sync-env.sh <dir>     e.g. scripts/sync-env.sh backend
set -euo pipefail

dir="${1:?usage: sync-env.sh <dir> (e.g. backend or frontend)}"
example="$dir/.env.example"
env_file="$dir/.env"

if [ ! -f "$example" ]; then
	echo "❌ $example not found — cannot bootstrap $env_file" >&2
	exit 1
fi

if [ ! -f "$env_file" ]; then
	echo "📋 $env_file not found — copying from .env.example (dev defaults)"
	cp "$example" "$env_file"
	exit 0
fi

# Match lines that define a variable (KEY=value); ignore comments and blanks.
# The key is everything before the first '='.
key_re='^[A-Za-z_][A-Za-z0-9_]*='

missing="$(comm -23 \
	<(grep -E "$key_re" "$example" | cut -d= -f1 | sort -u) \
	<(grep -E "$key_re" "$env_file" | cut -d= -f1 | sort -u))"

if [ -z "$missing" ]; then
	echo "✓ $env_file is up to date with .env.example"
	exit 0
fi

count="$(printf '%s\n' "$missing" | grep -c .)"
echo "📋 $env_file is missing $count new key(s) from .env.example — appending defaults:"
echo "   $(printf '%s\n' "$missing" | tr '\n' ' ')"

{
	echo ""
	echo "# --- added by scripts/sync-env.sh on $(date +%F) (new keys from .env.example) ---"
	while IFS= read -r key; do
		[ -n "$key" ] || continue
		# Append the first matching definition line from .env.example verbatim.
		grep -E "^${key}=" "$example" | head -n 1
	done <<< "$missing"
} >> "$env_file"

echo "✓ appended $count key(s) to $env_file — review the new values before relying on them"
