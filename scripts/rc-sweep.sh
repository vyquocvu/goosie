#!/usr/bin/env bash
# rc-sweep.sh - collect the seven release-candidate verdicts and their evidence.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec python3 - "$ROOT" "$@" <<'PY'
import argparse
import datetime
import json
import os
import pathlib
import platform
import re
import shlex
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = pathlib.Path(sys.argv[1]).resolve()
sys.argv = [sys.argv[0], *sys.argv[2:]]

def die(message):
    print(f"rc-sweep: {message}", file=sys.stderr)
    raise SystemExit(2)

parser = argparse.ArgumentParser(description="Run the seven release-candidate checks.")
parser.add_argument("-out", default="/tmp/goosie-rc-sweep")
parser.add_argument("-steps", help="seven lines; shell command replaces a step, '-' keeps built-in")
parser.add_argument("-binary", default="./bin/goosie")
parser.add_argument("-live-url", default="https://about.google/products/")
parser.add_argument("-sha", default="")
parser.add_argument("-allow-sha-mismatch", action="store_true")
parser.add_argument("-golden", default="test/qc/traceability.golden.json")
parser.add_argument("-ratchet", default="docs/qa-ratchet.json")
parser.add_argument("-defects", default="docs/qa-open-defects.json")
parser.add_argument("-signoffs", default="docs/qa-signoffs")
args = parser.parse_args()

def rooted(value):
    path = pathlib.Path(value)
    return path if path.is_absolute() else ROOT / path

out = rooted(args.out)
try:
    out.mkdir(parents=True, exist_ok=True)
    if not os.access(out, os.W_OK):
        die(f"output directory is not writable: {out}")
    out = out.resolve()
except OSError as exc:
    die(f"cannot create output directory: {exc}")
report_path = out / "rc-sweep.json"
try:
    report_path.unlink(missing_ok=True)
except OSError as exc:
    die(f"cannot prepare report: {exc}")

steps = None
if args.steps:
    try:
        steps = [line.strip() for line in pathlib.Path(args.steps).read_text().splitlines() if line.strip() and not line.lstrip().startswith("#")]
    except OSError as exc:
        die(f"cannot read steps file: {exc}")
    if len(steps) != 7:
        die(f"-steps requires exactly seven non-comment lines, got {len(steps)}")

head = ""
fingerprint = {"sha": "", "os": platform.platform(), "binary": str(rooted(args.binary)), "binary_sha256": ""}
try:
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True, stderr=subprocess.DEVNULL).strip()
    if not head or not re.fullmatch(r"[0-9a-fA-F]{40,64}", head):
        raise ValueError("invalid git SHA")
except Exception as exc:
    fingerprint_error = f"cannot read build SHA: {exc}"
    fingerprint_failure_code = 2
else:
    binary = rooted(args.binary)
    if not binary.is_file() or not os.access(binary, os.X_OK):
        fingerprint_error = f"missing executable binary: {binary}"
        fingerprint_failure_code = 1
    else:
        try:
            fingerprint["sha"] = head
            fingerprint["binary_sha256"] = subprocess.check_output(["shasum", "-a", "256", str(binary)], text=True).split()[0]
            fingerprint_error = ""
            fingerprint_failure_code = 0
        except OSError as exc:
            fingerprint_error = f"cannot read binary SHA-256 for fingerprint: {exc}"
            fingerprint_failure_code = 2
    if not fingerprint_error:
        fingerprint_failure_code = 0

if args.sha and head and args.sha != head and not args.allow_sha_mismatch:
    fingerprint_error = f"-sha {args.sha} does not match HEAD {head}"
    fingerprint_failure_code = 1

def load_json(path, what):
    try:
        return json.loads(path.read_text())
    except Exception as exc:
        raise RuntimeError(f"cannot read {what} {path}: {exc}")

def tagged_groups():
    try:
        golden = load_json(rooted(args.golden), "traceability golden")
        ratchet = load_json(rooted(args.ratchet), "QA ratchet")
    except RuntimeError as exc:
        raise FileNotFoundError(str(exc)) from exc
    p0 = [case for case in golden if case.get("priority") == "P0"]
    p01 = [case for case in golden if case.get("priority") in ("P0", "P1")]
    p0_tagged = [case for case in p0 if case.get("tests")]
    p01_tagged = [case for case in p01 if case.get("tests")]
    if any(not case.get("tests") for case in p0):
        raise ValueError("one or more P0 cases have no tagged test")
    if len(p0_tagged) < int(ratchet["p0_tagged"]) or len(p01_tagged) < int(ratchet["p0p1_tagged"]):
        raise ValueError(f"tagged-case count fell below ratchet: P0 {len(p0_tagged)}/{ratchet['p0_tagged']}, P0/P1 {len(p01_tagged)}/{ratchet['p0p1_tagged']}")
    groups = {}
    for case in p01_tagged:
        for ref in case["tests"]:
            filename, testname = ref.rsplit(":", 1)
            pkg = "./" + str(pathlib.PurePosixPath(filename).parent)
            groups.setdefault(pkg, set()).add(testname)
    return groups

def run_case_tests(log):
    try:
        groups = tagged_groups()
    except Exception as exc:
        log.write_text(f"rc-sweep per-case: {exc}\n")
        if isinstance(exc, FileNotFoundError):
            return 2, f"cannot read per-case input ({exc})", "go test -count=1 -run <tagged tests> <package>"
        return 1, f"per-case tagged tests and ratchet ({exc})", "go test -count=1 -run <tagged tests> <package>"
    total_tests = 0
    failed = False
    no_verdict = False
    commands = []
    with log.open("w") as stream:
        for pkg in sorted(groups):
            names = sorted(groups[pkg])
            total_tests += len(names)
            pattern = "^(" + "|".join(re.escape(name) for name in names) + ")$"
            command = ["go", "test", "-json", "-count=1", "-run", pattern, pkg]
            commands.append(shlex.join(command))
            stream.write("$ " + shlex.join(command) + "\n")
            stream.flush()
            try:
                proc = subprocess.Popen(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                matched = set()
                for line in proc.stdout:
                    stream.write(line)
                    try:
                        event = json.loads(line)
                        if event.get("Action") == "run" and event.get("Test"):
                            matched.add(event["Test"].split("/", 1)[0])
                    except json.JSONDecodeError:
                        pass
                code = proc.wait()
                missing = sorted(set(names) - matched)
                if missing:
                    stream.write(f"rc-sweep: tagged tests did not match in {pkg}: {', '.join(missing)}\n")
                    no_verdict = True
                if code == 127:
                    no_verdict = True
                elif code != 0:
                    failed = True
            except OSError as exc:
                stream.write(f"rc-sweep: could not start {command[0]}: {exc}\n")
                no_verdict = True
    if no_verdict:
        return 2, f"per-case run did not reach a verdict ({total_tests} tagged test names)", "; ".join(commands)
    code = 1 if failed else 0
    detail = f"one or more P0/P1 tests failed ({total_tests} tagged test names)" if failed else f"all {total_tests} tagged P0/P1 test names ran"
    return code, detail, "; ".join(commands)

def live_smoke(log):
    parsed = urllib.parse.urlparse(args.live_url)
    if parsed.scheme not in ("http", "https") or not parsed.hostname:
        log.write_text(f"invalid live URL: {args.live_url}\n")
        return 2, "live smoke URL is invalid"
    try:
        req = urllib.request.Request(args.live_url, method="HEAD", headers={"User-Agent": "Goosie-RC-Sweep/1"})
        with urllib.request.urlopen(req, timeout=5) as response:
            if response.status >= 400:
                log.write_text(f"network preflight HTTP {response.status}\n")
                return 2, f"live host preflight returned HTTP {response.status}"
    except Exception as exc:
        log.write_text(f"network preflight unavailable: {exc}\n")
        return 2, f"live host unreachable; no live-smoke verdict ({exc})"
    report = out / "live-smoke-frame.json"
    binary = rooted(args.binary)
    command = [str(binary), "-gate", "-frames", "120", "-backend", "headless", "-url", args.live_url, "-out", str(report)]
    log.write_text("$ " + shlex.join(command) + "\n")
    try:
        with log.open("a") as stream:
            proc = subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT, timeout=90)
    except (OSError, subprocess.TimeoutExpired) as exc:
        with log.open("a") as stream:
            stream.write(f"live smoke could not run: {exc}\n")
        return 2, f"live smoke could not run ({exc})"
    if proc.returncode != 0:
        return (2 if proc.returncode == 127 else 1), f"live binary exited {proc.returncode}"
    try:
        frame = load_json(report, "live frame report")
        summary = frame.get("report", frame)
        frames = frame.get("frames", summary.get("frames", []))
        frame_count = len(frames) if isinstance(frames, list) else int(frames or 0)
        if frame_count <= 0 or summary.get("document") != args.live_url or summary.get("doc_height", 0) <= 0:
            return 1, "live page did not produce a non-empty frame report for the requested URL"
        return 0, f"live page drew {frame_count} frames, doc_height={summary['doc_height']}"
    except Exception as exc:
        return 1, f"live frame report failed grading ({exc})"

def defects_signoff(log):
    try:
        defects = load_json(rooted(args.defects), "open defects registry")
    except Exception as exc:
        log.write_text(f"rc-sweep defects/sign-off: {exc}\n")
        return 2, str(exc)
    open_blockers = [d for d in defects if d.get("priority") in ("P0", "P1") and d.get("status") == "open"]
    signoff_path = rooted(args.signoffs) / f"{head}.json"
    if not signoff_path.exists():
        signoff = None
    else:
        try:
            signoff = load_json(signoff_path, "build sign-off")
        except Exception as exc:
            log.write_text(f"rc-sweep sign-off: {exc}\n")
            return 2, str(exc)
    reasons = []
    if open_blockers:
        reasons.append("open P0/P1 defects: " + ", ".join(d.get("id", "<unknown>") for d in open_blockers))
    if signoff is None:
        reasons.append(f"human sign-off is missing for build {head}")
    elif signoff.get("build_sha") != head or signoff.get("reviewer") in (None, "", "TEMPLATE") or not signoff.get("date") or not signoff.get("cua_cases") or not signoff.get("statement"):
        reasons.append(f"invalid or incomplete human sign-off for {head}")
    if reasons:
        log.write_text("\n".join(reasons) + "\n")
        return 1, "; ".join(reasons)
    log.write_text(f"no open P0/P1 defects; human sign-off found for {head}\n")
    return 0, "no open P0/P1 defects and human sign-off matches build SHA"

names = ["fingerprint", "gate", "parity_render", "parity_interaction", "per_case_run", "live_smoke", "defects_signoff"]
custom = steps or ["-", "-", "-", "-", "-", "-", "-"]
results = []
any_fail = False
any_no_verdict = False

for index, (name, override) in enumerate(zip(names, custom), 1):
    log = out / f"{index:02d}-{name}.log"
    started = time.monotonic()
    command_text = ""
    if override != "-":
        command_text = override
        try:
            with log.open("w") as stream:
                proc = subprocess.run(override, cwd=ROOT, shell=True, stdout=stream, stderr=subprocess.STDOUT)
            code = 2 if proc.returncode in (2, 127) else (1 if proc.returncode != 0 else 0)
            detail = "custom step passed" if code == 0 else f"custom step exited {proc.returncode}"
        except OSError as exc:
            log.write_text(f"cannot run custom step: {exc}\n")
            code, detail = 2, str(exc)
    elif index == 1:
        command_text = f"git rev-parse HEAD; shasum -a 256 {fingerprint['binary']}"
        if fingerprint_error:
            log.write_text(f"fingerprint unavailable: {fingerprint_error}\n")
            code, detail = fingerprint_failure_code, fingerprint_error
        else:
            log.write_text(json.dumps(fingerprint, sort_keys=True) + "\n")
            code, detail = 0, "build SHA, OS, and binary SHA-256 recorded"
    elif index == 2:
        command_text = f"scripts/release-gate.sh -out {out / 'gate'}"
        try:
            with log.open("w") as stream:
                proc = subprocess.run(["bash", str(ROOT / "scripts/release-gate.sh"), "-out", str(out / "gate")], cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
            code = 2 if proc.returncode in (2, 127) else (1 if proc.returncode != 0 else 0)
            detail = f"release gate exited {proc.returncode}"
        except OSError as exc:
            log.write_text(str(exc) + "\n")
            code, detail = 2, str(exc)
    elif index == 3:
        command = ["python3", "testdata/parity.py", "--render", "--binary", str(rooted(args.binary)), "--json", str(out / "parity.json")]
        command_text = shlex.join(command)
        try:
            with log.open("w") as stream:
                proc = subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
            output = log.read_text(errors="replace")
            code = (2 if proc.returncode == 127 or re.search(r"(chromium|fixture|binary).*(not found|missing|unavailable)|no chromium", output, re.I) else (1 if proc.returncode != 0 else 0))
            detail = f"parity render exited {proc.returncode}"
        except OSError as exc:
            log.write_text(str(exc) + "\n")
            code, detail = 2, str(exc)
    elif index == 4:
        command = ["go", "test", "-count=1", "-run", "TestParity", "./test/engine/"]
        command_text = shlex.join(command)
        try:
            with log.open("w") as stream:
                proc = subprocess.run(command, cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
            code = 2 if proc.returncode in (2, 127) else (1 if proc.returncode != 0 else 0)
            detail = f"interaction parity exited {proc.returncode}"
        except OSError as exc:
            log.write_text(str(exc) + "\n")
            code, detail = 2, str(exc)
    elif index == 5:
        code, detail, command_text = run_case_tests(log)
    elif index == 6:
        command_text = f"network preflight HEAD {args.live_url}; {rooted(args.binary)} -gate -frames 120 -backend headless -url {args.live_url} -out {out / 'live-smoke-frame.json'}"
        code, detail = live_smoke(log)
    else:
        command_text = f"read {rooted(args.defects)} and {rooted(args.signoffs)}/{head}.json"
        code, detail = defects_signoff(log)
    elapsed = time.monotonic() - started
    results.append({"index": index, "name": name, "command": command_text, "exit": code, "duration_seconds": round(elapsed, 3), "log": str(log), "detail": detail})
    if code == 1:
        any_fail = True
    elif code == 2:
        any_no_verdict = True
    print(f"rc-sweep: {name}: {'PASS' if code == 0 else ('FAIL' if code == 1 else 'NO VERDICT')} - {detail}")

status = "fail" if any_fail else ("no_verdict" if any_no_verdict else "pass")
report = {"status": status, "fingerprint": fingerprint, "steps": results}
try:
    report_path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
except OSError as exc:
    die(f"cannot write report: {exc}")
if any_fail:
    print(f"rc-sweep: FAIL (report: {report_path})")
    raise SystemExit(1)
if any_no_verdict:
    print(f"rc-sweep: NO VERDICT (report: {report_path})")
    raise SystemExit(2)
print(f"rc-sweep: PASS (7 steps, report: {report_path})")
PY
