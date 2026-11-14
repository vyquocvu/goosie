#!/usr/bin/env python3
"""Live frame stability: does a page stay drawn, measured one region at a time.

QA-002 (docs/qa-test-plan.md) asks a question a screenshot cannot answer: open
the page once, then look at the same viewport again a second later, five times,
and say whether anything moved. `docs/qa-test-plan.md` QA-002 forbids closing that
question on PNG byte hashes, because a hash says only "these bytes differ" and
says nothing about where, how much, or whether the engine thought the frame was
finished when it took the shot.

So this tool captures the same timed sequence from both engines at each scroll
position - once from Goosie through `-screenshot-seq`, which also records what
the frame path said about itself at each capture, and once from Chromium at the
offset Goosie reported - and scores pixels:

* the **stability** axis compares each engine's own consecutive frames inside a
  fixed set of content regions, so "the page flickered" becomes "band y=318..362
  changed 1.812 pixels between captures 2 and 3";
* the **parity** axis compares the two engines capture by capture with
  `testdata/parity.py`'s own region functions, so "what a live page should look
  like" has exactly one definition in this repo.

QA-002's second half asks for the same five captures one screen further down, so
the tool runs a **pass** per scroll position: `top`, and `scrolled` when
`--scroll-screens` is non-zero. Each pass is one navigation per engine, because a
Goosie run scrolls before its first capture and cannot shoot the top and the
bottom of one page. The parity axis of a pass is refused - not scored - unless
both engines sat at the same pixel offset, which the engines report themselves:
Goosie's per-capture `scroll_y`, Chromium's `window.scrollY`. A pass that cannot
be measured makes the whole run NOT_MEASURABLE rather than disappearing from it.

It is a report, not a gate: it needs the network, and the web changes hourly.
It is fail-closed in the ways that matter - a capture the engine itself did not
consider finished is refused rather than counted, a pass whose captures hold no
ink at all is refused because an empty frame cannot flicker and so cannot be
called stable, a missing PNG or a shape mismatch ends the run with exit 2
instead of a percentage, and no threshold on the parity axis was invented after
seeing the data. `--fail-on-flicker` turns only the stability axis into a
non-zero exit, because that axis is a fact about this repo's own frame path
rather than a claim about a moving website.

`--selftest` exercises all of that on synthetic frames with no browser and no
network, which is what `test/gatecheck/live_frame_stability_test.go` runs.
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import numpy as np
from PIL import Image

HERE = Path(__file__).resolve().parent
REPO = HERE.parent

_spec = importlib.util.spec_from_file_location("parity", REPO / "testdata" / "parity.py")
parity = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(parity)

# Both engines shoot the same window, and it is the window the live sweep already
# uses (testdata/live-parity.py VIEWPORT_*), so a stability row and a render
# parity row describe the same page.
VIEWPORT_W = 1280
VIEWPORT_H = 1024
VIEWPORT_DPR = 1.0

# Five captures one second apart is what docs/qa-test-plan.md QA-002 specifies.
# They are defaults, not a contract: the report records what was actually asked.
DEFAULT_FRAMES = 5
DEFAULT_INTERVAL_MS = 1000

# QA-002's second half is "scroll down one screen and repeat", so the scrolled
# pass is the default; 0 shoots only the top, the shape this tool had first.
DEFAULT_SCROLL_SCREENS = 1

# A Goosie run that spends longer than this on a sequence is not a slow page, it
# is a run that never settled; the number is the screenshot deadline the binary
# itself carries (cmd/goosie/drive.go) plus the sequence horizon.
RUN_TIMEOUT_S = 180

EXIT_OK = 0
EXIT_FLICKER = 1
EXIT_NOT_MEASURABLE = 2

STABLE = "STABLE"
FLICKERS = "FLICKERS"
NOT_MEASURABLE = "NOT_MEASURABLE"


# --- pixel maths ----------------------------------------------------------

def load_rgb(path: Path) -> np.ndarray:
    return np.array(Image.open(path).convert("RGB"))


def content_regions(reference: np.ndarray) -> list[dict]:
    """The fixed region set, from Chromium's first capture.

    Regions come from the reference rather than from either engine's later
    frames so a band cannot appear or vanish mid-sequence and make a stable page
    look unstable. parity.content_bands already drops thin and sparse bands, so
    what is left is the page's own content.
    """
    return [
        {"y0": int(y0), "y1": int(y1), "x0": int(x0), "x1": int(x1),
         "reference_ink": int(count)}
        for y0, y1, x0, x1, count in parity.content_bands(parity.ink_mask(reference))
    ]


def changed_pixels(a: np.ndarray, b: np.ndarray, region: dict) -> int:
    """Pixels inside one region that disagree by more than parity.THRESH."""
    window_a = a[region["y0"]:region["y1"] + 1, region["x0"]:region["x1"]]
    window_b = b[region["y0"]:region["y1"] + 1, region["x0"]:region["x1"]]
    if window_a.shape != window_b.shape:
        return -1
    diff = np.abs(window_a.astype(int) - window_b.astype(int))
    return int(np.count_nonzero(np.any(diff > parity.THRESHOLD, axis=2)))


def temporal_axis(frames: list[np.ndarray], regions: list[dict]) -> dict:
    """Compare each consecutive pair of one engine's own captures."""
    pairs = []
    per_region = {index: [] for index in range(len(regions))}
    for i in range(len(frames) - 1):
        counts = []
        for index, region in enumerate(regions):
            count = changed_pixels(frames[i], frames[i + 1], region)
            counts.append(count)
            per_region[index].append(count)
        pairs.append({"pair": [i, i + 1], "total_changed": int(sum(c for c in counts if c >= 0)),
                      "region_changed": counts})
    unstable = []
    for index, region in enumerate(regions):
        counts = per_region[index]
        changed = [p for p in pairs if p["region_changed"][index] > 0]
        if changed:
            unstable.append({
                "region": region,
                "pairs": [p["pair"] for p in changed],
                "max_changed": int(max(counts)),
                "total_changed": int(sum(counts)),
            })
    ink = [int(parity.ink_mask(f).sum()) for f in frames]
    return {
        "pairs": pairs,
        "unstable_regions": unstable,
        "n_unstable_regions": len(unstable),
        "ink_per_frame": ink,
        "ink_spread": int(max(ink) - min(ink)) if ink else 0,
    }


def parity_axis(goosie: list[np.ndarray], chromium: list[np.ndarray]) -> dict:
    """Compare the two engines capture by capture, at equal offsets."""
    rows = []
    for i, (g, c) in enumerate(zip(goosie, chromium)):
        if g.shape != c.shape:
            rows.append({"frame": i, "shape_mismatch": [list(g.shape[:2]), list(c.shape[:2])]})
            continue
        diff = np.abs(g.astype(int) - c.astype(int))
        worst, coverage, blanks = parity.score_regions(g, c)
        rows.append({
            "frame": i,
            "pct_exact": round(float(np.mean(np.all(diff <= parity.THRESHOLD, axis=2)) * 100), 2),
            "ink_goosie": int(parity.ink_mask(g).sum()),
            "ink_chromium": int(parity.ink_mask(c).sum()),
            "worst_iou": round(worst, 3) if worst is not None else None,
            "coverage": round(coverage, 3) if coverage is not None else None,
            "blank_bands": [[int(a), int(b)] for a, b in blanks],
        })
    return {"frames": rows}


# --- capture --------------------------------------------------------------

def goosie_cmd(binary: str, url: str, seq_dir: Path, frames: int, interval_ms: int,
               screens: int) -> list[str]:
    """The command for one pass: the shared window, the sequence, and the scroll."""
    cmd = [binary, "-backend", "headless", "-url", url,
           "-width", str(VIEWPORT_W), "-height", str(VIEWPORT_H),
           "-dpr", str(VIEWPORT_DPR), "-screenshot-seq", str(seq_dir),
           "-seq-frames", str(frames), "-seq-interval", f"{interval_ms}ms"]
    if screens:
        cmd += ["-seq-scroll-screens", str(screens)]
    return cmd


def capture_goosie(binary: str, url: str, out_dir: Path, frames: int, interval_ms: int,
                   notes: dict, tag: str, screens: int) -> tuple[list[Path], dict]:
    """One `-screenshot-seq` run: N PNGs plus the frames.json that describes them.

    A pass gets its own directory and its own navigation: the sequence scrolls
    once, before capture 0, so one run cannot cover two scroll positions.
    """
    seq_dir = out_dir / f"goosie-{tag}"
    if seq_dir.exists():
        shutil.rmtree(seq_dir)
    seq_dir.mkdir(parents=True)
    cmd = goosie_cmd(binary, url, seq_dir, frames, interval_ms, screens)
    started = time.time()
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=RUN_TIMEOUT_S)
    except OSError as exc:
        raise MeasurementError(f"cannot run the goosie binary {binary}: {exc}") from exc
    notes[f"goosie_{tag}_rc"] = proc.returncode
    notes[f"goosie_{tag}_secs"] = round(time.time() - started, 2)
    tail = proc.stderr.strip()[-400:]
    notes[f"goosie_{tag}_stderr_tail"] = tail
    report_path = seq_dir / "frames.json"
    if not report_path.exists():
        raise MeasurementError(f"the {tag} pass wrote no frames.json (rc={proc.returncode}): "
                               f"{tail[:160]}")
    report = json.loads(report_path.read_text())
    notes[f"goosie_{tag}_json"] = report
    paths = [seq_dir / f"frame-{i:03d}.png" for i in range(frames)]
    missing = [str(p.name) for p in paths if not p.exists()]
    if missing:
        raise MeasurementError(f"the {tag} pass wrote no PNGs for {missing}")
    return paths, report


def capture_chromium(url: str, out_dir: Path, frames: int, interval_ms: int,
                     notes: dict, tag: str, scroll_to: int) -> list[Path]:
    """N screenshots at the offset Goosie sat at, from the pinned render oracle.

    Chromium is driven to `scroll_to` and then asked for its own `window.scrollY`:
    the parity gate compares the two offsets the engines report for themselves,
    never the position this tool asked for.
    """
    try:
        from playwright.sync_api import sync_playwright
    except ImportError as exc:
        raise MeasurementError(f"the pinned render oracle is unavailable: {exc}") from exc

    seq_dir = out_dir / f"chromium-{tag}"
    seq_dir.mkdir(parents=True, exist_ok=True)
    paths = [seq_dir / f"frame-{i:03d}.png" for i in range(frames)]
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        notes["chromium_version"] = browser.version
        ctx = browser.new_context(viewport={"width": VIEWPORT_W, "height": VIEWPORT_H},
                                  device_scale_factor=VIEWPORT_DPR)
        page = ctx.new_page()
        started = time.time()
        page.goto(url, wait_until="load")
        page.evaluate("y => window.scrollTo(0, y)", scroll_to)
        page.wait_for_timeout(interval_ms)
        notes[f"chromium_{tag}_scroll_y"] = page.evaluate("window.scrollY")
        notes[f"chromium_{tag}_max_scroll_px"] = page.evaluate(
            "Math.max(0, document.documentElement.scrollHeight - window.innerHeight)")
        offsets = []
        for i, path in enumerate(paths):
            page.screenshot(path=str(path))
            offsets.append(round(time.time() - started, 3))
            if i + 1 < frames:
                time.sleep(interval_ms / 1000.0)
        notes[f"chromium_{tag}_offsets_s"] = offsets
        ctx.close()
        browser.close()
    return paths


def valid_captures(captures: dict, paths: list[Path]) -> list[int]:
    """Which Goosie captures the engine itself called finished.

    The driver only consults the sequence on a frame it would have ended the run
    on, so every capture should be idle with nothing wanted and nothing on
    order. A capture that says otherwise was not a settled frame, and pixels
    taken from it prove nothing about stability either way.
    """
    good = []
    for index, path in enumerate(paths):
        row = next((f for f in captures.get("frames", []) if f.get("png") == path.name), None)
        if row is None:
            raise MeasurementError(f"frames.json has no entry for {path.name}")
        if row.get("idle") and row.get("tiles_needed") == 0 and row.get("painting_bytes") == 0:
            good.append(index)
    return good


def capture_offsets(report: dict, paths: list[Path]) -> list[int]:
    """Where the engine says each of its own captures was taken."""
    rows = {frame.get("png"): frame for frame in report.get("frames", [])}
    offsets = []
    for path in paths:
        row = rows.get(path.name)
        if row is None:
            raise MeasurementError(f"frames.json has no entry for {path.name}")
        offsets.append(int(row.get("scroll_y", 0)))
    return offsets


def offset_gate(goosie_offsets: list[int], chromium_offset: int) -> str | None:
    """Why this pass may not be scored on the parity axis, or None if it may.

    Parity is a pixel-by-pixel comparison, so it means something only when both
    engines looked at the same slice of the page. Each offset is a number the
    engine reported for itself, never one this tool assumed.
    """
    distinct = sorted(set(goosie_offsets))
    if len(distinct) > 1:
        return (f"goosie captured at {len(distinct)} different offsets {distinct}, "
                "so no single Chromium position can be compared against them")
    if distinct and distinct[0] != chromium_offset:
        return (f"goosie sat at y={distinct[0]} while Chromium sat at y={chromium_offset}")
    return None


def landing_gate(tag: str, screens: int, report: dict) -> None:
    """Refuse a pass that asked to move and reports that it did not.

    `scroll_landed` is the engine's own answer, not this tool's guess: a page
    whose ceiling is its top would otherwise be scored as a scrolled view of
    itself and reported as a parity success at y=0.
    """
    if screens <= 0:
        return
    landed = report.get("scroll_landed")
    if landed is not True:
        raise MeasurementError(
            f"the {tag} pass asked for {screens} screen(s) but the engine reported "
            f"scroll_landed={landed} after {report.get('scroll_presses')} press(es)")


# --- verdict --------------------------------------------------------------

class MeasurementError(Exception):
    """The run could not be measured at all; the tool refuses to score it."""


def verdict(goosie: list[np.ndarray], regions: list[dict], valid: list[int]) -> dict:
    """Score stability on the captures the engine called finished."""
    used = [i for i in valid if i < len(goosie)]
    if len(used) < 2:
        raise MeasurementError(f"only {len(used)} settled capture(s); stability needs 2")
    frames = [goosie[i] for i in used]
    axis = temporal_axis(frames, regions)
    axis["used_captures"] = used
    axis["verdict"] = STABLE if axis["n_unstable_regions"] == 0 else FLICKERS
    return axis


def combine_verdicts(verdicts: list[str]) -> str:
    """One word for a run of passes, with no averaging.

    A pass that could not be measured cannot be reported as stable, so it drags
    the run to NOT_MEASURABLE; a pass that flicked is a fact the run cannot
    average away either. Nothing measured is nothing claimed.
    """
    if not verdicts or NOT_MEASURABLE in verdicts:
        return NOT_MEASURABLE
    if FLICKERS in verdicts:
        return FLICKERS
    return STABLE


def measure_pass(url: str, tag: str, screens: int, frames: int, interval_ms: int,
                 binary: str, out: Path, notes: dict,
                 shoot_goosie=capture_goosie, shoot_chromium=capture_chromium) -> dict:
    """One scroll position, both engines, both axes.

    The two capture functions are arguments because a pass is wiring, and wiring
    should be provable without a browser or the network.
    """
    goosie_paths, report = shoot_goosie(
        binary, url, out, frames, interval_ms, notes, tag, screens)
    offsets = capture_offsets(report, goosie_paths)
    row = {
        "pass": tag,
        "scroll_screens": screens,
        "scroll_presses": report.get("scroll_presses"),
        "scroll_landed": report.get("scroll_landed"),
        "goosie_offsets": offsets,
    }
    landing_gate(tag, screens, report)
    chromium_paths = shoot_chromium(url, out, frames, interval_ms, notes, tag, offsets[0])
    g = [load_rgb(p) for p in goosie_paths]
    c = [load_rgb(p) for p in chromium_paths]
    if g[0].shape != c[0].shape:
        raise MeasurementError(f"the {tag} pass cannot be scored: shape mismatch goosie "
                               f"{list(g[0].shape[:2])} vs chromium {list(c[0].shape[:2])}")
    regions = content_regions(c[0])
    if not regions:
        raise MeasurementError(f"the {tag} pass cannot be scored: Chromium's own first capture "
                               "has no content band of MIN_BAND_HEIGHT/MIN_INK, so this page "
                               "cannot test stability")
    chromium_y = notes[f"chromium_{tag}_scroll_y"]
    row["chromium_offset"] = chromium_y
    row["chromium_max_scroll_px"] = notes[f"chromium_{tag}_max_scroll_px"]
    row["n_regions"] = len(regions)
    row["parity_refused"] = offset_gate(offsets, int(round(chromium_y)))
    ink = [int(parity.ink_mask(f).sum()) for f in g]
    row["goosie_ink"] = ink
    row["chromium_ink"] = [int(parity.ink_mask(f).sum()) for f in c]
    if all(i == 0 for i in ink):
        raise MeasurementError(f"the {tag} pass captured nothing at y={offsets[0]}: every Goosie "
                               "capture has zero content ink, and a blank viewport cannot be "
                               "evidence of stability because it has nothing to change")
    valid = valid_captures(report, goosie_paths)
    row["settled_captures"] = valid
    row["rejected_captures"] = [i for i in range(frames) if i not in valid]
    row["goosie"] = temporal_axis(g, regions)
    row["chromium"] = temporal_axis(c, regions)
    row["parity"] = None if row["parity_refused"] else parity_axis(g, c)
    try:
        settled = verdict(g, regions, valid)
    except MeasurementError as exc:
        row["verdict"] = NOT_MEASURABLE
        row["verdict_reason"] = str(exc)
        return row
    row["verdict"] = settled["verdict"]
    row["verdict_captures"] = settled["used_captures"]
    return row


def pass_plan(scroll_screens: int) -> list[tuple[str, int]]:
    """The scroll positions a run measures: the top, and below the fold if asked."""
    return [("top", 0)] + ([("scrolled", scroll_screens)] if scroll_screens > 0 else [])


def run_pass(url: str, tag: str, screens: int, frames: int, interval_ms: int,
             binary: str, out: Path, notes: dict) -> dict:
    """One pass, with a refusal reported as a pass rather than ending the run.

    The other pass may still be worth reading, and the run can only be dragged
    down to NOT_MEASURABLE if a refusal survives as a row.
    """
    try:
        return measure_pass(url, tag, screens, frames, interval_ms, binary, out, notes)
    except MeasurementError as exc:
        return {"pass": tag, "scroll_screens": screens, "verdict": NOT_MEASURABLE,
                "verdict_reason": str(exc)}


def build_payload(url: str, frames: int, interval_ms: int, binary: str,
                  scroll_screens: int, notes: dict) -> dict:
    rows = [run_pass(url, tag, screens, frames, interval_ms, binary, Path(notes["_out"]), notes)
            for tag, screens in pass_plan(scroll_screens)]
    return {
        "url": url,
        "measured": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "frames_asked": frames,
        "interval_ms": interval_ms,
        "viewport": [VIEWPORT_W, VIEWPORT_H, VIEWPORT_DPR],
        "binary": binary,
        "binary_sha256": parity.binary_sha256(binary),
        "oracle_pins": parity.load_oracle_pins(),
        "scroll_screens": scroll_screens,
        "passes": rows,
        "verdict": combine_verdicts([row["verdict"] for row in rows]),
        "notes": {k: v for k, v in notes.items() if not k.startswith("_")},
    }


def render(payload: dict) -> str:
    lines = [
        f"live-frame-stability  {payload['url']}",
        f"  {payload['frames_asked']} captures x {payload['interval_ms']}ms "
        f"at {payload['viewport'][0]}x{payload['viewport'][1]} dpr {payload['viewport'][2]}",
        f"  VERDICT {payload.get('verdict', 'UNKNOWN')} across passes: "
        + ", ".join(f"{row['pass']}={row.get('verdict', 'UNKNOWN')}" for row in payload["passes"]),
    ]
    for row in payload["passes"]:
        lines.append(f"  pass {row['pass']} ({row['scroll_screens']} screen(s)): "
                     f"VERDICT {row.get('verdict', 'UNKNOWN')}"
                     + (f" - {row['verdict_reason']}" if row.get("verdict_reason") else ""))
        if "goosie" not in row:
            continue
        lines.append(f"    scroll asked {row['scroll_screens']} screen(s), pressed "
                     f"{row['scroll_presses']}, landed {row['scroll_landed']}; goosie sat at "
                     f"{sorted(set(row['goosie_offsets']))}, chromium at "
                     f"{row['chromium_offset']} of a {row['chromium_max_scroll_px']}px range")
        lines.append(f"    regions {row['n_regions']}; settled {row['settled_captures']}; "
                     f"rejected {row['rejected_captures']}; ink goosie {row['goosie_ink'][0]} "
                     f"vs chromium {row['chromium_ink'][0]}")
        for side in ("goosie", "chromium"):
            axis = row[side]
            lines.append(f"    {side}: {axis['n_unstable_regions']} unstable region(s), "
                         f"ink spread {axis['ink_spread']}")
            for bad in axis["unstable_regions"][:5]:
                r = bad["region"]
                lines.append(f"      band y={r['y0']}..{r['y1']} x={r['x0']}..{r['x1']}: "
                             f"max {bad['max_changed']} px changed on pairs {bad['pairs']}")
        if row["parity_refused"]:
            lines.append(f"    parity refused: {row['parity_refused']}")
            continue
        lines.append(f"    parity at y={row['goosie_offsets'][0]}")
        worst = None
        for frame in row["parity"]["frames"]:
            if "shape_mismatch" in frame:
                lines.append(f"      parity frame {frame['frame']}: shape mismatch "
                             f"{frame['shape_mismatch']}")
                continue
            worst = frame["worst_iou"] if worst is None else min(worst, frame["worst_iou"])
            lines.append(f"      parity frame {frame['frame']}: pct_exact {frame['pct_exact']}% "
                         f"worst_iou {frame['worst_iou']} coverage {frame['coverage']} "
                         f"blank_bands {len(frame['blank_bands'])}")
        if worst is not None:
            lines.append("      (no pass mark on the parity axis; "
                         f"worst_iou across frames = {worst})")
    return "\n".join(lines)


# --- selftest -------------------------------------------------------------

def _blank(h: int = 240, w: int = 320) -> np.ndarray:
    return np.full((h, w, 3), 255, dtype=np.uint8)


def _band(arr: np.ndarray, y0: int, y1: int, x0: int, x1: int, colour=(0, 0, 0)) -> None:
    arr[y0:y1, x0:x1] = colour


def selftest() -> int:
    """Exercise every fail-closed property without a browser or the network."""
    fails: list[str] = []

    def check(name: str, ok: bool, detail: str = "") -> None:
        if not ok:
            fails.append(f"{name}: {detail}")

    regions_src = _blank()
    _band(regions_src, 20, 40, 10, 300)
    _band(regions_src, 100, 130, 10, 300)
    regions = content_regions(regions_src)
    check("regions", len(regions) == 2, f"got {len(regions)} from a two-band reference")

    # (a) identical captures are stable, and stable means zero changed pixels.
    frames = [regions_src.copy() for _ in range(4)]
    axis = temporal_axis(frames, regions)
    check("identical-frames", axis["n_unstable_regions"] == 0 and
          all(p["total_changed"] == 0 for p in axis["pairs"]), json.dumps(axis["pairs"]))

    # (b) a planted change inside one band names that band and counts its pixels.
    # The reference bands are black, so the change has to flip pixels to white -
    # planting black on black would change nothing and prove nothing.
    moved = frames[1].copy()
    _band(moved, 105, 115, 20, 30, (255, 255, 255))  # 10x10 = 100 px in the second band only
    axis = temporal_axis([frames[0], moved, frames[2], frames[3]], regions)
    bad = axis["unstable_regions"]
    check("planted-change-detects-one-region", len(bad) == 1, f"got {len(bad)}")
    if len(bad) == 1:
        check("planted-change-counts-pixels", bad[0]["max_changed"] == 100,
              f"max_changed {bad[0]['max_changed']}")
        check("planted-change-names-pairs", bad[0]["pairs"] == [[0, 1], [1, 2]],
              str(bad[0]["pairs"]))  # a one-frame blip is a change on the way in and on the way out
        check("planted-change-region-matches", bad[0]["region"]["y0"] == regions[1]["y0"],
              json.dumps(bad[0]["region"]))

    # (c) the tolerance is parity.THRESHOLD, not zero: a band pixel moved by
    # exactly the tolerance is a match, one step over is not. This is inherited
    # from the corpus scorer, so it must be asserted rather than assumed. The
    # reference bands are black, so the movement starts from black.
    edge = frames[0].copy()
    edge[25:26, 10:20] = (parity.THRESHOLD, parity.THRESHOLD, parity.THRESHOLD)
    axis = temporal_axis([frames[0], edge], regions)
    check("threshold-is-inclusive", axis["pairs"][0]["total_changed"] == 0,
          f"{axis['pairs'][0]['total_changed']} px changed at exactly the tolerance")
    edge2 = frames[0].copy()
    edge2[25:26, 10:20] = (parity.THRESHOLD + 1, 0, 0)
    axis = temporal_axis([frames[0], edge2], regions)
    check("one-step-over-is-counted", axis["pairs"][0]["total_changed"] == 10,
          f"{axis['pairs'][0]['total_changed']} px changed one step over")

    # (d) an unsettled capture is refused, and two settled ones still measure.
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        names = [f"frame-{i:03d}.png" for i in range(3)]
        for name, arr in zip(names, [regions_src.copy() for _ in range(3)]):
            Image.fromarray(arr).save(root / name)
        settled = valid_captures(
            {"frames": [{"png": names[0], "idle": True, "tiles_needed": 0, "painting_bytes": 0},
                        {"png": names[1], "idle": False, "tiles_needed": 4, "painting_bytes": 8192},
                        {"png": names[2], "idle": True, "tiles_needed": 0, "painting_bytes": 0}]},
            [root / n for n in names])
        check("unsettled-capture-rejected", settled == [0, 2], str(settled))
        try:
            valid_captures({"frames": [{"png": "other.png", "idle": True}]}, [root / names[0]])
            check("missing-framesjson-entry", False, "no MeasurementError raised")
        except MeasurementError:
            check("missing-framesjson-entry", True)
        # One settled capture is not a sequence.
        try:
            verdict([regions_src.copy() for _ in range(3)], regions, [1])
            check("single-settled-capture", False, "measured stability from one frame")
        except MeasurementError as exc:
            check("single-settled-capture", "settled" in str(exc), str(exc))
        # The offset a pass reports is the engine's own scroll_y, per capture.
        check("capture-offsets-read-back",
              capture_offsets({"frames": [{"png": names[0], "scroll_y": 921},
                                          {"png": names[1], "scroll_y": 921}]},
                              [root / names[0], root / names[1]]) == [921, 921],
              "a reported scroll_y never reached the pass row")
        try:
            capture_offsets({"frames": []}, [root / names[0]])
            check("capture-offsets-missing-entry", False, "no MeasurementError raised")
        except MeasurementError:
            check("capture-offsets-missing-entry", True)

    # (e) the parity axis refuses a shape mismatch instead of scoring it.
    other = regions_src.copy()
    other = np.pad(other, ((0, 8), (0, 0), (0, 0)), constant_values=255)
    rows = parity_axis([regions_src], [other])["frames"]
    check("parity-shape-mismatch", "shape_mismatch" in rows[0], json.dumps(rows[0])[:120])
    rows = parity_axis([regions_src], [regions_src.copy()])["frames"]
    check("parity-identical", rows[0]["pct_exact"] == 100.0 and not rows[0]["blank_bands"],
          json.dumps(rows[0]))

    # (f) the two scroll gates and the run verdict. A pass may only be scored on
    # the parity axis when both engines report the same single offset; a pass
    # that asked to move is refused when the engine says it did not; and a run is
    # only as measurable as its worst pass, never as good as its average.
    check("gate-one-offset", offset_gate([921, 921, 921], 921) is None,
          str(offset_gate([921, 921, 921], 921)))
    check("gate-splits-inside-goosie",
          "2 different offsets" in str(offset_gate([0, 921, 921], 921)),
          str(offset_gate([0, 921, 921], 921)))
    check("gate-engines-apart", "while Chromium sat at y=320" in str(offset_gate([921], 320)),
          str(offset_gate([921], 320)))
    landing_gate("top", 0, {})  # a pass that never asked to move cannot fail to move
    for landed, why in ((False, "refused"), (None, "absent")):
        try:
            landing_gate("scrolled", 1, {"scroll_landed": landed, "scroll_presses": 1})
            check(f"gate-scroll-{why}", False, "a pass that never moved was accepted")
        except MeasurementError as exc:
            check(f"gate-scroll-{why}", "scroll_landed=" in str(exc), str(exc))
    check("combine-all-stable", combine_verdicts([STABLE, STABLE]) == STABLE,
          combine_verdicts([STABLE, STABLE]))
    check("combine-one-flicker", combine_verdicts([STABLE, FLICKERS]) == FLICKERS,
          combine_verdicts([STABLE, FLICKERS]))
    check("combine-cannot-average-a-refusal",
          combine_verdicts([STABLE, NOT_MEASURABLE]) == NOT_MEASURABLE,
          combine_verdicts([STABLE, NOT_MEASURABLE]))
    check("combine-nothing-measured", combine_verdicts([]) == NOT_MEASURABLE,
          combine_verdicts([]))

    # (g) the shape of a run. A tool that quietly went back to shooting only the
    # top of the page is the defect this exists to close, so the plan itself is
    # pinned, and so is the rule that a pass which cannot run is a reported row.
    check("plan-default-has-both-passes",
          pass_plan(DEFAULT_SCROLL_SCREENS) == [("top", 0), ("scrolled", DEFAULT_SCROLL_SCREENS)],
          str(pass_plan(DEFAULT_SCROLL_SCREENS)))
    check("plan-zero-shoots-the-top-only", pass_plan(0) == [("top", 0)], str(pass_plan(0)))
    plan3 = pass_plan(3)
    check("plan-carries-the-request", len(plan3) == 2 and plan3[1] == ("scrolled", 3), str(plan3))
    plain = goosie_cmd("g", "about:blank", Path("/tmp/seq"), 5, 1000, 0)
    scrolled_cmd = goosie_cmd("g", "about:blank", Path("/tmp/seq"), 5, 1000, 2)
    check("top-pass-sends-no-scroll-flag", "-seq-scroll-screens" not in plain, str(plain))
    at = scrolled_cmd.index("-seq-scroll-screens") if "-seq-scroll-screens" in scrolled_cmd else -1
    check("scrolled-pass-sends-its-own-count",
          scrolled_cmd[at:at + 2] == ["-seq-scroll-screens", "2"]
          and str(VIEWPORT_W) in scrolled_cmd, str(scrolled_cmd))
    with tempfile.TemporaryDirectory() as tmp:
        try:
            dead = run_pass("about:blank", "top", 0, 2, 100, str(Path(tmp) / "no-such-goosie"),
                            Path(tmp), {"_out": tmp})
        except MeasurementError as exc:
            check("unrunnable-pass-is-reported-not-raised", False,
                  f"escaped the run as an error: {exc}")
            dead = {}
        else:
            check("unrunnable-pass-is-reported-not-raised",
                  dead.get("verdict") == NOT_MEASURABLE
                  and "no-such-goosie" in dead.get("verdict_reason", ""), json.dumps(dead)[:200])
        check("unrunnable-pass-keeps-its-request",
              dead.get("pass") == "top" and dead.get("scroll_screens") == 0, json.dumps(dead)[:200])

    # (h) a verdict is a word about the data, and --fail-on-flicker can reach it.
    top = {
        "pass": "top", "scroll_screens": 0, "scroll_presses": 0, "scroll_landed": None,
        "goosie_offsets": [0, 0], "chromium_offset": 0.0, "chromium_max_scroll_px": 4200,
        "n_regions": len(regions), "settled_captures": [0, 1], "rejected_captures": [],
        "goosie_ink": [14500, 14500], "chromium_ink": [14500, 14500],
        "parity_refused": None,
        "goosie": temporal_axis([regions_src.copy(), regions_src.copy()], regions),
        "chromium": temporal_axis([regions_src.copy(), regions_src.copy()], regions),
        "parity": parity_axis([regions_src], [regions_src.copy()]),
        "verdict": STABLE,
    }
    scrolled = {
        "pass": "scrolled", "scroll_screens": 1, "scroll_presses": 1, "scroll_landed": True,
        "goosie_offsets": [921, 921], "chromium_offset": 921.0, "chromium_max_scroll_px": 4200,
        "n_regions": len(regions), "settled_captures": [0, 1], "rejected_captures": [],
        "goosie_ink": [14500, 14500], "chromium_ink": [5800, 5800],
        "parity_refused": None,
        "goosie": temporal_axis([regions_src.copy(), moved], regions),
        "chromium": temporal_axis([regions_src.copy(), regions_src.copy()], regions),
        "parity": parity_axis([regions_src], [regions_src.copy()]),
        "verdict": FLICKERS,
    }
    payload = {
        "url": "about:blank", "frames_asked": 2, "interval_ms": 1000,
        "viewport": [VIEWPORT_W, VIEWPORT_H, VIEWPORT_DPR], "binary": "x",
        "binary_sha256": None, "oracle_pins": {}, "scroll_screens": 1,
        "passes": [top, scrolled], "verdict": combine_verdicts([STABLE, FLICKERS]),
        "notes": {},
    }
    text = render(payload)
    check("report-carries-verdict", f"VERDICT {FLICKERS}" in text[:240], text[:240])
    check("report-names-every-pass", "pass top" in text and "pass scrolled" in text, text[:400])
    check("report-names-the-band", "band y=100" in text, text[:600])
    check("report-names-the-offsets", "goosie sat at [921], chromium at 921.0" in text, text)
    check("report-names-the-ink", "ink goosie 14500 vs chromium 5800" in text, text)
    check("report-refuses-a-pass-mark", "no pass mark" in text, text[-500:])
    refused = {**payload,
               "passes": [{"pass": "scrolled", "scroll_screens": 1, "verdict": NOT_MEASURABLE,
                           "verdict_reason": "the scrolled pass wrote no frames.json"}],
               "verdict": NOT_MEASURABLE}
    check("report-survives-an-unmeasured-pass", "no frames.json" in render(refused),
          render(refused))

    # (i) the whole pass pipeline, on synthetic captures standing in for both
    # engines: what Goosie says it sat at is where Chromium must be driven, and
    # the two answers are what the parity axis is allowed to rest on.
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        engine_report = {
            "frames": [{"png": f"frame-{i:03d}.png", "idle": True, "tiles_needed": 0,
                        "painting_bytes": 0, "scroll_y": 921} for i in range(3)],
            "scroll_landed": True, "scroll_presses": 1,
        }

        def fake_goosie(binary, url, out_dir, frames, interval_ms, notes, tg, screens):
            seq_dir = Path(out_dir) / f"goosie-{tg}"
            seq_dir.mkdir(parents=True, exist_ok=True)
            for i in range(frames):
                Image.fromarray(regions_src.copy()).save(seq_dir / f"frame-{i:03d}.png")
            return [seq_dir / f"frame-{i:03d}.png" for i in range(frames)], engine_report

        asked: list[int] = []

        def fake_chromium(url, out_dir, frames, interval_ms, notes, tg, scroll_to,
                          sits_at=921.0, sparse=False):
            asked.append(scroll_to)
            seq_dir = Path(out_dir) / f"chromium-{tg}"
            seq_dir.mkdir(parents=True, exist_ok=True)
            for i in range(frames):
                arr = regions_src.copy()
                if sparse:
                    arr[100:130, 10:300] = 255
                Image.fromarray(arr).save(seq_dir / f"frame-{i:03d}.png")
            notes[f"chromium_{tg}_scroll_y"] = sits_at
            notes[f"chromium_{tg}_max_scroll_px"] = 4200
            return [seq_dir / f"frame-{i:03d}.png" for i in range(frames)]

        row = measure_pass("about:blank", "scrolled", 1, 3, 1000, "g", root, {},
                           fake_goosie, fake_chromium)
        check("pass-drives-chromium-to-the-reported-offset", asked == [921], str(asked))
        check("pass-scores-parity-when-engines-agree",
              row["verdict"] == STABLE and row["parity_refused"] is None
              and len(row["parity"]["frames"]) == 3, json.dumps(row)[:200])
        check("pass-carries-the-oracle-range", row["chromium_max_scroll_px"] == 4200
              and row["chromium_offset"] == 921.0, json.dumps(row)[:200])
        check("pass-carries-the-ink-of-both-engines",
              row["goosie_ink"] == row["chromium_ink"] == [14500, 14500, 14500],
              json.dumps(row)[:200])
        crowded = measure_pass("about:blank", "scrolled", 1, 3, 1000, "g", root, {},
                               fake_goosie,
                               lambda *a, **k: fake_chromium(*a, sparse=True, **k))
        check("pass-inks-each-engine-on-its-own",
              crowded["goosie_ink"] == [14500, 14500, 14500]
              and crowded["chromium_ink"] == [5800, 5800, 5800], json.dumps(crowded)[:200])
        check("pass-keeps-the-engine-answers",
              row["scroll_landed"] is True and row["settled_captures"] == [0, 1, 2]
              and row["verdict_captures"] == [0, 1, 2], json.dumps(row)[:200])
        apart = measure_pass("about:blank", "scrolled", 1, 3, 1000, "g", root, {},
                             fake_goosie,
                             lambda *a, **k: fake_chromium(*a, sits_at=0.0, **k))
        check("pass-refuses-parity-when-engines-split",
              apart["parity"] is None and "while Chromium sat at y=0" in apart["parity_refused"]
              and apart["verdict"] == STABLE, json.dumps(apart)[:200])

        stuck_report = dict(engine_report, scroll_landed=False)

        def fake_stuck(*args):
            return fake_goosie(*args)[0], stuck_report

        try:
            measure_pass("about:blank", "scrolled", 1, 3, 1000, "g", root, {},
                         fake_stuck, fake_chromium)
            check("pass-refused-when-the-scroll-never-landed", False,
                  "a page stuck at the top was scored as a scrolled view")
        except MeasurementError as exc:
            check("pass-refused-when-the-scroll-never-landed", "scroll_landed=False" in str(exc),
                  str(exc))

        def fake_blank(*args):
            paths, rep = fake_goosie(*args)
            for path in paths:
                Image.fromarray(_blank()).save(path)
            return paths, rep

        try:
            measure_pass("about:blank", "scrolled", 1, 3, 1000, "g", root, {},
                         fake_blank, fake_chromium)
            check("blank-captures-refused", False,
                  "blank viewports were reported as a stability result")
        except MeasurementError as exc:
            check("blank-captures-refused", "zero content ink" in str(exc), str(exc))

    if fails:
        for line in fails:
            print("SELFTEST FAIL " + line)
        return 1
    print("selftest green")
    return 0


# --- cli ------------------------------------------------------------------

def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--url", help="the page to open and watch, once per pass")
    ap.add_argument("--binary", default=str(REPO / "goosie"),
                    help="goosie binary to run (default ./goosie)")
    ap.add_argument("--frames", type=int, default=DEFAULT_FRAMES)
    ap.add_argument("--interval-ms", type=int, default=DEFAULT_INTERVAL_MS)
    ap.add_argument("--scroll-screens", type=int, default=DEFAULT_SCROLL_SCREENS,
                    help="screens to scroll for the second pass (0 means shoot the top only)")
    ap.add_argument("--workdir", help="where to keep the captures (default: a temp dir)")
    ap.add_argument("--json", help="write the machine-readable report here")
    ap.add_argument("--fail-on-flicker", action="store_true",
                    help="exit 1 when a settled Goosie capture changed pixels")
    ap.add_argument("--selftest", action="store_true",
                    help="score synthetic frames and prove the fail-closed properties")
    args = ap.parse_args()

    if args.selftest:
        return selftest()
    if not args.url:
        print("live-frame-stability.py: --url is required (or --selftest)", file=sys.stderr)
        return EXIT_NOT_MEASURABLE
    if args.scroll_screens < 0:
        print("live-frame-stability.py: --scroll-screens must not be negative", file=sys.stderr)
        return EXIT_NOT_MEASURABLE

    out = Path(args.workdir) if args.workdir else Path(tempfile.mkdtemp(prefix="frame-stability-"))
    out.mkdir(parents=True, exist_ok=True)
    notes = {"_out": str(out)}
    try:
        payload = build_payload(args.url, args.frames, args.interval_ms, args.binary,
                                args.scroll_screens, notes)
    except MeasurementError as exc:
        print(f"live-frame-stability: NOT MEASURABLE - {exc}", file=sys.stderr)
        return EXIT_NOT_MEASURABLE
    except subprocess.TimeoutExpired:
        print(f"live-frame-stability: NOT MEASURABLE - goosie exceeded {RUN_TIMEOUT_S}s",
              file=sys.stderr)
        return EXIT_NOT_MEASURABLE
    print(render(payload))
    if args.json:
        Path(args.json).write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n")
        print(f"report: {args.json}")
    if payload["verdict"] == NOT_MEASURABLE:
        print("live-frame-stability: NOT MEASURABLE - the report carries no verdict for "
              "every pass; see the pass reasons above", file=sys.stderr)
        return EXIT_NOT_MEASURABLE
    if args.fail_on_flicker and payload["verdict"] == FLICKERS:
        return EXIT_FLICKER
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
