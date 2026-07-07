#!/usr/bin/env bash
set -e

COMMAND=$1
shift || true

if [ "$COMMAND" = "run" ]; then
    PKG=${1:-./...}
    echo "Running benchmarks for $PKG..."
    go test -run=^$ -bench=. -benchmem "$PKG"
elif [ "$COMMAND" = "profile-cpu" ]; then
    PKG=$1
    if [ -z "$PKG" ]; then
      echo "Usage: $0 profile-cpu <package> [benchmark-regex]"
    else
      BENCH=${2:-.}
      echo "Capturing CPU profile for $PKG (bench: $BENCH)..."
      go test -run=^$ -bench="$BENCH" -cpuprofile=cpu.prof "$PKG"
      echo "Profile saved to cpu.prof. To view: go tool pprof cpu.prof"
    fi
elif [ "$COMMAND" = "profile-mem" ]; then
    PKG=$1
    if [ -z "$PKG" ]; then
      echo "Usage: $0 profile-mem <package> [benchmark-regex]"
    else
      BENCH=${2:-.}
      echo "Capturing memory profile for $PKG (bench: $BENCH)..."
      go test -run=^$ -bench="$BENCH" -memprofile=mem.prof "$PKG"
      echo "Profile saved to mem.prof. To view: go tool pprof mem.prof"
    fi
elif [ "$COMMAND" = "trace" ]; then
    PKG=$1
    if [ -z "$PKG" ]; then
      echo "Usage: $0 trace <package> [benchmark-regex]"
    else
      BENCH=${2:-.}
      echo "Capturing runtime trace for $PKG (bench: $BENCH)..."
      go test -run=^$ -bench="$BENCH" -trace=trace.out "$PKG"
      echo "Trace saved to trace.out. To view: go tool trace trace.out"
    fi
elif [ "$COMMAND" = "compare" ]; then
    OLD_FILE=$1
    NEW_FILE=$2
    if [ -z "$OLD_FILE" ] || [ -z "$NEW_FILE" ]; then
      echo "Usage: $0 compare <old-results.txt> <new-results.txt>"
      echo "Note: requires benchstat (go install golang.org/x/perf/cmd/benchstat@latest)"
    else
      if ! command -v benchstat &> /dev/null; then
          echo "benchstat not found, installing..."
          go install golang.org/x/perf/cmd/benchstat@latest
      fi
      benchstat "$OLD_FILE" "$NEW_FILE"
    fi
elif [ "$COMMAND" = "suite" ]; then
    echo "Running full performance suite..."
    go test -run=^$ -bench=. -benchmem ./... > perf-suite.txt
    echo "Results saved to perf-suite.txt"
else
    echo "Usage: $0 <command> [args...]"
    echo "Commands:"
    echo "  run [package]                       Run benchmarks (default: ./...)"
    echo "  suite                               Run full performance suite and save to perf-suite.txt"
    echo "  profile-cpu <package> [regex]       Capture CPU profile"
    echo "  profile-mem <package> [regex]       Capture memory profile"
    echo "  trace <package> [regex]             Capture runtime trace"
    echo "  compare <old.txt> <new.txt>         Compare results using benchstat"
fi
