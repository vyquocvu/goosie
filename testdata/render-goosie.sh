#!/bin/bash
# Render all test fixtures with Goosie headless

set -e

INPUT_DIR="testdata/render"
OUTPUT_DIR="/tmp/goosie-renders"
BINARY="/tmp/goosie-headless"

mkdir -p "$OUTPUT_DIR"

find "$INPUT_DIR" -name "*.html" | while read -r html_file; do
  relative_path="${html_file#$INPUT_DIR/}"
  output_path="$OUTPUT_DIR/${relative_path%.html}.png"
  output_subdir="$(dirname "$output_path")"

  mkdir -p "$output_subdir"

  # Convert to absolute path for file:// URL
  abs_path="$(pwd)/$html_file"

  echo "Rendering: $relative_path"
  "$BINARY" -in "$abs_path" -out "$output_path" -width 800 -height 600 -dpr 1 2>&1 | grep -v "^goosie:" || true
done

echo "Done! Rendered to $OUTPUT_DIR"
