#!/bin/bash
# run-wpt.sh - Run Web Platform Tests against goosie's headless renderer.
#
# Usage:
#   scripts/run-wpt.sh                    # run full curated suite
#   scripts/run-wpt.sh --dirs css/css-color  # run one directory
#   scripts/run-wpt.sh --download         # download WPT first
#   scripts/run-wpt.sh --report-only      # generate report from existing results
#
# Environment:
#   WPT_DIR          - path to WPT checkout (default: /tmp/goosie-wpt)
#   WPT_GOOSIE_BINARY - path to goosie binary (default: ./goosie)
#   WPT_CONFIG       - path to config JSON (default: testdata/wpt-config.json)
#   WPT_OUTPUT_DIR   - where to write results (default: /tmp/wpt-results)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Defaults.
WPT_DIR="${WPT_DIR:-/tmp/goosie-wpt}"
WPT_GOOSIE_BINARY="${WPT_GOOSIE_BINARY:-$PROJECT_ROOT/goosie}"
WPT_CONFIG="${WPT_CONFIG:-$PROJECT_ROOT/testdata/wpt-config.json}"
WPT_OUTPUT_DIR="${WPT_OUTPUT_DIR:-/tmp/wpt-results}"
WPT_COMMIT="${WPT_COMMIT:-623eb3a3e1b6b04b4b098a1e8a7b3e6b0e8c4d2a}"
WPT_DIRS=""
DOWNLOAD=false
REPORT_ONLY=false

# Parse arguments.
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dirs)
      WPT_DIRS="$2"
      shift 2
      ;;
    --download)
      DOWNLOAD=true
      shift
      ;;
    --report-only)
      REPORT_ONLY=true
      shift
      ;;
    --wpt-dir)
      WPT_DIR="$2"
      shift 2
      ;;
    --binary)
      WPT_GOOSIE_BINARY="$2"
      shift 2
      ;;
    --output)
      WPT_OUTPUT_DIR="$2"
      shift 2
      ;;
    --commit)
      WPT_COMMIT="$2"
      shift 2
      ;;
    -h|--help)
      head -20 "$0" | tail -15
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

echo "=== Goosie WPT Runner ==="
echo "Binary:    $WPT_GOOSIE_BINARY"
echo "Config:    $WPT_CONFIG"
echo "WPT Dir:   $WPT_DIR"
echo "Output:    $WPT_OUTPUT_DIR"
echo "Commit:    $WPT_COMMIT"
echo ""

# Check that the goosie binary exists.
if [[ ! -x "$WPT_GOOSIE_BINARY" ]]; then
  echo "Building goosie..."
  cd "$PROJECT_ROOT"
  go build -trimpath -ldflags "-s -w" -o goosie ./cmd/goosie
  echo "Built: $WPT_GOOSIE_BINARY"
fi

# Download WPT if requested or if the directory doesn't exist.
if [[ "$DOWNLOAD" == "true" ]] || [[ ! -d "$WPT_DIR" ]]; then
  echo "Downloading WPT at commit $WPT_COMMIT..."
  mkdir -p "$(dirname "$WPT_DIR")"

  TARBALL="/tmp/wpt-${WPT_COMMIT}.tar.gz"
  if [[ ! -f "$TARBALL" ]]; then
    curl -sL "https://github.com/web-platform-tests/wpt/archive/${WPT_COMMIT}.tar.gz" -o "$TARBALL"
  fi

  rm -rf "$WPT_DIR"
  mkdir -p "$WPT_DIR"
  tar xzf "$TARBALL" -C "$WPT_DIR" --strip-components=1
  echo "WPT extracted to $WPT_DIR"
fi

# Create output directory.
mkdir -p "$WPT_OUTPUT_DIR"

# Export environment for the Go test.
export WPT_DIR
export WPT_GOOSIE_BINARY
export WPT_CONFIG
export WPT_OUTPUT_DIR

if [[ -n "$WPT_DIRS" ]]; then
  export WPT_DIRS
fi

# Run the WPT test suite.
echo ""
echo "Running WPT tests..."
echo ""

cd "$PROJECT_ROOT"
go test -v -timeout 30m -run TestWPTSuite ./test/wpt/ 2>&1 | tee "$WPT_OUTPUT_DIR/test-output.txt"

EXIT_CODE=${PIPESTATUS[0]}

# Print summary.
echo ""
echo "=== Results ==="
if [[ -f "$WPT_OUTPUT_DIR/results.json" ]]; then
  echo "JSON report: $WPT_OUTPUT_DIR/results.json"
  # Extract key numbers from JSON.
  if command -v python3 &>/dev/null; then
    python3 -c "
import json
with open('$WPT_OUTPUT_DIR/results.json') as f:
    r = json.load(f)
print(f\"Total: {r['total']}, Passed: {r['passed']}, Failed: {r['failed']}, Skipped: {r['skipped']}\")
print(f\"Pass Rate: {r['pass_rate']:.1f}%\")
" 2>/dev/null || true
  fi
fi
if [[ -f "$WPT_OUTPUT_DIR/report.html" ]]; then
  echo "HTML report: $WPT_OUTPUT_DIR/report.html"
fi

echo ""
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "WPT run completed successfully."
else
  echo "WPT run completed with failures (exit code: $EXIT_CODE)."
fi

exit $EXIT_CODE
