#!/usr/bin/env python3
"""Self-tests for testdata/parity.py — the render-parity oracle.

GOOSIE-PARITYGATE-001 recorded that the harness could report success while its
own evidence shrank: ``--no-strict`` deleted fixtures from the denominator, and
the known-gaps baseline was an upper bound (``<=``) so inflating it hid blank
bands.  This file proves the opposite is now true.  It runs with NO network and
NO browser: every case builds synthetic PNGs into a throwaway temp corpus and
points parity.py's module path globals at it, so the real /tmp/goosie-renders
and /tmp/playwright-renders directories are never touched.

Each case is chosen so it can *kill* a specific fraud path.  A case that would
stay green under a mutation is a vacuous oracle — the exact failure mode the
acceptance section of the task refuses to accept.  See
docs/tasks/TASK-036-PARITY-HARNESS-FAIL-CLOSED.md.

GOOSIE-PARITYTMP-001 added cases 6-8: a run's render roots can be moved off the
machine-global /tmp directories, two runs then share no file at all, and a PNG
left by an earlier run is deleted before the binary is invoked rather than being
scored as this build's output.

GOOSIE-CIPARITY-001 added case 9: the known-gaps baseline must record the host
its blank bands were measured on, and a run on a different host must say so
instead of letting "known-gap baseline is stale" read as an engine regression.

Run directly: ``python3 testdata/parity_selftest.py`` (exit 0 = green).
"""
from __future__ import annotations

import importlib.util
import inspect
import io
import json
import os
import shutil
import sys
import tempfile
from contextlib import redirect_stderr
from pathlib import Path

import numpy as np
from PIL import Image

HERE = Path(__file__).resolve().parent
PARITY = HERE / "parity.py"
REAL_GAPS = HERE / "render-known-gaps.json"

# Image geometry shared by every synthetic pair.  Bars are separated well
# beyond BAND_GAP so each becomes its own content band, and are kept small so a
# blanked goosie band still scores >90% pixel match (only the equality gate,
# never the threshold, is what these cases exercise).
_H = 1200
_W = 1000


def load_parity():
    """Import testdata/parity.py by path so the globals we mutate are exactly
    the ones score_fixtures/main read."""
    spec = importlib.util.spec_from_file_location("parity_under_test", PARITY)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _bars(n: int) -> np.ndarray:
    """White page with *n* dark content bars, each its own band."""
    arr = np.full((_H, _W, 3), 255, dtype=np.uint8)
    y = 100
    for _ in range(n):
        arr[y:y + 8, 100:900] = 40
        y += 200
    return arr


def _blank() -> np.ndarray:
    return np.full((_H, _W, 3), 255, dtype=np.uint8)


def _save(arr: np.ndarray, path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    Image.fromarray(arr).save(path)


def _mkcorpus() -> tuple[str, Path, Path, Path]:
    """Fresh (tmpdir, fixtures, goosie, chromium) tree."""
    tmp = tempfile.mkdtemp(prefix="parity-selftest-")
    fix = Path(tmp) / "render"
    goos = Path(tmp) / "goosie"
    chrom = Path(tmp) / "chromium"
    for d in (fix, goos, chrom):
        d.mkdir(parents=True, exist_ok=True)
    return tmp, fix, goos, chrom


def _fixture(tmp, fix, goos, chrom, png_name: str, n_bands: int,
             goosie_bars: int, chromium: bool = True) -> None:
    """Lay down one html + the matching goosie/chromium PNG pair.

    goosie_bars is how many of chromium's content bands goosie actually painted;
    the remaining (n_bands - goosie_bands) become measured blank bands.
    """
    html = fix / Path(png_name).with_suffix(".html")
    html.parent.mkdir(parents=True, exist_ok=True)
    html.write_text("<html><body>synthetic</body></html>")
    _save(_bars(goosie_bars), goos / png_name)
    if chromium:
        _save(_bars(n_bands), chrom / png_name)


class Harness:
    """Runs one case against a freshly-imported parity module inside a temp
    corpus, restoring every global it touched."""

    def __init__(self) -> None:
        self.mod = load_parity()
        self._tmps: list[str] = []
        m = self.mod
        self._saved = (m.FIXTURES, m.GOOSIE, m.CHROMIUM, m.KNOWN_GAPS,
                       m.load_known_gaps.__defaults__, list(sys.argv))

    def use_corpus(self, fix, goos, chrom, gaps_path: Path) -> None:
        m = self.mod
        m.FIXTURES, m.GOOSIE, m.CHROMIUM, m.KNOWN_GAPS = fix, goos, chrom, gaps_path

    def set_gaps_default(self, path: Path) -> None:
        # load_known_gaps(path=KNOWN_GAPS) binds its default at def-time, so
        # pointing main() at a specific gaps file means rewriting the default.
        self.mod.load_known_gaps.__defaults__ = (path,)

    def run_main(self, argv=("parity.py",)) -> int:
        sys.argv = list(argv)
        return self.mod.main()

    def track(self, tmp: str) -> None:
        self._tmps.append(tmp)

    def close(self) -> None:
        m = self.mod
        (m.FIXTURES, m.GOOSIE, m.CHROMIUM, m.KNOWN_GAPS,
         m.load_known_gaps.__defaults__, sys.argv) = self._saved
        for t in self._tmps:
            shutil.rmtree(t, ignore_errors=True)


FAILURES: list[str] = []


def check(name: str, cond: bool, detail: str = "") -> None:
    print(f"  {'PASS' if cond else 'FAIL'}  {name}"
          + ("" if cond else f"  -- {detail}"))
    if not cond:
        FAILURES.append(name)


def empty_gaps_file(tmp: str) -> Path:
    p = Path(tmp) / "gaps-empty.json"
    p.write_text(json.dumps({"version": 1, "known_gaps": []}))
    return p


# --- case 1: missing chromium is reported even though there is no escape hatch
def case_missing_render():
    print("case 1: a missing chromium render is always a MISSING row (BR-177)")
    h = Harness()
    try:
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        h.use_corpus(fix, goos, chrom, empty_gaps_file(tmp))
        _fixture(tmp, fix, goos, chrom, "a.png", n_bands=1, goosie_bars=1,
                 chromium=False)
        results = h.mod.score_fixtures(h.mod.THRESHOLD, h.mod.PASS_AT,
                                       known_gaps={})
        got = (results[0].name, results[0].status) if len(results) == 1 else None
        check("one MISSING FixtureResult is reported",
              got == ("a.png", "MISSING"),
              repr([tuple((r.name, r.status)) for r in results]))
        check("the missing reason is named",
              len(results) == 1 and "chromium render missing" in (results[0].error or ""),
              results[0].error if results else "no rows")
        params = inspect.signature(h.mod.score_fixtures).parameters
        check("score_fixtures no longer takes a 'strict' parameter",
              "strict" not in params, f"params={list(params)}")
        raised = False
        try:
            h.mod.score_fixtures(h.mod.THRESHOLD, h.mod.PASS_AT, strict=False)
        except TypeError:
            raised = True
        check("calling score_fixtures(strict=False) raises TypeError "
              "(the non-strict escape hatch no longer exists)", raised)
    finally:
        h.close()


# --- case 2: an inflated baseline (declared 5, measured 0) must fail
def case_stale_baseline_fails():
    print("case 2: declared 5 bands against 0 measured is FAIL, not PASS (BR-176)")
    h = Harness()
    try:
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        h.use_corpus(fix, goos, chrom, empty_gaps_file(tmp))
        # Identical all-white pair: 100% match, 0 blank bands.
        _save(_blank(), goos / "a.png")
        _save(_blank(), chrom / "a.png")
        (fix / "a.html").write_text("<html></html>")
        results = h.mod.score_fixtures(h.mod.THRESHOLD, h.mod.PASS_AT,
                                       known_gaps={"a.png": 5})
        r = results[0]
        measured = (r.name, r.status, round(r.pct, 1), list(r.blank_region_rows),
                    r.known_blank_regions, r.error)
        print(f"    measured: {measured}")
        check("status is FAIL (was PASS before the fix)", r.status == "FAIL",
              r.status)
        check("error names a stale baseline", r.error is not None
              and "stale" in r.error and "declared 5" in r.error
              and "measured 0" in r.error, r.error)
    finally:
        h.close()


# Blank band counts the real Chromium/goosie renders physically produce for each
# known-gap fixture, measured at this commit (the "before" run of
# python3 testdata/parity.py shows forms/04 2, html-features/01 1, selectors/02
# 3, selectors/03 3, unicode/02 1).  These are ground-truth *evidence*, kept
# deliberately separate from the *declared* baseline in
# render-known-gaps.json: the equality gate is only meaningful if a pass means
# "declared == measured", so an inflated declaration must fail against a fixed
# measurement.  If a real render changes one of these counts, the parity run
# itself goes red and both sides are re-measured together.
#
# selectors/03 re-measured 3 -> 2 on 2026-11-12 by BR-272: the band at rows
# (579, 599) held 0 goosie pixels before the fix and 18 after, against 3017 in
# Chromium's exact rows, so `blank == zero ink in those rows` no longer flags it.
# The drop-cap content it was counting is still missing (the drift view keeps it
# `shifted`, never `truly_blank`); only the band's physical ink count moved, and
# render-known-gaps.json moved with it in the same commit.
MEASURED = {
    "forms/04-advanced-inputs.png": 1,
    "html-features/01-forms-complete.png": 1,
    "selectors/03-pseudo-elements.png": 2,
    "unicode/02-emoji-symbols.png": 1,
}


# --- case 3: measured == declared passes; keyed off the REAL baseline file so
#     that bumping any entry in render-known-gaps.json fails this oracle.
def case_baseline_matches_passes():
    print("case 3: baseline equal to measured bands is a genuine PASS (BR-176)")
    print("        (positive oracle; driven by the real render-known-gaps.json)")
    h = Harness()
    try:
        kg = h.mod.load_known_gaps(REAL_GAPS)
        check("real baseline loaded (expected 4 entries)", len(kg) == 4,
              f"{len(kg)} entries: {kg}")
        check("baseline and measured cover the same fixtures", set(kg) == set(MEASURED),
              f"declared={sorted(kg)} measured={sorted(MEASURED)}")
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        h.use_corpus(fix, goos, chrom, empty_gaps_file(tmp))
        for name, measured in MEASURED.items():
            # chromium paints `measured` bands; goosie paints none of them, so
            # the measured blank count is a fixed physical fact.  It passes only
            # while the declared baseline still equals it.
            _fixture(tmp, fix, goos, chrom, name, n_bands=measured, goosie_bars=0)
        results = h.mod.score_fixtures(h.mod.THRESHOLD, h.mod.PASS_AT,
                                       known_gaps=kg)
        by = {r.name: r for r in results}
        check("one row per known-gap fixture", len(results) == len(MEASURED),
              f"{len(results)} rows vs {len(MEASURED)} fixtures")
        for name, measured in MEASURED.items():
            r = by.get(name)
            declared = kg.get(name)
            ok = (r is not None and r.status == "PASS"
                  and r.blank_regions == measured and r.known_blank_regions == declared)
            check(f"{name}: measured {measured} == declared {declared} -> PASS", ok,
                  "missing row" if r is None else
                  f"{r.status} measured={r.blank_regions} declared={r.known_blank_regions}")
    finally:
        h.close()


# --- case 4: a corpus below the floor is broken (main returns 2)
def case_corpus_floor():
    print("case 4: scoring fewer fixtures than MIN_SCORED_FIXTURES -> main() rc 2")
    h = Harness()
    try:
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        gaps = empty_gaps_file(tmp)
        h.use_corpus(fix, goos, chrom, gaps)
        h.set_gaps_default(gaps)
        # A single passing pair: healthy-looking, but 1 < floor.
        _save(_blank(), goos / "a.png")
        _save(_blank(), chrom / "a.png")
        (fix / "a.html").write_text("<html></html>")
        floor = h.mod.MIN_SCORED_FIXTURES
        rc = h.run_main(("parity.py",))
        check(f"main() returns 2 for 1 fixture (floor {floor})", rc == 2, f"rc={rc}")
    finally:
        h.close()


# --- case 5: a missing baseline file can never yield rc 0
def case_missing_gaps_file():
    print("case 5: a missing render-known-gaps.json never returns rc 0 (BR-177)")
    h = Harness()
    try:
        missing = HERE / "definitely-not-a-real-gaps-file.json"
        raised = False
        try:
            h.mod.load_known_gaps(missing)
        except FileNotFoundError:
            raised = True
        check("load_known_gaps(missing) raises FileNotFoundError (never defaults to {})",
              raised)

        # And through main(): a missing baseline must not quietly pass.
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        h.use_corpus(fix, goos, chrom, empty_gaps_file(tmp))
        h.set_gaps_default(missing)
        _save(_blank(), goos / "a.png")
        _save(_blank(), chrom / "a.png")
        (fix / "a.html").write_text("<html></html>")
        code: int | None = None
        try:
            code = h.run_main(("parity.py",))
        except OSError:
            code = None  # traceback -> nonzero process exit
        check("main() with a missing baseline never returns 0",
              code is None or code != 0, f"rc={code}")
    finally:
        h.close()


# --- GOOSIE-PARITYTMP-001: one render directory per run --------------------
# The harness used to read and write two machine-global /tmp roots, so two
# concurrent runs scored each other's PNGs - and, one level worse, a binary
# that stopped rendering a fixture at all was still scored 100% against the
# PREVIOUS run's image.  A script that exits 1 without writing anything stands
# in for that binary and records whether the output path was already gone when
# it ran, which is the only way to tell "deleted before the render" from
# "deleted after the failure".
FAKE_FAIL_BINARY = """#!/usr/bin/env python3
import os, pathlib, sys
argv = sys.argv
out = pathlib.Path(argv[argv.index("-out") + 1])
marker = os.environ.get("PARITY_FAKE_MARKER")
if marker:
    pathlib.Path(marker).write_text("stale" if out.exists() else "clean")
sys.exit(1)
"""


# --- case 6: two runs on one host cannot read each other's renders
def case_workdir_isolation():
    print("case 6: --workdir roots a run away from every other run's PNGs")
    h = Harness()
    try:
        m = h.mod
        default_g, default_c = m.GOOSIE, m.CHROMIUM
        check("no flag keeps the machine-global roots (scripts/run-parity.sh "
              "and the Makefile score pre-generated Chromium renders)",
              m.resolve_dirs(None, False) == (default_g, default_c),
              str(m.resolve_dirs(None, False)))
        wa, wb = Path("/tmp/run-a"), Path("/tmp/run-b")
        ga, ca = m.resolve_dirs(str(wa), False)
        gb, cb = m.resolve_dirs(str(wb), False)
        check("--workdir moves BOTH sides together",
              (ga, ca) == (wa / "goosie", wa / "chromium"), f"{ga} {ca}")
        check("two --workdir runs share no path",
              {ga, ca}.isdisjoint({gb, cb}), f"{ga} {ca} vs {gb} {cb}")
        i1, i2 = m.resolve_dirs(None, True), m.resolve_dirs(None, True)
        # --isolate actually mkdtemp'd these, so the harness owns their cleanup.
        h.track(str(i1[0].parent))
        h.track(str(i2[0].parent))
        check("--isolate gives each run its own root",
              i1[0].parent != i2[0].parent, f"{i1[0].parent} == {i2[0].parent}")

        # The measured half: run A's evidence must not be readable by run B.
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        tmp2, fix2, goos2, chrom2 = _mkcorpus()
        h.track(tmp2)
        gaps = empty_gaps_file(tmp)
        h.use_corpus(fix, goos, chrom, gaps)
        _fixture(tmp, fix, goos, chrom, "a.png", n_bands=1, goosie_bars=1)
        rows = m.score_fixtures(m.THRESHOLD, m.PASS_AT, known_gaps={})
        check("run A scores its own pair", len(rows) == 1 and rows[0].status == "PASS",
              repr([(r.status, r.error) for r in rows]))
        check("and records the exact files it measured (GOOSIE-PARITYTMP-001)",
              rows[0].sources == (str(goos / "a.png"), str(chrom / "a.png")),
              str(rows[0].sources))
        # Same fixtures, run B's empty roots: it must come back MISSING and
        # name its own paths, never A's.
        h.use_corpus(fix, goos2, chrom2, gaps)
        rows2 = m.score_fixtures(m.THRESHOLD, m.PASS_AT, known_gaps={})
        r = rows2[0] if rows2 else None
        check("a second run reads nothing from the first",
              r is not None and r.status == "MISSING", repr(r.status if r else None))
        check("naming the directory it looked in, not run A's",
              r is not None and r.sources == (str(goos2 / "a.png"), str(chrom2 / "a.png")),
              str(r.sources if r else None))
    finally:
        h.close()


# --- case 7: a stale PNG is never scored as this run's output
def case_render_deletes_stale_png():
    print("case 7: --render deletes the previous run's PNG before invoking the binary")
    h = Harness()
    saved_env = os.environ.get("PARITY_FAKE_MARKER")
    try:
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        h.use_corpus(fix, goos, chrom, empty_gaps_file(tmp))
        (fix / "a.html").write_text("<html><body>synthetic</body></html>")
        _save(_bars(1), chrom / "a.png")  # the reference this run scores against
        _save(_bars(1), goos / "a.png")   # the PREVIOUS run's evidence
        binary = Path(tmp) / "fake-goosie"
        binary.write_text(FAKE_FAIL_BINARY)
        binary.chmod(0o755)
        marker = Path(tmp) / "invoked"
        os.environ["PARITY_FAKE_MARKER"] = str(marker)

        raised = False
        try:
            h.mod.render_goosie(str(binary))
        except h.mod.RenderError:
            raised = True
        check("a binary that produces no PNG fails the render step", raised)
        check("the output path was already deleted when the binary ran "
              "(after-the-fact cleanup would still score the stale image)",
              marker.exists() and marker.read_text() == "clean",
              marker.read_text() if marker.exists() else "binary never invoked")
        check("and the previous run's PNG is gone", not (goos / "a.png").exists())
        results = h.mod.score_fixtures(h.mod.THRESHOLD, h.mod.PASS_AT,
                                       known_gaps={})
        check("scoring that directory reports MISSING, never a 100% pass",
              len(results) == 1 and results[0].status == "MISSING"
              and "goosie render missing" in (results[0].error or ""),
              repr([(r.status, r.error) for r in results]))
    finally:
        if saved_env is None:
            os.environ.pop("PARITY_FAKE_MARKER", None)
        else:
            os.environ["PARITY_FAKE_MARKER"] = saved_env
        h.close()


# --- case 8: main() publishes the roots every reader uses
def case_main_applies_workdir():
    print("case 8: main() publishes --workdir into the globals readers use")
    h = Harness()
    try:
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        gaps = empty_gaps_file(tmp)
        h.use_corpus(fix, goos, chrom, gaps)
        h.set_gaps_default(gaps)
        work = Path(tmp) / "work"
        rc = h.run_main(("parity.py", "--workdir", str(work)))
        check("main() set the goosie root to WORK/goosie (the `global` statement "
              "is not optional: a local assignment would leave readers on the old root)",
              str(h.mod.GOOSIE) == str(work / "goosie"), str(h.mod.GOOSIE))
        check("and the chromium root to WORK/chromium",
              str(h.mod.CHROMIUM) == str(work / "chromium"), str(h.mod.CHROMIUM))
        check("an empty private corpus is refused (rc 2), not scored as a pass",
              rc == 2, f"rc={rc}")
        rc = h.run_main(("parity.py", "--workdir", str(work), "--isolate"))
        check("--workdir together with --isolate is a bad invocation (rc 2)",
              rc == 2, f"rc={rc}")
    finally:
        h.close()


# --- case 9: the baseline names the machine that measured it
def case_baseline_records_its_host():
    print("case 9: the known-gaps baseline records, and main() reports, its host")
    h = Harness()
    try:
        m = h.mod
        tmp, fix, goos, chrom = _mkcorpus()
        h.track(tmp)
        # The corpus stays empty, so every main() call below stops at the corpus
        # floor (rc 2) after the provenance line has been decided.
        def gaps_measured_on(value):
            p = Path(tmp) / f"gaps-{value}.json"
            body = {"version": 1, "known_gaps": []}
            if value != "none":
                body["measured_on"] = value
            p.write_text(json.dumps(body))
            h.use_corpus(fix, goos, chrom, p)
            h.set_gaps_default(p)
            return p

        def run_main_stderr() -> str:
            buf = io.StringIO()
            with redirect_stderr(buf):
                h.run_main(("parity.py",))
            return buf.getvalue()

        foreign = "win32" if sys.platform != "win32" else "linux"
        gaps_measured_on(foreign)
        check("known_gap_provenance() reads measured_on from the file KNOWN_GAPS "
              "points at (not a hard-coded path)",
              m.known_gap_provenance() == foreign, str(m.known_gap_provenance()))
        text = run_main_stderr()
        check("main() says the declared bands come from another host",
              f"measured on {foreign}" in text and "known-gaps baseline" in text,
              text)
        check("and names the cure: a baseline measured here, never an edit of "
              "the verdict's input",
              "never an edit made here to match" in text, text)

        gaps_measured_on(sys.platform)
        check("a baseline measured on this host prints no stale-host NOTE "
              "(kills both an unconditional print and an inverted condition)",
              "known-gaps baseline" not in run_main_stderr(),
              f"host={sys.platform}")

        gaps_measured_on("none")
        check("an unrecorded baseline is None, not silently this host",
              m.known_gap_provenance() is None, str(m.known_gap_provenance()))
        check("and it still prints the NOTE, as unrecorded",
              "an unrecorded platform" in run_main_stderr())

        m.KNOWN_GAPS = REAL_GAPS
        real = m.known_gap_provenance()
        check("the committed render-known-gaps.json records its measuring host "
              "(asserted non-empty so the check is host-independent)",
              isinstance(real, str) and real != "", repr(real))
    finally:
        h.close()


# --- GOOSIE-BLANKMETRIC-001: drift-adjusted blank-band metric --------------
# These cases drive score_regions_drift directly on synthetic images (no corpus,
# no browser, no network) and prove the new signal adds a tolerance WITHOUT
# redefining score_regions.  Every band is one Chromium bar; the goosie bar is
# moved to each shape the task names: exact, off-by-one-line, off-by-more-than-
# the-window, and deleted-entirely.  Each case is a *positive* expectation
# [R-7]: it must be able to kill a specific regression, not merely pass quietly.
_BAND_TOP = 500
_BAND_H = 12  # bar is >= MIN_INK (12 rows x 800 cols = 9,600 ink px)


def _drift_images(goosie_bar_top=None, band_top=_BAND_TOP, band_h=_BAND_H):
    """Return (goosie, chromium) RGB arrays with one reference band.

    chromium always has a bar at [band_top, band_top+band_h); goosie has the
    same bar at goosie_bar_top, or nothing when goosie_bar_top is None.
    """
    ch = np.full((_H, _W, 3), 255, dtype=np.uint8)
    ch[band_top:band_top + band_h, 100:900] = 40
    go = np.full((_H, _W, 3), 255, dtype=np.uint8)
    if goosie_bar_top is not None:
        go[goosie_bar_top:goosie_bar_top + band_h, 100:900] = 40
    return go, ch


def _band_rows(bands):
    return [(int(y0), int(y1)) for y0, y1 in bands]


def case_drift_matched():
    """Exact match is neither blank nor shifted under either signal."""
    print("case D1: goosie paints the band exactly -> no blank, no shift")
    m = load_parity()
    go, ch = _drift_images(goosie_bar_top=_BAND_TOP)
    exact_blank = _band_rows(m.score_regions(go, ch)[2])
    blank_exact, truly, shifted = m.score_regions_drift(go, ch)
    check("score_regions reports zero blank bands", exact_blank == [], str(exact_blank))
    check("drift signal reports zero blank/shifted bands",
          _band_rows(blank_exact) == [] and _band_rows(truly) == []
          and _band_rows(shifted) == [],
          f"blank_exact={_band_rows(blank_exact)} truly={_band_rows(truly)} "
          f"shifted={_band_rows(shifted)}")


def case_drift_off_by_one_line():
    """A line painted one band-height away is a DRIFT, not missing content.

    This is the whole point of GOOSIE-BLANKMETRIC-001: score_regions still (and
    must) list the band as blank -- the old signal keeps its meaning -- while the
    new signal explains it as shifted content.  Killing the drift window makes
    this assertion fail (mutation (i)).
    """
    print("case D2: goosie paints it one band-height below -> shifted, old signal still blank")
    m = load_parity()
    go, ch = _drift_images(goosie_bar_top=_BAND_TOP + _BAND_H)  # directly below
    old_blank = _band_rows(m.score_regions(go, ch)[2])
    blank_exact, truly, shifted = m.score_regions_drift(go, ch)
    band = (_BAND_TOP, _BAND_TOP + _BAND_H - 1)
    check("the OLD score_regions still lists the band blank (meaning unchanged)",
          old_blank == [band], f"old blank={old_blank}")
    check("drift signal lists it as shifted, not truly blank",
          _band_rows(shifted) == [band] and _band_rows(truly) == [],
          f"shifted={_band_rows(shifted)} truly={_band_rows(truly)}")
    check("drift blank_exact equals the old blank list exactly",
          _band_rows(blank_exact) == old_blank,
          f"{_band_rows(blank_exact)} vs {old_blank}")


def case_drift_beyond_window():
    """Content displaced far away is NOT pardoned: it stays truly blank.

    A bar 100 rows below sits outside the +/-band_height window, so the drift
    window cannot rubber-stamp it.  Widening the window without bound, or
    counting any band with a stray pixel, is what this kills.
    """
    print("case D3: goosie paints it 100 rows away -> stays blank (window is bounded)")
    m = load_parity()
    go, ch = _drift_images(goosie_bar_top=_BAND_TOP + 100)
    blank_exact, truly, shifted = m.score_regions_drift(go, ch)
    band = (_BAND_TOP, _BAND_TOP + _BAND_H - 1)
    check("far-away ink is not treated as a shift",
          _band_rows(shifted) == [] and _band_rows(truly) == [band],
          f"shifted={_band_rows(shifted)} truly={_band_rows(truly)}")


def case_drift_deleted_entirely():
    """Genuine absence survives the widest drift window (§2.4 / mutation ii).

    With nothing painted, every window is empty, so the band is truly_blank no
    matter how far the window reaches.  This is the acceptance-2 witness: the
    new tolerance never forgives a missing element.
    """
    print("case D4: goosie paints nothing there at all -> truly blank, never shifted")
    m = load_parity()
    go, ch = _drift_images(goosie_bar_top=None)  # element deleted
    blank_exact, truly, shifted = m.score_regions_drift(go, ch)
    band = (_BAND_TOP, _BAND_TOP + _BAND_H - 1)
    check("a deleted band is truly_blank", _band_rows(truly) == [band],
          f"truly={_band_rows(truly)}")
    check("a deleted band is never labeled shifted", _band_rows(shifted) == [],
          f"shifted={_band_rows(shifted)}")
    check("blank == truly_blank + shifted partitions the old blank list",
          len(truly) + len(shifted) == len(blank_exact),
          f"{len(truly)}+{len(shifted)} != {len(blank_exact)}")


def run_drift_cases() -> None:
    print("\ndrift metric (GOOSIE-BLANKMETRIC-001): synthetic, no corpus/browser")
    case_drift_matched()
    case_drift_off_by_one_line()
    case_drift_beyond_window()
    case_drift_deleted_entirely()


def selftest(drift_only: bool = False) -> int:
    if drift_only:
        run_drift_cases()
        print(f"\n{len(FAILURES)} drift selftest failure(s)" if FAILURES
              else "\ndrift selftest green")
        return 1 if FAILURES else 0
    print(f"parity selftest: driving {PARITY.name} with synthetic temp corpora")
    case_missing_render()
    case_stale_baseline_fails()
    case_baseline_matches_passes()
    case_corpus_floor()
    case_missing_gaps_file()
    case_workdir_isolation()
    case_render_deletes_stale_png()
    case_main_applies_workdir()
    case_baseline_records_its_host()
    run_drift_cases()
    print(f"\n{len(FAILURES)} selftest failure(s)" if FAILURES
          else "\nselftest green")
    return 1 if FAILURES else 0


if __name__ == "__main__":
    sys.exit(selftest(drift_only="--drift" in sys.argv))
