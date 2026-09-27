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
#       [-present-p99 MS] [-startup-ms MS] [-tile-mib MIB]
#       [-bench bench.txt -warm-ms 5 -cold-ms 8]
#   scripts/v2-gate-check.sh -h
#
# Exit status: 0 pass, 1 a budget was missed, 2 the invocation or the artifact is bad.
# Bad includes a metric a budget measures against being absent or non-numeric. Absent does
# not read as zero: a budget compared against nothing is a gate that is not gating.
#
# startup_ms is the cold start: the instant the process began to the first frame a window
# presented. A run that never presented one reports it null, which is printed as "not
# measured" and refused by a budget rather than read as the fastest possible start.
#
# tile_mib is the tile cache's ledger: the tile buffers a run holds, against the buffers its
# own configuration authorised, plus the evictions that kept it there. A gate run at the
# nightly's geometry holds hundreds of mebibytes, which reads as a leak to anyone without the
# authorised figure beside it - the paced scene budgets the whole document, so a full cache
# is the configuration working and not a defect. Held bytes are therefore reported always and
# gated only against a number someone named.
#
# test/gatecheck is the contract for this file. Change an exit status here and it fails there.
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

# -h is reachable as the first word, and not only after an artifact path. The argument
# loop below cannot see it, because that loop runs after the artifact has been consumed.
if [[ ${1:-} == -h || ${1:-} == --help ]]; then
  usage
  exit 0
fi

ARTIFACT=${1:-}
[[ -n "$ARTIFACT" ]] || {
  usage >&2
  exit 2
}
shift

MEAN=16.0
P99=33.0
PRESENT_P99=''
STARTUP_MS_BUDGET=''
BENCH=''
WARM_MS=5.0
COLD_MS=8.0
TILE_MIB=''

# assert_budget flag value - a budget that is not a number is a broken invocation. This
# cannot be folded into the assignments, because an empty value has to stay distinguishable
# from an absent flag: the branches below read "" as "this metric is reported, not gated",
# and -present-p99 with an empty value would otherwise skip its own gate in silence.
assert_budget() {
  [[ $2 =~ ^[0-9]+([.][0-9]+)?$ ]] || die "$1 needs a non-negative number, got '${2:-<absent>}'"
}

while [[ $# -gt 0 ]]; do
  case $1 in
    -mean) MEAN=${2:-}; assert_budget -mean "$MEAN"; shift 2 ;;
    -p99) P99=${2:-}; assert_budget -p99 "$P99"; shift 2 ;;
    -present-p99) PRESENT_P99=${2:-}; assert_budget -present-p99 "$PRESENT_P99"; shift 2 ;;
    -startup-ms) STARTUP_MS_BUDGET=${2:-}; assert_budget -startup-ms "$STARTUP_MS_BUDGET"; shift 2 ;;
    -bench) BENCH=${2:-}; shift 2 ;;
    -warm-ms) WARM_MS=${2:-}; assert_budget -warm-ms "$WARM_MS"; shift 2 ;;
    -cold-ms) COLD_MS=${2:-}; assert_budget -cold-ms "$COLD_MS"; shift 2 ;;
    -tile-mib) TILE_MIB=${2:-}; assert_budget -tile-mib "$TILE_MIB"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) die "unknown argument $1" ;;
  esac
done

[[ -r "$ARTIFACT" ]] || die "cannot read $ARTIFACT"
jq -e . "$ARTIFACT" >/dev/null 2>&1 || die "$ARTIFACT is not JSON"
# Any non-null JSON is valid JSON, including [] and {"report":"gone"}, and every read
# below would then fail inside jq with whatever status jq happens to exit with. Checking
# the shape here is what keeps a structurally wrong artifact an exit 2.
jq -e '.report | type == "object"' "$ARTIFACT" >/dev/null 2>&1 ||
  die "$ARTIFACT has no .report object; it is not a gate artifact"

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
# mib bytes - the tile ledger is counted in bytes because that is what a grid reports, and
# read in mebibytes because that is the unit a budget is worth stating in.
mib() { awk -v b="$1" 'BEGIN { printf "%.3f", b / 1048576 }'; }
le() { awk -v a="$1" -v b="$2" 'BEGIN { print (a + 0 <= b + 0) ? 1 : 0 }'; }
# gated name value - a metric a budget is about to be measured against has to be there.
# An absent field reads as empty, awk compares empty as zero, and zero is under every
# budget, so a report from a build that stopped emitting mean_ms would PASS. That is the
# same failure the frame count guard below already refuses for an empty window, and it
# reaches the milliseconds too. No sign is accepted either: a negative latency or pass
# counter is a broken artifact, not a measurement comfortably under budget.
gated() {
  [[ $2 =~ ^[0-9]+([.][0-9]+)?$ ]] ||
    die "$ARTIFACT's $1 is missing, negative, or not a number (${2:-<absent>}); the artifact changed shape"
  printf '%s' "$2"
}

# A positive whole number, spelled as one. The guard exists because an empty window passes
# every budget by having nothing in it, and comparing against the string "0" refuses only
# one spelling of zero: JSON writes a counted zero as 0.0 as happily as 0.
FRAMES=$(num '.report.frames')
[[ $FRAMES =~ ^[1-9][0-9]*$ ]] ||
  die "the artifact records $FRAMES frames; an empty window would pass every budget below by having nothing in it"

MEAN_MS=$(gated mean_ms "$(num '.report.mean_ms')")
P99_MS=$(gated p99_ms "$(num '.report.p99_ms')")
MAX_MS=$(num '.report.max_ms')
PRES_MEAN=$(num '.report.present_mean_ms')
PRES_P99=$(num '.report.present_p99_ms')
STYLE=$(gated style_passes "$(num '.report.style_passes')")
LAYOUT=$(gated layout_passes "$(num '.report.layout_passes')")
# Gated rather than merely reported: the tile note below adds them, and a non-numeric
# value there aborts the run mid-report on a bash arithmetic error, which reads as a
# missed budget rather than the broken artifact it is.
RASTER=$(gated tiles_rasterized "$(num '.report.tiles_rasterized')")
REUSED=$(gated tiles_reused "$(num '.report.tiles_reused')")

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
  PRES_P99=$(gated present_p99_ms "$PRES_P99")
  check 'present_p99_ms' "$(ms "$PRES_P99")" "$PRESENT_P99" ms "$(le "$PRES_P99" "$PRESENT_P99")"
else
  note 'present_p99_ms' "$(ms "$PRES_P99")" 'reported, not gated'
fi
# The cold start. Null means the run never presented a frame, and bash's printf reads an
# empty value as 0.000 - a start fast enough to beat any budget - so an absent figure is
# labelled and, when someone has asked for it to be gated, refused rather than formatted.
STARTUP_MS=$(num '.report.startup_ms')
if [[ -n "$STARTUP_MS" && ! "$STARTUP_MS" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  die "$ARTIFACT's startup_ms is not a number ($STARTUP_MS); the artifact changed shape"
fi
if [[ -n "$STARTUP_MS_BUDGET" ]]; then
  STARTUP_MS=$(gated startup_ms "$STARTUP_MS")
  check 'startup_ms' "$(ms "$STARTUP_MS")" "$STARTUP_MS_BUDGET" ms "$(le "$STARTUP_MS" "$STARTUP_MS_BUDGET")"
elif [[ -n "$STARTUP_MS" ]]; then
  note 'startup_ms' "$(ms "$STARTUP_MS")" 'reported, not gated'
else
  note 'startup_ms' 'not measured' 'no frame reached a window'
fi
if [[ $RASTER != "0" ]]; then
  note 'tiles_rasterized' "$RASTER" "of $((RASTER + REUSED)) tile slots offered"
fi
# The tile cache's ledger. Held bytes and authorised bytes are two separate numbers on
# purpose: the gap between them is the only thing that distinguishes a cache that filled its
# configuration from buffers escaping the accounting that authorised them, and the eviction
# count says which of the two the grid was doing about it.
TILE_BYTES=$(num '.report.tile_bytes')
TILE_BUDGET=$(num '.report.tile_budget')
TILE_EVICT=$(num '.report.tile_evictions')
for pair in tile_bytes:"$TILE_BYTES" tile_budget:"$TILE_BUDGET" tile_evictions:"$TILE_EVICT"; do
  v=${pair#*:}
  [[ -z "$v" || "$v" =~ ^[0-9]+$ ]] ||
    die "$ARTIFACT's ${pair%%:*} is not a whole number of bytes ($v); the artifact changed shape"
done
if [[ -n "$TILE_BYTES" ]]; then
  HELD=$(mib "$TILE_BYTES")
  if [[ -n "$TILE_BUDGET" ]]; then
    AUTHORISED="of $(mib "$TILE_BUDGET") MiB configured"
  else
    AUTHORISED='configured budget not reported'
  fi
  if [[ -n "$TILE_MIB" ]]; then
    check 'tile_mib' "$HELD" "$TILE_MIB" MiB "$(le "$HELD" "$TILE_MIB")"
  else
    note 'tile_mib' "$HELD" "$AUTHORISED, reported, not gated"
  fi
else
  # A budget against no ledger is the same fail-open the startup check refuses: an absent
  # figure would read as zero mebibytes, which is under any budget worth naming.
  [[ -z "$TILE_MIB" ]] || gated tile_bytes "$TILE_BYTES"
  note 'tile_mib' 'not reported' 'the build carries no tile ledger'
fi
note 'tile_evictions' "${TILE_EVICT:-not reported}" 'reported, not gated'

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
# The empty transcript has to stay empty. Reaching the even branch with no records at all
# averages two unset elements, which is zero, and zero is under budget - so a benchmark
# that stopped running would report a pass rather than trip the check below it.
bench_median() { # bench-name file
  awk -v name="$1" '$1 ~ "^"name "(-[0-9]+)?$" { print $(NF - 5) }' "$2" |
    sort -n |
    awk '{ a[NR] = $1 } END { if (!NR) exit; print (NR % 2) ? a[(NR + 1) / 2] : (a[NR / 2] + a[NR / 2 + 1]) / 2 }'
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
