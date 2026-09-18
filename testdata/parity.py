#!/usr/bin/env python3
"""Score Goosie's renders against the Playwright (Chromium) reference renders.

A pixel matches when every RGB channel differs by at most THRESHOLD; a fixture
passes when at least 90% of pixels match. Pass --render to re-render the
fixtures with the local ./goosie binary first.
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path

import numpy as np
from PIL import Image

THRESHOLD = 30
PASS_AT = 90.0
FIXTURES = Path("testdata/render")
GOOSIE = Path("/tmp/goosie-renders")
CHROMIUM = Path("/tmp/playwright-renders")


def render_all(binary: str) -> None:
    for html in sorted(FIXTURES.rglob("*.html")):
        rel = html.relative_to(FIXTURES).with_suffix(".png")
        out = GOOSIE / rel
        out.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            [binary, "-backend", "headless", "-width", "800", "-height", "600",
             "-dpr", "1", "-screenshot", "-out", str(out),
             "-url", "file://" + str(html.resolve())],
            check=False, capture_output=True,
        )


def score() -> list[tuple[float, str, str]]:
    rows = []
    for html in sorted(FIXTURES.rglob("*.html")):
        rel = html.relative_to(FIXTURES).with_suffix(".png")
        gp, pp = GOOSIE / rel, CHROMIUM / rel
        if not gp.exists() or not pp.exists():
            continue
        g = np.array(Image.open(gp).convert("RGB"))
        p = np.array(Image.open(pp).convert("RGB"))
        if g.shape != p.shape:
            p = np.array(Image.open(pp).convert("RGB").resize(
                (g.shape[1], g.shape[0]), Image.Resampling.LANCZOS))
        pct = float(np.mean(np.all(np.abs(g.astype(int) - p.astype(int))
                                  <= THRESHOLD, axis=2)) * 100)
        rows.append((pct, "PASS" if pct >= PASS_AT else "FAIL", str(rel)))
    rows.sort(key=lambda r: -r[0])
    return rows


def inspect(name: str, band: int) -> None:
    """Print where a fixture's mismatches land: per-row-band, per-column-band.

    A run of bad bands starting at one row and never recovering is a vertical
    offset; one bad band is a single misplaced box.
    """
    gp, pp = GOOSIE / name, CHROMIUM / name
    g = np.array(Image.open(gp).convert("RGB")).astype(int)
    p = np.array(Image.open(pp).convert("RGB")).astype(int)
    if g.shape != p.shape:
        p = np.array(Image.open(pp).convert("RGB").resize(
            (g.shape[1], g.shape[0]), Image.Resampling.LANCZOS)).astype(int)
    bad = ~np.all(np.abs(g - p) <= THRESHOLD, axis=2)
    h, w = bad.shape
    print(f"{name}: {bad.mean() * 100:.2f}% of {w}x{h} mismatched")
    print("rows")
    for y in range(0, h - h % band, band):
        frac = bad[y:y + band].mean() * 100
        if frac > 1:
            print(f"  {y:4d}-{y + band - 1:4d} {frac:5.1f}% {'#' * int(frac / 2)}")
    print("cols")
    for x in range(0, w - w % band, band):
        frac = bad[:, x:x + band].mean() * 100
        if frac > 1:
            print(f"  {x:4d}-{x + band - 1:4d} {frac:5.1f}% {'#' * int(frac / 2)}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--render", action="store_true")
    ap.add_argument("--binary", default="./goosie")
    ap.add_argument("--inspect", metavar="REL.png", help="print a fixture's mismatch bands")
    ap.add_argument("--band", type=int, default=32)
    args = ap.parse_args()

    if args.inspect:
        inspect(args.inspect, args.band)
        return 0

    if args.render:
        render_all(args.binary)

    rows = score()
    if not rows:
        print("no renders found", file=sys.stderr)
        return 1
    for pct, status, name in rows:
        print(f"{pct:6.2f}% {status} {name}")
    passing = sum(1 for r in rows if r[1] == "PASS")
    print(f"\n{passing}/{len(rows)} passing "
          f"({passing / len(rows) * 100:.1f}%), "
          f"average {sum(r[0] for r in rows) / len(rows):.2f}%")
    return 0


if __name__ == "__main__":
    sys.exit(main())
