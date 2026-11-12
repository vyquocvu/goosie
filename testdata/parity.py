#!/usr/bin/env python3
"""Score Goosie's renders against the Playwright (Chromium) reference renders.

A pixel matches when every RGB channel differs by at most THRESHOLD; a fixture
passes when at least PASS_AT percent of pixels match and it has no new blank
content regions. Region similarity metrics are reported but are not gates.

Modes:
    --render          Re-render fixtures with the local goosie binary first.
    --workdir DIR     Render into and score from DIR/goosie + DIR/chromium, so
                      concurrent runs on one host cannot read each other's PNGs.
    --isolate         Like --workdir, with a fresh mkdtemp directory.
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
    * --render deletes each fixture's existing PNG before invoking the binary
      (GOOSIE-PARITYTMP-001), so a build that stops rendering a fixture can
      never be scored against the previous build's image of it.

Exit codes:
    0  All scored fixtures passed the threshold and the corpus is at full size.
    1  One or more fixtures failed, or a render step failed (fail-closed).
    2  Bad invocation (missing binary, missing fixtures, corpus below the
       MIN_SCORED_FIXTURES floor, etc.).
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import sys
import tempfile
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
# 89: css-features/15-anonymous-block-gradient-once.html (TASK-049,
# GOOSIE-ANONBG-001) - the same double composite one layer up: an anonymous block
# re-composited its generating element's gradient ramp across both bands it splits.
# 90: js-dom/02-tree-mutation-and-lifecycle.html (TASK-047) - a jQuery page's node
# tree and window lifecycle, the bootstrap DOM the engine could not answer before.
# 91: css-features/16-url-paren-data-image.html (TASK-050, GOOSIE-CSSURLPAREN-001)
# - a quoted url() payload containing ')' survives whole at all six readers.
# 92: css-features/17-anonymous-block-shadow-once.html (TASK-053, GOOSIE-SHADOWRUN-001) - the ring belonged
# to the element, not to the two anonymous bands it generated.
# 93: js-dom/03-intersection-observer-first-reflow.html (TASK-054, GOOSIE-IOEXPORT-001)
# - registering an IntersectionObserver used to wedge the load; the gate is reaching a first frame.
# 105: SP-2 (2026-11-12) locks the full corpus; the floor had 12 fixtures of
# un-ratcheted slack (93 vs 105 scored).
# 106: borders/04-inline-fragment.html (SP-7, GOOSIE-INLINEBORDER-001) - a
# bordered inline span wraps over a translucent block; its border fragments
# per line instead of vanishing behind the non-inline paint guard.
# 107: css-features/18-group-opacity-overlap.html (SP-8, GOOSIE-COMPOSITE-001) -
# two translucent boxes overlapping inside one opacity:0.5 group composite against
# each other and fade once, not twice, against the tile.
# 108: css-features/19-attr-selector-escaped-value.html (GOOSIE-CSSSELECT-001) -
# a quoted attribute-selector value is a string, so `[data-x="a\"b"]` names `a"b`;
# the band only paints once the escapes decode and ']' and ',' stay inside the
# value instead of ending the condition or splitting the list.
# 109: css-features/20-url-escaped-quote-data-image.html (GOOSIE-CSSURLESCAPE-001) -
# a url() payload is a string too, so `url("…xmlns=\"…\"…")` denotes a URI whose
# quotes are plain; the blue band only paints once the escapes decode before the
# image loader sees the payload.
# 110: layout-features/07-atomic-line-strut.html (GOOSIE-INLINESTRUT-001) - a line
# of nothing but inline-blocks and images still owes the block's font a strut, and
# that strut reaches below the baseline; the green and rust bands sit under nine
# such rows, so a line box sized only from the tallest margin box drops them ~30px.
# 111: layout-features/08-vertical-align-classes.html (GOOSIE-INLINEBASELINE-001) -
# every `vertical-align` class on the same 4px chip over the same 16px strut, once
# alone on its line and once after a word, each row followed by a solid band; the
# classes move the baseline or stretch the line, so a placement that ignores them
# slides every band below the row it follows.
# 112: layout-features/09-text-run-align.html (GOOSIE-VALIGNTEXT-001) - the same
# classes on a run of *text*, where the box an edge value pins is the run's own
# line height rather than a margin box: a 64px run with `vertical-align: bottom`
# takes the line's baseline 24px down with it, and a 64px run under
# `line-height: 10px` starts its ink 32px above the line it belongs to.
# 113: forms/09-control-baseline.html (GOOSIE-CONTROLBASELINE-001) - checkbox,
# radio and range align on their own bottom BORDER edge and carry UA margins, so
# a 13px box asks 16px above the baseline and every band under a row of controls
# rides 2 to 13px lower than a strut-only line gives it; the fixture also stacks
# the controls that align on an interior baseline instead - a text field, a
# button and a select - so a rule that reaches past the widgets moves a band this
# corpus compares.
# 114: layout-features/10-middle-script-align.html (GOOSIE-VALIGNMIDDLE-001,
# GOOSIE-VALIGNSUB-001) - the two values that resolve against the containing font
# rather than against an edge of the line: `middle` puts a box's centre half the
# parent's x-height above the baseline, and `super`/`sub` shift a box's own
# baseline by the parent's script numbers, so the line grows instead of the box
# leaving it. Chips, runs of text and three different parent sizes are all in the
# rows, each followed by a solid band, and every one of those bands sat 6 to 30px
# somewhere else before the rules were measured.
MIN_SCORED_FIXTURES = 114
FIXTURES = Path("testdata/render")
GOOSIE = Path("/tmp/goosie-renders")
CHROMIUM = Path("/tmp/playwright-renders")
KNOWN_GAPS = Path("testdata/render-known-gaps.json")
ORACLE_PINS = Path("testdata/oracle-pins.json")
MIN_INK = 200
INK_CUTOFF = 245
BAND_GAP = 6
MIN_BAND_HEIGHT = 6


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------

class RenderError(Exception):
    """A render step produced an error that must not be silently skipped."""


def binary_sha256(binary: str) -> str | None:
    """Digest of the binary this run renders with, or None if it cannot be
    read.  Provenance must never be the reason a run fails (GOOSIE-
    PARITYTMP-001): which build produced a PNG belongs in the report, not in a
    gate."""
    try:
        return hashlib.sha256(Path(binary).read_bytes()).hexdigest()
    except OSError:
        return None


def resolve_dirs(workdir: str | None, isolate: bool) -> tuple[Path, Path]:
    """The (goosie, chromium) render roots this run reads and writes.

    Both sides move together: a run that renders goosie into a private
    directory must not score those PNGs against whatever Chromium reference
    happens to sit in the shared one (GOOSIE-PARITYTMP-001).  With neither
    flag the machine-global defaults are kept, which is what scripts/run-parity.sh
    relies on - it scores against Chromium renders generated once per host and
    never passes --render-chromium.
    """
    if isolate:
        root = Path(tempfile.mkdtemp(prefix="goosie-parity-"))
    elif workdir:
        root = Path(workdir)
    else:
        return GOOSIE, CHROMIUM
    return root / "goosie", root / "chromium"


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
        # Anything already here is another run's evidence, and this binary may
        # not produce a replacement at all: keep it and a render failure scores
        # the previous build's PNG as if it were this one's output.
        out.unlink(missing_ok=True)

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
                # Same rule as the goosie side: a fixture whose screenshot
                # fails has to come back MISSING, not be scored against an
                # older run's reference image.
                out.unlink(missing_ok=True)

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
                 shifted_region_rows: list[tuple[int, int]] | None = None,
                 sources: tuple[str, str] | None = None):
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
        # The two files this verdict was measured from, as the harness resolved
        # them at scoring time (GOOSIE-PARITYTMP-001).
        self.sources = sources

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
        if self.sources:
            d["goosie_render"], d["chromium_render"] = self.sources
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


def known_gap_provenance() -> str | None:
    """The platform the declared blank-band counts were measured on.

    A blank band is partly a host fact: font rasterisation differs per OS, so
    the counts in render-known-gaps.json describe the machine that produced
    them, not the engine (GOOSIE-CIPARITY-001).  Recording that machine in the
    file turns "the baseline is stale" on a second platform from an unexplained
    verdict into a named condition.  None means the file records no host, which
    is reported rather than assumed to be this one.

    Reads the module global at call time, so a caller (or the selftest) that
    repoints KNOWN_GAPS is answered about the file it actually points at.
    """
    with KNOWN_GAPS.open() as f:
        return json.load(f).get("measured_on")


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
        sources = (str(gp), str(pp))

        if not gp.exists() and not pp.exists():
            results.append(FixtureResult(
                rel, 0.0, "MISSING", sources=sources,
                error="neither goosie nor chromium render exists"))
            continue

        if not gp.exists():
            results.append(FixtureResult(
                rel, 0.0, "MISSING", sources=sources,
                error="goosie render missing"))
            continue

        if not pp.exists():
            results.append(FixtureResult(
                rel, 0.0, "MISSING", sources=sources,
                error="chromium render missing"))
            continue

        g_img = Image.open(gp).convert("RGB")
        p_img = Image.open(pp).convert("RGB")
        g = np.array(g_img)
        p = np.array(p_img)

        if g.shape != p.shape:
            results.append(FixtureResult(
                rel, 0.0, "ERROR", sources=sources,
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
            rel, pct, status, sources=sources,
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
# Oracle pin
# ---------------------------------------------------------------------------

def load_oracle_pins() -> dict:
    with ORACLE_PINS.open() as f:
        return json.load(f)


def check_oracle_version(installed: str, pins: dict) -> str | None:
    """Return an error message when the installed playwright is not the
    pinned render oracle, else None. The parity baseline is only meaningful
    against the Chromium build this playwright ships."""
    want = pins["render"]["playwright"]
    if installed != want:
        return (f"oracle mismatch: installed playwright {installed} != pinned "
                f"{want} (testdata/oracle-pins.json); install with "
                f"pip install -r testdata/requirements.txt")
    return None


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
    ap.add_argument("--workdir", metavar="DIR",
                    help="render into and score from DIR/goosie and "
                         "DIR/chromium instead of the machine-global /tmp "
                         "roots, so two runs on one host cannot read each "
                         "other's PNGs (GOOSIE-PARITYTMP-001)")
    ap.add_argument("--isolate", action="store_true",
                    help="like --workdir with a fresh mkdtemp root; combine "
                         "with --render and --render-chromium, since an empty "
                         "root has no Chromium reference to score against")
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

    if args.workdir and args.isolate:
        print("parity: --workdir and --isolate are alternatives; pass one.",
              file=sys.stderr)
        return 2

    # Publish this run's render roots before anything reads or writes them.
    # These are the globals score_fixtures/render_*/inspect read, so the
    # `global` declaration is what makes --workdir mean anything at all.
    # With neither flag resolve_dirs hands back the current globals, so an
    # embedding caller (testdata/parity_selftest.py) keeps its own corpus.
    global GOOSIE, CHROMIUM
    GOOSIE, CHROMIUM = resolve_dirs(args.workdir, args.isolate)
    if args.workdir or args.isolate:
        print(f"parity: per-run render corpus rooted at {GOOSIE.parent} "
              f"({GOOSIE}/ + {CHROMIUM}/); delete it when you are done with "
              f"this evidence.", file=sys.stderr)
    elif args.render or args.render_chromium:
        print(f"parity: NOTE --render writes the machine-global roots "
              f"{GOOSIE} and {CHROMIUM}. Another run on this host reads those "
              f"PNGs as its own evidence; use --workdir DIR or --isolate for a "
              f"per-run corpus (GOOSIE-PARITYTMP-001).", file=sys.stderr)

    # --inspect is a standalone mode.
    if args.inspect:
        inspect(args.inspect, args.band, args.threshold)
        return 0

    # Which host declared these blank bands?  The counts are partly a font
    # rasterisation fact, so borrowing them across platforms is a claim about
    # an unmeasured machine (GOOSIE-CIPARITY-001).  This only names the
    # condition; the verdict still comes from the equality ratchet below.
    baseline_host = known_gap_provenance()
    if baseline_host != sys.platform:
        print(f"parity: NOTE the known-gaps baseline records bands measured on "
              f"{baseline_host or 'an unrecorded platform'}, this host is "
              f"{sys.platform}. A 'known-gap baseline is stale' FAIL here may be "
              f"a HOST difference, not an engine regression; the answer is a "
              f"baseline measured on this host, never an edit made here to match "
              f"the verdict (GOOSIE-CIPARITY-001).", file=sys.stderr)

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
        from importlib.metadata import version as _pkg_version
        err = check_oracle_version(_pkg_version("playwright"), load_oracle_pins())
        if err:
            print(f"ERROR: {err}", file=sys.stderr)
            return 2
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
        # Which build and which directory this verdict came from.  Without
        # these a green report is one HOST's evidence, not one build's
        # (GOOSIE-PARITYTMP-001), and per-fixture paths in the report are
        # meaningless because the root they hang off is unknown.
        "render_provenance": {
            "binary": str(Path(args.binary).resolve()),
            "binary_sha256": binary_sha256(args.binary),
            "goosie_dir": str(GOOSIE),
            "chromium_dir": str(CHROMIUM),
            "per_run_corpus": bool(args.workdir or args.isolate),
        },
        # The blank-band equality ratchet compares this run against counts
        # some other machine measured.  An archived report has to say which,
        # or a green Linux run is indistinguishable from a green darwin run
        # scored against darwin's baseline (GOOSIE-CIPARITY-001).
        "known_gaps_provenance": {
            "file": str(KNOWN_GAPS),
            "measured_on": baseline_host,
            "host_platform": sys.platform,
            "matches_host": baseline_host == sys.platform,
        },
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
