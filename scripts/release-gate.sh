#!/usr/bin/env bash
# release-gate.sh - run the checks a candidate commit has to pass before it ships, and keep
# the evidence.
#
# BR-179 in the QC catalog asks for exactly this: "run go test ./..., go vet -all ./...,
# archtest and the gate suite on the candidate commit; every command exits 0; keep the
# logs/artifact."  Before this script, running those four by hand was the process, which
# means the process was whatever the person doing the release remembered.  A release cut
# with `go test ./...` but no `-count=1` reuses cached results; a release cut without
# archtest ships an import-graph nobody checked; a release cut with the commands but no
# logs has no evidence to attach to the sign-off.
#
# What it does: run each gate command, tee its stdout+stderr to a log under -out, record
# the exit status and wall time per command, and write release-gate.json.  It deliberately
# asserts nothing about *which* test failed - that is what the per-command log is for.
#
# Exit status: 0 every command passed, 1 a command failed, 2 no verdict was reached
# (unrunnable command, unwritable artifact directory, unreadable report).  2 exists so this
# script can never be the check that both fails to run and reports success.
#
# Usage:
#   scripts/release-gate.sh [-out DIR] [-commands FILE] [-list]
#   scripts/release-gate.sh -h
#
# -commands FILE replaces the gate list (one shell command per line, '#' starts a comment);
# it exists so test/gatecheck/release_gate_test.go can prove the fail-closed behaviour
# without running the whole suite four times over.  -list prints the built-in gate list as
# JSON and exits 0 without running anything.
#
# test/gatecheck/release_gate_test.go is the contract for this file.  Change the built-in
# list, an exit status, or the report shape and it fails there.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="/tmp/goosie-release-gate"
COMMANDS=""
LIST_ONLY=0

# The gate. Order is cheapest-signal first so a developer watching the run learns the most
# per minute; every entry runs even after a failure, because "vet was clean" is worth
# knowing on the same commit where one test failed.
GATE_COMMANDS=(
  "go test -count=1 ./..."
  "go vet -all ./..."
  "go test -count=1 ./internal/archtest/"
  "go test -count=1 -run TestGate ./test/gate/"
)

usage() {
  sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    -out)
      [ $# -ge 2 ] || { echo "release-gate: -out needs a directory" >&2; exit 2; }
      OUT="$2"
      shift 2
      ;;
    -commands)
      [ $# -ge 2 ] || { echo "release-gate: -commands needs a file" >&2; exit 2; }
      COMMANDS="$2"
      shift 2
      ;;
    -list)
      LIST_ONLY=1
      shift
      ;;
    *)
      echo "release-gate: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [ "$LIST_ONLY" = "1" ]; then
  printf '{"commands":['
  first=1
  for cmd in "${GATE_COMMANDS[@]}"; do
    [ "$first" = "1" ] || printf ','
    first=0
    printf '"%s"' "$(json_escape "$cmd")"
  done
  printf ']}\n'
  exit 0
fi

if [ -n "$COMMANDS" ]; then
  [ -r "$COMMANDS" ] || { echo "release-gate: cannot read $COMMANDS" >&2; exit 2; }
  GATE_COMMANDS=()
  while IFS= read -r line; do
    line="${line%%#*}"
    # trim surrounding whitespace
    line="$(printf '%s' "$line" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
    [ -n "$line" ] && GATE_COMMANDS+=("$line")
  done < "$COMMANDS"
  [ ${#GATE_COMMANDS[@]} -gt 0 ] || { echo "release-gate: $COMMANDS has no commands" >&2; exit 2; }
fi

command -v go >/dev/null 2>&1 || { echo "release-gate: go is not on PATH" >&2; exit 2; }

mkdir -p "$OUT" 2>/dev/null || { echo "release-gate: cannot create $OUT" >&2; exit 2; }
[ -w "$OUT" ] || { echo "release-gate: $OUT is not writable" >&2; exit 2; }
OUT="$(cd "$OUT" && pwd)"

REPORT="$OUT/release-gate.json"
rm -f "$REPORT"

rows=""
failed=0
no_verdict=0
index=0

for cmd in "${GATE_COMMANDS[@]}"; do
  index=$((index + 1))
  slug="$(printf '%s' "$cmd" | tr -c 'A-Za-z0-9' '-' | sed 's/-\{2,\}/-/g; s/^-*//; s/-*$//')"
  [ -n "$slug" ] || slug="command-$index"
  log="$OUT/$(printf '%02d' "$index")-$slug.log"

  printf 'release-gate: %s\n' "$cmd"
  start=$(date +%s)
  set +e
  (cd "$ROOT" && bash -c "$cmd") > "$log" 2>&1
  code=$?
  set -e
  end=$(date +%s)
  secs=$((end - start))

  # 127 is the shell's "command not found": no check happened, so this run gets to report
  # neither success nor a product failure.
  if [ "$code" -eq 127 ]; then
    echo "release-gate: '$cmd' did not run (exit 127)" >&2
    no_verdict=1
  elif [ "$code" -ne 0 ]; then
    failed=1
    echo "release-gate: FAILED ($code) $cmd - see $log" >&2
  else
    printf 'release-gate: ok (%ds) %s\n' "$secs" "$cmd"
  fi

  row="$(printf '{"index":%d,"command":"%s","exit":%d,"seconds":%d,"log":"%s"}' \
    "$index" "$(json_escape "$cmd")" "$code" "$secs" "$(json_escape "$log")")"
  [ -z "$rows" ] && rows="$row" || rows="$rows,$row"
done

[ -n "$rows" ] || { echo "release-gate: no commands were attempted" >&2; exit 2; }

total=${#GATE_COMMANDS[@]}
status="pass"
[ "$failed" = "1" ] && status="fail"
printf '{"status":"%s","commands":%d,"failed":%s,"no_verdict":%s,"results":[%s]}\n' \
  "$status" "$total" "$failed" "$no_verdict" "$rows" > "$REPORT"

[ -r "$REPORT" ] || { echo "release-gate: report was not written to $REPORT" >&2; exit 2; }

if [ "$no_verdict" = "1" ]; then
  echo "release-gate: no verdict - a gate command could not be run" >&2
  exit 2
fi
if [ "$failed" = "1" ]; then
  echo "release-gate: FAIL (report: $REPORT)" >&2
  exit 1
fi
echo "release-gate: PASS ($total commands, report: $REPORT)"
