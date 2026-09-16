#!/usr/bin/env bash
# v2-gate-check.sh - the M1 pacing gate. Reads the JSON artifact `go run ./cmd/goosie
# -gate -out FILE` leaves behind and asserts the wall-clock budgets that a counter cannot
# carry, because they are only meaningful on the machine with the display attached.
#
# This is deliberately not the CI gate. CI (ubuntu-latest, headless, fake vsync) asserts
# deterministic counters only - see test/gate/m1_gate_test.go - because runner core
# counts and memory bandwidth vary enough that a timing gate there would flake, and a
# flaky gate is ignored within a month. The numbers below are the macOS nightly's.
#
# Usage:
#   scripts/v2-gate-check.sh /tmp/gate.json [-mean 16] [-p99 33]
#       [-present-p99 MS] [-bench bench.txt -warm-ms 5 -cold-ms 8]
#
# Exit status: 0 pass, 1 a budget was missed, 2 the invocation or the artifact is bad.
set -euo pipefail

usage() {
  # The header block, which is the whole manual, printed by reading forward to the
  # first line that is not a comment rather than by a line range a later edit would
  # silently get wrong.
  awk 'NR > 1 { if (!/^#/) exit; sub(/^# ?/, ""); print }' "$0"
}

die() {
  echo "v2-gate-check: $*" >&2
  exit 2
}

command -v jq >/dev/null 2>&1 || die "jq is required to read the gate artifact"

ARTIFACT=${1:-}
[[ -n "$ARTIFACT" ]] || {
  usage >&2
  exit 2
}
shift

MEAN=16.0
P99=33.0
PRESENT_P99=''
BENCH=''
WARM_MS=5.0
COLD_MS=8.0

while [[ $# -gt 0 ]]; do
  case $1 in
    -mean) MEAN=${2:-}; shift 2 ;;
    -p99) P99=${2:-}; shift 2 ;;
    -present-p99) PRESENT_P99=${2:-}; shift 2 ;;
    -bench) BENCH=${2:-}; shift 2 ;;
    -warm-ms) WARM_MS=${2:-}; shift 2 ;;
    -cold-ms) COLD_MS=${2:-}; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) die "unknown argument $1" ;;
  esac
done

[[ -r "$ARTIFACT" ]] || die "cannot read $ARTIFACT"
jq -e . "$ARTIFACT" >/dev/null 2>&1 || die "$ARTIFACT is not JSON"

fail=0
note() { printf '  %-24s %10s  %s\n' "$1" "$2" "$3"; }
# check name value budget unit ok - the fifth argument is the verdict, because a shell
# function cannot both compare floats and say which way the comparison was meant to go.
check() {
  if [[ $5 == 1 ]]; then
    note "$1" "$2" "ok, budget $3 $4"
  else
    note "$1" "$2" "FAIL over budget $3 $4"
    fail=1
  fi
}

num() { jq -r "$1 // empty" "$ARTIFACT"; }
ms() { printf '%.3f' "$1"; }
le() { awk -v a="$1" -v b="$2" 'BEGIN { print (a + 0 <= b + 0) ? 1 : 0 }'; }

FRAMES=$(num '.report.frames')
[[ -n "$FRAMES" && "$FRAMES" != "0" ]] ||
  die "the artifact records $FRAMES frames; an empty window would pass every budget below by having nothing in it"

MEAN_MS=$(num '.report.mean_ms')
P99_MS=$(num '.report.p99_ms')
MAX_MS=$(num '.report.max_ms')
PRES_MEAN=$(num '.report.present_mean_ms')
PRES_P99=$(num '.report.present_p99_ms')
STYLE=$(num '.report.style_passes')
LAYOUT=$(num '.report.layout_passes')
RASTER=$(num '.report.tiles_rasterized')
REUSED=$(num '.report.tiles_reused')

echo "v2-gate-check: $ARTIFACT ($FRAMES frames)"
check 'mean_ms' "$(ms "$MEAN_MS")" "$MEAN" ms "$(le "$MEAN_MS" "$MEAN")"
check 'p99_ms' "$(ms "$P99_MS")" "$P99" ms "$(le "$P99_MS" "$P99")"

# Zero style and zero layout passes across scroll-only frames is the spec's cheap guard
# against the regression class that capped v1, and the artifact carries the counters, so
# they are checked here whether or not anyone passed a millisecond budget.
check 'style_passes' "$STYLE" 0 total "$(le "$STYLE" 0)"
check 'layout_passes' "$LAYOUT" 0 total "$(le "$LAYOUT" 0)"

note 'max_ms' "$(ms "$MAX_MS")" 'reported, not gated'
note 'present_mean_ms' "$(ms "$PRES_MEAN")" 'reported, not gated'
if [[ -n "$PRESENT_P99" ]]; then
  check 'present_p99_ms' "$(ms "$PRES_P99")" "$PRESENT_P99" ms "$(le "$PRES_P99" "$PRESENT_P99")"
else
  note 'present_p99_ms' "$(ms "$PRES_P99")" 'reported, not gated'
fi
if [[ $RASTER != "0" ]]; then
  note 'tiles_rasterized' "$RASTER" "of $((RASTER + REUSED)) tile slots offered"
fi

# The two ms/op figures are M1 criteria 7's, and they arrive as `go test -bench` text
# rather than as part of the artifact. With -count > 1 there are several lines per
# benchmark; the median is the number, because one noisy repetition is a property of the
# runner and the mean of three would let it move the verdict.
#
# The columns are counted back from the end of the line, because the leading ones move:
#
#   name-CPUs  iterations  ns/op-value  ns/op  ms/op-value  ms/op  B/op-value  B/op  allocs-value  allocs/op
#
# so the ms/op figure is the sixth field from the right and the allocs count the second.
bench_median() { # bench-name file
  awk -v name="$1" '$1 ~ "^"name "(-[0-9]+)?$" { print $(NF - 5) }' "$2" |
    sort -n |
    awk '{ a[NR] = $1 } END { print (NR % 2) ? a[(NR + 1) / 2] : (a[NR / 2] + a[NR / 2 + 1]) / 2 }'
}

if [[ -n "$BENCH" ]]; then
  [[ -r "$BENCH" ]] || die "cannot read $BENCH"
  echo "v2-gate-check: $BENCH (median of repetitions, ms/op)"
  for spec in "BenchmarkWarmScroll:$WARM_MS" "BenchmarkColdScrollBurst:$COLD_MS"; do
    bname=${spec%%:*}
    budget=${spec##*:}
    got=$(bench_median "$bname" "$BENCH")
    [[ -n "$got" ]] || die "$BENCH records no $bname result; the benchmark did not run"
    # A number, specifically: extracting the wrong column - which is what a changed
    # benchmem layout does to a field count - would otherwise compare "ms/op" to a
    # budget and report a pass.
    [[ "$got" =~ ^[0-9]+([.][0-9]+)?$ ]] ||
      die "$BENCH's $bname result is not a number ($got); the benchmark output changed shape"
    check "$bname" "$(ms "$got")" "$budget" 'ms/op' "$(le "$got" "$budget")"
  done
  allocs=$(awk '/^BenchmarkWarmScroll(-[0-9]+)?[ \t]/ { print $(NF - 1); exit }' "$BENCH")
  note 'warm allocs/op' "${allocs:-?}" 'criterion 4 gates this in CI; reported here'
fi

if [[ $fail == 0 ]]; then
  echo 'v2-gate-check: PASS'
else
  echo 'v2-gate-check: FAIL' >&2
fi
exit $fail
