#!/usr/bin/env python3
"""Score Goosie's renders of testdata/urls.txt against Playwright references.

Same rules as parity.py (THRESHOLD per channel, PASS_AT percent of matching
pixels), but the corpus is live URLs instead of local fixtures. References
come from render-urls.js; goosie renders with -screenshot, which now loads the
URL synchronously before capturing.

  python3 testdata/parity_urls.py --render-ref --render --update-refs

--snapshot goes one step past --paired: it fetches each document ONCE, rewrites
its URLs to absolute, serves the copy over localhost, and renders both engines
from that copy. Pages whose HTML rotates per request (a random hero header, a
banner that changes hourly) then compare identical bytes, so the score measures
the engine instead of which ad was served.
"""

import argparse
import concurrent.futures
import functools
import hashlib
import http.server
import json
import os
import re
import subprocess
import sys
import threading
import urllib.parse
import urllib.request
from pathlib import Path

import numpy as np
from PIL import Image

THRESHOLD = 30

# FREEZE_ASSETS also localizes each page's images and stylesheets. It is off by
# default: a page that loses a sub-resource to the rewrite diverges for a
# harness reason, not an engine one, and that is worse than a rotating header.
FREEZE_ASSETS = False
PASS_AT = 90.0
WIDTH, HEIGHT = 1280, 800
GOOSIE_DIR = Path("/tmp/goosie-url-renders")
CHROMIUM_DIR = Path("/tmp/playwright-url-renders")
SNAP_DIR = Path("/tmp/goosie-url-snapshots")
MANIFEST = Path("testdata/urls.txt")
USER_AGENT = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
              "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")



def slug(url: str) -> str:
    s = re.sub(r"^https?://", "", url)
    s = re.sub(r"[^A-Za-z0-9]+", "_", s)
    return re.sub(r"^_+|_+$", "", s)[:80]


def urls():
    return [l.strip() for l in MANIFEST.read_text().splitlines()
            if l.strip() and not l.startswith("#")]


def render_one(binary: str, url: str, name: str = None) -> None:
    out = GOOSIE_DIR / ((name or slug(url)) + ".png")
    out.parent.mkdir(parents=True, exist_ok=True)
    r = subprocess.run(
        [binary, "-backend", "headless", "-width", str(WIDTH), "-height", str(HEIGHT),
         "-dpr", "1", "-screenshot", "-out", str(out), "-url", url, "-frames", "120"],
        capture_output=True, text=True, timeout=90,
    )
    if r.returncode != 0:
        print(f"goosie FAIL {url}: {r.stderr.strip().splitlines()[-1] if r.stderr else ''}",
              file=sys.stderr)


def decode_body(body: bytes, ctype: str) -> str:
    for token in ctype.split(";"):
        if token.strip().lower().startswith("charset="):
            try:
                return body.decode(token.split("=", 1)[1].strip().strip("\"'"), "replace")
            except LookupError:
                break
    return body.decode("utf-8", "replace")


def _abs(base: str, ref: str) -> str:
    ref = ref.strip()
    if not ref or ref[0] == "#" or ":" in ref.split("/", 1)[0]:
        return ref
    return urllib.parse.urljoin(base, ref)


def absolutize_html(html: str, base: str) -> str:
    """Point every document reference at the origin, so the copy served from
    localhost still loads its real stylesheets and images.

    The document's own charset declaration has to go with it: the bytes below
    are re-encoded as UTF-8, and a stale <meta charset> would make Chromium
    decode them as mojibake while goosie read them correctly.
    """
    def attr(m):
        return f'{m.group(1)}="{_abs(base, m.group(2))}"'

    def srcset(m):
        parts = []
        for cand in m.group(2).split(","):
            fields = cand.split()
            if fields:
                fields[0] = _abs(base, fields[0])
            if fields:
                parts.append(" ".join(fields))
        return f'{m.group(1)}="{", ".join(parts)}"'

    def cssurl(m):
        return f"url({m.group(1)}{_abs(base, m.group(2))}{m.group(1)})"

    html = re.sub(r"\b(src|href|poster|action|data-src)=\"([^\"]*)\"", attr, html, flags=re.I)
    html = re.sub(r"\b(srcset)=\"([^\"]*)\"", srcset, html, flags=re.I)
    html = re.sub(r"url\(([\"']?)([^\"')]+)\1\)", cssurl, html, flags=re.I)
    html = re.sub(r"<meta[^>]+charset=[^>]*>", "", html, flags=re.I)
    return html.replace("<head>", '<head><meta charset="utf-8">', 1)


ASSET_EXT = {".css": "text/css", ".png": "image/png", ".jpg": "image/jpeg",
             ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
             ".svg": "image/svg+xml", ".ico": "image/x-icon", ".avif": "image/avif",
             ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf"}


def asset_fetch(url: str, referer: str, cache: dict):
    """Download one sub-resource into the snapshot dir, returning its local path.

    Each engine used to fetch the document's images and stylesheets itself at
    render time, so a site that rotates its header per request scored as a
    rendering divergence. On any failure the caller keeps the remote URL, which
    is what happened before.
    """
    if url in cache:
        return cache[url]
    path = urllib.parse.urlparse(url).path.lower()
    ext = next((e for e in ASSET_EXT if path.endswith(e)), None)
    if ext is None:
        return None
    try:
        req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT,
                                                  "Accept": ASSET_EXT[ext],
                                                  "Referer": referer})
        with urllib.request.urlopen(req, timeout=20) as resp:
            body = resp.read(16 << 20)
    except Exception:
        cache[url] = None
        return None
    local = f"assets/{hashlib.sha1(url.encode()).hexdigest()[:16]}{ext}"
    (SNAP_DIR / local).write_bytes(body)
    cache[url] = local
    return local


def localize_css(text: str, base: str, referer: str, cache: dict, depth: int) -> str:
    """Rewrite a stylesheet's own url() and @import references to local copies.

    Every reference has to be resolved against the sheet's origin before it is
    written out: the sheet now lives under assets/, so a relative path left in
    place would resolve against the snapshot host and 404. That silently
    deleted gridbyexample's imported rules and dropped it from 94.62 to 42.22.
    """
    if depth > 3:
        return text

    def reroute(raw: str):
        """Return (absolute url, local copy or None) for one sheet reference."""
        url = _abs(base, raw)
        if not url.startswith("http"):
            return None, None
        return url, asset_fetch(url, referer, cache)

    def embed_child(url: str, local):
        """Recurse into a localized sheet so its own references get resolved."""
        if not local or not url.lower().split("?", 1)[0].endswith(".css"):
            return
        path = SNAP_DIR / local
        path.write_text(localize_css(path.read_text(encoding="utf-8", errors="replace"),
                                     url, referer, cache, depth + 1), encoding="utf-8")

    def written(url: str, local):
        # A localized asset sits flat in assets/ next to this sheet, so it is
        # named, not pathed. Anything still remote has to stay a full URL: a
        # relative path here would resolve against the snapshot host and 404.
        return Path(local).name if local else url

    def sub(m):
        url, local = reroute(m.group(2))
        if url is None:
            return m.group(0)
        embed_child(url, local)
        return m.group(1) + written(url, local) + m.group(3)

    def importsub(m):
        url, local = reroute(m.group(2).strip("\"'"))
        if url is None:
            return m.group(0)
        embed_child(url, local)
        return f'{m.group(1)}"{written(url, local)}"'

    text = re.sub(r"(url\([\"']?)([^\"')]+)([\"']?\))", sub, text, flags=re.I)
    return re.sub(r"(@import\s+)([\"'][^\"']+[\"'])", importsub, text, flags=re.I)


def localize_assets(html: str, referer: str) -> str:
    """Point the snapshot's images and stylesheets at local copies of the bytes."""
    cache: dict = {}

    def attr(m):
        url = m.group(2)
        if not url.startswith("http"):
            return m.group(0)
        local = asset_fetch(url, referer, cache)
        return f'{m.group(1)}="{local}"' if local else m.group(0)

    def srcset(m):
        parts = []
        for cand in m.group(2).split(","):
            fields = cand.split()
            if fields and fields[0].startswith("http"):
                fields[0] = asset_fetch(fields[0], referer, cache) or fields[0]
            if fields:
                parts.append(" ".join(fields))
        return f'{m.group(1)}="{", ".join(parts)}"'

    def cssurl(m):
        url = m.group(2)
        if not url.startswith("http"):
            return m.group(0)
        local = asset_fetch(url, referer, cache)
        return f"url({m.group(1)}{local}{m.group(1)})" if local else m.group(0)

    html = re.sub(r"\b(src|href|poster|data-src|srcset)=\"([^\"]*)\"", lambda m: (
        srcset(m) if m.group(1).lower() == "srcset" else attr(m)), html, flags=re.I)
    html = re.sub(r"url\(([\"']?)([^\"')]+)\1\)", cssurl, html, flags=re.I)
    # Stylesheets are localized after their own references, so a sheet downloaded
    # above has already been rewritten and its images resolve as siblings.
    for url, local in list(cache.items()):
        if local and url.lower().split("?", 1)[0].endswith(".css"):
            path = SNAP_DIR / local
            path.write_text(localize_css(path.read_text(encoding="utf-8", errors="replace"),
                                         url, referer, cache, 1), encoding="utf-8")
    return html


def capture_with_playwright(url: str):
    """Fetch a document through Chromium and return (final url, html).

    Some corpus URLs refuse urllib - a Cloudflare challenge answers 403, a
    moved path answers 404 - and skipping them leaves their rows scored against
    whatever render an earlier run left behind. Chromium reaches both, and a
    snapshot only has to be the same bytes each engine draws.
    """
    r = subprocess.run(["node", "testdata/fetch-url.js", url],
                       capture_output=True, text=True, timeout=90,
                       env={**os.environ, "NODE_PATH": os.path.join(os.getcwd(), "node_modules")})
    if r.returncode != 0 or not r.stdout.strip():
        lines = (r.stderr or r.stdout).strip().splitlines()
        raise RuntimeError(lines[-1] if lines else "no output")
    doc = json.loads(r.stdout)
    return doc["url"], doc["html"].encode("utf-8")


def snapshot_one(url: str):
    final = url
    try:
        req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT,
                                                  "Accept": "text/html,*/*"})
        with urllib.request.urlopen(req, timeout=25) as resp:
            body, final = resp.read(), resp.geturl()
            ctype = resp.headers.get("Content-Type", "text/html")
    except Exception as err:
        print(f"snapshot RETRY {url} through Chromium: {err}", file=sys.stderr)
        try:
            final, body = capture_with_playwright(url)
            ctype = "text/html"
        except Exception as err2:
            print(f"snapshot FAIL {url}: {err2}", file=sys.stderr)
            return None
    if "html" not in ctype.lower():
        print(f"snapshot SKIP {url}: Content-Type {ctype}", file=sys.stderr)
        return None
    name = slug(url)
    text = absolutize_html(decode_body(body, ctype), final)
    if FREEZE_ASSETS:
        (SNAP_DIR / "assets").mkdir(exist_ok=True)
        text = localize_assets(text, final)
    (SNAP_DIR / (name + ".html")).write_text(text, encoding="utf-8")
    return name


def serve_snaps():
    class Quiet(http.server.SimpleHTTPRequestHandler):
        def log_message(self, *args):
            pass

    httpd = http.server.ThreadingHTTPServer(
        ("127.0.0.1", 0), functools.partial(Quiet, directory=str(SNAP_DIR)))
    threading.Thread(target=httpd.serve_forever, daemon=True).start()
    return httpd


def render_snapshots(binary: str, only: str = None) -> None:
    """Freeze the corpus: one fetch per URL, then both engines on those bytes."""
    SNAP_DIR.mkdir(parents=True, exist_ok=True)
    GOOSIE_DIR.mkdir(parents=True, exist_ok=True)
    CHROMIUM_DIR.mkdir(parents=True, exist_ok=True)
    corpus = [u for u in urls() if not only or only in u]
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex:
        names = [n for n in ex.map(snapshot_one, corpus) if n]
    httpd = serve_snaps()
    port = httpd.server_address[1]
    entries = [{"url": f"http://127.0.0.1:{port}/{n}.html",
                "out": str(CHROMIUM_DIR / (n + ".png"))} for n in names]
    manifest = SNAP_DIR.parent / "goosie-url-snapshot-manifest.json"
    manifest.write_text(json.dumps(entries))
    print(f"snapshot: {len(entries)} pages on port {port}", file=sys.stderr)
    render_refs(manifest=str(manifest))
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:
        list(ex.map(lambda e: render_one(binary, e["url"], Path(e["out"]).stem), entries))
    httpd.shutdown()



def refresh_snapshot(binary: str, only: str = None) -> None:
    """Re-render goosie from the snapshot already on disk; Chromium keeps its refs.

    A sweep recaptures every document and reference, which costs minutes and gives
    a rotating site a fresh chance to move. After an engine change the frozen copy
    is still the one Chromium drew, so only goosie has to look at it again.
    """
    names = sorted(p.stem for p in SNAP_DIR.glob("*.html") if not only or only in p.stem)
    if not names:
        print("refresh: no snapshot pages on disk, run --snapshot first", file=sys.stderr)
        return
    httpd = serve_snaps()
    port = httpd.server_address[1]
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:
            list(ex.map(lambda n: render_one(
                binary, f"http://127.0.0.1:{port}/{n}.html", n), names))
    finally:
        httpd.shutdown()


def render_all(binary: str, only: str = None) -> None:
    corpus = urls()
    if only:
        corpus = [u for u in corpus if slug(u) in only or only in u]
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:
        list(ex.map(lambda u: render_one(binary, u), corpus))


def render_refs(only=None, manifest=None) -> None:
    cmd = ["node", "testdata/render-urls.js"]
    env = dict(os.environ)
    if manifest:
        env["URLS_JSON"] = manifest
    elif "URLS_JSON" in env:
        del env["URLS_JSON"]
    if only:
        cmd.append(only)
    subprocess.run(cmd, check=False, env=env)


def render_paired(binary: str, only: str = None) -> None:
    """Capture each page's Chromium ref and goosie render back to back.

    The corpus is live, so a single ref pass followed by a single goosie pass
    can be minutes apart per page and scores article churn instead of the
    engine. Pairing the two captures per URL keeps the comparison a rendering
    measurement.
    """
    GOOSIE_DIR.mkdir(parents=True, exist_ok=True)
    for url in urls():
        if only and only not in url:
            continue
        render_refs(url)
        try:
            render_one(binary, url)
        except subprocess.TimeoutExpired:
            print(f"goosie TIMEOUT {url}", file=sys.stderr)


def update_refs() -> None:
    dst = Path("testdata/url-refs")
    dst.mkdir(exist_ok=True)
    for url in urls():
        src = CHROMIUM_DIR / (slug(url) + ".png")
        if src.exists():
            (dst / src.name).write_bytes(src.read_bytes())


def score(refs_dir):
    rows = []
    for url in urls():
        name = slug(url) + ".png"
        gp, pp = GOOSIE_DIR / name, refs_dir / name
        if not gp.exists() or not pp.exists():
            rows.append((0.0, "MISS", name))
            continue
        g = np.array(Image.open(gp).convert("RGB"))
        p = np.array(Image.open(pp).convert("RGB"))
        if g.shape != p.shape:
            p = np.array(Image.open(pp).convert("RGB").resize(
                (g.shape[1], g.shape[0]), Image.Resampling.LANCZOS))
        pct = float(np.mean(np.all(np.abs(g.astype(int) - p.astype(int))
                                  <= THRESHOLD, axis=2)) * 100)
        rows.append((pct, "PASS" if pct >= PASS_AT else "FAIL", name))
    rows.sort(key=lambda r: -r[0])
    return rows


def inspect(name, band):
    gp, pp = GOOSIE_DIR / name, CHROMIUM_DIR / name
    if not gp.exists():
        pp = Path("testdata/url-refs") / name
    g = np.array(Image.open(gp).convert("RGB")).astype(int)
    p = np.array(Image.open(pp).convert("RGB")).astype(int)
    if g.shape != p.shape:
        p = np.array(Image.open(pp).convert("RGB").resize(
            (g.shape[1], g.shape[0]), Image.Resampling.LANCZOS)).astype(int)
    bad = ~np.all(np.abs(g - p) <= THRESHOLD, axis=2)
    h, w = bad.shape
    print(f"{name}: {bad.mean() * 100:.2f}% of {w}x{h} mismatched")
    print("rows")
    for y in range(0, h - h % band, band):
        frac = bad[y:y + band].mean() * 100
        if frac > 1:
            print(f"  {y:4d}-{y + band - 1:4d} {frac:5.1f}% {'#' * int(frac / 2)}")
    print("cols")
    for x in range(0, w - w % band, band):
        frac = bad[:, x:x + band].mean() * 100
        if frac > 1:
            print(f"  {x:4d}-{x + band - 1:4d} {frac:5.1f}% {'#' * int(frac / 2)}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--render", action="store_true", help="render URLs with goosie")
    ap.add_argument("--render-ref", action="store_true", help="render references with playwright")
    ap.add_argument("--paired", action="store_true",
                    help="capture ref and goosie render back to back per URL (live corpus)")
    ap.add_argument("--snapshot", action="store_true",
                    help="fetch each document once and render both engines from that copy")
    ap.add_argument("--update-refs", action="store_true", help="cache references to testdata/url-refs")
    ap.add_argument("--binary", default="./goosie")
    ap.add_argument("--live-refs", action="store_true", help="score against /tmp refs instead of cached")
    ap.add_argument("--refresh", action="store_true",
                    help="re-render goosie from the snapshot already on disk, keeping Chromium's refs")
    ap.add_argument("--freeze-assets", action="store_true",
                    help="with --snapshot, also download images and stylesheets locally")
    ap.add_argument("--only", metavar="SUBSTR", help="score only slugs containing SUBSTR")
    ap.add_argument("--inspect", metavar="NAME.png", help="print mismatch bands for one render")
    ap.add_argument("--band", type=int, default=32)
    args = ap.parse_args()

    if args.inspect:
        inspect(args.inspect, args.band)
        return 0

    if args.refresh:
        refresh_snapshot(args.binary, args.only)
    if args.paired:
        render_paired(args.binary, args.only)
    if args.snapshot:
        global FREEZE_ASSETS
        FREEZE_ASSETS = bool(args.freeze_assets)
        render_snapshots(args.binary, args.only)
    if args.render_ref:
        render_refs(args.only)
    if args.update_refs:
        update_refs()
    if args.render:
        render_all(args.binary, args.only)

    refs = CHROMIUM_DIR if (args.live_refs or args.snapshot or args.refresh) else Path("testdata/url-refs")
    rows = score(refs)
    if args.only:
        rows = [r for r in rows if args.only in r[2]]
    if not rows:
        print("no renders found", file=sys.stderr)
        return 1
    for pct, status, name in rows:
        print(f"{pct:6.2f}% {status} {name}")
    passing = sum(1 for r in rows if r[1] == "PASS")
    print(f"\n{passing}/{len(rows)} passing "
          f"({passing / len(rows) * 100:.1f}%), "
          f"average {sum(r[0] for r in rows) / len(rows):.2f}%")
    return 0


if __name__ == "__main__":
    sys.exit(main())
