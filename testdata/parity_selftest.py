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

Run directly: ``python3 testdata/parity_selftest.py`` (exit 0 = green).
"""
from __future__ import annotations

import importlib.util
import inspect
import json
import shutil
import sys
import tempfile
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
MEASURED = {
    "forms/04-advanced-inputs.png": 2,
    "html-features/01-forms-complete.png": 1,
    "selectors/02-pseudo-classes.png": 3,
    "selectors/03-pseudo-elements.png": 3,
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
        check("real baseline loaded (expected 5 entries)", len(kg) == 5,
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


def selftest() -> int:
    print(f"parity selftest: driving {PARITY.name} with synthetic temp corpora")
    case_missing_render()
    case_stale_baseline_fails()
    case_baseline_matches_passes()
    case_corpus_floor()
    case_missing_gaps_file()
    print(f"\n{len(FAILURES)} selftest failure(s)" if FAILURES
          else "\nselftest green")
    return 1 if FAILURES else 0


if __name__ == "__main__":
    sys.exit(selftest())
