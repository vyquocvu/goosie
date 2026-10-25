#!/usr/bin/env python3
"""Score Goosie's renders against the Playwright (Chromium) reference renders.

A pixel matches when every RGB channel differs by at most THRESHOLD; a fixture
passes when at least PASS_AT percent of pixels match and it has no new blank
content regions. Region similarity metrics are reported but are not gates.

Modes:
    --render          Re-render fixtures with the local goosie binary first.
    --json FILE       Write a JSON report to FILE.
    --junit FILE      Write a JUnit XML report to FILE.
    --diffs DIR       Write per-fixture diff images to DIR.
    --threshold N     Per-channel tolerance (default 30).
    --pass-at N       Pass percentage (default 90.0).
    --inspect NAME    Print per-band mismatch for one fixture.

This oracle is deliberately impossible to fool (GOOSIE-PARITYGATE-001):
    * A missing goosie/chromium render is ALWAYS reported as MISSING; there is
      no --no-strict escape hatch that shrinks the denominator. --strict is
      still accepted for call-site compatibility but enables nothing.
    * A fixture passes its known-gaps baseline only when the measured blank
      band count EQUALS the declared count, so inflating the baseline fails.
    * A corpus that has shrunk below MIN_SCORED_FIXTURES is treated as broken,
      not as a clean run.

Exit codes:
    0  All scored fixtures passed the threshold and the corpus is at full size.
    1  One or more fixtures failed, or a render step failed (fail-closed).
    2  Bad invocation (missing binary, missing fixtures, corpus below the
       MIN_SCORED_FIXTURES floor, etc.).
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
# Floor on the number of scored fixtures.  Equals the count of
# testdata/render/**/*.html as of the last time the corpus grew; it may only
# rise, and raising it by 1 in the same change that adds a fixture is how that
# gain gets locked in.  A scored corpus below this number means fixtures
# silently disappeared, so the run is reported as broken (exit 2) rather than
# as a healthy 100%.  test/gatecheck/parity_harness_test.go counts the real HTML
# files and fails if the count falls below this exact line, so the two cannot
# drift apart - and adding a fixture must never red that test by itself.
# 85: css-features/11-digit-leading-custom-property.html (TASK-040,
# GOOSIE-LIVE-002) - a legal --8e8e8e name that used to cost the whole render.
# 87: css-features/13-anonymous-group-opacity.html (TASK-044, GOOSIE-VIS-001)
# - group opacity charged once per element, not once per generated box.
# 88: css-features/14-inline-background-repaint.html (TASK-048,
# GOOSIE-INLINEBG-001) - a translucent block background composited a second
# time inside every word box of the text it carries.
MIN_SCORED_FIXTURES = 88
FIXTURES = Path("testdata/render")
GOOSIE = Path("/tmp/goosie-renders")
CHROMIUM = Path("/tmp/playwright-renders")
KNOWN_GAPS = Path("testdata/render-known-gaps.json")
MIN_INK = 200
INK_CUTOFF = 245
BAND_GAP = 6
MIN_BAND_HEIGHT = 6


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


def render_chromium(width: int = 800, height: int = 600) -> tuple[list[str], str | None]:
    """Render every fixture with Playwright/Chromium.  Returns the list of
    relative PNG paths and the browser version string.
    Raises RenderError if playwright is unavailable."""
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        raise RenderError(
            "playwright is not installed; run 'pip install playwright && "
            "playwright install chromium' to generate reference renders"
        )

    produced: list[str] = []
    errors: list[str] = []
    browser_version: str | None = None

    try:
        with sync_playwright() as pw:
            browser = pw.chromium.launch()
            browser_version = browser.version
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

    return produced, browser_version


# ---------------------------------------------------------------------------
# Scoring
# ---------------------------------------------------------------------------

class FixtureResult:
    """One fixture's comparison outcome."""

    def __init__(self, name: str, pct: float, status: str,
                 goosie_size: tuple[int, int] | None = None,
                 chromium_size: tuple[int, int] | None = None,
                 error: str | None = None,
                 worst_region_iou: float | None = None,
                 coverage_ratio: float | None = None,
                 blank_region_rows: list[tuple[int, int]] | None = None,
                 known_blank_regions: int = 0,
                 truly_blank_region_rows: list[tuple[int, int]] | None = None,
                 shifted_region_rows: list[tuple[int, int]] | None = None):
        self.name = name
        self.pct = pct
        self.status = status          # "PASS", "FAIL", "ERROR", "MISSING"
        self.goosie_size = goosie_size
        self.chromium_size = chromium_size
        self.error = error
        self.worst_region_iou = worst_region_iou
        self.coverage_ratio = coverage_ratio
        self.blank_region_rows = blank_region_rows or []
        self.known_blank_regions = known_blank_regions
        self.blank_regions = len(self.blank_region_rows)
        # GOOSIE-BLANKMETRIC-001 drift report ONLY: a further split of the
        # unchanged blank_region_rows above.  These never feed the gate or the
        # match %; blank_regions/known_blank_regions keep their old meaning.
        self.truly_blank_region_rows = truly_blank_region_rows or []
        self.shifted_region_rows = shifted_region_rows or []
        self.truly_blank_regions = len(self.truly_blank_region_rows)
        self.shifted_regions = len(self.shifted_region_rows)

    def to_dict(self) -> dict:
        d: dict = {
            "fixture": self.name,
            "match_pct": round(self.pct, 3),
            "status": self.status,
            "worst_region_iou": (round(self.worst_region_iou, 4)
                                 if self.worst_region_iou is not None else None),
            "coverage_ratio": (round(self.coverage_ratio, 4)
                               if self.coverage_ratio is not None else None),
            "blank_regions": self.blank_regions,
            "blank_region_rows": [list(rows) for rows in self.blank_region_rows],
            "known_blank_regions": self.known_blank_regions,
            "truly_blank_regions": self.truly_blank_regions,
            "shifted_regions": self.shifted_regions,
            "shifted_region_rows": [list(rows) for rows in self.shifted_region_rows],
        }
        if self.goosie_size:
            d["goosie_size"] = list(self.goosie_size)
        if self.chromium_size:
            d["chromium_size"] = list(self.chromium_size)
        if self.error:
            d["error"] = self.error
        return d


def ink_mask(rgb: np.ndarray) -> np.ndarray:
    """Count visible pixels away from a near-white page background as ink.

    A modest cutoff includes antialiased glyph edges; MIN_INK filters small
    marks so the gate stays focused on substantial content regions.
    """
    return np.min(rgb, axis=2) < INK_CUTOFF


def content_bands(chromium_ink: np.ndarray) -> list[tuple[int, int, int, int, int]]:
    """Return (y0, y1 inclusive, x0, x1 exclusive, ink count) content bands.

    Adjacent ink rows separated by at most BAND_GAP empty rows form one band.
    Horizontal bounds follow Chromium's ink extent so blank page margins do
    not affect region measurements.
    """
    occupied = np.flatnonzero(chromium_ink.any(axis=1))
    if not len(occupied):
        return []
    groups: list[tuple[int, int]] = []
    start = previous = int(occupied[0])
    for row in occupied[1:]:
        row = int(row)
        if row - previous > BAND_GAP + 1:
            groups.append((start, previous))
            start = row
        previous = row
    groups.append((start, previous))

    bands = []
    for y0, y1 in groups:
        if y1 - y0 + 1 < MIN_BAND_HEIGHT:
            continue
        xs = np.flatnonzero(chromium_ink[y0:y1 + 1].any(axis=0))
        x0, x1 = int(xs[0]), int(xs[-1]) + 1
        count = int(chromium_ink[y0:y1 + 1, x0:x1].sum())
        if count >= MIN_INK:
            bands.append((y0, y1, x0, x1, count))
    return bands


def _dilate_one(mask: np.ndarray) -> np.ndarray:
    padded = np.pad(mask, 1)
    h, w = mask.shape
    dilated = np.zeros_like(mask)
    for dy in range(3):
        for dx in range(3):
            dilated |= padded[dy:dy + h, dx:dx + w]
    return dilated


def score_regions(goosie: np.ndarray, chromium: np.ndarray
                  ) -> tuple[float | None, float | None, list[tuple[int, int]]]:
    """Measure tolerant ink overlap and identify truly empty reference bands.

    The gate is deliberately only an entirely blank Goosie band: normal glyph
    rasterization differences make similarity scores noisy across browsers.
    """
    gm, cm = ink_mask(goosie), ink_mask(chromium)
    bands = content_bands(cm)
    if not bands:
        return None, None, []
    ious: list[float] = []
    chromium_total = goosie_total = 0
    blanks = []
    for y0, y1, x0, x1, chromium_count in bands:
        a, b = cm[y0:y1 + 1, x0:x1], gm[y0:y1 + 1, x0:x1]
        goosie_count = int(b.sum())
        chromium_total += chromium_count
        goosie_total += goosie_count
        if goosie_count == 0:
            blanks.append((y0, y1))
        denominator = chromium_count + goosie_count
        if denominator:
            overlap = int(np.logical_and(a, _dilate_one(b)).sum())
            overlap += int(np.logical_and(b, _dilate_one(a)).sum())
            ious.append(overlap / denominator)
    return min(ious) if ious else None, goosie_total / chromium_total, blanks


def score_regions_drift(goosie: np.ndarray, chromium: np.ndarray
                        ) -> tuple[list[tuple[int, int]],
                                   list[tuple[int, int]],
                                   list[tuple[int, int]]]:
    """Drift-adjusted second view of the SAME blank-band test (GOOSIE-BLANKMETRIC-001).

    score_regions calls a reference band blank when Goosie has zero ink inside
    Chromium's exact rows, with no vertical tolerance -- while its IoU sibling
    dilates by one row.  A line painted a few rows away therefore reads as
    "missing content".  This function adds the missing tolerance WITHOUT
    touching score_regions' meaning; nothing here is a gate, it is a report.

    For every band score_regions would already have flagged blank, a drift
    window grows by the band's OWN height on each side over the band's own
    columns and the band is classified as:

      shifted     -- no ink in Chromium's exact rows, but at least MIN_INK ink
                     inside the +/-band_height window (content is present, only
                     displaced), or
      truly_blank -- no ink in the exact rows AND nothing substantial in the
                     drift window (the content is genuinely missing).

    Design constraints (docs/tasks/TASK-042-BLANK-BAND-DRIFT.md §2.3): the
    window half-width is ``y1 - y0 + 1``, the band height content_bands already
    returns, so it scales to each region's own size instead of a new tuned
    constant; the substantiality bar is the existing MIN_INK, which is what this
    harness already uses to decide "this is real content" (see ink_mask's
    docstring).  No threshold -- PASS_AT, THRESHOLD, MIN_INK,
    MIN_SCORED_FIXTURES, LIVE_COVERAGE_OK -- is moved.

    The two returned drift sets partition score_regions' blank list exactly and
    disjointly: ``blank_exact == truly_blank + shifted``.  blank_exact is
    returned as the first element -- byte-identical to score_regions' third
    value -- so a caller can prove the old signal was never redefined.
    """
    gm, cm = ink_mask(goosie), ink_mask(chromium)
    bands = content_bands(cm)
    blank_exact: list[tuple[int, int]] = []
    truly_blank: list[tuple[int, int]] = []
    shifted: list[tuple[int, int]] = []
    height = gm.shape[0]
    for y0, y1, x0, x1, _ in bands:
        if int(gm[y0:y1 + 1, x0:x1].sum()) != 0:
            continue  # ink present in Chromium's exact rows: not blank at all
        blank_exact.append((y0, y1))
        band_height = y1 - y0 + 1
        lo = max(0, y0 - band_height)
        hi = min(height - 1, y1 + band_height)
        window_ink = int(gm[lo:hi + 1, x0:x1].sum())
        if window_ink >= MIN_INK:
            shifted.append((y0, y1))
        else:
            truly_blank.append((y0, y1))
    return blank_exact, truly_blank, shifted


def load_known_gaps(path: Path = KNOWN_GAPS) -> dict[str, int]:
    """Load fixture -> allowed existing blank band count baseline."""
    with path.open() as f:
        data = json.load(f)
    return {entry["fixture"]: int(entry["blank_regions"])
            for entry in data["known_gaps"]}


def _category(name: str) -> str:
    """Extract the category from a fixture path like 'typography/01-font.png'."""
    parts = Path(name).parts
    return parts[0] if len(parts) > 1 else "uncategorized"


def score_fixtures(threshold: int, pass_at: float,
                   known_gaps: dict[str, int] | None = None) -> list[FixtureResult]:
    """Compare every goosie render to its Chromium counterpart.

    Strictness is not optional: a missing render, a dimension mismatch, and a
    known-gaps baseline that no longer matches the measured blank band count
    each produce a row and a non-PASS status, so the harness can never report
    success on shrinking evidence.
    """
    results: list[FixtureResult] = []
    known_gaps = load_known_gaps() if known_gaps is None else known_gaps

    for html in sorted(FIXTURES.rglob("*.html")):
        rel = str(html.relative_to(FIXTURES).with_suffix(".png"))
        gp = GOOSIE / rel
        pp = CHROMIUM / rel

        if not gp.exists() and not pp.exists():
            results.append(FixtureResult(
                rel, 0.0, "MISSING",
                error="neither goosie nor chromium render exists"))
            continue

        if not gp.exists():
            results.append(FixtureResult(
                rel, 0.0, "MISSING",
                error="goosie render missing"))
            continue

        if not pp.exists():
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

        worst_iou, coverage, blank_rows = score_regions(g, p)
        _, truly_blank_rows, shifted_rows = score_regions_drift(g, p)
        allowed_blanks = known_gaps.get(rel, 0)
        error = None
        if pct < pass_at:
            status = "FAIL"
        elif len(blank_rows) != allowed_blanks:
            # The known-gaps baseline is an exact ratchet, not an upper bound:
            # inflating it to hide a band, or fixing a band without shrinking
            # the baseline file, both land here.
            status = "FAIL"
            error = (f"known-gap baseline is stale: declared {allowed_blanks}, "
                     f"measured {len(blank_rows)}")
        else:
            status = "PASS"
        results.append(FixtureResult(
            rel, pct, status,
            goosie_size=(g.shape[1], g.shape[0]),
            chromium_size=(p.shape[1], p.shape[0]),
            error=error,
            worst_region_iou=worst_iou,
            coverage_ratio=coverage,
            blank_region_rows=blank_rows,
            known_blank_regions=allowed_blanks,
            truly_blank_region_rows=truly_blank_rows,
            shifted_region_rows=shifted_rows,
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
                      elapsed: float,
                      metadata: dict | None = None) -> None:
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
        "metadata": metadata or {},
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
            "known_gap_fixtures": sum(1 for r in results
                                       if r.known_blank_regions > 0),
            "known_gap_bands": sum(r.blank_regions for r in results
                                    if r.known_blank_regions > 0),
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
            ET.SubElement(tc, "properties")
            props = tc.find("properties")
            assert props is not None
            for key, value in (
                ("match_pct", f"{r.pct:.3f}"),
                ("worst_region_iou", "" if r.worst_region_iou is None
                 else f"{r.worst_region_iou:.4f}"),
                ("coverage_ratio", "" if r.coverage_ratio is None
                 else f"{r.coverage_ratio:.4f}"),
                ("blank_regions", str(r.blank_regions)),
                ("blank_region_rows", json.dumps(r.blank_region_rows)),
                ("known_blank_regions", str(r.known_blank_regions)),
                ("truly_blank_regions", str(r.truly_blank_regions)),
                ("shifted_regions", str(r.shifted_regions)),
            ):
                ET.SubElement(props, "property", {"name": key, "value": value})
            if r.status == "FAIL":
                total_failures += 1
                failure = ET.SubElement(tc, "failure", {
                    "message": (f"match={r.pct:.2f}% blank_regions="
                                f"{r.blank_regions} allowed={r.known_blank_regions}"),
                    "type": "ParityMismatch",
                })
                failure.text = (
                    f"Fixture: {r.name}\n"
                    f"Match: {r.pct:.2f}%\n"
                    f"Threshold: {pass_at}%\n"
                    f"Goosie size: {r.goosie_size}\n"
                    f"Chromium size: {r.chromium_size}\n"
                    f"Blank regions: {r.blank_regions} at {r.blank_region_rows}\n"
                    f"Known blank regions allowed: {r.known_blank_regions}\n"
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
            print(f"  {r.pct:6.2f}%  {r.status:4s}  {r.name}"
                  f"  worst_region_iou={r.worst_region_iou if r.worst_region_iou is not None else 'n/a'}"
                  f"  coverage_ratio={r.coverage_ratio if r.coverage_ratio is not None else 'n/a'}"
                  f"  blank_regions={r.blank_regions}/{r.known_blank_regions}"
                  f"  rows={r.blank_region_rows}"
                  f"  shifted={r.shifted_regions}")
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
    known = [r for r in results if r.known_blank_regions > 0]
    known_bands = sum(r.blank_regions for r in known)
    print(f"  known gaps: {len(known)} fixtures / {known_bands} bands")
    drift_shifted = sum(r.shifted_regions for r in known)
    drift_blank = sum(r.truly_blank_regions for r in known)
    print(f"  drift-adjusted (GOOSIE-BLANKMETRIC-001): "
          f"of those {known_bands} blank bands, {drift_shifted} have ink in a "
          f"+/-band-height window (shifted) and {drift_blank} stay blank "
          f"(truly missing)")
    for r in known:
        print(f"  WARNING known gap: {r.name}: {r.blank_regions} blank band(s), "
              f"baseline {r.known_blank_regions}; rows={r.blank_region_rows}"
              f" [truly_blank={r.truly_blank_regions} "
              f"shifted={r.shifted_regions} shifted_rows={r.shifted_region_rows}]")

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
                    help="accepted for compatibility, strictness is not "
                         "optional: missing renders and dimension mismatches "
                         "are always reported (this flag enables nothing)")
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

    browser_version: str | None = None

    # Render steps.  Each raises RenderError on total failure.
    if args.render:
        try:
            render_goosie(args.binary, args.width, args.height)
        except RenderError as exc:
            print(f"ERROR: goosie render failed: {exc}", file=sys.stderr)
            return 1

    if args.render_chromium:
        try:
            _, browser_version = render_chromium(args.width, args.height)
        except RenderError as exc:
            print(f"ERROR: Chromium render failed: {exc}", file=sys.stderr)
            return 1

    # Score.
    results = score_fixtures(args.threshold, args.pass_at)
    elapsed = time.monotonic() - t0

    if len(results) < MIN_SCORED_FIXTURES:
        print(f"parity: only {len(results)} fixture(s) scored — floor is "
              f"{MIN_SCORED_FIXTURES}. The render corpus shrank or fixtures "
              f"are missing, so this run cannot be trusted; re-run with "
              f"--render and/or --render-chromium.", file=sys.stderr)
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
    metadata = {
        "browser_version": browser_version,
        "python_version": sys.version,
        "platform": sys.platform,
    }
    if args.json:
        write_json_report(results, Path(args.json), args.threshold,
                          args.pass_at, elapsed, metadata=metadata)
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
