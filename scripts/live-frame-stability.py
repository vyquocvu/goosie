#!/usr/bin/env python3
"""Live frame stability: does a page stay drawn, measured one region at a time.

QA-002 (docs/qa-test-plan.md) asks a question a screenshot cannot answer: open
the page once, then look at the same viewport again a second later, five times,
and say whether anything moved. `docs/qa-test-plan.md` QA-002 forbids closing that
question on PNG byte hashes, because a hash says only "these bytes differ" and
says nothing about where, how much, or whether the engine thought the frame was
finished when it took the shot.

So this tool captures a timed sequence twice - once from Goosie through
`-screenshot-seq`, which also records what the frame path said about itself at
each capture, and once from Chromium at the same offsets - and scores pixels:

* the **stability** axis compares each engine's own consecutive frames inside a
  fixed set of content regions, so "the page flickered" becomes "band y=318..362
  changed 1.812 pixels between captures 2 and 3";
* the **parity** axis compares the two engines capture by capture with
  `testdata/parity.py`'s own region functions, so "what a live page should look
  like" has exactly one definition in this repo.

It is a report, not a gate: it needs the network, and the web changes hourly.
It is fail-closed in the ways that matter - a capture the engine itself did not
consider finished is refused rather than counted, a missing PNG or a shape
mismatch ends the run with exit 2 instead of a percentage, and no threshold on
the parity axis was invented after seeing the data. `--fail-on-flicker` turns
only the stability axis into a non-zero exit, because that axis is a fact about
this repo's own frame path rather than a claim about a moving website.

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

def capture_goosie(binary: str, url: str, out_dir: Path, frames: int,
                   interval_ms: int, notes: dict) -> list[Path]:
    """One `-screenshot-seq` run: N PNGs plus the frames.json that describes them."""
    seq_dir = out_dir / "goosie"
    if seq_dir.exists():
        shutil.rmtree(seq_dir)
    seq_dir.mkdir(parents=True)
    cmd = [binary, "-backend", "headless", "-url", url,
           "-width", str(VIEWPORT_W), "-height", str(VIEWPORT_H),
           "-dpr", str(VIEWPORT_DPR), "-screenshot-seq", str(seq_dir),
           "-seq-frames", str(frames), "-seq-interval", f"{interval_ms}ms"]
    started = time.time()
    proc = subprocess.run(cmd, capture_output=True, text=True, timeout=RUN_TIMEOUT_S)
    notes["goosie_rc"] = proc.returncode
    notes["goosie_secs"] = round(time.time() - started, 2)
    notes["goosie_stderr_tail"] = proc.stderr.strip()[-400:]
    report = seq_dir / "frames.json"
    if not report.exists():
        raise MeasurementError(f"goosie wrote no frames.json (rc={proc.returncode}): "
                               f"{notes['goosie_stderr_tail'][:160]}")
    notes["goosie_frames_json"] = json.loads(report.read_text())
    paths = [seq_dir / f"frame-{i:03d}.png" for i in range(frames)]
    missing = [str(p.name) for p in paths if not p.exists()]
    if missing:
        raise MeasurementError(f"goosie wrote no PNGs for {missing}")
    return paths


def capture_chromium(url: str, out_dir: Path, frames: int, interval_ms: int,
                     notes: dict) -> list[Path]:
    """N screenshots at the same offsets, from the pinned render oracle."""
    from playwright.sync_api import sync_playwright

    seq_dir = out_dir / "chromium"
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
        offsets = []
        for i, path in enumerate(paths):
            page.screenshot(path=str(path))
            offsets.append(round(time.time() - started, 3))
            if i + 1 < frames:
                time.sleep(interval_ms / 1000.0)
        notes["chromium_offsets_s"] = offsets
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


# --- verdict --------------------------------------------------------------

class MeasurementError(Exception):
    """The run could not be measured at all; the tool refuses to score it."""


def verdict(goosie: list[np.ndarray], chromium: list[np.ndarray], regions: list[dict],
            valid: list[int]) -> dict:
    """Score stability on the captures the engine called finished."""
    used = [i for i in valid if i < len(goosie)]
    if len(used) < 2:
        raise MeasurementError(f"only {len(used)} settled capture(s); stability needs 2")
    frames = [goosie[i] for i in used]
    axis = temporal_axis(frames, regions)
    axis["used_captures"] = used
    axis["verdict"] = STABLE if axis["n_unstable_regions"] == 0 else FLICKERS
    return axis


def build_payload(url: str, frames: int, interval_ms: int, binary: str,
                  notes: dict) -> dict:
    goosie_paths = capture_goosie(binary, url, Path(notes["_out"]), frames, interval_ms, notes)
    chromium_paths = capture_chromium(url, Path(notes["_out"]), frames, interval_ms, notes)
    g = [load_rgb(p) for p in goosie_paths]
    c = [load_rgb(p) for p in chromium_paths]
    if g[0].shape != c[0].shape:
        raise MeasurementError(f"shape mismatch goosie {list(g[0].shape[:2])} "
                               f"vs chromium {list(c[0].shape[:2])}")
    regions = content_regions(c[0])
    if not regions:
        raise MeasurementError("Chromium's own first capture has no content band "
                               "of MIN_BAND_HEIGHT/MIN_INK, so this page cannot test stability")
    valid = valid_captures(notes.get("goosie_frames_json", {}), goosie_paths)
    payload = {
        "url": url,
        "measured": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "frames_asked": frames,
        "interval_ms": interval_ms,
        "viewport": [VIEWPORT_W, VIEWPORT_H, VIEWPORT_DPR],
        "binary": binary,
        "binary_sha256": parity.binary_sha256(binary),
        "oracle_pins": parity.load_oracle_pins(),
        "n_regions": len(regions),
        "settled_captures": valid,
        "rejected_captures": [i for i in range(frames) if i not in valid],
        "goosie": temporal_axis(g, regions),
        "chromium": temporal_axis(c, regions),
        "parity": parity_axis(g, c),
        "notes": {k: v for k, v in notes.items() if not k.startswith("_")},
    }
    try:
        payload["verdict"] = verdict(g, c, regions, valid)["verdict"]
    except MeasurementError as exc:
        payload["verdict"] = NOT_MEASURABLE
        payload["verdict_reason"] = str(exc)
    return payload


def render(payload: dict) -> str:
    lines = [
        f"live-frame-stability  {payload['url']}",
        f"  captures {payload['frames_asked']} x {payload['interval_ms']}ms "
        f"at {payload['viewport'][0]}x{payload['viewport'][1]} dpr {payload['viewport'][2]}",
        f"  regions {payload['n_regions']}; settled {payload['settled_captures']}; "
        f"rejected {payload['rejected_captures']}",
        f"  VERDICT {payload.get('verdict', 'UNKNOWN')}"
        + (f" - {payload['verdict_reason']}" if payload.get("verdict_reason") else ""),
    ]
    for side in ("goosie", "chromium"):
        axis = payload[side]
        lines.append(f"  {side}: {axis['n_unstable_regions']} unstable region(s), "
                     f"ink spread {axis['ink_spread']}")
        for bad in axis["unstable_regions"][:5]:
            r = bad["region"]
            lines.append(f"    band y={r['y0']}..{r['y1']} x={r['x0']}..{r['x1']}: "
                         f"max {bad['max_changed']} px changed on pairs {bad['pairs']}")
    worst = None
    for row in payload["parity"]["frames"]:
        if "shape_mismatch" in row:
            lines.append(f"  parity frame {row['frame']}: shape mismatch {row['shape_mismatch']}")
            continue
        worst = row["worst_iou"] if worst is None else min(worst, row["worst_iou"])
        lines.append(f"  parity frame {row['frame']}: pct_exact {row['pct_exact']}% "
                     f"worst_iou {row['worst_iou']} coverage {row['coverage']} "
                     f"blank_bands {len(row['blank_bands'])}")
    if worst is not None:
        lines.append(f"  (no pass mark on the parity axis; worst_iou across frames = {worst})")
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
            verdict([regions_src.copy() for _ in range(3)], [regions_src.copy() for _ in range(3)],
                    regions, [1])
            check("single-settled-capture", False, "measured stability from one frame")
        except MeasurementError as exc:
            check("single-settled-capture", "settled" in str(exc), str(exc))

    # (e) the parity axis refuses a shape mismatch instead of scoring it.
    other = regions_src.copy()
    other = np.pad(other, ((0, 8), (0, 0), (0, 0)), constant_values=255)
    rows = parity_axis([regions_src], [other])["frames"]
    check("parity-shape-mismatch", "shape_mismatch" in rows[0], json.dumps(rows[0])[:120])
    rows = parity_axis([regions_src], [regions_src.copy()])["frames"]
    check("parity-identical", rows[0]["pct_exact"] == 100.0 and not rows[0]["blank_bands"],
          json.dumps(rows[0]))

    # (f) a verdict is a word about the data, and --fail-on-flicker can reach it.
    payload = {
        "url": "about:blank", "frames_asked": 2, "interval_ms": 1000,
        "viewport": [VIEWPORT_W, VIEWPORT_H, VIEWPORT_DPR], "binary": "x",
        "binary_sha256": None, "oracle_pins": {}, "n_regions": len(regions),
        "settled_captures": [0, 1], "rejected_captures": [],
        "goosie": temporal_axis([regions_src.copy(), moved], regions),
        "chromium": temporal_axis([regions_src.copy(), regions_src.copy()], regions),
        "parity": parity_axis([regions_src], [regions_src.copy()]),
        "notes": {},
    }
    payload["verdict"] = "FLICKERS"
    text = render(payload)
    check("report-names-the-band", "band y=100" in text, text[:200])
    check("report-carries-verdict", "VERDICT FLICKERS" in text, text[:200])
    check("report-refuses-a-pass-mark", "no pass mark" in text, text[-300:])

    if fails:
        for line in fails:
            print("SELFTEST FAIL " + line)
        return 1
    print("selftest green")
    return 0


# --- cli ------------------------------------------------------------------

def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--url", help="the page to open once and watch")
    ap.add_argument("--binary", default=str(REPO / "goosie"),
                    help="goosie binary to run (default ./goosie)")
    ap.add_argument("--frames", type=int, default=DEFAULT_FRAMES)
    ap.add_argument("--interval-ms", type=int, default=DEFAULT_INTERVAL_MS)
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

    out = Path(args.workdir) if args.workdir else Path(tempfile.mkdtemp(prefix="frame-stability-"))
    out.mkdir(parents=True, exist_ok=True)
    notes = {"_out": str(out)}
    try:
        payload = build_payload(args.url, args.frames, args.interval_ms, args.binary, notes)
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
    if args.fail_on_flicker and payload.get("verdict") == FLICKERS:
        return EXIT_FLICKER
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
