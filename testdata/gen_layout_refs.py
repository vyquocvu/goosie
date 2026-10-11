#!/usr/bin/env python3
"""Generate Chromium reference boxes for layout-parity fixtures.

Renders each testdata/layout-parity/*.html at 800x600 (the viewport the Go
harness lays out at) and records getBoundingClientRect per id-bearing
element into <name>.reference.json next to the fixture. Re-run when
fixtures change; the Go test fails closed on missing or stale references.

Usage: python3 testdata/gen_layout_refs.py
"""

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent / "layout-parity"
VIEWPORT = {"width": 800, "height": 600}


def main() -> int:
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        print("playwright is not installed", file=sys.stderr)
        return 2
    fixtures = sorted(ROOT.glob("*.html"))
    if not fixtures:
        print("no fixtures in", ROOT, file=sys.stderr)
        return 2
    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        page = browser.new_page(viewport=VIEWPORT)
        for html in fixtures:
            page.goto(html.resolve().as_uri(), wait_until="load")
            boxes = page.evaluate(
                """() => {
                    const out = {};
                    for (const el of document.querySelectorAll('[id]')) {
                        const r = el.getBoundingClientRect();
                        out[el.id] = {
                            x: Math.round(r.x), y: Math.round(r.y),
                            w: Math.round(r.width), h: Math.round(r.height),
                        };
                    }
                    return out;
                }"""
            )
            if not boxes:
                print(f"no id-bearing elements in {html.name}", file=sys.stderr)
                return 2
            ref = {"viewport": [800, 600], "boxes": boxes}
            out = html.with_suffix(".reference.json")
            out.write_text(json.dumps(ref, indent=2) + "\n")
            print(f"{html.name}: {len(boxes)} boxes -> {out.name}")
        browser.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
