#!/usr/bin/env sh
# Reject staged changes that reintroduce literal values from a gitignored .env.
#
# The blocklist IS the .env file: any value defined there must not appear as a
# newly-added line in any other tracked file. This catches the exact local
# strings (your machine's Tailscale name, your personal tunnel URL, etc.) that
# the generic .gitleaks.toml shape rules might miss.
#
# Only runs when a .env file exists at the repo root. CI never has one, so this
# is a local-only belt-and-suspenders on top of gitleaks.

set -eu

env_file=".env"
[ -f "$env_file" ] || exit 0

min_len=6
exit_code=0

# Collect staged adds from every file except .env / .env.example themselves.
# --unified=0 keeps only changed lines; we grep for lines starting with "+".
diff_output=$(git diff --cached --unified=0 -- ':(exclude).env' ':(exclude).env.example' 2>/dev/null || true)
[ -n "$diff_output" ] || exit 0

while IFS= read -r line; do
  # Skip blanks and comments.
  case "$line" in
    ''|'#'*) continue ;;
  esac

  # Strip Make-style "KEY ?= " / "KEY = " / shell-style "KEY=" prefixes.
  value=$(printf '%s\n' "$line" | sed -E 's/^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*\??=[[:space:]]*//')

  # Strip surrounding single or double quotes.
  value=$(printf '%s' "$value" | sed -E "s/^['\"]//; s/['\"]\$//")

  # Skip very short values to avoid noise on ports, booleans, etc.
  [ "${#value}" -ge "$min_len" ] || continue

  # Fixed-string search through staged additions only.
  hits=$(printf '%s\n' "$diff_output" | grep -F -- "$value" | grep -E '^\+[^+]' || true)
  if [ -n "$hits" ]; then
    if [ "$exit_code" -eq 0 ]; then
      echo "Staged changes contain values from $env_file:" >&2
      echo "" >&2
    fi
    echo "  value: $value" >&2
    echo "$hits" | sed 's/^/    /' >&2
    echo "" >&2
    exit_code=1
  fi
done < "$env_file"

if [ "$exit_code" -ne 0 ]; then
  echo "These values come from your local $env_file and should not be committed." >&2
  echo "Move the literal into the existing variable / config and unstage the line." >&2
fi

exit "$exit_code"
