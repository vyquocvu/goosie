#!/usr/bin/env bash
# CI benchmark runner — runs microbenchmarks for engine packages,
# compares against baseline with benchstat, and exits with codes:
#   0 = pass
#   1 = timing regression warning
#   2 = allocation regression failure
set -euo pipefail

PACKAGES=(
  ./test/internal/dom
  ./test/internal/css
  ./test/internal/renderer
  ./test/internal/engine/session
  ./test/internal/engine/navigation
  ./test/internal/engine/metrics
  ./test/internal/engine/testpages
  ./test/internal/js
  ./test/internal/net
)

BENCHTIME="${BENCHTIME:-100ms}"
RESULTS_DIR="${RESULTS_DIR:-testdata/benchmarks}"
RESULTS_FILE="${RESULTS_DIR}/current.txt"
BASELINE_FILE="${RESULTS_DIR}/main.txt"
BENCHSTAT="$(go env GOPATH)/bin/benchstat"

ALLOC_FAIL_THRESHOLD="${ALLOC_FAIL_THRESHOLD:-5}"    # allocs/op % increase → failure
MEM_FAIL_THRESHOLD="${MEM_FAIL_THRESHOLD:-10}"        # B/op % increase → failure
TIMING_WARN_THRESHOLD="${TIMING_WARN_THRESHOLD:-10}"  # ns/op % increase → warning

mkdir -p "$RESULTS_DIR"

ensure_benchstat() {
  if ! command -v "$BENCHSTAT" &>/dev/null; then
    echo "Installing benchstat..."
    go install golang.org/x/perf/cmd/benchstat@latest
  fi
}

run_benchmarks() {
  local output="$1"
  echo "Running microbenchmarks..."
  for pkg in "${PACKAGES[@]}"; do
    echo "  bench: $pkg"
    go test -run='^$' -bench='.' -benchmem -benchtime="$BENCHTIME" -timeout=10m "$pkg" >> "$output"
  done
  echo "Results saved to $output"
}

record_baseline() {
  ensure_benchstat
  run_benchmarks "$BASELINE_FILE"
  echo "Baseline recorded at $BASELINE_FILE"
}

check_regressions() {
  ensure_benchstat

  if [ ! -f "$BASELINE_FILE" ]; then
    echo "No baseline found at $BASELINE_FILE — skipping comparison"
    run_benchmarks "$RESULTS_FILE"
    return 0
  fi

  run_benchmarks "$RESULTS_FILE"

  echo "Comparing against baseline..."
  "$BENCHSTAT" "$BASELINE_FILE" "$RESULTS_FILE" > "$RESULTS_DIR/delta.txt" 2>&1 || true

  echo ""
  echo "=== benchstat delta ==="
  cat "$RESULTS_DIR/delta.txt"

  local has_alloc_fail=false
  local has_mem_fail=false
  local has_timing_warn=false

  while IFS= read -r line; do
    # benchstat output lines look like:
    #   {Name}  {old}  {new}  {delta}
    # The delta column has format like "+5.2%" or "-3.1%"
    if [[ "$line" =~ ([+-]?[0-9]+(\.[0-9]+)?)% ]]; then
      pct="${BASH_REMATCH[1]}"
      pct_num="${pct//[%+-]/}"
      pct_num="${pct_num:-0}"

      # Determine if this is an alloc, mem, or timing line
      if echo "$line" | grep -q 'allocs/op'; then
        if [ "$(echo "$pct_num > $ALLOC_FAIL_THRESHOLD" | bc -l 2>/dev/null)" = 1 ]; then
          echo "FAIL: Allocation regression in: $line"
          has_alloc_fail=true
        fi
      elif echo "$line" | grep -q 'B/op'; then
        if [ "$(echo "$pct_num > $MEM_FAIL_THRESHOLD" | bc -l 2>/dev/null)" = 1 ]; then
          echo "FAIL: Memory regression in: $line"
          has_mem_fail=true
        fi
      elif echo "$line" | grep -q 'ns/op'; then
        if [ "$(echo "$pct_num > $TIMING_WARN_THRESHOLD" | bc -l 2>/dev/null)" = 1 ]; then
          echo "WARN: Timing regression in: $line"
          has_timing_warn=true
        fi
      fi
    fi
  done < <(grep '%' "$RESULTS_DIR/delta.txt" 2>/dev/null || true)

  echo ""
  local exit_code=0
  if [ "$has_alloc_fail" = true ] || [ "$has_mem_fail" = true ]; then
    echo "RESULT: FAIL (allocation/memory regression detected)"
    exit_code=2
  elif [ "$has_timing_warn" = true ]; then
    echo "RESULT: WARNING (timing regression detected)"
    exit_code=1
  else
    echo "RESULT: PASS"
  fi

  return "$exit_code"
}

main() {
  local mode="${1:-check}"

  case "$mode" in
    record)
      record_baseline
      ;;
    check)
      check_regressions
      ;;
    run-only)
      run_benchmarks "$RESULTS_FILE"
      ;;
    *)
      echo "Usage: $0 {check|record|run-only}"
      echo ""
      echo "  check      Run benchmarks and compare against baseline (default)"
      echo "  record     Run benchmarks and save as new baseline"
      echo "  run-only   Run benchmarks without comparison"
      exit 1
      ;;
  esac
}

main "$@"
