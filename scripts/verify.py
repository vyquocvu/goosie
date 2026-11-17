import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import signal
from pathlib import Path
import subprocess
import tempfile
import threading
import time

from PIL import Image

JOURNEYS = (
    "TestE2ECoreBrowserJourney",
    "TestE2ESearchEngineSettingsJourney",
    "TestE2EFilePickerSelectionReachesScript",
    "TestE2EFindJourney",
    "TestE2ETabIsolationJourney",
)
DOCUMENT = b'''<!doctype html><html><head><style>
html,body{margin:0;background:white}
div{position:absolute;top:16px;width:96px;height:64px}
#static{left:16px;background:red}#script{left:160px;background:blue}
</style></head><body><div id="static"></div><div id="script"></div>
<script>document.getElementById('script').style.backgroundColor='green';</script>
</body></html>'''


def check_journeys(output):
    events = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
    if any(event.get("Action") in ("skip", "fail") for event in events):
        raise ValueError("journey failed or was skipped")
    passed = {event.get("Test") for event in events if event.get("Action") == "pass"}
    missing = set(JOURNEYS) - passed
    if missing:
        raise ValueError("journeys did not run: " + ", ".join(sorted(missing)))


def check_image(path):
    with Image.open(path) as original:
        image = original.convert("RGB")
        if image.size != (320, 200):
            raise ValueError("screenshot dimensions differ from 320x200")
        for point, expected in (((40, 40), (255, 0, 0)), ((180, 40), (0, 128, 0)), ((300, 180), (255, 255, 255))):
            actual = image.getpixel(point)
            if actual != expected:
                raise ValueError(f"pixel {point}: got {actual}, expected {expected}")


def execute(command, root, log, timeout, env=None):
    process = subprocess.Popen(command, cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, start_new_session=True)
    try:
        output, _ = process.communicate(timeout=timeout)
        code = process.returncode
    except (subprocess.TimeoutExpired, KeyboardInterrupt):
        os.killpg(process.pid, signal.SIGKILL)
        output, _ = process.communicate()
        code = 124
    log.write_text(output)
    return code


def source_digest(root):
    names = subprocess.check_output(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "cmd", "internal", "test", "third_party", "go.mod", "go.sum", "scripts/verify.py", "scripts/verify_test.py"], cwd=root).split(b"\0")
    digest = hashlib.sha256()
    for name in sorted(set(names) - {b""}):
        path = root / os.fsdecode(name)
        digest.update(name + b"\0")
        digest.update(path.read_bytes() if path.is_file() else b"DELETED")
        digest.update(b"\0")
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description="Run a fresh, isolated Goosie browser smoke and preserve evidence.")
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--out", type=Path, help="new evidence directory; must not already exist")
    args = parser.parse_args()
    root = args.root.resolve()
    out = args.out.resolve() if args.out else Path(tempfile.mkdtemp(prefix="goosie-verify-"))
    if args.out:
        out.mkdir(parents=True, exist_ok=False)
    report = {"status": "fail", "steps": [], "evidence": str(out)}
    print(f"evidence: {out}", flush=True)

    def run(name, command, env=None, timeout=120):
        started = time.monotonic()
        print(f"RUN {name}: {command}", flush=True)
        log = out / (name + ".log")
        code = execute(command, root, log, timeout, env)
        report["steps"].append({"name": name, "command": command, "exit": code, "seconds": round(time.monotonic() - started, 3), "log": str(log)})
        print(f"{name}: exit={code} log={log}", flush=True)
        if code:
            raise ValueError(f"{name} failed; see {log}")
        return log.read_text()

    try:
        report["commit"] = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip()
        report["source_sha256"] = source_digest(root)
        with tempfile.TemporaryDirectory(prefix="runtime-", dir=out) as runtime:
            binary = str(Path(runtime) / "goosie")
            run("build", ["go", "build", "-mod=readonly", "-o", binary, "./cmd/goosie"])
            report["binary_sha256"] = hashlib.sha256(Path(binary).read_bytes()).hexdigest()
            output = run("journeys", ["go", "test", "-mod=readonly", "-count=1", "-timeout=90s", "-json", "-run", "^TestE2E", "./cmd/goosie"])
            check_journeys(output)

            class Handler(BaseHTTPRequestHandler):
                def do_GET(self):
                    self.send_response(200)
                    self.send_header("Content-Type", "text/html; charset=utf-8")
                    self.send_header("Content-Length", str(len(DOCUMENT)))
                    self.end_headers()
                    self.wfile.write(DOCUMENT)

                def log_message(self, *_args):
                    pass

            server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                env = dict(os.environ, HOME=runtime)
                run("render", [binary, "-private", "-backend", "headless", "-width", "320", "-height", "200", "-dpr", "1", "-download-dir", runtime, "-screenshot", "-url", f"http://127.0.0.1:{server.server_port}/", "-out", str(out / "document.png")], env=env, timeout=30)
                check_image(out / "document.png")
                report["pixels"] = "PASS: static red, script-mutated green, white background; 320x200"
            finally:
                server.shutdown()
                server.server_close()
                thread.join()
        if source_digest(root) != report["source_sha256"]:
            raise ValueError("source changed during verification; no verdict for this candidate")
        report["status"] = "pass"
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        report["error"] = str(error)
        print(f"FAIL: {error}", flush=True)
    finally:
        (out / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"{report['status'].upper()}: {out / 'report.json'}", flush=True)
    return 0 if report["status"] == "pass" else 1


def interrupted(signum, frame):
    raise KeyboardInterrupt("Smoke interrupted by signal " + str(signum))


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    raise SystemExit(main())
