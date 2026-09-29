#!/usr/bin/env bash
# run-parity.sh - build goosie, render all fixtures, score against Chromium, and report.
#
# This is the CI entry point for parity testing.  It builds the goosie binary, runs the
# parity scoring script in fail-closed mode, and produces JSON + JUnit artifacts that CI
# can consume.
#
# Prerequisites:
#   - Python 3 with numpy and Pillow installed
#   - Chromium reference renders in /tmp/playwright-renders/ (pre-generated)
#   - Go toolchain to build the binary
#
# Usage:
#   scripts/run-parity.sh [-binary PATH] [-threshold N] [-pass-at N]
#                         [-artifacts DIR] [-skip-build] [-skip-render]
#   scripts/run-parity.sh -h
#
# Exit status:
#   0  All fixtures passed the threshold.
#   1  One or more fixtures failed, or a render step failed.
#   2  Bad invocation or missing prerequisite.
#
# Artifacts (written to -artifacts DIR, default ./parity-artifacts):
#   parity-report.json   - structured JSON report
#   parity-report.xml    - JUnit XML for CI systems
#   diffs/               - per-fixture diff images for failures
set -euo pipefail

usage() {
  awk 'NR > 1 { if (!/^#/) exit; sub(/^# ?/, ""); print }' "$0"
}

die() {
  echo "run-parity: $*" >&2
  exit 2
}

if [[ ${1:-} == -h || ${1:-} == --help ]]; then
  usage
  exit 0
fi

BINARY='./goosie'
THRESHOLD=30
PASS_AT=90.0
ARTIFACTS='./parity-artifacts'
SKIP_BUILD=0
SKIP_RENDER=0
WIDTH=800
HEIGHT=600

while [[ $# -gt 0 ]]; do
  case "$1" in
    -binary)
      [[ $# -ge 2 ]] || die "-binary needs a value"
      BINARY=$2; shift 2 ;;
    -threshold)
      [[ $# -ge 2 ]] || die "-threshold needs a value"
      THRESHOLD=$2; shift 2 ;;
    -pass-at)
      [[ $# -ge 2 ]] || die "-pass-at needs a value"
      PASS_AT=$2; shift 2 ;;
    -artifacts)
      [[ $# -ge 2 ]] || die "-artifacts needs a value"
      ARTIFACTS=$2; shift 2 ;;
    -skip-build)
      SKIP_BUILD=1; shift ;;
    -skip-render)
      SKIP_RENDER=1; shift ;;
    -width)
      [[ $# -ge 2 ]] || die "-width needs a value"
      WIDTH=$2; shift 2 ;;
    -height)
      [[ $# -ge 2 ]] || die "-height needs a value"
      HEIGHT=$2; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done

# -----------------------------------------------------------------------
# Validate prerequisites
# -----------------------------------------------------------------------

command -v python3 >/dev/null 2>&1 || die "python3 is required"

python3 -c "import numpy" 2>/dev/null || die "numpy is required (pip install numpy)"
python3 -c "from PIL import Image" 2>/dev/null || die "Pillow is required (pip install Pillow)"

[[ -d testdata/render ]] || die "testdata/render/ not found; run from the project root"

FIXTURE_COUNT=$(find testdata/render -name '*.html' | wc -l | tr -d ' ')
((FIXTURE_COUNT > 0)) || die "no HTML fixtures found in testdata/render/"

# -----------------------------------------------------------------------
# Prepare artifact directory
# -----------------------------------------------------------------------

mkdir -p "$ARTIFACTS/diffs"

echo "run-parity: $FIXTURE_COUNT fixtures, threshold=$THRESHOLD, pass_at=$PASS_AT%"
echo "run-parity: artifacts -> $ARTIFACTS/"

# -----------------------------------------------------------------------
# Build goosie (unless skipped)
# -----------------------------------------------------------------------

if [[ $SKIP_BUILD -eq 0 ]]; then
  echo "run-parity: building goosie..."
  if ! go build -o "$BINARY" ./cmd/goosie 2>&1; then
    echo "run-parity: FAIL: go build failed" >&2
    exit 1
  fi
  echo "run-parity: built $BINARY"
else
  echo "run-parity: skipping build (-skip-build)"
fi

[[ -x "$BINARY" ]] || die "$BINARY is not executable; build it first or remove -skip-build"

# -----------------------------------------------------------------------
# Render with goosie (unless skipped)
# -----------------------------------------------------------------------

RENDER_FLAGS=""
if [[ $SKIP_RENDER -eq 0 ]]; then
  RENDER_FLAGS="--render"
  echo "run-parity: rendering fixtures with goosie..."
else
  echo "run-parity: skipping goosie render (-skip-render)"
fi

# -----------------------------------------------------------------------
# Run parity scoring
# -----------------------------------------------------------------------

echo "run-parity: scoring..."

PARITY_EXIT=0
python3 testdata/parity.py \
  $RENDER_FLAGS \
  --binary "$BINARY" \
  --width "$WIDTH" \
  --height "$HEIGHT" \
  --threshold "$THRESHOLD" \
  --pass-at "$PASS_AT" \
  --strict \
  --json "$ARTIFACTS/parity-report.json" \
  --junit "$ARTIFACTS/parity-report.xml" \
  --diffs "$ARTIFACTS/diffs" \
  || PARITY_EXIT=$?

# -----------------------------------------------------------------------
# Report results
# -----------------------------------------------------------------------

echo ""
echo "run-parity: artifacts:"
echo "  JSON report: $ARTIFACTS/parity-report.json"
echo "  JUnit XML:   $ARTIFACTS/parity-report.xml"
echo "  Diff images: $ARTIFACTS/diffs/"

if [[ -f "$ARTIFACTS/parity-report.json" ]]; then
  # Extract summary from JSON for CI log.
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "
import json, sys
with open('$ARTIFACTS/parity-report.json') as f:
    r = json.load(f)
s = r['summary']
print(f\"run-parity: {s['passing']}/{s['total']} passing ({s['passing']/s['total']*100:.1f}%), avg {s['avg_match_pct']:.2f}%\")
if s['failing'] > 0:
    print(f\"run-parity: {s['failing']} fixture(s) below threshold\")
if s['errors'] > 0:
    print(f\"run-parity: {s['errors']} error(s) (missing renders or dimension mismatches)\")
" 2>/dev/null || true
  fi
fi

echo ""
if [[ $PARITY_EXIT -eq 0 ]]; then
  echo "run-parity: PASS"
elif [[ $PARITY_EXIT -eq 1 ]]; then
  echo "run-parity: FAIL (some fixtures did not meet threshold)" >&2
else
  echo "run-parity: FAIL (exit $PARITY_EXIT)" >&2
fi

exit $PARITY_EXIT
