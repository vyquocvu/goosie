#!/usr/bin/env bash
# scripts/idle-footprint.sh - the brief's "<50MB idle" as a measured number.
#
# Reports process peak resident set size for the headless render path at fixed
# geometries. /usr/bin/time -l prints two memory figures - "maximum resident set size"
# and "peak memory footprint" - and they are different quantities that are never
# interchangeable. Only the resident column is used here, because that is the number a
# "<50MB idle" claim is about.
#
# A run that exits non-zero is a failure, not a small number: an errored process peaks
# under 6 MiB and would otherwise read as a pass.
set -euo pipefail
BIN="$(mktemp -d)/goosie-headless"
MAX_MIB="${1:-}"
go build -o "$BIN" ./cmd/goosie-headless
printf '<html><body><p>hi</p></body></html>' >/tmp/goosie-blank.html
printf '%-34s %10s %8s\n' config "max RSS" "wall"
while read -r label file w h d; do
  out="$(mktemp).png"
  t0=$(date +%s)
  usr_time="$(/usr/bin/time -l "$BIN" -in "$file" -out "$out" -width "$w" -height "$h" -dpr "$d" 2>&1 >/dev/null)" || {
    echo "$label $w x $h dpr$d: FAILED to render: $(echo "$usr_time" | head -1)" >&2; exit 1; }
  mib="$(echo "$usr_time" | awk '/maximum resident set size/{printf "%.1f", $1/1048576}')"
  printf '%-34s %8s MiB %6ss\n' "$label $w x $h @dpr$d" "$mib" "$(( $(date +%s) - t0 ))"
  if [ -n "$MAX_MIB" ]; then
    awk -v m="$mib" -v lim="$MAX_MIB" 'BEGIN{exit !(m+0 > lim+0)}' && {
      echo "regression: $label exceeds ${MAX_MIB} MiB" >&2; exit 1; }
  fi
  rm -f "$out"
done <<'CASES'
blank /tmp/goosie-blank.html 800 600 1
tables testdata/render/tables/02-colspan-rowspan.html 320 480 1
tables testdata/render/tables/02-colspan-rowspan.html 800 600 1
tables testdata/render/tables/02-colspan-rowspan.html 1440 900 1
tables testdata/render/tables/02-colspan-rowspan.html 1440 900 2
CASES
