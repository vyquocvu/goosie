#!/usr/bin/env python3
"""Live-website render parity: real URLs, scored against Chromium/Playwright.

The fixture corpus (`testdata/render/`) proves the engine draws what a test
author wrote to control every variable. It cannot answer the question a user
asks: does a page that exists on the internet open. Measured on `main`,
https://vnexpress.net/ renders to a near-blank PNG (32 ink pixels against a
282,682-pixel reference) with exit code 0 and nothing on stderr, while
`-gate` on the same URL reports doc_height=1694 — laid out, rasterised, and
empty. No test in this repo and no line in `scripts/rc-sweep.sh` can see that.

This tool is a report, not a CI gate: it needs the network, and the web
changes daily. It is fail-closed in the way that matters — every URL in the
corpus gets exactly one row, a missing artifact is a label rather than a skip,
and if too few pages could be compared at all the tool calls itself broken
(exit 2) instead of printing a misleading percentage.

Scoring reuses `testdata/parity.py` by import so that "what is a blank band"
has exactly one definition in this repo. See
`docs/tasks/TASK-038-LIVE-WBSITE-PARITY.md`.
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import re
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

_spec = importlib.util.spec_from_file_location("parity", HERE / "parity.py")
parity = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(parity)

# --- measurement contract -------------------------------------------------
# Both sides must shoot the same window. Goosie's screenshot driver never
# scrolls (cmd/goosie/drive.go:134), so a full-page reference would compare a
# whole article against one viewport and call the difference an engine bug.
VIEWPORT_W = 1280
VIEWPORT_H = 1024
DPR = 1
GATE_FRAMES = 120
CHROMIUM_TIMEOUT_MS = 30_000
GOOSIE_TIMEOUT_S = 120
SPACING_S = 0.15
# cmd/goosie/drive.go:170 hard-codes a 25 s deadline for paced modes and still
# exits 0 when it fires. A PNG from such a run may be cut mid-paint, so it is
# reported as NOT_SETTLED instead of OK.
SCREENSHOT_DEADLINE_S = 25.0
DEADLINE_TOLERANCE_S = 0.7

# --- thresholds owned by this tool ---------------------------------------
# Never edit parity.py's PASS_AT / MIN_INK for the fixture harness to make a
# live page pass; these are the live sweep's own numbers.
#
# pct_exact is deliberately NOT a pass criterion: Chromium and Goosie differ in
# font rasterisation and sub-pixel position, so a page can be perfectly usable
# at 71% exact pixels (Wikipedia measures coverage 0.872) and useless at 86%
# (MDN /docs/Learn measures coverage 0.218). Coverage and blank bands decide.
LIVE_COVERAGE_OK = 0.60
LIVE_MIN_INK = parity.MIN_INK
LIVE_MAX_BLANK_BANDS = 0
# A run that compared fewer pages than this is a broken run, not a low score.
MIN_SCORED = 75

# The engine's own refusal vocabulary, from internal/engine/limits.go. A refusal
# can arrive with a nonzero exit (measured on cafef.vn: "CSS numeric value
# outside safe geometry range", rc 1), so this family is checked before the
# crash branch: a document the engine declined to open is not the same defect
# as a run that fell over.
CAP_RE = re.compile(r"limit exceeded|must be finite|unsafe document|"
                    r"outside safe geometry range")
FATAL_RE = re.compile(r"goosie: goosie|build session|fatal|error|refus", re.I)

# Ordered failure ladder: the first match wins, so a page is never counted in
# two buckets and a crash is never misread as a coverage problem.
LADDER = [
    "FETCH_FAILED",          # reference browser could not load the page
    "GOOSIE_TIMEOUT",        # Goosie ran past GOOSIE_TIMEOUT_S
    "ADMISSION_CAPPED",      # a document limit refused the page (rc 0 or 1)
    "GOOSIE_CRASH",          # nonzero exit that is not a refusal
    "GOOSIE_RENDER_MISSING",  # exit 0, no PNG
    "STRUCTURE_MISSING",     # -gate produced no readable doc_height
    "DIMENSION_MISMATCH",    # the two shots are not the same window
    "REFERENCE_EMPTY",       # Chromium itself painted nothing: no conclusion possible
    "LAYOUT_EMPTY",          # laid out to nothing and painted nothing
    "STRUCTURE_DISAGREES",   # doc_height says no layout, the paint says otherwise
    "BLANK_OUTPUT",          # painted (near) nothing where Chromium painted a page
    "MISSING_REGIONS",       # a reference content band is entirely empty
    "LOW_COVERAGE",          # far less of the reference ink present
    "NOT_SETTLED",           # hit the hard-coded screenshot deadline
    "OK",
]
# Labels where both sides exist as comparable images, i.e. the sweep actually
# measured the engine. Everything above it in the ladder is a run that did not
# produce a comparison and must not be folded into a percentage.
SCORED_LABELS = {
    "LAYOUT_EMPTY", "STRUCTURE_DISAGREES", "BLANK_OUTPUT", "MISSING_REGIONS",
    "LOW_COVERAGE", "NOT_SETTLED", "OK",
}


def load_corpus(path: Path) -> list[dict]:
    doc = json.loads(Path(path).read_text())
    sites = doc["sites"] if isinstance(doc, dict) else doc
    if len({s["url"] for s in sites}) != len(sites):
        raise SystemExit("live-parity: corpus has duplicate URLs")
    return sites


def noise_indices(n: int, count: int) -> list[int]:
    """Evenly spaced indices for the double-render noise floor."""
    if n <= 1:
        return [0]
    return sorted({round(k * (n - 1) / (count - 1)) for k in range(count)})


def _load_rgb(path: Path) -> np.ndarray:
    return np.array(Image.open(path).convert("RGB"))


def _diag(stderr: str, pattern: re.Pattern | None = None) -> str:
    """The line that explains a run, or "" when there is nothing to explain.

    Only ever a refusal or a fatal: a clean run's stderr is the frame report's
    counters and timings, which differ every run and would make two sweeps of
    the same page produce different reports.
    """
    lines = [l.strip() for l in stderr.splitlines() if l.strip()]
    for pat in (pattern, FATAL_RE):
        if pat is None:
            continue
        hits = [l for l in lines if pat.search(l)]
        if hits:
            return hits[0][:220]
    return ""


def compare(goosie_png: Path, chromium_png: Path) -> dict:
    """Measure both shots with the fixture harness's own functions."""
    g, c = _load_rgb(goosie_png), _load_rgb(chromium_png)
    out = {"goosie_shape": [int(v) for v in g.shape[:2]],
           "chromium_shape": [int(v) for v in c.shape[:2]]}
    if g.shape != c.shape:
        return out
    diff = np.abs(g.astype(int) - c.astype(int))
    out["pct_exact"] = round(float(np.mean(np.all(diff <= parity.THRESHOLD, axis=2)) * 100), 2)
    out["ink_goosie"] = int(parity.ink_mask(g).sum())
    out["ink_chromium"] = int(parity.ink_mask(c).sum())
    worst, coverage, blanks = parity.score_regions(g, c)
    out["worst_iou"] = round(worst, 3) if worst is not None else None
    out["coverage"] = round(coverage, 3) if coverage is not None else None
    out["blank_bands"] = [[int(a), int(b)] for a, b in blanks]
    out["n_blank_bands"] = len(blanks)
    out["n_reference_bands"] = len(parity.content_bands(parity.ink_mask(c)))
    return out


def classify(caps: dict, m: dict) -> tuple[str, list[str]]:
    """Assign exactly one label, following LADDER order."""
    notes: list[str] = []
    if caps.get("fetch_error"):
        return "FETCH_FAILED", [caps["fetch_error"]]
    if caps.get("goosie_timed_out"):
        return "GOOSIE_TIMEOUT", [f"ran > {GOOSIE_TIMEOUT_S}s"]
    stderr = caps.get("goosie_stderr", "")
    if CAP_RE.search(stderr):
        return "ADMISSION_CAPPED", [f"rc={caps.get('goosie_rc')}", _diag(stderr, CAP_RE)]
    if caps.get("goosie_rc") not in (0, None):
        return "GOOSIE_CRASH", [f"rc={caps['goosie_rc']}", _diag(stderr)]
    if not caps.get("goosie_png"):
        return "GOOSIE_RENDER_MISSING", ["exit 0 but no PNG"]
    if caps.get("doc_height") is None:
        return "STRUCTURE_MISSING", ["-gate JSON had no doc_height"]
    if m.get("pct_exact") is None:
        return "DIMENSION_MISMATCH", [f"{m.get('goosie_shape')} vs {m.get('chromium_shape')}"]
    # Checked before anything attributed to the engine: where the reference
    # itself shows no content, no statement about Goosie's layout or paint is
    # supported by the data, whatever doc_height says.
    if m["ink_chromium"] < LIVE_MIN_INK:
        return "REFERENCE_EMPTY", [f"Chromium itself painted {m['ink_chromium']} ink px; "
                                   "this page cannot test the engine"]
    if caps["doc_height"] <= 0:
        if m["ink_goosie"] >= LIVE_MIN_INK:
            # Measured on nytimes.com and usatoday.com: the structure run
            # reported no document while the paint run put 268,190 ink pixels
            # on the same window. Those two claims cannot both describe one
            # page, so the sweep refuses to blame layout for it.
            return "STRUCTURE_DISAGREES", [f"doc_height=0 but paint has "
                                           f"{m['ink_goosie']} ink px"]
        return "LAYOUT_EMPTY", ["doc_height<=0", f"ink {m['ink_goosie']}"]
    if m["ink_goosie"] < LIVE_MIN_INK:
        return "BLANK_OUTPUT", [f"ink {m['ink_goosie']} of {m['ink_chromium']}"]
    if m["n_blank_bands"] > LIVE_MAX_BLANK_BANDS:
        return "MISSING_REGIONS", [f"{m['n_blank_bands']} empty reference band(s)"]
    if m["coverage"] is None or m["coverage"] < LIVE_COVERAGE_OK:
        return "LOW_COVERAGE", [f"coverage {m['coverage']} < {LIVE_COVERAGE_OK}"]
    if caps.get("goosie_secs", 0) >= SCREENSHOT_DEADLINE_S - DEADLINE_TOLERANCE_S:
        return "NOT_SETTLED", [f"screenshot ran {caps['goosie_secs']:.1f}s "
                               f"against the hardcoded {SCREENSHOT_DEADLINE_S:.0f}s deadline"]
    return "OK", notes


def measure_site(entry: dict, renderers, artifacts: Path, tag: str) -> dict:
    """Capture both sides for one URL and return the report row."""
    url = entry["url"]
    slug = re.sub(r"[^A-Za-z0-9._-]+", "_", url[len("https://"):])[:80]
    chromium_png = artifacts / f"{tag}chromium-{slug}.png"
    goosie_png = artifacts / f"{tag}goosie-{slug}.png"

    caps = {
        "fetch_error": renderers.chromium(url, chromium_png),
        "goosie_rc": None, "goosie_stderr": "", "goosie_secs": 0.0,
        "goosie_timed_out": False, "doc_height": None,
    }
    rc, secs, stderr, timed_out = renderers.goosie(url, goosie_png)
    caps.update({"goosie_rc": rc, "goosie_secs": round(secs, 2),
                 "goosie_stderr": stderr, "goosie_timed_out": timed_out})
    doc_height, gate_error = renderers.structure(url)
    caps["doc_height"] = doc_height
    if gate_error:
        caps["gate_error"] = gate_error

    m = compare(goosie_png, chromium_png) if (
        goosie_png.exists() and chromium_png.exists()) else {}
    if goosie_png.exists():
        caps["goosie_png"] = str(goosie_png)
    label, notes = classify(caps, m)
    row = {
        "url": url, "category": entry.get("category", ""),
        "expect": entry.get("expect", ""), "label": label, "notes": notes,
        "chromium_secs": round(renderers.last_chromium_secs, 2),
        "goosie_secs": caps["goosie_secs"],
        "doc_height": caps["doc_height"],
        "goosie_rc": caps["goosie_rc"],
        "goosie_diag": _diag(stderr, CAP_RE) or _diag(stderr),
        "structure_diag": gate_error,
        "maybe_partial": label == "NOT_SETTLED",
    }
    row.update({k: v for k, v in m.items() if k != "blank_bands"})
    row["blank_bands"] = m.get("blank_bands", [])
    row["fetch_error"] = caps["fetch_error"] or ""
    return row


class LiveRenderers:
    """Real captures: Playwright for the reference, the goosie binary for the run."""

    def __init__(self, binary: str):
        from playwright.sync_api import sync_playwright
        self._pw = sync_playwright().start()
        self._browser = self._pw.chromium.launch(args=["--force-device-scale-factor=1"])
        self._page = self._browser.new_page(viewport={"width": VIEWPORT_W, "height": VIEWPORT_H},
                                            device_scale_factor=DPR)
        self.binary = binary
        self.last_chromium_secs = 0.0

    def close(self) -> None:
        self._browser.close()
        self._pw.stop()

    def chromium(self, url: str, path: Path) -> str:
        t0 = time.monotonic()
        try:
            self._page.goto(url, wait_until="load", timeout=CHROMIUM_TIMEOUT_MS)
            self._page.screenshot(path=str(path), animations="disabled")
            return ""
        except Exception as exc:  # a dead host is a label, not a crash of this tool
            return f"{type(exc).__name__}: {str(exc)[:160]}"
        finally:
            self.last_chromium_secs = time.monotonic() - t0

    def _run(self, argv: list[str]) -> tuple[int, float, str, bool]:
        t0 = time.monotonic()
        try:
            proc = subprocess.run(argv, capture_output=True, text=True,
                                  timeout=GOOSIE_TIMEOUT_S)
        except subprocess.TimeoutExpired:
            return 124, time.monotonic() - t0, "timed out", True
        return proc.returncode, time.monotonic() - t0, proc.stderr, False

    def goosie(self, url: str, path: Path) -> tuple[int, float, str, bool]:
        rc, secs, stderr, timed_out = self._run([
            self.binary, "-backend", "headless", "-width", str(VIEWPORT_W),
            "-height", str(VIEWPORT_H), "-dpr", str(DPR),
            "-screenshot", "-url", url, "-out", str(path)])
        return rc, secs, stderr, timed_out

    def structure(self, url: str) -> tuple[int | None, str]:
        with tempfile.NamedTemporaryFile(suffix=".json") as fh:
            rc, _, stderr, _ = self._run([
                self.binary, "-backend", "headless", "-width", str(VIEWPORT_W),
                "-height", str(VIEWPORT_H), "-dpr", str(DPR),
                "-gate", "-frames", str(GATE_FRAMES), "-url", url, "-out", fh.name])
            if rc != 0:
                return None, (stderr or f"rc={rc}")[:200]
            try:
                report = json.loads(Path(fh.name).read_text())["report"]
            except Exception as exc:
                return None, f"gate JSON: {type(exc).__name__}"
        return int(report.get("doc_height", -1)), ""


def noise_floor(rows: list[dict], seconds: list[dict]) -> dict:
    """Spread between two full renders of the same pages, both sides included."""
    by_url = {r["url"]: r for r in rows}
    deltas_pct, deltas_cov, detail = [], [], []
    for second in seconds:
        first = by_url[second["url"]]
        dp = dc = None
        if first.get("pct_exact") is not None and second.get("pct_exact") is not None:
            dp = round(abs(first["pct_exact"] - second["pct_exact"]), 2)
            deltas_pct.append(dp)
        if first.get("coverage") is not None and second.get("coverage") is not None:
            dc = round(abs(first["coverage"] - second["coverage"]), 4)
            deltas_cov.append(dc)
        detail.append({"url": second["url"], "delta_pct": dp, "delta_coverage": dc,
                       "label_changed": first["label"] != second["label"]})
    return {
        "sites": len(seconds),
        "comparable": len(deltas_pct),
        "max_abs_delta_pct": max(deltas_pct) if deltas_pct else None,
        "max_abs_delta_coverage": max(deltas_cov) if deltas_cov else None,
        "labels_that_flipped": sum(1 for d in detail if d["label_changed"]),
        "detail": detail,
    }


def write_reports(rows: list[dict], counts: dict, scored: int, run_id: str,
                  floor: dict, out_dir: Path, corpus_name: str) -> tuple[Path, Path]:
    floor = floor or {"sites": 0, "comparable": 0, "max_abs_delta_pct": None,
                      "max_abs_delta_coverage": None, "labels_that_flipped": 0,
                      "detail": []}
    payload = {
        "run_id": run_id,
        "tool": "testdata/live-parity.py",
        "corpus": corpus_name,
        "contract": {
            "viewport": [VIEWPORT_W, VIEWPORT_H], "dpr": DPR,
            "chromium": {"wait_until": "load", "timeout_ms": CHROMIUM_TIMEOUT_MS,
                         "animations": "disabled", "full_page": False},
            "goosie": {"screenshot_floor_frames": 8,
                       "deadline_seconds_hardcoded": SCREENSHOT_DEADLINE_S,
                       "structure_mode": ["-gate", "-frames", str(GATE_FRAMES)],
                       "scrolls": False},
            "spacing_ms": int(SPACING_S * 1000),
        },
        "thresholds": {"coverage_ok": LIVE_COVERAGE_OK, "min_ink": LIVE_MIN_INK,
                       "max_blank_bands": LIVE_MAX_BLANK_BANDS,
                       "pixel_threshold": parity.THRESHOLD,
                       "min_scored": MIN_SCORED},
        "pages_in_corpus": len(rows),
        "pages_scored": scored,
        "floor_met": scored >= MIN_SCORED,
        "counts": counts,
        "noise_floor": floor,
        "rows": rows,
    }
    out_dir.mkdir(parents=True, exist_ok=True)
    json_path = out_dir / f"{run_id}.json"
    json_path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n")
    md_path = out_dir / f"{run_id}.md"
    md_path.write_text(render_markdown(payload))
    return json_path, md_path


def render_markdown(payload: dict) -> str:
    rows = payload["rows"]
    lines = [
        f"# Live render parity — {payload['run_id']}",
        "",
        f"Corpus `{payload['corpus']}`: **{payload['pages_in_corpus']} pages**, of which "
        f"**{payload['pages_scored']} produced a two-sided comparison** "
        f"(floor {payload['thresholds']['min_scored']}"
        f", {'met' if payload['floor_met'] else 'NOT met — the run is broken'}).",
        "",
        "Viewport-only on both sides; Goosie's screenshot driver never scrolls. "
        "This is a report, not a gate — see docs/tasks/TASK-038-LIVE-WBSITE-PARITY.md.",
        "",
        "## Noise floor (same pages rendered twice)",
        "",
        f"- {payload['noise_floor']['sites']} sites, {payload['noise_floor']['comparable']} comparable: "
        f"max |Δpct_exact| = {payload['noise_floor']['max_abs_delta_pct']}, "
        f"max |Δcoverage| = {payload['noise_floor']['max_abs_delta_coverage']}, "
        f"{payload['noise_floor']['labels_that_flipped']} label flips.",
        "No conclusion below may be stated unless it clears this spread.",
        "",
        "## Labels",
        "",
        "| label | pages |",
        "| --- | --- |",
    ]
    for label in LADDER:
        if payload["counts"].get(label):
            lines.append(f"| `{label}` | {payload['counts'][label]} |")
    lines += ["", "## By category", "",
              "| category | OK | other labels |",
              "| --- | --- | --- |"]
    for cat in sorted({r["category"] for r in rows}):
        sub = [r for r in rows if r["category"] == cat]
        other = {}
        for r in sub:
            if r["label"] != "OK":
                other[r["label"]] = other.get(r["label"], 0) + 1
        other_txt = ", ".join(f"`{k}`×{v}" for k, v in sorted(other.items())) or "—"
        lines.append(f"| {cat} | {sum(1 for r in sub if r['label'] == 'OK')} | {other_txt} |")
    lines += ["", "## Per page", "",
              "| url | label | coverage | blank bands | pct_exact | ink G/C | doc_height | s |",
              "| --- | --- | --- | --- | --- | --- | --- | --- |"]
    for r in sorted(rows, key=lambda x: (LADDER.index(x["label"]), x["url"])):
        lines.append(
            f"| {r['url']} | `{r['label']}` | {r.get('coverage', '—')} | "
            f"{r.get('n_blank_bands', '—')} | {r.get('pct_exact', '—')} | "
            f"{r.get('ink_goosie', '—')}/{r.get('ink_chromium', '—')} | "
            f"{r.get('doc_height', '—')} | {r.get('goosie_secs', '—')} |")
    lines.append("")
    return "\n".join(lines)


def run_sweep(sites: list[dict], renderers, artifacts: Path,
              verbose: bool = True) -> tuple[list[dict], list[dict]]:
    rows, noise_rows = [], []
    noise = set(noise_indices(len(sites), 12))

    def say(msg: str) -> None:
        if verbose:
            print(msg, flush=True)

    for i, entry in enumerate(sites):
        rows.append(measure_site(entry, renderers, artifacts, ""))
        say(f"[{i + 1}/{len(sites)}] {rows[-1]['label']:<20} {entry['url']}")
        if i in noise:
            time.sleep(SPACING_S)
            second = measure_site(entry, renderers, artifacts, "rerender-")
            say(f"        rerender {second['label']:<20} {entry['url']}")
            noise_rows.append(second)
        time.sleep(SPACING_S)
    return rows, noise_rows


def exit_code(counts: dict, scored: int) -> int:
    """2 = the sweep could not measure, 1 = at least one page is not OK, 0 = clean."""
    if scored < MIN_SCORED:
        return 2
    return 1 if scored != counts.get("OK", 0) or len(counts) > 1 else 0


# --- selftest --------------------------------------------------------------
# Drives the same classify/aggregate/report code with stub renderers and
# synthetic images, so the harness itself is under test without a network.
# Each case must be able to kill a specific behaviour; cases that could not be
# detected by any assertion would be the vacuous-oracle failure this repo
# already tracks as [R-7].

def _synth(bands: list[int], h: int = VIEWPORT_H, w: int = VIEWPORT_W,
           thickness: int = 8) -> np.ndarray:
    """White page with dark horizontal bars at the given y offsets."""
    arr = np.full((h, w, 3), 255, dtype=np.uint8)
    for y in bands:
        arr[y:y + thickness, 100:1100] = 40
    return arr


def _save(arr: np.ndarray, path: Path) -> Path:
    Image.fromarray(arr).save(path)
    return path


class StubRenderers:
    def __init__(self, plan: dict, artifacts: Path):
        self.plan, self.artifacts = plan, artifacts
        self.last_chromium_secs = 0.0

    def chromium(self, url: str, path: Path) -> str:
        case = self.plan[url]
        if case.get("fetch_error"):
            return case["fetch_error"]
        _save(case["chromium"], path)
        return ""

    def goosie(self, url: str, path: Path) -> tuple[int, float, str, bool]:
        case = self.plan[url]
        rc = case.get("rc", 0)
        if rc != 0 or not case.get("painted", True) or "goosie" not in case:
            return rc, case.get("secs", 1.0), case.get("stderr", ""), False
        _save(case["goosie"], path)
        return 0, case.get("secs", 1.0), case.get("stderr", ""), False

    def structure(self, url: str) -> tuple[int | None, str]:
        case = self.plan[url]
        if "doc_height" not in case:
            return None, "stub: no gate JSON"
        return case["doc_height"], ""


def selftest() -> int:
    failures: list[str] = []

    def check(name: str, condition: bool, detail: str = "") -> None:
        print(f"  {'PASS' if condition else 'FAIL'}  {name}{'' if condition else f' — {detail}'}")
        if not condition:
            failures.append(name)

    tmp = Path(tempfile.mkdtemp(prefix="live-parity-selftest-"))
    try:
        reference = _synth([100, 300, 500, 700])
        cases = {
            # 1. Goosie paints almost nothing where Chromium painted a page.
            "blank": {"chromium": reference, "goosie": _synth([]), "doc_height": 1694},
            # 2. Goosie's shot is a different window: cropping must not read as OK.
            "cropped": {"chromium": reference,
                        "goosie": _synth([100, 300], h=VIEWPORT_H // 2), "doc_height": 1024},
            # 3. Layout produced nothing and nothing was painted: apnews-class.
            "layout_empty": {"chromium": reference, "goosie": _synth([]),
                             "doc_height": 0},
            # 4. Exit 0 and no PNG at all.
            "missing": {"chromium": reference, "painted": False, "doc_height": 1024},
            # 5. Nonzero exit must take the crash branch, not the coverage branch.
            "crash": {"chromium": reference, "goosie": reference.copy(), "rc": 1,
                      "stderr": "goosie: goosie: build session: image decode failed"},
            # 6. Three of four bands present: usable-looking, one region empty.
            "regions": {"chromium": reference, "goosie": _synth([100, 300, 500, 900]),
                        "doc_height": 1024},
            # 7. Everything on: the only OK in this corpus. Its stderr is what a
            #    real paced run prints - counters and timings, which differ every
            #    run and must never reach the report.
            "clean": {"chromium": reference, "goosie": reference.copy(), "doc_height": 1024,
                      "stderr": "goosie: run backend=headless frames=8 elapsed_s=1.23\n"
                                "goosie: timings mean_ms=1.234 p50_ms=1.100 max_ms=3.400\n"},
            # 8. A dead host is a label, never an exception out of the sweep.
            "dead": {"fetch_error": "Error: net::ERR_NAME_NOT_RESOLVED"},
            # 9. Chromium itself paints nothing: the page cannot test the engine,
            #    so it must not be folded into a percentage either.
            "ref_empty": {"chromium": _synth([]), "goosie": _synth([]), "doc_height": 1024},
            # 10. A slow page that hit the hard-coded deadline is not a clean pass.
            "slow": {"chromium": reference, "goosie": reference.copy(),
                     "doc_height": 1024, "secs": SCREENSHOT_DEADLINE_S},
            # 11. A refusal that also exits 1 (measured on cafef.vn) is the
            #     admission family, not the crash family: the page was declined.
            "capped": {"chromium": reference, "goosie": _synth([]), "rc": 1,
                       "stderr": "goosie: goosie: build session: CSS numeric value "
                                 "outside safe geometry range"},
            # 12. nytimes/usatoday: the structure probe and the paint disagree,
            #     so the sweep must not blame layout.
            "disagrees": {"chromium": reference, "goosie": reference.copy(),
                          "doc_height": 0},
        }
        want = {
            "blank": "BLANK_OUTPUT", "cropped": "DIMENSION_MISMATCH",
            "layout_empty": "LAYOUT_EMPTY", "missing": "GOOSIE_RENDER_MISSING",
            "crash": "GOOSIE_CRASH", "regions": "MISSING_REGIONS",
            "clean": "OK", "dead": "FETCH_FAILED", "ref_empty": "REFERENCE_EMPTY",
            "slow": "NOT_SETTLED", "capped": "ADMISSION_CAPPED",
            "disagrees": "STRUCTURE_DISAGREES",
        }
        names = list(want)
        sites = [{"url": f"https://{n}.test/", "category": "selftest", "expect": "static"}
                 for n in names]
        plan = {f"https://{n}.test/": cases[n] for n in names}

        rows, _ = run_sweep(sites, StubRenderers(plan, tmp), tmp, verbose=False)
        by = dict(zip(names, rows))

        print("ladder: each measured condition must land in exactly one label")
        for n in names:
            check(f"{n} -> {want[n]}", by[n]["label"] == want[n], by[n]["label"])

        print("structure: nothing is skipped, and unmeasurable pages are not scored")
        check("one row per URL, no skips", len(rows) == len(sites) == len(by))
        scored = sum(1 for r in rows if r["label"] in SCORED_LABELS)
        check("scored counts only two-sided comparisons", scored == 6, f"scored={scored}")
        check("crash carries no coverage number", by["crash"].get("coverage") is None)
        check("deadline page is flagged maybe_partial", by["slow"]["maybe_partial"] is True)
        check("a refusal that exits 1 outranks both crash and blank-paint",
              by["capped"]["label"] == "ADMISSION_CAPPED" and by["capped"]["goosie_rc"] == 1)
        check("the refusal that caused it is named in the report",
              "outside safe geometry range" in by["capped"]["goosie_diag"],
              by["capped"]["goosie_diag"])
        check("a crash names its fatal line too",
              "image decode failed" in by["crash"]["goosie_diag"], by["crash"]["goosie_diag"])
        check("a clean run records no diagnostic (timings would break reproducibility)",
              by["clean"]["goosie_diag"] == "", by["clean"]["goosie_diag"])
        check("doc_height=0 against a painted page is not blamed on layout",
              by["disagrees"]["label"] == "STRUCTURE_DISAGREES"
              and "doc_height<=0" not in by["disagrees"]["notes"][0])

        mini = [r for r in rows if r["label"] in ("OK", "MISSING_REGIONS", "BLANK_OUTPUT")]
        mcounts: dict[str, int] = {}
        for r in mini:
            mcounts[r["label"]] = mcounts.get(r["label"], 0) + 1
        mscored = sum(1 for r in mini if r["label"] in SCORED_LABELS)
        check("OK + MISSING_REGIONS + BLANK_OUTPUT -> nonzero exit, all 3 rows reported",
              exit_code(mcounts, mscored) != 0 and len(mini) == 3,
              f"exit={exit_code(mcounts, mscored)} rows={len(mini)}")
        check("a full-corpus all-OK run is the only exit 0",
              exit_code({"OK": MIN_SCORED}, MIN_SCORED) == 0)
        check("the floor outranks cleanliness: 6 OK pages is an unmeasured run (exit 2)",
              exit_code({"OK": 6}, 6) == 2)
        check("below MIN_SCORED the tool calls itself broken (exit 2), "
              "not a low percentage", exit_code(mcounts, MIN_SCORED - 1) == 2)

        print("noise floor: a re-render delta is reported, never treated as a result")
        seconds = [dict(r, pct_exact=r.get("pct_exact"), coverage=r.get("coverage"))
                   for r in rows if r["label"] == "OK"]
        for r in seconds:
            r["coverage"] = round((r["coverage"] or 0) + 0.02, 3)
        floor = noise_floor(rows, seconds)
        check("delta between two renders of the same page is measured",
              floor["comparable"] == 1 and floor["max_abs_delta_coverage"] == 0.02,
              json.dumps(floor))

        print("determinism: same inputs, identical JSON — and only run_id differs per run")
        first, second = tmp / "first", tmp / "second"
        first.mkdir()
        second.mkdir()
        r1, _ = run_sweep(sites, StubRenderers(plan, first), first, verbose=False)
        r2, _ = run_sweep(sites, StubRenderers(plan, second), second, verbose=False)
        c1 = {r["label"]: 1 for r in r1}
        j1 = write_reports(r1, c1, scored, "SELFTEST-R", {}, first, "selftest.json")[0]
        j2 = write_reports(r2, c1, scored, "SELFTEST-R", {}, second, "selftest.json")[0]
        a, b = json.loads(j1.read_text()), json.loads(j2.read_text())
        check("two runs into different artifact dirs produce identical JSON "
              "(a leaked PNG path would break this)", a == b, "rows differ")
        check("the report names its own run", a.get("run_id") == "SELFTEST-R")
        check("artifact dirs are not the report's addressable output",
              j1.parent != j2.parent and j1.name == j2.name)

        print(f"\n{len(failures)} selftest failure(s)" if failures else "\nselftest green")
        return 1 if failures else 0
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


# --- CLI -------------------------------------------------------------------

def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--corpus", default=str(HERE / "live-sites.json"))
    ap.add_argument("--binary", default=shutil.which("goosie") or "goosie",
                    help="goosie build to measure; required for a real run")
    ap.add_argument("--out-dir", default=str(REPO / "docs" / "qa-live-parity"))
    ap.add_argument("--artifacts", default="",
                    help="where to keep the PNGs (default: a temp dir, deleted on exit)")
    ap.add_argument("--limit", type=int, default=0, help="measure the first N sites only")
    ap.add_argument("--only-category", default="", help="restrict the sweep to one category")
    ap.add_argument("--only", action="append", default=[], metavar="SUBSTRING",
                    help="measure only URLs containing this substring (repeatable)")
    ap.add_argument("--selftest", action="store_true",
                    help="drive the classifier with stub renderers; needs no network")
    args = ap.parse_args()

    if args.selftest:
        return selftest()

    sites = load_corpus(Path(args.corpus))
    if args.only_category:
        sites = [s for s in sites if s["category"] == args.only_category]
    for needle in args.only:
        sites = [s for s in sites if needle in s["url"]]
    if args.limit:
        sites = sites[:args.limit]
    if not Path(args.binary).exists() and not shutil.which(args.binary):
        print(f"live-parity: no goosie binary at {args.binary}; build it first",
              file=sys.stderr)
        return 2

    keep = bool(args.artifacts)
    artifacts = Path(args.artifacts or tempfile.mkdtemp(prefix="goosie-live-"))
    artifacts.mkdir(parents=True, exist_ok=True)
    sys.path.insert(0, str(HERE))
    renderers = LiveRenderers(args.binary)
    try:
        rows, noise_rows = run_sweep(sites, renderers, artifacts)
    finally:
        renderers.close()
        if not keep:
            shutil.rmtree(artifacts, ignore_errors=True)

    counts = {}
    for r in rows:
        counts[r["label"]] = counts.get(r["label"], 0) + 1
    scored = sum(1 for r in rows if r["label"] in SCORED_LABELS)
    run_id = time.strftime("%Y-%m-%dT%H%MZ", time.gmtime()) + "-" + _sha8()
    json_path, md_path = write_reports(rows, counts, scored, run_id,
                                       noise_floor(rows, noise_rows),
                                       Path(args.out_dir), Path(args.corpus).name)

    print(f"\nscored {scored}/{len(rows)} pages (floor {MIN_SCORED})")
    for label in LADDER:
        if counts.get(label):
            print(f"  {label:<20} {counts[label]}")
    print(f"report: {md_path}\njson:   {json_path}")
    code = exit_code(counts, scored)
    if code == 2:
        print("live-parity: too few pages compared — this run is broken, "
              "its percentages are meaningless", file=sys.stderr)
    return code


def _sha8() -> str:
    try:
        return subprocess.run(["git", "-C", str(REPO), "rev-parse", "--short=8", "HEAD"],
                              capture_output=True, text=True).stdout.strip() or "unknown"
    except Exception:
        return "unknown"


if __name__ == "__main__":
    sys.exit(main())
