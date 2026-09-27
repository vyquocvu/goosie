#!/usr/bin/env bash
# release-smoke.sh - run the binary a release job just built, and prove it drew.
#
# This exists because the step it replaced did not. release.yml's "Smoke test" chmod +x'd a
# path with `|| true` and then echoed `PASS: smoke test for <os>/<arch>` for every matrix
# entry without executing anything, so the check could not fail: an empty dist/ directory,
# a binary built for the wrong architecture, and a build that dies on its first syscall all
# went out the door with a green smoke test next to their name. `go test ./...` in the same
# job compiles and runs test binaries, which is not the same artifact: this one is built
# with -trimpath and -ldflags "-s -w" for five targets, and nothing until now looked at it
# after the linker finished.
#
# What it does: run the built binary headless over a short frame burst, grade its own
# report, and require the JSON artifact it writes. It asserts that the binary ran, that
# the frame path was reached, that every frame it drew was presented, and that no tile
# raster failed and no worker panicked on the way. It deliberately asserts nothing about
# timing: a release runner's frame rate is not a property of the artifact, and the nightly
# macOS job owns that.
#
# Usage:
#   scripts/release-smoke.sh <path-to-binary> [-frames 16] [-out report.json]
#   scripts/release-smoke.sh -h
#
# Exit status: 0 pass, 1 the artifact is bad, 2 the invocation or the transcript is bad.
# 2 means no verdict was reached - a checker that could not read the report does not get to
# claim the binary works, and it does not get to claim it is broken either.
#
# test/gatecheck/release_smoke_test.go is the contract for this file. Change an exit status
# here and it fails there.
set -euo pipefail

usage() {
  awk 'NR > 1 { if (!/^#/) exit; sub(/^# ?/, ""); print }' "$0"
}

die() {
  echo "release-smoke: $*" >&2
  exit 2
}

# field NAME < line - the value of a `key=value` token, matched on the whole key so that
# frames= cannot be satisfied by zero_work_frames= on an adjacent line.
field() {
  awk -v want="$1" '{ for (i = 1; i <= NF; i++) { eq = index($i, "="); if (eq > 0 && substr($i, 1, eq - 1) == want) { print substr($i, eq + 1); exit } } }'
}

is_uint() {
  case "${1:-}" in
    '' | *[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

if [[ ${1:-} == -h || ${1:-} == --help ]]; then
  usage
  exit 0
fi

BIN=${1:-}
[[ -n "$BIN" ]] || {
  usage >&2
  exit 2
}
shift

FRAMES=16
OUT=''
while [[ $# -gt 0 ]]; do
  case "$1" in
    -frames)
      [[ $# -ge 2 ]] || die "-frames needs a value"
      FRAMES=$2
      shift 2
      ;;
    -out)
      [[ $# -ge 2 ]] || die "-out needs a value"
      OUT=$2
      shift 2
      ;;
    *) die "unknown argument: $1" ;;
  esac
done

is_uint "$FRAMES" || die "-frames must be a positive whole number, got '$FRAMES'"
((FRAMES > 0)) || die "-frames must be positive, got $FRAMES"

[[ -e "$BIN" ]] || die "nothing to smoke test: $BIN does not exist"
[[ -f "$BIN" ]] || die "$BIN is not a file"
[[ -x "$BIN" ]] || die "$BIN is not executable"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
LOG=$TMP/run.txt
[[ -n "$OUT" ]] || OUT=$TMP/report.json
: >"$LOG"

# -bench rather than -gate because -bench forces the headless backend and runs without
# waiting on a display that a release runner does not have. The run is short on purpose:
# this is proof of life, not a measurement.
status=0
"$BIN" -bench -frames "$FRAMES" -backend headless -scene checkerboard -out "$OUT" >"$TMP/stdout.txt" 2>"$LOG" || status=$?
if [[ $status -ne 0 ]]; then
  echo "release-smoke: FAIL: $BIN exit $status" >&2
  sed -n '1,20p' "$LOG" >&2 || true
  exit 1
fi

RUN=$(grep -m1 '^goosie: run ' "$LOG" || true)
[[ -n "$RUN" ]] || {
  echo "release-smoke: FAIL: no run line in the binary's output" >&2
  sed -n '1,20p' "$LOG" >&2 || true
  exit 2
}
COUNTERS=$(grep -m1 '^goosie: counters ' "$LOG" || true)
[[ -n "$COUNTERS" ]] || {
  echo "release-smoke: FAIL: no counters line in the binary's output" >&2
  exit 2
}

BACKEND=$(field backend <<<"$RUN")
REPORTED_FRAMES=$(field frames <<<"$RUN")
STAMPED=$(field stamped <<<"$RUN")
FAILED=$(field failed <<<"$COUNTERS")
PANICS=$(field panics <<<"$COUNTERS")

for pair in "stamped=$STAMPED" "frames=$REPORTED_FRAMES" "failed=$FAILED" "panics=$PANICS"; do
  is_uint "${pair#*=}" || die "the transcript has a non-numeric ${pair%%=*}: '$pair'"
done

# A run line describing a different burst is not evidence about this one: it would mean the
# binary ignored -frames, or that the line came from a leftover log.
[[ "$REPORTED_FRAMES" == "$FRAMES" ]] ||
  die "the binary reported frames=$REPORTED_FRAMES for a -frames $FRAMES run"

if [[ "$BACKEND" != headless ]]; then
  echo "release-smoke: FAIL: asked for headless, the run reported backend=$BACKEND" >&2
  exit 1
fi

if [[ $STAMPED -lt 1 ]]; then
  echo "release-smoke: FAIL: the frame path presented 0 of $FRAMES frames" >&2
  exit 1
fi
((STAMPED <= FRAMES)) || die "the transcript presents $STAMPED frames out of $FRAMES"

# failed tiles and panicking workers are both swallowed by the frame path, so a run can
# present every frame and still be drawing a page full of holes.
if [[ $FAILED -ne 0 || $PANICS -ne 0 ]]; then
  echo "release-smoke: FAIL: failed=$FAILED panics=$PANICS" >&2
  exit 1
fi

if [[ ! -s "$OUT" ]] || ! grep -q '"report"' "$OUT"; then
  echo "release-smoke: FAIL: the run left no usable JSON report at $OUT" >&2
  exit 1
fi

echo "release-smoke: PASS $BIN drew $STAMPED/$FRAMES headless frames, backend=$BACKEND, failed=0 panics=0"
