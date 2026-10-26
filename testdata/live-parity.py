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

Two things this file's own history got wrong, and now says out loud
(`docs/tasks/TASK-045-LIVE-LABEL-SEMANTICS.md`):

* GOOSIE-LIVE-004 — the last rung of the ladder used to be called `OK`, and it
  fired on ink, blank bands and an ink-mass ratio only. It never read
  `pct_exact` or `worst_iou`, so a live page whose pixels almost entirely
  disagreed was printed under a goodness word. The rung is now `UNFLAGGED`:
  nothing above it fired, which is a statement about the ladder, not about the
  render. Its row note and the report's per-page table carry the two numbers it
  used to ignore, plus a distribution section. No threshold moved and the exit
  code is unchanged — inventing a pass mark after seeing the data is forbidden
  (TASK-038 §2.4, §6.5).
* GOOSIE-LIVE-008 — `run_id` names the commit that was HEAD when the report was
  *written*, which a sweep running across a merge makes a different commit from
  the one whose pixels are in the table. A `build` block captured at the start
  of the run (source SHA, binary path, binary SHA-256) now travels in the JSON
  and heads the markdown, and the report warns when the two SHAs differ.
"""

from __future__ import annotations

import argparse
import hashlib
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
# reported as NOT_SETTLED instead of TERMINAL_LABEL.
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
#
# What they decide is *which rung fires*, never whether the render agrees — that
# is why the terminal rung is named TERMINAL_LABEL below instead of `OK`, and
# why the report prints pct_exact and worst_iou beside it (GOOSIE-LIVE-004).
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
#
# The last rung is a name, not a verdict. It used to be `OK`, which a reader
# (and a label-count table) takes as "this render agrees". Nothing above it
# compares pixels: it fires on ink, blank bands and coverage, and coverage is
# the ink-MASS ratio goosie_total / chromium_total, so it rises past 1 when
# Goosie simply prints more than the reference. Measured in
# docs/qa-live-parity/2026-10-25T2045Z-ec0f7fb9.json: 17 of its 44 terminal rows
# report pct_exact below 50 (www.target.com 0.24, w3.org/Style/CSS/ 0.63,
# tuoitre.vn 0.84, plo.vn 1.96, nld.com.vn 3.6, ebay.com 6.4) and 28 of all rows
# report coverage above 1.0, the worst being w3.org/Style/CSS/ at 2.834 with
# 0.63% of pixels agreeing. `worst_iou` was already written into every row and
# never read. See GOOSIE-LIVE-004; the fix is the name, the note and a
# distribution section, not a new pass mark.
TERMINAL_LABEL = "UNFLAGGED"
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
    TERMINAL_LABEL,          # nothing above fired; no claim about the pixels
]
# Labels where both sides exist as comparable images, i.e. the sweep actually
# measured the engine. Everything above it in the ladder is a run that did not
# produce a comparison and must not be folded into a percentage.
SCORED_LABELS = {
    "LAYOUT_EMPTY", "STRUCTURE_DISAGREES", "BLANK_OUTPUT", "MISSING_REGIONS",
    "LOW_COVERAGE", "NOT_SETTLED", TERMINAL_LABEL,
}

# Keys every row carries, whatever the label, so the integrator's cross-sweep
# diff (by url/label/pct_exact/coverage/doc_height) never loses a page to a
# missing key. Renaming or removing one of these is a breaking change to
# docs/qa-live-parity/*.json; adding is fine. An unmeasured page reports its
# measurements as null rather than as an absent field.
MEASURED_KEYS = ("pct_exact", "ink_goosie", "ink_chromium", "worst_iou",
                 "coverage", "n_blank_bands", "n_reference_bands",
                 "goosie_shape", "chromium_shape")
STABLE_ROW_KEYS = ("url", "label", "notes", "doc_height") + MEASURED_KEYS
# The heading of the report section that sorts the terminal-labelled pages by
# pct_exact. Its wording is load-bearing: a reader must not mistake a decade
# cut for a pass mark, so the heading itself says what the table is not.
DECADE_SECTION_TITLE = ("Terminal-labelled pages by pct_exact decade — "
                        "a distribution, not a gate")


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


def _diag(stderr: str, pattern: re.Pattern | None = None, fallback: bool = False) -> str:
    """The line that explains a run, or "" when there is nothing to explain.

    Only ever a refusal or a fatal: a clean run's stderr is the frame report's
    counters and timings, which differ every run and would make two sweeps of
    the same page produce different reports. A crash row passes fallback=True
    so the reason survives even when it matches none of the patterns: on sweep
    10 target.com answered with something goosie downloaded instead of
    rendered, and the only explanatory line, "goosie: downloaded to ...", was
    dropped, leaving GOOSIE_CRASH with rc=1 and an empty note.
    """
    lines = [l.strip() for l in stderr.splitlines() if l.strip()]
    for pat in (pattern, FATAL_RE):
        if pat is None:
            continue
        hits = [l for l in lines if pat.search(l)]
        if hits:
            return hits[0][:220]
    if fallback and lines:
        return lines[0][:220]
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
    if caps.get("fetch_error"):
        return "FETCH_FAILED", [caps["fetch_error"]]
    if caps.get("goosie_timed_out"):
        return "GOOSIE_TIMEOUT", [f"ran > {GOOSIE_TIMEOUT_S}s"]
    stderr = caps.get("goosie_stderr", "")
    if CAP_RE.search(stderr):
        return "ADMISSION_CAPPED", [f"rc={caps.get('goosie_rc')}", _diag(stderr, CAP_RE)]
    if caps.get("goosie_rc") not in (0, None):
        return "GOOSIE_CRASH", [f"rc={caps['goosie_rc']}", _diag(stderr, fallback=True)]
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
    # The terminal rung. It reports what the ladder did NOT test instead of
    # letting the reader assume it passed: pixel agreement and band position.
    # No threshold here, deliberately -- see GOOSIE-LIVE-004.
    return TERMINAL_LABEL, [
        "no rung above fired; this is not a claim that the render agrees",
        f"pct_exact {m['pct_exact']}% of pixels match within {parity.THRESHOLD}/channel, "
        f"worst_iou {m['worst_iou']} is the weakest reference-band overlap",
        f"coverage {m['coverage']} is ink mass (goosie/chromium), not position, "
        f"and {m['n_blank_bands']} of {m['n_reference_bands']} reference bands are empty",
    ]


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
    diag = _diag(stderr, CAP_RE) or _diag(stderr)
    if not diag and caps["goosie_rc"] not in (0, None):
        diag = _diag(stderr, fallback=True)  # a crash's reason may match nothing
    row = {
        "url": url, "category": entry.get("category", ""),
        "expect": entry.get("expect", ""), "label": label, "notes": notes,
        "chromium_secs": round(renderers.last_chromium_secs, 2),
        "goosie_secs": caps["goosie_secs"],
        "doc_height": caps["doc_height"],
        "goosie_rc": caps["goosie_rc"],
        "goosie_diag": diag,
        "structure_diag": gate_error,
        "maybe_partial": label == "NOT_SETTLED",
    }
    row.update({k: v for k, v in m.items() if k != "blank_bands"})
    row["blank_bands"] = m.get("blank_bands", [])
    row["fetch_error"] = caps["fetch_error"] or ""
    # A page with no two-sided comparison used to simply have no pct_exact /
    # coverage / worst_iou key at all (measured: 7 of the 100 rows in sweep
    # 2026-10-25T2045Z-ec0f7fb9), which makes any diff keyed on those fields
    # skip rows silently. The keys are now always present; absence is null.
    for k in MEASURED_KEYS:
        row.setdefault(k, None)
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
        # When a site answers with a download (target.com did on sweep 10; a
        # local Content-Disposition: attachment repro shows the shape), goosie
        # saves the body instead of rendering it. Without a private directory,
        # a sweep drops that body into ~/Downloads (six copies found) and the
        # run exits 1 under a note that said nothing about why.
        self._download_dir = Path(tempfile.mkdtemp(prefix="live-parity-dl-"))

    def close(self) -> None:
        self._browser.close()
        self._pw.stop()
        shutil.rmtree(self._download_dir, ignore_errors=True)

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
            "-download-dir", str(self._download_dir),
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


def _git_rev(abbrev: int = 0) -> str:
    """HEAD of this tree, full or abbreviated; "" when git cannot answer.

    `--short=N` is not a query by itself: git needs the revision to abbreviate,
    so HEAD goes with it in both branches. A sweep that cannot name its build is
    a sweep that cannot be compared with the next one (GOOSIE-LIVE-008), so an
    empty return here has to mean "git really cannot answer", not "typo".
    """
    args = ["HEAD"] if abbrev <= 0 else [f"--short={abbrev}", "HEAD"]
    try:
        proc = subprocess.run(["git", "-C", str(REPO), "rev-parse", *args],
                              capture_output=True, text=True)
    except Exception:
        return ""
    return proc.stdout.strip() if proc.returncode == 0 else ""


def _parse_go_version_m(text: str) -> dict:
    """Read the VCS stamp `go version -m` prints.

    GOOSIE-LIVE-009. `source_sha` is `git rev-parse HEAD`, which is true of a
    clean tree and a lie about the compiled content of a dirty one. Go already
    records the difference (`vcs.modified=true`, and `+dirty` on the module
    line), so the report can say which tree produced the bytes instead of
    assuming HEAD did. Missing stamps stay missing: this never invents one.
    """
    info = {}
    for raw in text.splitlines():
        line = raw.strip()
        m = re.match(r"build\s+vcs\.(revision|time|modified)=(.+)$", line)
        if m:
            key, value = m.group(1), m.group(2).strip()
            if key == "modified":
                info["vcs_modified"] = value.lower() == "true"
            else:
                info["vcs_" + key] = value
        if re.match(r"mod\s", line) and line.endswith("+dirty"):
            info.setdefault("vcs_modified", True)
    return info


def _go_build_info(path: Path) -> dict:
    try:
        proc = subprocess.run(["go", "version", "-m", str(path)],
                              capture_output=True, text=True)
    except Exception:
        return {}
    if proc.returncode != 0:
        return {}
    return _parse_go_version_m(proc.stdout)


def capture_build(binary: str) -> dict:
    """Identify the artifact whose pixels this sweep is about to measure.

    GOOSIE-LIVE-008. `run_id` names the commit that was HEAD when the report was
    written, and a sweep longer than a merge therefore names a build it never
    ran: sweep 2026-10-25T2045Z-ec0f7fb9 measured the `bin/goosie` built at
    2026-10-25T20:31:52Z from `6071b8b`, while main moved through `0cefc1d`,
    `bcfab51` and `ec0f7fb` before the file appeared at 20:45Z. Everything in
    this block is captured ONCE, at the start of the run, before the first page
    is fetched: the source SHA the tree points at, the binary path, and the hash
    of the bytes that will produce every PNG. The binary hash is the load-bearing
    one -- it identifies the artifact even when the tree moved underneath it.

    GOOSIE-LIVE-009. A binary built from a dirty tree carries HEAD's revision
    plus `vcs.modified=true`. The revision alone then names a commit that was
    never compiled, so the report must carry the flag or a reader will attribute
    in-progress edits to a released SHA.
    """
    path = Path(binary)
    if not path.is_file():
        found = shutil.which(binary)
        if found:
            path = Path(found)
    try:
        digest = hashlib.sha256(path.resolve().read_bytes()).hexdigest()
    except OSError:
        digest = ""
    info = _go_build_info(path)
    return {"source_sha": _git_rev(), "source_sha8": _git_rev(8),
            "binary_path": str(path), "binary_sha256": digest,
            "vcs_revision": info.get("vcs_revision", ""),
            "vcs_modified": info.get("vcs_modified")}


def run_id_sha8(run_id: str) -> str:
    """The SHA-8 a report filename claims, or "" when it names no build at all."""
    tail = run_id.rsplit("-", 1)[-1] if "-" in run_id else ""
    return tail if re.fullmatch(r"[0-9a-f]{8}", tail or "") else ""


def pct_exact_decades(rows: list[dict], label: str = TERMINAL_LABEL) -> list[dict]:
    """Sort the terminal-labelled pages into equal-width decades of pct_exact.

    A distribution, not a gate. The 10-point cut is the shape of a percentage,
    chosen before looking at where the pages land and carrying no verdict: no
    bucket here changes a label, a threshold, or the exit code. It exists
    because `coverage` is an ink-mass ratio and the ladder never reads
    `worst_iou`, so the label alone cannot tell a reader that half the
    terminal-labelled pages agree on under 50% of their pixels
    (GOOSIE-LIVE-004, measured in docs/qa-live-parity/2026-10-25T2045Z-ec0f7fb9).
    """
    buckets: dict[int, list[dict]] = {}
    for r in rows:
        if r["label"] != label or r.get("pct_exact") is None:
            continue
        buckets.setdefault(min(9, int(r["pct_exact"] // 10)), []).append(r)
    out = []
    for k in range(10):
        # Lowest agreement first inside a decade: that is the page a reader
        # should look at, and it makes the row deterministic by (pct, url).
        got = sorted(buckets.get(k, []), key=lambda r: (r["pct_exact"], r["url"]))
        out.append({"decade": f"{k * 10}-{k * 10 + 9}" if k < 9 else "90-100",
                    "pages": len(got),
                    "lowest_url": got[0]["url"] if got else "",
                    "lowest_pct_exact": got[0]["pct_exact"] if got else None,
                    "lowest_worst_iou": got[0]["worst_iou"] if got else None,
                    "lowest_coverage": got[0]["coverage"] if got else None})
    return out


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
                  floor: dict, out_dir: Path, corpus_name: str,
                  build: dict | None = None) -> tuple[Path, Path]:
    floor = floor or {"sites": 0, "comparable": 0, "max_abs_delta_pct": None,
                      "max_abs_delta_coverage": None, "labels_that_flipped": 0,
                      "detail": []}
    # The build that produced these pixels, plus what the filename claims. Both
    # are in the JSON so a reader never has to trust the name alone
    # (GOOSIE-LIVE-008); the existing keys below keep their old meanings --
    # rows are diffed across sweeps by url/label/pct_exact/coverage/doc_height.
    build = dict(build or {})
    build["report_sha8"] = run_id_sha8(run_id)
    build["report_names_measured_build"] = bool(build.get("source_sha8")) and (
        build["report_sha8"] == build["source_sha8"])
    payload = {
        "run_id": run_id,
        "tool": "testdata/live-parity.py",
        "corpus": corpus_name,
        "build": build,
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


def build_header_lines(payload: dict) -> list[str]:
    """The lines a reader sees first: which build these pixels came from.

    Three states are worth a line each: the build is named, the filename names a
    different commit, or the harness could not identify the binary at all.
    """
    b = payload.get("build") or {}
    src8, run8 = b.get("source_sha8") or "", run_id_sha8(payload["run_id"])
    lines = [
        f"**Pixels measured from:** source `{src8 or b.get('source_sha') or 'UNKNOWN'}`"
        + (f" (`{b['source_sha']}`)" if b.get("source_sha") and src8 else "")
        + f", binary `{b.get('binary_path') or 'UNKNOWN'}`, SHA-256 "
        f"`{b.get('binary_sha256') or 'UNKNOWN'}` — captured at the start of the run, "
        "before the first page was fetched.",
    ]
    missing = [k for k in ("source_sha", "binary_path", "binary_sha256") if not b.get(k)]
    if missing:
        lines.append(f"**WARNING — this run cannot identify its own build:** `build` "
                     f"field(s) {', '.join(missing)} are empty, so every label below "
                     "comes from an unnamed binary.")
    else:
        vcs_rev = b.get("vcs_revision") or ""
        if b.get("vcs_modified") is True:
            lines.append(
                f"**WARNING — the binary was built from a tree with uncommitted "
                f"changes:** `vcs.modified=true`, so `{src8 or 'the source SHA'}` names "
                "the commit the tree pointed at, not the compiled content"
                + (f" (Go stamped `{vcs_rev[:8]}`)" if vcs_rev else "")
                + ". Labels in this report are not attributable to that commit; "
                "rebuild from a clean tree before comparing this sweep with another "
                "(GOOSIE-LIVE-009).")
        elif vcs_rev and b.get("source_sha") and vcs_rev != b["source_sha"]:
            lines.append(
                f"**WARNING — the binary was built from a different commit than the "
                f"tree's HEAD:** Go stamped `{vcs_rev[:8]}`, HEAD is `{src8}`. The "
                "labels below describe the binary, not this tree (GOOSIE-LIVE-009).")
        if run8 and src8 and run8 != src8:
            lines.append(f"**WARNING — the filename names a different commit than these "
                         f"pixels:** `{payload['run_id']}` ends in `{run8}`, the HEAD when "
                         f"the report was WRITTEN, while the rows were measured with the "
                         f"build of `{src8}`. Do not attribute a label change in this file "
                         f"to `{run8}` (GOOSIE-LIVE-008).")
        elif not run8 and src8:
            lines.append(f"*Note:* the report name `{payload['run_id']}` carries no build "
                         f"SHA; the build that produced these pixels is `{src8}`.")
    return lines


def render_markdown(payload: dict) -> str:
    rows = payload["rows"]
    lines = [
        f"# Live render parity — {payload['run_id']}",
        "",
    ]
    lines += build_header_lines(payload)
    lines += [
        "",
        f"Corpus `{payload['corpus']}`: **{payload['pages_in_corpus']} pages**, of which "
        f"**{payload['pages_scored']} produced a two-sided comparison** "
        f"(floor {payload['thresholds']['min_scored']}"
        f", {'met' if payload['floor_met'] else 'NOT met — the run is broken'}).",
        "",
        "Viewport-only on both sides; Goosie's screenshot driver never scrolls. "
        "This is a report, not a gate — see docs/tasks/TASK-038-LIVE-WBSITE-PARITY.md.",
        "",
        f"`{TERMINAL_LABEL}` is the ladder's terminal rung, not a verdict: it means no "
        "rung above it fired, which the ink/blank-band/coverage checks decide. Coverage "
        "is ink MASS and may exceed 1; pixel agreement (`pct_exact`) and band position "
        f"(`worst_iou`) are printed beside the label below and sorted into decades in "
        f"“{DECADE_SECTION_TITLE}” (GOOSIE-LIVE-004).",
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
              f"| category | {TERMINAL_LABEL} | other labels |",
              "| --- | --- | --- |"]
    for cat in sorted({r["category"] for r in rows}):
        sub = [r for r in rows if r["category"] == cat]
        other = {}
        for r in sub:
            if r["label"] != TERMINAL_LABEL:
                other[r["label"]] = other.get(r["label"], 0) + 1
        other_txt = ", ".join(f"`{k}`×{v}" for k, v in sorted(other.items())) or "—"
        lines.append(f"| {cat} | {sum(1 for r in sub if r['label'] == TERMINAL_LABEL)} "
                     f"| {other_txt} |")
    lines += ["", f"## {DECADE_SECTION_TITLE}", "",
              f"Every `{TERMINAL_LABEL}` page, sorted by `pct_exact` in equal 10-point "
              f"decades. **This is a distribution, not a gate**: the cut is the shape of "
              "a percentage, no decade is a pass mark, no bucket changes a label or a "
              "threshold, and the exit code is computed from the ladder above and "
              "`min_scored` alone.", ""]
    decades = pct_exact_decades(rows)
    total_terminal = sum(d["pages"] for d in decades)
    lines += ["| pct_exact decade | pages | lowest-agreement page in the decade | "
              "its coverage | its worst_iou |",
              "| --- | --- | --- | --- | --- |"]
    for d in decades:
        if not d["pages"]:
            lines.append(f"| {d['decade']} | 0 | — | — | — |")
            continue
        lines.append(f"| {d['decade']} | {d['pages']} | {d['lowest_url']} | "
                     f"{d['lowest_coverage']} | {d['lowest_worst_iou']} |")
    below_50 = sum(d["pages"] for d in decades[:5])
    lines += ["", f"{total_terminal} page(s) carry `{TERMINAL_LABEL}`; {below_50} of them "
                  "agree on fewer than 50% of their pixels. Neither number is a verdict — "
                  "cross-browser font rasterisation makes a low `pct_exact` normal, and the "
                  "spread between two renders of the same page (noise floor above) is what "
                  "says how much of it is signal.", ""]
    lines += ["## Per page", "",
              f"| url | label | coverage | blank bands | pct_exact | worst_iou | "
              "ink G/C | doc_height | s |",
              "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"]

    def cell(r: dict, key: str) -> str:
        """Em dash for "never measured". Rows now carry the key with a null, so a
        dict default would print the word None."""
        v = r.get(key)
        return "—" if v is None else str(v)

    for r in sorted(rows, key=lambda x: (LADDER.index(x["label"]), x["url"])):
        lines.append(
            f"| {r['url']} | `{r['label']}` | {cell(r, 'coverage')} | "
            f"{cell(r, 'n_blank_bands')} | {cell(r, 'pct_exact')} | {cell(r, 'worst_iou')} | "
            f"{cell(r, 'ink_goosie')}/{cell(r, 'ink_chromium')} | "
            f"{cell(r, 'doc_height')} | {r.get('goosie_secs', '—')} |")
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
    """2 = the sweep could not measure, 1 = at least one page did not reach the
    ladder's terminal rung, 0 = every compared page reached it and nothing else
    fired. Renaming the terminal rung changes no number here: the rule is the
    same one TASK-038 shipped with, and the pct_exact distribution above feeds
    nothing into it."""
    if scored < MIN_SCORED:
        return 2
    return 1 if scored != counts.get(TERMINAL_LABEL, 0) or len(counts) > 1 else 0


# --- selftest --------------------------------------------------------------
# Drives the same classify/aggregate/report code with stub renderers and
# synthetic images, so the harness itself is under test without a network.
# Each case must be able to kill a specific behaviour; cases that could not be
# detected by any assertion would be the vacuous-oracle failure this repo
# already tracks as [R-7].

def _synth(bands: list[int], h: int = VIEWPORT_H, w: int = VIEWPORT_W,
           thickness: int = 8, cols: tuple = ((100, 1100),)) -> np.ndarray:
    """White page with dark horizontal bars at the given y offsets.

    `cols` defaults to the single span every existing case uses; a case can pass
    several spans to draw a reference whose ink EXTENT is wide (so the band
    window is wide) while its ink COUNT stays small.
    """
    arr = np.full((h, w, 3), 255, dtype=np.uint8)
    for y in bands:
        for x0, x1 in cols:
            arr[y:y + thickness, x0:x1] = 40
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
        # The measured shape of www.w3.org/Style/CSS/ (sweep
        # 2026-10-25T2045Z-ec0f7fb9: Goosie ink 1,307,000 of a 1,310,720-px
        # viewport, Chromium ink 15,188, coverage 2.834, pct_exact 0.63, label
        # `OK`): a sparse reference whose ink EXTENT spans the page, and a Goosie
        # shot that inks everything else. Every rung above the terminal one is
        # satisfied -- reference ink clears MIN_INK, no reference band is empty,
        # the ink-MASS ratio is huge -- and not one pixel agrees.
        sparse = _synth([100, 300], cols=((100, 120), (1080, 1100)))
        overlay = np.full((VIEWPORT_H, VIEWPORT_W, 3), 40, dtype=np.uint8)
        for y in (100, 300):
            for x0, x1 in ((100, 120), (1080, 1100)):
                overlay[y:y + 8, x0:x1] = 255
        cases = {
            # 1. Goosie paints almost nothing where Chromium painted a page.
            "blank": {"chromium": reference, "goosie": _synth([]), "doc_height": 1694},
            # 2. Goosie's shot is a different window: cropping must not reach the
            #    terminal rung.
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
            # 7. Everything on: terminal-labelled, and its stderr is what a real
            #    paced run prints - counters and timings, which differ every run
            #    and must never reach the report.
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
            # 13. GOOSIE-LIVE-004: the pixels disagree completely and the ladder
            #     still has nothing to fire. This is the row that used to be
            #     printed under a goodness word; it must now be printed under a
            #     name that does not claim one, with the ignored numbers beside
            #     it. 0.0% agreement, coverage 24.0, worst_iou 0.004.
            "solid_overlay": {"chromium": sparse, "goosie": overlay, "doc_height": 1024},
        }
        want = {
            "blank": "BLANK_OUTPUT", "cropped": "DIMENSION_MISMATCH",
            "layout_empty": "LAYOUT_EMPTY", "missing": "GOOSIE_RENDER_MISSING",
            "crash": "GOOSIE_CRASH", "regions": "MISSING_REGIONS",
            "clean": TERMINAL_LABEL, "dead": "FETCH_FAILED", "ref_empty": "REFERENCE_EMPTY",
            "slow": "NOT_SETTLED", "capped": "ADMISSION_CAPPED",
            "disagrees": "STRUCTURE_DISAGREES", "solid_overlay": TERMINAL_LABEL,
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
        check("scored counts only two-sided comparisons", scored == 7, f"scored={scored}")
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
        # 2026-10-26 target.com: a download response makes goosie save the body
        # and exit 1 with one line, "goosie: downloaded to ...", which matches no
        # fatal pattern. The line is the whole explanation.
        attachment = {"goosie_rc": 1,
                      "goosie_stderr": "goosie: downloaded to /tmp/dl/download\n"}
        a_label, a_notes = classify(attachment, {})
        check("a crash whose only line matches no fatal pattern still reports it",
              a_label == "GOOSIE_CRASH" and "downloaded to" in a_notes[-1],
              f"{a_label}: {a_notes}")
        check("doc_height=0 against a painted page is not blamed on layout",
              by["disagrees"]["label"] == "STRUCTURE_DISAGREES"
              and "doc_height<=0" not in by["disagrees"]["notes"][0])

        print("terminal rung: a name, not a verdict (GOOSIE-LIVE-004)")
        goodness = {"OK", "PASS", "PASSED", "GOOD", "FINE", "CLEAN", "GREEN",
                    "SUCCESS", "TRUE", "WORKS"}
        check("no rung of the ladder is a goodness word, and the terminal rung is "
              "the last one and still counted as scored",
              not goodness & set(LADDER) and LADDER[-1] == TERMINAL_LABEL
              and TERMINAL_LABEL in SCORED_LABELS,
              f"vocabulary={sorted(goodness & set(LADDER))} last={LADDER[-1]}")
        disagreeing = by["solid_overlay"]
        check("a page that agrees on 0.0% of its pixels reaches the terminal rung — "
              "the ladder has no rung for 'measured, and the pixels disagree', so the "
              "name must not claim otherwise",
              disagreeing["label"] == TERMINAL_LABEL and disagreeing["pct_exact"] < 1.0
              and disagreeing["coverage"] > 1.0,
              f"label={disagreeing['label']} pct={disagreeing['pct_exact']} "
              f"coverage={disagreeing['coverage']}")
        note = " ".join(disagreeing["notes"])
        check("the terminal rung's row note carries the two numbers the ladder ignores",
              f"pct_exact {disagreeing['pct_exact']}" in note
              and f"worst_iou {disagreeing['worst_iou']}" in note
              and "not a claim that the render agrees" in note, note)
        decades = pct_exact_decades(rows)
        terminal_rows = [r for r in rows if r["label"] == TERMINAL_LABEL]
        check("the decades bucket every terminal page into ten equal-width spans and "
              "no other label",
              len(decades) == 10 and [d["decade"] for d in decades][:2] == ["0-9", "10-19"]
              and decades[9]["decade"] == "90-100"
              and sum(d["pages"] for d in decades) == len(terminal_rows),
              json.dumps(decades))
        check("0% agreement and 100% agreement land in opposite decades of the same label",
              decades[0]["pages"] == 1 and decades[0]["lowest_pct_exact"] == 0.0
              and decades[9]["pages"] == 1 and decades[9]["lowest_pct_exact"] == 100.0,
              json.dumps([decades[0], decades[9]]))

        mini = [r for r in rows if r["label"] in
                (TERMINAL_LABEL, "MISSING_REGIONS", "BLANK_OUTPUT")]
        mcounts: dict[str, int] = {}
        for r in mini:
            mcounts[r["label"]] = mcounts.get(r["label"], 0) + 1
        mscored = sum(1 for r in mini if r["label"] in SCORED_LABELS)
        check(f"{TERMINAL_LABEL} + MISSING_REGIONS + BLANK_OUTPUT -> nonzero exit, "
              "all 4 rows reported",
              exit_code(mcounts, mscored) != 0 and len(mini) == 4,
              f"exit={exit_code(mcounts, mscored)} rows={len(mini)}")
        check(f"a full-corpus all-{TERMINAL_LABEL} run is the only exit 0",
              exit_code({TERMINAL_LABEL: MIN_SCORED}, MIN_SCORED) == 0)
        check("the floor outranks reaching the terminal rung: 6 pages is an unmeasured "
              "run (exit 2)", exit_code({TERMINAL_LABEL: 6}, 6) == 2)
        check("below MIN_SCORED the tool calls itself broken (exit 2), "
              "not a low percentage", exit_code(mcounts, MIN_SCORED - 1) == 2)
        check("no pass mark was invented: a sweep whose pages all agree on 0.0% of "
              "their pixels still exits 0 at the floor, and one page short of it "
              "still exits 2",
              by["solid_overlay"]["label"] == by["clean"]["label"] == TERMINAL_LABEL
              and exit_code({TERMINAL_LABEL: MIN_SCORED}, MIN_SCORED) == 0
              and exit_code({TERMINAL_LABEL: MIN_SCORED - 1}, MIN_SCORED - 1) == 2)

        print("row keys: stable for the integrator's cross-sweep diff (GOOSIE-LIVE-008)")
        keysets = {frozenset(r) for r in rows}
        check("every row carries the same keys, whatever its label",
              len(keysets) == 1 and set(STABLE_ROW_KEYS) <= set(rows[0]),
              f"{len(keysets)} key sets; missing "
              f"{sorted(set(STABLE_ROW_KEYS) - set(rows[0]))}")
        check("an unmeasured row reports null, it does not go missing",
              "pct_exact" in by["dead"] and by["dead"]["pct_exact"] is None
              and by["dead"]["coverage"] is None and by["dead"]["worst_iou"] is None,
              json.dumps({k: by["dead"].get(k) for k in MEASURED_KEYS}))

        print("noise floor: a re-render delta is reported, never treated as a result")
        seconds = [dict(r, pct_exact=r.get("pct_exact"), coverage=r.get("coverage"))
                   for r in rows if r["label"] == TERMINAL_LABEL]
        for r in seconds:
            r["coverage"] = round((r["coverage"] or 0) + 0.02, 3)
        floor = noise_floor(rows, seconds)
        check("delta between two renders of the same page is measured",
              floor["comparable"] == 2 and floor["max_abs_delta_coverage"] == 0.02,
              json.dumps(floor))

        print("determinism: same inputs, identical JSON — and only run_id differs per run")
        # What a real sweep captures at its start with capture_build(); literal
        # here so two selftest runs stay byte-identical with no git and no binary.
        build = {"source_sha": "0" * 40, "source_sha8": "00000000",
                 "binary_path": "/selftest/bin/goosie",
                 "binary_sha256": "0123456789abcdef" * 4}
        first, second = tmp / "first", tmp / "second"
        first.mkdir()
        second.mkdir()
        r1, _ = run_sweep(sites, StubRenderers(plan, first), first, verbose=False)
        r2, _ = run_sweep(sites, StubRenderers(plan, second), second, verbose=False)
        c1 = {r["label"]: 1 for r in r1}
        j1, m1 = write_reports(r1, c1, scored, "SELFTEST-R", {}, first, "selftest.json", build)
        j2, _ = write_reports(r2, c1, scored, "SELFTEST-R", {}, second, "selftest.json", build)
        a, b = json.loads(j1.read_text()), json.loads(j2.read_text())
        check("two runs into different artifact dirs produce identical JSON "
              "(a leaked PNG path would break this)", a == b, "rows differ")
        check("the report names its own run", a.get("run_id") == "SELFTEST-R")
        check("artifact dirs are not the report's addressable output",
              j1.parent != j2.parent and j1.name == j2.name)

        print("build identity: the report says which pixels it measured (GOOSIE-LIVE-008)")
        check("the JSON names the source SHA, the binary path and the binary SHA-256",
              a.get("build", {}).get("source_sha") == build["source_sha"]
              and a["build"].get("source_sha8") == build["source_sha8"]
              and a["build"].get("binary_path") == build["binary_path"]
              and a["build"].get("binary_sha256") == build["binary_sha256"],
              json.dumps(a.get("build")))
        md1 = m1.read_text().splitlines()
        check("the markdown names the measured build in its first lines, above any table",
              2 <= md1.index(next(l for l in md1 if "Pixels measured from" in l)) <
              md1.index("## Labels")
              and build["source_sha"] in md1[2] and build["binary_sha256"] in md1[2]
              and build["binary_path"] in md1[2], md1[2][:120] if len(md1) > 2 else "")
        check("a report name that carries no build SHA says so instead of implying one",
              run_id_sha8("SELFTEST-R") == ""
              and a["build"]["report_names_measured_build"] is False
              and "carries no build SHA" in m1.read_text(), a["build"])
        check("a filename that names a real SHA-8 is parsed as one",
              run_id_sha8("2026-10-25T2045Z-ec0f7fb9") == "ec0f7fb9"
              and run_id_sha8("2026-10-25T2045Z-6071b8b9") == "6071b8b9"
              and run_id_sha8("2026-10-25T2045Z-6071b8b") == ""
              and run_id_sha8("2026-10-25T2045Z-nota-sha") == "")
        named = tmp / "naming"
        named.mkdir()
        same = write_reports(r1, c1, scored, "2026-10-26T0900Z-00000000", {}, named,
                             "selftest.json", build)[1]
        moved = write_reports(r1, c1, scored, "2026-10-26T0900Z-deadbeef", {}, named,
                              "selftest.json", build)[1]
        check("a sweep that outlived a merge warns in the header that its filename "
              "names a commit that never ran",
              "WARNING" in moved.read_text() and "deadbeef" in moved.read_text()
              and "00000000" in moved.read_text()
              and moved.read_text().count("WARNING") == 1, moved.read_text()[:400])
        check("when the filename does name the measured build, no warning is printed",
              "WARNING" not in same.read_text()
              and json.loads((named / "2026-10-26T0900Z-00000000.json").read_text())
              ["build"]["report_names_measured_build"] is True,
              same.read_text()[:400])
        lost = write_reports(r1, c1, scored, "2026-10-26T0900Z-00000000", {}, named,
                             "selftest.json", {})[1]
        check("a run that cannot hash its own binary says the build is unidentified "
              "instead of leaving the reader to assume",
              "cannot identify its own build" in lost.read_text()
              and "UNKNOWN" in lost.read_text(), lost.read_text()[:400])
        check("the decade section is in the report and calls itself a distribution",
              DECADE_SECTION_TITLE in same.read_text()
              and "**This is a distribution, not a gate**" in same.read_text()
              and f"| `{TERMINAL_LABEL}` |" in same.read_text(),
              same.read_text()[:200])
        # The checks above all read the literal fixture, so none of them could see
        # the two functions that fill it in. A real sweep did: `rev-parse --short=8`
        # without a revision prints nothing, and the report came out named
        # `-unknown` with an empty source_sha8.
        full, short, name = _git_rev(), _git_rev(8), _sha8()
        check("a run in a git tree names the build's source SHA, and the abbreviated "
              "form is a real prefix of the full one — not the empty string git prints "
              "when --short has no revision to abbreviate",
              (full == "" and short == "" and name == "unknown")
              or (re.fullmatch(r"[0-9a-f]{8}", short or "")
                  and full.startswith(short) and name == short),
              f"full={full!r} short={short!r} _sha8={name!r}")
        real_bin = tmp / "fake-goosie"
        real_bin.write_bytes(b"goosie bytes the selftest can hash for real")
        got = capture_build(str(real_bin))
        check("capture_build() hashes the bytes it is given: the binary SHA-256 in the "
              "report is the SHA-256 of the file at that path",
              got["binary_sha256"] == hashlib.sha256(real_bin.read_bytes()).hexdigest()
              and got["binary_path"] == str(real_bin)
              and bool(got["source_sha"]) == bool(got["source_sha8"]),
              json.dumps(got))
        unreadable = capture_build(str(tmp / "there-is-no-such-binary"))
        check("a binary that cannot be read is recorded as unhashed, never as a guess",
              unreadable["binary_sha256"] == ""
              and unreadable["binary_path"] != "", json.dumps(unreadable))
        info = _parse_go_version_m(
            "\tpath\tgithub.com/vyquocvu/goosie/cmd/goosie\n"
            "\tmod\tgithub.com/vyquocvu/goosie\tv0.0.0-20261026054651-80a2f617b70f+dirty\n"
            "\tbuild\tvcs=git\n"
            "\tbuild\tvcs.revision=80a2f617b70f55657b884aa9de33d41e1859ccc5\n"
            "\tbuild\tvcs.modified=true\n")
        clean_info = _parse_go_version_m(
            "\tmod\texample.test/x\tv1.0.0\n"
            "\tbuild\tvcs.revision=80a2f617b70f55657b884aa9de33d41e1859ccc5\n"
            "\tbuild\tvcs.modified=false\n")
        check("Go's own VCS stamp distinguishes a dirty build from a clean one, and a "
              "binary with no stamp is unknown rather than assumed clean",
              info.get("vcs_modified") is True
              and clean_info.get("vcs_modified") is False
              and _parse_go_version_m("\tpath\tx\n").get("vcs_modified") is None,
              json.dumps([info, clean_info]))
        dirty_build = dict(build, vcs_modified=True,
                           vcs_revision=build["source_sha"])
        dirty_md = write_reports(r1, c1, scored, "2026-10-26T0901Z-00000000", {},
                                 named, "selftest.json", dirty_build)[1].read_text()
        check("a dirty-tree build is called out as not attributable to its commit, "
              "so in-progress edits are never scored as a released SHA",
              "uncommitted changes" in dirty_md
              and "not attributable" in dirty_md
              and "GOOSIE-LIVE-009" in dirty_md, dirty_md[:500])

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
    # Captured here, before the first page is fetched, so the report identifies
    # the bytes that made these PNGs instead of whatever HEAD becomes while the
    # sweep runs (GOOSIE-LIVE-008). `make live-parity` depends on `build` for the
    # same reason: the binary must already exist when this line executes.
    build = capture_build(args.binary)
    if not build["binary_sha256"]:
        print(f"live-parity: WARNING cannot read {build['binary_path']} to hash it; "
              "the report will say the build is unidentified", file=sys.stderr)
    if build.get("vcs_modified"):
        print(f"live-parity: WARNING {build['binary_path']} was built from a dirty "
              "tree; the report will not attribute these labels to its commit "
              "(GOOSIE-LIVE-009)", file=sys.stderr)

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
                                       Path(args.out_dir), Path(args.corpus).name,
                                       build)

    print(f"\nscored {scored}/{len(rows)} pages (floor {MIN_SCORED})")
    for label in LADDER:
        if counts.get(label):
            print(f"  {label:<20} {counts[label]}")
    print(f"measured build: source {build['source_sha8'] or 'UNKNOWN'} "
          f"binary {build['binary_path']} "
          f"sha256 {build['binary_sha256'][:12] or 'UNKNOWN'}…")
    print(f"report: {md_path}\njson:   {json_path}")
    code = exit_code(counts, scored)
    if code == 2:
        print("live-parity: too few pages compared — this run is broken, "
              "its percentages are meaningless", file=sys.stderr)
    return code


def _sha8() -> str:
    """HEAD when the report is WRITTEN. Used for the filename only, which stays
    `<timestamp>Z-<sha8>` for compatibility with the reports already in
    docs/qa-live-parity/; the build that produced the pixels is `build` in the
    JSON and the first lines of the markdown (GOOSIE-LIVE-008)."""
    return _git_rev(8) or "unknown"


if __name__ == "__main__":
    sys.exit(main())
