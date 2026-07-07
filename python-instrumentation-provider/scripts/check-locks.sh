#!/usr/bin/env bash
# Hermetic check that the committed lock files cover the release-config matrix.
# Does NOT touch PyPI/Docker — it only reads committed files, so it is
# deterministic over time (unlike `make lock`, which re-resolves live). CI runs
# this. It catches the mistakes that actually break the image build:
#   * a (traceloop x python) entry in release-config.json with no committed lock
#   * a lock whose pinned traceloop-sdk version doesn't match its filename/entry
#   * an orphan lock not referenced by any release-config entry
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"   # python-instrumentation-provider/
CONFIG="$HERE/../.github/release-config.json"
LOCK_DIR="$HERE/locks"

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
[ -f "$CONFIG" ] || { echo "release-config.json not found at $CONFIG" >&2; exit 1; }

fail=0
err() { echo "::error::$*"; fail=1; }

# Expected locks from the matrix.
pairs="$(jq -r '.["python-instrumentation-provider"][]
  | .traceloop_version as $t
  | .python_versions[]
  | "\($t) \(.)"' "$CONFIG" | sort -u)"
[ -n "$pairs" ] || { echo "No (traceloop x python) pairs found in $CONFIG" >&2; exit 1; }

expected_files=""
while read -r T PY; do
  [ -z "${T:-}" ] && continue
  f="$LOCK_DIR/traceloop-${T}-python${PY}.txt"
  expected_files="$expected_files $(basename "$f")"
  if [ ! -f "$f" ]; then
    err "missing lock $(basename "$f") — run 'make lock' and commit it."
    continue
  fi
  pin="$(grep -iE '^traceloop-sdk==' "$f" | head -1 || true)"
  if [ "$pin" != "traceloop-sdk==${T}" ]; then
    err "$(basename "$f") pins '${pin:-<none>}', expected 'traceloop-sdk==${T}'."
  fi
done <<< "$pairs"

# Orphan locks (present on disk but not in the matrix) — e.g. left behind after
# dropping a version.
if [ -d "$LOCK_DIR" ]; then
  for f in "$LOCK_DIR"/*.txt; do
    [ -e "$f" ] || continue
    base="$(basename "$f")"
    case " $expected_files " in
      *" $base "*) ;;
      *) err "orphan lock $base is not in release-config.json — remove it or add the entry." ;;
    esac
  done
fi

if [ "$fail" -ne 0 ]; then
  echo "Lock check failed. Fix the above (usually: 'cd python-instrumentation-provider && make lock' then commit)." >&2
  exit 1
fi
echo "Lock check passed: all release-config entries have a matching lock."
