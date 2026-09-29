#!/usr/bin/env python3
"""Score Goosie's renders against the Playwright (Chromium) reference renders.

A pixel matches when every RGB channel differs by at most THRESHOLD; a fixture
passes when at least PASS_AT percent of pixels match.

Modes:
    --render          Re-render fixtures with the local goosie binary first.
    --json FILE       Write a JSON report to FILE.
    --junit FILE      Write a JUnit XML report to FILE.
    --diffs DIR       Write per-fixture diff images to DIR.
    --threshold N     Per-channel tolerance (default 30).
    --pass-at N       Pass percentage (default 90.0).
    --inspect NAME    Print per-band mismatch for one fixture.

Exit codes:
    0  All scored fixtures passed the threshold.
    1  One or more fixtures failed, or a render step failed (fail-closed).
    2  Bad invocation (missing binary, missing fixtures, etc.).
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
import xml.etree.ElementTree as ET
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFont

THRESHOLD = 30
PASS_AT = 90.0
FIXTURES = Path("testdata/render")
GOOSIE = Path("/tmp/goosie-renders")
CHROMIUM = Path("/tmp/playwright-renders")


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------

class RenderError(Exception):
    """A render step produced an error that must not be silently skipped."""


def render_goosie(binary: str, width: int = 800, height: int = 600) -> list[str]:
    """Render every fixture with the goosie binary.  Returns the list of
    relative PNG paths that were produced.  Raises RenderError if the binary
    itself cannot be executed."""
    binary_path = Path(binary)
    if not binary_path.is_file():
        raise RenderError(f"goosie binary not found: {binary}")

    produced: list[str] = []
    errors: list[str] = []

    for html in sorted(FIXTURES.rglob("*.html")):
        rel = html.relative_to(FIXTURES).with_suffix(".png")
        out = GOOSIE / rel
        out.parent.mkdir(parents=True, exist_ok=True)

        result = subprocess.run(
            [binary, "-backend", "headless", "-width", str(width),
             "-height", str(height), "-dpr", "1", "-screenshot",
             "-out", str(out), "-url", "file://" + str(html.resolve())],
            capture_output=True, text=True, timeout=30,
        )
        if result.returncode != 0:
            errors.append(f"{rel}: exit {result.returncode}: {result.stderr.strip()[:200]}")
            continue
        if not out.exists():
            errors.append(f"{rel}: exit 0 but no output file produced")
            continue
        produced.append(str(rel))

    if errors:
        print(f"WARNING: {len(errors)} fixture(s) failed to render:", file=sys.stderr)
        for e in errors:
            print(f"  {e}", file=sys.stderr)

    if not produced and errors:
        raise RenderError(
            f"all {len(errors)} fixture(s) failed to render; "
            f"first error: {errors[0]}"
        )

    return produced


def render_chromium(width: int = 800, height: int = 600) -> list[str]:
    """Render every fixture with Playwright/Chromium.  Returns the list of
    relative PNG paths.  Raises RenderError if playwright is unavailable."""
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        raise RenderError(
            "playwright is not installed; run 'pip install playwright && "
            "playwright install chromium' to generate reference renders"
        )

    produced: list[str] = []
    errors: list[str] = []

    try:
        with sync_playwright() as pw:
            browser = pw.chromium.launch()
            page = browser.new_page(viewport={"width": width, "height": height})

            for html in sorted(FIXTURES.rglob("*.html")):
                rel = html.relative_to(FIXTURES).with_suffix(".png")
                out = CHROMIUM / rel
                out.parent.mkdir(parents=True, exist_ok=True)

                try:
                    page.goto("file://" + str(html.resolve()), wait_until="load")
                    page.screenshot(path=str(out))
                    produced.append(str(rel))
                except Exception as exc:
                    errors.append(f"{rel}: {exc}")

            browser.close()
    except Exception as exc:
        raise RenderError(f"playwright launch failed: {exc}")

    if errors:
        print(f"WARNING: {len(errors)} Chromium render(s) failed:", file=sys.stderr)
        for e in errors:
            print(f"  {e}", file=sys.stderr)

    if not produced and errors:
        raise RenderError(
            f"all {len(errors)} Chromium render(s) failed; "
            f"first error: {errors[0]}"
        )

    return produced


# ---------------------------------------------------------------------------
# Scoring
# ---------------------------------------------------------------------------

class FixtureResult:
    """One fixture's comparison outcome."""

    def __init__(self, name: str, pct: float, status: str,
                 goosie_size: tuple[int, int] | None = None,
                 chromium_size: tuple[int, int] | None = None,
                 error: str | None = None):
        self.name = name
        self.pct = pct
        self.status = status          # "PASS", "FAIL", "ERROR", "MISSING"
        self.goosie_size = goosie_size
        self.chromium_size = chromium_size
        self.error = error

    def to_dict(self) -> dict:
        d: dict = {
            "fixture": self.name,
            "match_pct": round(self.pct, 3),
            "status": self.status,
        }
        if self.goosie_size:
            d["goosie_size"] = list(self.goosie_size)
        if self.chromium_size:
            d["chromium_size"] = list(self.chromium_size)
        if self.error:
            d["error"] = self.error
        return d


def _category(name: str) -> str:
    """Extract the category from a fixture path like 'typography/01-font.png'."""
    parts = Path(name).parts
    return parts[0] if len(parts) > 1 else "uncategorized"


def score_fixtures(threshold: int, pass_at: float,
                   strict: bool = True) -> list[FixtureResult]:
    """Compare every goosie render to its Chromium counterpart.

    When *strict* is True (the default), missing pairs and dimension mismatches
    are reported as errors rather than silently skipped.
    """
    results: list[FixtureResult] = []

    for html in sorted(FIXTURES.rglob("*.html")):
        rel = str(html.relative_to(FIXTURES).with_suffix(".png"))
        gp = GOOSIE / rel
        pp = CHROMIUM / rel

        if not gp.exists() and not pp.exists():
            if strict:
                results.append(FixtureResult(
                    rel, 0.0, "MISSING",
                    error="neither goosie nor chromium render exists"))
            continue

        if not gp.exists():
            if strict:
                results.append(FixtureResult(
                    rel, 0.0, "MISSING",
                    error="goosie render missing"))
            continue

        if not pp.exists():
            if strict:
                results.append(FixtureResult(
                    rel, 0.0, "MISSING",
                    error="chromium render missing"))
            continue

        g_img = Image.open(gp).convert("RGB")
        p_img = Image.open(pp).convert("RGB")
        g = np.array(g_img)
        p = np.array(p_img)

        if g.shape != p.shape:
            results.append(FixtureResult(
                rel, 0.0, "ERROR",
                goosie_size=(g.shape[1], g.shape[0]),
                chromium_size=(p.shape[1], p.shape[0]),
                error=f"dimension mismatch: goosie {g.shape[1]}x{g.shape[0]} "
                      f"vs chromium {p.shape[1]}x{p.shape[0]}"))
            continue

        diff = np.abs(g.astype(int) - p.astype(int))
        match_mask = np.all(diff <= threshold, axis=2)
        pct = float(np.mean(match_mask) * 100)

        results.append(FixtureResult(
            rel, pct, "PASS" if pct >= pass_at else "FAIL",
            goosie_size=(g.shape[1], g.shape[0]),
            chromium_size=(p.shape[1], p.shape[0]),
        ))

    results.sort(key=lambda r: (-r.pct, r.name))
    return results


# ---------------------------------------------------------------------------
# Diff image generation
# ---------------------------------------------------------------------------

def generate_diff_image(name: str, out_dir: Path, threshold: int) -> Path | None:
    """Produce a side-by-side diff image: goosie | chromium | highlighted diff.

    Returns the output path, or None if inputs are missing/incompatible.
    """
    gp = GOOSIE / name
    pp = CHROMIUM / name
    if not gp.exists() or not pp.exists():
        return None

    g_img = Image.open(gp).convert("RGB")
    p_img = Image.open(pp).convert("RGB")
    g = np.array(g_img)
    p = np.array(p_img)

    if g.shape != p.shape:
        return None

    diff = np.abs(g.astype(int) - p.astype(int))
    bad = ~np.all(diff <= threshold, axis=2)

    # Red-channel highlight of mismatched pixels on the goosie image.
    highlight = g.copy()
    highlight[bad] = [255, 0, 0]

    w, h = g_img.size
    gap = 4
    canvas = Image.new("RGB", (w * 3 + gap * 2, h), (40, 40, 40))
    canvas.paste(g_img, (0, 0))
    canvas.paste(p_img, (w + gap, 0))
    canvas.paste(Image.fromarray(highlight), (w * 2 + gap * 2, 0))

    draw = ImageDraw.Draw(canvas)
    try:
        font = ImageFont.load_default()
    except Exception:
        font = None
    draw.text((4, 4), "Goosie", fill=(255, 255, 255), font=font)
    draw.text((w + gap + 4, 4), "Chromium", fill=(255, 255, 255), font=font)
    draw.text((w * 2 + gap * 2 + 4, 4), "Diff", fill=(255, 255, 255), font=font)

    out_dir.mkdir(parents=True, exist_ok=True)
    out_path = out_dir / Path(name)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    canvas.save(str(out_path))
    return out_path


# ---------------------------------------------------------------------------
# Inspect (per-band analysis)
# ---------------------------------------------------------------------------

def inspect(name: str, band: int, threshold: int) -> None:
    """Print where a fixture's mismatches land: per-row-band, per-column-band.

    A run of bad bands starting at one row and never recovering is a vertical
    offset; one bad band is a single misplaced box.
    """
    gp, pp = GOOSIE / name, CHROMIUM / name
    if not gp.exists() or not pp.exists():
        print(f"{name}: render(s) missing", file=sys.stderr)
        return

    g = np.array(Image.open(gp).convert("RGB")).astype(int)
    p = np.array(Image.open(pp).convert("RGB")).astype(int)
    if g.shape != p.shape:
        print(f"{name}: dimension mismatch "
              f"goosie={g.shape[1]}x{g.shape[0]} "
              f"chromium={p.shape[1]}x{p.shape[0]}", file=sys.stderr)
        return

    bad = ~np.all(np.abs(g - p) <= threshold, axis=2)
    h, w = bad.shape
    print(f"{name}: {bad.mean() * 100:.2f}% of {w}x{h} mismatched")
    print("rows")
    for y in range(0, h - h % band, band):
        frac = bad[y:y + band].mean() * 100
        if frac > 1:
            print(f"  {y:4d}-{y + band - 1:4d} {frac:5.1f}% "
                  f"{'#' * int(frac / 2)}")
    print("cols")
    for x in range(0, w - w % band, band):
        frac = bad[:, x:x + band].mean() * 100
        if frac > 1:
            print(f"  {x:4d}-{x + band - 1:4d} {frac:5.1f}% "
                  f"{'#' * int(frac / 2)}")


# ---------------------------------------------------------------------------
# Report: JSON
# ---------------------------------------------------------------------------

def write_json_report(results: list[FixtureResult], path: Path,
                      threshold: int, pass_at: float,
                      elapsed: float) -> None:
    """Write a structured JSON report."""
    passing = [r for r in results if r.status == "PASS"]
    failing = [r for r in results if r.status == "FAIL"]
    errors = [r for r in results if r.status in ("ERROR", "MISSING")]

    scored = [r for r in results if r.status in ("PASS", "FAIL")]
    avg = sum(r.pct for r in scored) / len(scored) if scored else 0.0

    # Per-category breakdown.
    categories: dict[str, dict] = {}
    for r in results:
        cat = _category(r.name)
        if cat not in categories:
            categories[cat] = {"total": 0, "pass": 0, "fail": 0,
                               "error": 0, "sum_pct": 0.0, "scored": 0}
        categories[cat]["total"] += 1
        if r.status == "PASS":
            categories[cat]["pass"] += 1
        elif r.status == "FAIL":
            categories[cat]["fail"] += 1
        else:
            categories[cat]["error"] += 1
        if r.status in ("PASS", "FAIL"):
            categories[cat]["scored"] += 1
            categories[cat]["sum_pct"] += r.pct

    cat_summary = {}
    for cat, c in sorted(categories.items()):
        cat_summary[cat] = {
            "total": c["total"],
            "pass": c["pass"],
            "fail": c["fail"],
            "error": c["error"],
            "avg_match_pct": round(c["sum_pct"] / c["scored"], 3)
            if c["scored"] else None,
        }

    report = {
        "summary": {
            "total": len(results),
            "passing": len(passing),
            "failing": len(failing),
            "errors": len(errors),
            "avg_match_pct": round(avg, 3),
            "threshold": threshold,
            "pass_at": pass_at,
            "elapsed_seconds": round(elapsed, 2),
            "all_pass": len(failing) == 0 and len(errors) == 0,
        },
        "categories": cat_summary,
        "fixtures": [r.to_dict() for r in results],
    }

    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w") as f:
        json.dump(report, f, indent=2)


# ---------------------------------------------------------------------------
# Report: JUnit XML
# ---------------------------------------------------------------------------

def write_junit_report(results: list[FixtureResult], path: Path,
                       threshold: int, pass_at: float) -> None:
    """Write a JUnit XML report suitable for CI systems."""
    testsuites = ET.Element("testsuites")

    # Group by category.
    by_cat: dict[str, list[FixtureResult]] = {}
    for r in results:
        cat = _category(r.name)
        by_cat.setdefault(cat, []).append(r)

    total_tests = 0
    total_failures = 0
    total_errors = 0

    for cat in sorted(by_cat):
        members = by_cat[cat]
        suite = ET.SubElement(testsuites, "testsuite", {
            "name": f"parity/{cat}",
            "tests": str(len(members)),
            "failures": str(sum(1 for r in members if r.status == "FAIL")),
            "errors": str(sum(1 for r in members
                              if r.status in ("ERROR", "MISSING"))),
        })

        for r in members:
            total_tests += 1
            tc = ET.SubElement(suite, "testcase", {
                "name": r.name,
                "classname": f"parity.{cat}",
            })
            if r.status == "FAIL":
                total_failures += 1
                failure = ET.SubElement(tc, "failure", {
                    "message": f"{r.pct:.2f}% match < {pass_at}% threshold",
                    "type": "PixelMismatch",
                })
                failure.text = (
                    f"Fixture: {r.name}\n"
                    f"Match: {r.pct:.2f}%\n"
                    f"Threshold: {pass_at}%\n"
                    f"Goosie size: {r.goosie_size}\n"
                    f"Chromium size: {r.chromium_size}\n"
                )
            elif r.status in ("ERROR", "MISSING"):
                total_errors += 1
                error = ET.SubElement(tc, "error", {
                    "message": r.error or r.status,
                    "type": r.status,
                })
                error.text = (
                    f"Fixture: {r.name}\n"
                    f"Error: {r.error}\n"
                )

    testsuites.set("tests", str(total_tests))
    testsuites.set("failures", str(total_failures))
    testsuites.set("errors", str(total_errors))

    path.parent.mkdir(parents=True, exist_ok=True)
    tree = ET.ElementTree(testsuites)
    ET.indent(tree, space="  ")
    tree.write(str(path), encoding="unicode", xml_declaration=True)


# ---------------------------------------------------------------------------
# Console output
# ---------------------------------------------------------------------------

def print_summary(results: list[FixtureResult], threshold: int,
                  pass_at: float) -> None:
    """Print a human-readable summary to stdout."""
    if not results:
        print("no renders found", file=sys.stderr)
        return

    # Find the longest fixture name for alignment.
    max_name = max(len(r.name) for r in results)

    for r in results:
        if r.status in ("PASS", "FAIL"):
            print(f"  {r.pct:6.2f}%  {r.status:4s}  {r.name}")
        else:
            print(f"  {'---':>6s}  {r.status:4s}  {r.name}  "
                  f"({r.error})")

    passing = sum(1 for r in results if r.status == "PASS")
    failing = sum(1 for r in results if r.status == "FAIL")
    errored = sum(1 for r in results if r.status in ("ERROR", "MISSING"))
    scored = [r for r in results if r.status in ("PASS", "FAIL")]
    avg = sum(r.pct for r in scored) / len(scored) if scored else 0.0

    print()
    print(f"  {passing}/{len(results)} passing  "
          f"({passing / len(results) * 100:.1f}%), "
          f"{failing} failing, {errored} errors")
    print(f"  average match: {avg:.2f}%  "
          f"(threshold={threshold}, pass_at={pass_at}%)")

    if failing > 0:
        print()
        print("  Failing fixtures:")
        for r in results:
            if r.status == "FAIL":
                print(f"    {r.name}  ({r.pct:.2f}%)")

    if errored > 0:
        print()
        print("  Error fixtures:")
        for r in results:
            if r.status in ("ERROR", "MISSING"):
                print(f"    {r.name}  ({r.error})")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    ap = argparse.ArgumentParser(
        description="Score Goosie renders against Chromium reference renders.")
    ap.add_argument("--render", action="store_true",
                    help="re-render fixtures with goosie before scoring")
    ap.add_argument("--render-chromium", action="store_true",
                    help="re-render fixtures with Playwright/Chromium")
    ap.add_argument("--binary", default="./goosie",
                    help="path to goosie binary (default: ./goosie)")
    ap.add_argument("--width", type=int, default=800)
    ap.add_argument("--height", type=int, default=600)
    ap.add_argument("--threshold", type=int, default=THRESHOLD,
                    help=f"per-channel pixel tolerance (default {THRESHOLD})")
    ap.add_argument("--pass-at", type=float, default=PASS_AT,
                    help=f"pass percentage (default {PASS_AT})")
    ap.add_argument("--strict", action="store_true", default=True,
                    help="fail on missing pairs and dimension mismatches "
                         "(default: True)")
    ap.add_argument("--no-strict", action="store_false", dest="strict",
                    help="skip missing pairs instead of reporting errors")
    ap.add_argument("--json", metavar="FILE",
                    help="write JSON report to FILE")
    ap.add_argument("--junit", metavar="FILE",
                    help="write JUnit XML report to FILE")
    ap.add_argument("--diffs", metavar="DIR",
                    help="write diff images to DIR")
    ap.add_argument("--inspect", metavar="REL.png",
                    help="print per-band mismatch for one fixture")
    ap.add_argument("--band", type=int, default=32,
                    help="band size for --inspect (default 32)")
    args = ap.parse_args()

    # --inspect is a standalone mode.
    if args.inspect:
        inspect(args.inspect, args.band, args.threshold)
        return 0

    t0 = time.monotonic()

    # Render steps.  Each raises RenderError on total failure.
    if args.render:
        try:
            render_goosie(args.binary, args.width, args.height)
        except RenderError as exc:
            print(f"ERROR: goosie render failed: {exc}", file=sys.stderr)
            return 1

    if args.render_chromium:
        try:
            render_chromium(args.width, args.height)
        except RenderError as exc:
            print(f"ERROR: Chromium render failed: {exc}", file=sys.stderr)
            return 1

    # Score.
    results = score_fixtures(args.threshold, args.pass_at,
                             strict=args.strict)
    elapsed = time.monotonic() - t0

    if not results:
        print("no renders found -- run with --render and/or "
              "--render-chromium first", file=sys.stderr)
        return 2

    # Diff images.
    if args.diffs:
        diff_dir = Path(args.diffs)
        diff_count = 0
        for r in results:
            if r.status in ("FAIL", "ERROR"):
                p = generate_diff_image(r.name, diff_dir, args.threshold)
                if p:
                    diff_count += 1
        if diff_count:
            print(f"  wrote {diff_count} diff image(s) to {args.diffs}/")

    # Reports.
    if args.json:
        write_json_report(results, Path(args.json), args.threshold,
                          args.pass_at, elapsed)
        print(f"  JSON report: {args.json}")

    if args.junit:
        write_junit_report(results, Path(args.junit), args.threshold,
                           args.pass_at)
        print(f"  JUnit report: {args.junit}")

    # Console summary.
    print_summary(results, args.threshold, args.pass_at)

    # Exit code: non-zero if anything failed or errored.
    failing = sum(1 for r in results if r.status != "PASS")
    return 1 if failing > 0 else 0


if __name__ == "__main__":
    sys.exit(main())
