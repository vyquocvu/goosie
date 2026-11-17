#!/usr/bin/env python3
"""Base-pinned verification; isolation is not an OS security sandbox."""

import sys

if __name__ == "__main__" and not sys.flags.isolated:
    import os
    os.execv(sys.executable, [sys.executable, "-I", __file__, *sys.argv[1:]])

import argparse
import datetime
import errno
import fcntl
import fnmatch
import hashlib
import json
import math
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import stat
import subprocess
import tempfile


RUNNER_PATH = "scripts/verification_gate.py"
POLICY_PATH = "docs/verification/gate.json"
REVIEW_PATH = "REVIEW.md"
MANDATORY_PROTECTED = (
    RUNNER_PATH, POLICY_PATH, REVIEW_PATH, "scripts/**", "test/**", "tests/**",
    "testdata/**", "fixtures/**", "**/fixtures/**", "**/*_test.go",
    "**/*_test.py", "**/test_*.py", "**/*.test.*", "**/*.spec.*",
)


def git(root, *args, env=None, input_data=None):
    result = subprocess.run(["git", "-C", str(root), *args], input=input_data,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            env=env, timeout=60)
    if result.returncode:
        raise ValueError("git " + args[0] + ": " + result.stderr.decode("utf-8", "replace").strip())
    return result.stdout


def validate_path(path):
    parts = path.split("/")
    if (not path or "\\" in path or "\x00" in path or
            any(p in ("", ".", "..") or p.casefold() == ".git" for p in parts) or
            PurePosixPath(path).is_absolute()):
        raise ValueError("Unsafe snapshot path: " + repr(path))
    return path


def admitted_path(path):
    validate_path(path)
    return "node_modules" not in path.split("/")


def regular_mode(mode, path):
    if mode not in ("100644", "100755"):
        kind = "symlink" if mode == "120000" else "submodule or nonregular entry"
        raise ValueError("Rejected " + kind + ": " + path)


def read_regular(root, path):
    validate_path(path)
    directory = os.open(str(root), os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        parts = path.split("/")
        for part in parts[:-1]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=directory)
            os.close(directory)
            directory = child
        fd = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=directory)
        with os.fdopen(fd, "rb") as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode):
                raise ValueError("Rejected nonregular file: " + path)
            return ("100755" if info.st_mode & 0o111 else "100644", stream.read())
    except OSError as exc:
        if exc.errno in (errno.ELOOP, errno.ENOTDIR):
            raise ValueError("Rejected symlink or non-directory ancestor: " + path) from exc
        raise
    finally:
        os.close(directory)


def blob_entries(root, records, index=False):
    entries = {}
    for record in records.split(b"\x00"):
        if not record:
            continue
        header, raw_path = record.split(b"\t", 1)
        path = os.fsdecode(raw_path)
        if not admitted_path(path):
            continue
        fields = header.decode("ascii").split()
        mode, oid = (fields[0], fields[1]) if index else (fields[0], fields[2])
        regular_mode(mode, path)
        if index and fields[2] != "0":
            raise ValueError("Unmerged index entry: " + path)
        entries[path] = (mode, oid)
    return entries


def load_blobs(root, entries):
    if not entries:
        return {}
    names = sorted(entries)
    requests = b"".join(entries[name][1].encode("ascii") + b"\n" for name in names)
    data = git(root, "cat-file", "--batch", input_data=requests)
    result = {}
    offset = 0
    for name in names:
        end = data.index(b"\n", offset)
        header = data[offset:end].split()
        if len(header) != 3 or header[1] != b"blob":
            raise ValueError("Missing regular blob: " + name)
        size = int(header[2])
        start = end + 1
        result[name] = (entries[name][0], data[start:start + size])
        offset = start + size + 1
    return result


def base_tree(root, base):
    return load_blobs(root, blob_entries(root, git(root, "ls-tree", "-rz", base)))


def candidate_tree(root, staged):
    entries = blob_entries(root, git(root, "ls-files", "--stage", "-z"), index=True)
    if staged:
        return load_blobs(root, entries)
    names = set(entries)
    for raw in git(root, "ls-files", "--others", "--exclude-standard", "-z").split(b"\x00"):
        if raw:
            path = os.fsdecode(raw)
            if admitted_path(path):
                names.add(path)
    result = {}
    for name in sorted(names):
        try:
            result[name] = read_regular(root, name)
        except FileNotFoundError:
            if name not in entries:
                raise ValueError("Untracked file changed during snapshot: " + name)
    return result


def digest_tree(tree, base):
    digest = hashlib.sha256()
    for chunk in [base.encode("ascii")] + [part for name in sorted(tree)
            for part in (os.fsencode(name), tree[name][0].encode("ascii"), tree[name][1])]:
        digest.update(len(chunk).to_bytes(8, "big"))
        digest.update(chunk)
    return digest.hexdigest()


def matches(path, patterns):
    return any(fnmatch.fnmatchcase(path, pattern) or
               (pattern.startswith("**/") and fnmatch.fnmatchcase(path, pattern[3:]))
               for pattern in patterns)


def is_go_test(command):
    return len(command) > 1 and Path(command[0]).name == "go" and command[1] == "test"


def validate_policy(policy):
    if not isinstance(policy, dict) or type(policy.get("version")) is not int or policy["version"] != 1:
        raise ValueError("Policy version must be 1")
    if policy.get("default_mode", "observe") not in ("observe", "enforce", "off"):
        raise ValueError("Invalid default_mode")
    interval = policy.get("review_interval_days")
    if type(interval) is not int or interval < 1:
        raise ValueError("review_interval_days must be positive")
    datetime.date.fromisoformat(policy["last_reviewed"])
    for key in ("protected",):
        if not isinstance(policy.get(key), list) or not all(isinstance(p, str) and p for p in policy[key]):
            raise ValueError("Invalid " + key)
    suites = policy.get("suites")
    if not isinstance(suites, dict) or not all(isinstance(suites.get(s), list) and suites[s] for s in ("smoke", "full")):
        raise ValueError("Nonempty smoke and full suites required")
    triggers = policy.get("triggers")
    if not isinstance(triggers, list):
        raise ValueError("triggers must be a list")
    ids = set()
    for entry in triggers:
        if not isinstance(entry, dict) or not isinstance(entry.get("id"), str) or not entry["id"] or entry["id"] in ids:
            raise ValueError("Trigger IDs must be nonempty and unique")
        ids.add(entry["id"])
        if not isinstance(entry.get("paths"), list) or not entry["paths"] or not all(isinstance(p, str) and p for p in entry["paths"]):
            raise ValueError("Trigger paths must be glob strings")
    for entry in suites["smoke"] + suites["full"] + triggers:
        if not isinstance(entry, dict):
            raise ValueError("Invalid command entry")
        name = entry.get("name", entry.get("id"))
        command = entry.get("command")
        timeout = entry.get("timeout")
        if not isinstance(name, str) or not name:
            raise ValueError("Command name is required")
        if not isinstance(command, list) or not command or not all(isinstance(arg, str) and arg and "\x00" not in arg for arg in command):
            raise ValueError("Commands must be nonempty argv arrays, never shell strings")
        if type(timeout) not in (int, float) or not math.isfinite(timeout) or not 0 < timeout <= 86400:
            raise ValueError("Command timeout must be positive and bounded to one day")
        if is_go_test(command) and "-json" not in command[2:]:
            raise ValueError("Every go test command must use -json")
    return policy


def policy_from_tree(tree):
    for name in (POLICY_PATH, REVIEW_PATH):
        if name not in tree or not tree[name][1].strip():
            raise ValueError("Missing trusted policy file: " + name)
    return validate_policy(json.loads(tree[POLICY_PATH][1]))


def review_due(policy, today=None):
    days = ((today or datetime.date.today()) - datetime.date.fromisoformat(policy["last_reviewed"])).days
    return days >= policy["review_interval_days"]


def private_environment(work):
    home = work / "home"
    home.mkdir()
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("GIT_", "PYTHON", "XDG_")) and k not in ("HOME", "TMPDIR", "TMP", "TEMP", "GOENV", "GOFLAGS", "GOWORK")}
    caller_home = Path.home()
    caches = [os.environ.get("GOCACHE"), os.environ.get("GOMODCACHE")]
    if not all(caches) and shutil.which("go"):
        query_env = dict(os.environ, GOTOOLCHAIN="local")
        try:
            result = subprocess.run(["go", "env", "GOCACHE", "GOMODCACHE"], env=query_env,
                                    cwd=str(work), stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                    text=True, timeout=10)
            found = result.stdout.strip().splitlines()
            if result.returncode == 0 and len(found) == 2:
                caches = [caches[i] or found[i] for i in range(2)]
        except (OSError, subprocess.TimeoutExpired):
            pass
    default_cache = caller_home / ("Library/Caches/go-build" if sys.platform == "darwin" else ".cache/go-build")
    env.update(HOME=str(home), GOCACHE=caches[0] or str(default_cache),
               GOMODCACHE=caches[1] or str(caller_home / "go/pkg/mod"),
               GOENV="off", GOWORK="off", GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
               PYTHONDONTWRITEBYTECODE="1", PYTHONNOUSERSITE="1")
    for key, name in (("XDG_CONFIG_HOME", "config"), ("XDG_CACHE_HOME", "cache"),
                      ("XDG_DATA_HOME", "data"), ("XDG_STATE_HOME", "state"),
                      ("XDG_RUNTIME_DIR", "runtime"), ("TMPDIR", "tmp")):
        directory = home / name
        directory.mkdir(mode=0o700)
        env[key] = str(directory)
    env["TMP"] = env["TEMP"] = env["TMPDIR"]
    return env


def make_snapshot(snapshot, tree, env):
    snapshot.mkdir()
    for name, (mode, content) in sorted(tree.items()):
        destination = snapshot / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(content)
        destination.chmod(0o755 if mode == "100755" else 0o644)
    git(snapshot, "init", "--template=", env=env)
    git(snapshot, "add", "--all", "--force", env=env)
    git(snapshot, "-c", "user.name=Goosie Verification Snapshot", "-c",
        "user.email=verification@example.invalid", "commit", "-m", "Disposable verification snapshot", env=env)


def kill_group(process):
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        pass
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    process.wait()


def wait_delegated(process):
    try:
        return process.wait()
    except (InterruptedError, KeyboardInterrupt):
        process.terminate()
        try:
            return process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            kill_group(process)
            return 1


def run_command(entry, snapshot, env, evidence, number, report):
    command = [sys.executable if arg == "{python}" else arg.replace("{evidence}", str(evidence)) for arg in entry["command"]]
    go_test = is_go_test(command)
    if go_test:
        command.insert(2, "-count=1")
        if any(arg.startswith("-count") and arg != "-count=1" for arg in command[3:]):
            raise ValueError("Go test caching/count overrides are not allowed")
    record = {"name": entry.get("name", entry.get("id")), "command": command,
              "timeout": entry["timeout"], "exit": None, "status": "running",
              "stdout": str(evidence / (str(number) + ".stdout.log")),
              "stderr": str(evidence / (str(number) + ".stderr.log")), "skips": []}
    report["commands"].append(record)
    process = None
    with open(record["stdout"], "wb") as stdout, open(record["stderr"], "wb") as stderr:
        try:
            process = subprocess.Popen(command, cwd=str(snapshot), env=env, stdin=subprocess.DEVNULL,
                                       stdout=stdout, stderr=stderr, start_new_session=True)
            record["pid"] = process.pid
            record["exit"] = process.wait(timeout=entry["timeout"])
            record["status"] = "passed" if record["exit"] == 0 else "failed"
        except subprocess.TimeoutExpired:
            record["status"] = "timeout"
        except InterruptedError:
            raise
        except OSError as exc:
            record["status"] = "failed"
            record["error"] = str(exc)
            stderr.write(str(exc).encode("utf-8", "replace"))
        finally:
            if process is not None:
                kill_group(process)
                record["exit"] = process.returncode
    if go_test:
        passed = 0
        invalid = False
        with open(record["stdout"], encoding="utf-8", errors="replace") as stream:
            for line in stream:
                try:
                    event = json.loads(line)
                except ValueError:
                    invalid = True
                    continue
                if not isinstance(event, dict):
                    invalid = True
                    continue
                action = event.get("Action")
                if action == "skip":
                    record["skips"].append(event)
                if action in ("fail", "skip"):
                    invalid = True
                if action == "pass" and str(event.get("Test", "")).startswith("Test"):
                    passed += 1
        record["passed_tests"] = passed
        if record["status"] == "passed" and (invalid or not passed):
            record["status"] = "failed"
            record["error"] = "Go JSON requires a passed Test event and no fail/skip/malformed events"
        report["skips"].extend(record["skips"])
    return record


def record_failure(state, base, digest, reset=False):
    state.mkdir(parents=True, exist_ok=True, mode=0o700)
    key = hashlib.sha256((base + ":" + digest).encode("ascii")).hexdigest()
    lock_fd = os.open(str(state / "attempts.lock"), os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(lock_fd, "r+") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        path = state / (key + ".json")
        count = json.loads(path.read_text())["failures"] if path.exists() else 0
        count = 0 if reset else count + 1
        fd, temporary = tempfile.mkstemp(prefix="attempt-", dir=str(state))
        try:
            with os.fdopen(fd, "w") as stream:
                json.dump({"base": base, "digest": digest, "failures": count}, stream)
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, path)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)
        return count


def failure_count(state, base, digest):
    key = hashlib.sha256((base + ":" + digest).encode("ascii")).hexdigest()
    path = state / (key + ".json")
    return json.loads(path.read_text())["failures"] if path.exists() else 0


def strip_verification(text):
    lines = []
    removed = False
    for line in text.splitlines():
        if re.match(r"^\s*(?:Verified-by|Verification-evidence)\s*:", line, re.I):
            removed = True
            continue
        if removed and line.startswith((" ", "\t")):
            continue
        removed = False
        lines.append(line)
    return "\n".join(lines).rstrip()


def skip_reason(text):
    # Only a trailer in the final paragraph can grant an explained escape.
    paragraph = text.strip().split("\n\n")[-1]
    reasons = re.findall(r"^Verification-skip:\s*(.*?)\s*$", paragraph, re.I | re.M)
    if len(reasons) != 1:
        return None
    reason = reasons[0]
    words = re.findall(r"[a-z0-9]+", reason.lower())
    if len(re.sub(r"\s", "", reason)) < 12 or not words or set(words) <= {"skip", "later"}:
        return None
    return reason


def write_message(path, original, verified, evidence):
    content = strip_verification(original)
    path.write_text(content + "\n\nVerified-by: " + verified + "\nVerification-evidence: " + str(evidence) + "\n")


def install(root, base, runner_blob, report):
    git_dir = Path(git(root, "rev-parse", "--absolute-git-dir").decode().strip())
    hooks = git_dir / "hooks"
    setting = subprocess.run(["git", "-C", str(root), "config", "--path", "--get", "core.hooksPath"],
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=10)
    if setting.returncode not in (0, 1):
        raise ValueError("Cannot inspect core.hooksPath")
    if setting.returncode == 0:
        configured = Path(setting.stdout.strip())
        if not configured.is_absolute():
            configured = root / configured
        if configured.resolve() != hooks.resolve():
            raise ValueError("core.hooksPath must point to the actual .git/hooks; no bypass installed")
    if hooks.is_symlink():
        raise ValueError("Refusing symlink hooks directory")
    hooks.mkdir(exist_ok=True)
    for name in ("goosie-verify", "commit-msg"):
        existing = hooks / name
        if existing.is_symlink() or (existing.exists() and (not existing.is_file() or existing.stat().st_size)):
            raise ValueError("Refusing existing nonempty hook: " + str(existing))
    launcher = hooks / "goosie-verify"
    wrapper = hooks / "commit-msg"
    git_binary = shutil.which("git")
    source = ("#!" + sys.executable + " -I\n"
              "import os, pathlib, signal, subprocess, sys, tempfile\n"
              "def interrupted(signum, frame): raise KeyboardInterrupt\n"
              "signal.signal(signal.SIGTERM, interrupted)\n"
              "signal.signal(signal.SIGINT, interrupted)\n"
              "BASE = " + repr(base) + "\n"
              "BLOB = " + repr(runner_blob) + "\n"
              "GIT = " + repr(git_binary) + "\n"
              "root = subprocess.check_output([GIT, 'rev-parse', '--show-toplevel'], text=True).strip()\n"
              "source = subprocess.check_output([GIT, '-C', root, 'show', BASE + ':" + RUNNER_PATH + "'])\n"
              "actual = subprocess.check_output([GIT, '-C', root, 'hash-object', '--stdin'], input=source).decode().strip()\n"
              "if actual != BLOB: raise SystemExit('Pinned runner blob mismatch')\n"
              "with tempfile.TemporaryDirectory(prefix='goosie-gate-runner-') as directory:\n"
              "    runner = pathlib.Path(directory) / 'runner.py'\n"
              "    runner.write_bytes(source)\n"
              "    process = subprocess.Popen([sys.executable, '-I', str(runner), *sys.argv[1:], '--base', BASE], cwd=root, start_new_session=True)\n"
              "    try:\n"
              "        code = process.wait()\n"
              "    except KeyboardInterrupt:\n"
              "        process.terminate()\n"
              "        try:\n"
              "            code = process.wait(timeout=10)\n"
              "        except subprocess.TimeoutExpired:\n"
              "            os.killpg(process.pid, signal.SIGKILL)\n"
              "            process.wait()\n"
              "            code = 1\n"
              "raise SystemExit(code)\n")
    wrapper_source = ("#!" + sys.executable + " -I\n"
                      "import os, pathlib, sys\n"
                      "launcher = pathlib.Path(__file__).with_name('goosie-verify')\n"
                      "os.execv(sys.executable, [sys.executable, '-I', str(launcher), 'commit-msg', *sys.argv[1:]])\n")
    created = []
    try:
        for path, content in ((launcher, source), (wrapper, wrapper_source)):
            if path.exists():
                if path.stat().st_size:
                    raise ValueError("Hook changed during install: " + str(path))
                path.unlink()
            with path.open("x") as stream:
                created.append(path)
                stream.write(content)
            path.chmod(0o755)
    except BaseException:
        for path in created:
            path.unlink()
        raise
    report.update(status="installed", hooks=[str(launcher), str(wrapper)])
    return 0


def execute(root, args, base, baseline, report, evidence):
    bootstrap = getattr(args, "bootstrap", False)
    staged = args.action == "commit-msg" or getattr(args, "staged", False)
    candidate_ref = git(root, "rev-parse", "HEAD").decode().strip() if args.action == "pr" else None
    candidate = base_tree(root, candidate_ref) if candidate_ref else candidate_tree(root, staged)
    report["candidate_ref"] = candidate_ref
    policy = policy_from_tree(candidate if bootstrap else baseline)
    report.update(mode=policy.get("default_mode", "observe"), trusted=not bootstrap,
                  staged=staged, digest=digest_tree(candidate, base), paths=sorted(candidate))
    changed = sorted(name for name in set(baseline) | set(candidate) if baseline.get(name) != candidate.get(name))
    patterns = tuple(policy["protected"]) + MANDATORY_PROTECTED
    protected = sorted(name for name in changed if matches(name, patterns))
    report["protected_edits"] = protected
    report["changed_paths"] = changed
    tree = dict(candidate)
    if not bootstrap:
        for name in list(tree):
            if matches(name, patterns):
                del tree[name]
        tree.update({name: value for name, value in baseline.items() if matches(name, patterns)})
    suite = "full" if args.action in ("commit-msg", "task", "pr") else args.suite
    report["suite"] = suite
    off_commit = args.action == "commit-msg" and report["mode"] == "off"
    state = Path(git(root, "rev-parse", "--absolute-git-dir").decode().strip()) / "goosie-verification"
    report["previous_failures"] = failure_count(state, base, report["digest"])
    session = hashlib.sha256(os.environ.get("GOOSIE_VERIFICATION_SESSION", "local").encode()).hexdigest()
    report["session_blocks"] = failure_count(state, "session", session)
    escape = args.action == "commit-msg" and report["mode"] == "enforce" and report["session_blocks"] >= 2
    if escape:
        record_failure(state, "session", session, reset=True)
    commands = [] if off_commit or escape else list(policy["suites"]["smoke"])
    if suite == "full" and not off_commit and not escape:
        commands += policy["suites"]["full"]
        for trigger in policy["triggers"]:
            if any(matches(name, trigger["paths"]) for name in changed):
                report["matched_triggers"].append(trigger["id"])
                commands.append(trigger)
    if commands and suite == "full" and policy.get("requires_disposable_host") and os.environ.get("GOOSIE_VERIFICATION_DISPOSABLE_HOST") != "1":
        report["preflight_error"] = "Full verification requires a disposable host: native tests write the system clipboard. Set GOOSIE_VERIFICATION_DISPOSABLE_HOST=1 only inside an isolated CI runner or VM."
        commands = []
    if commands:
        with tempfile.TemporaryDirectory(prefix="goosie-gate-work-") as directory:
            work = Path(directory)
            env = private_environment(work)
            snapshot = work / "snapshot"
            report.update(snapshot=str(snapshot), home=env["HOME"])
            make_snapshot(snapshot, tree, env)
            for number, command in enumerate(commands):
                try:
                    run_command(command, snapshot, env, evidence, number, report)
                finally:
                    if not report["attempts"] and any(c["status"] != "passed" for c in report["commands"]):
                        report["attempts"] = record_failure(state, base, report["digest"])
    current_ref = git(root, "rev-parse", "HEAD").decode().strip() if candidate_ref else None
    after = digest_tree(base_tree(root, current_ref) if current_ref else candidate_tree(root, staged), base)
    report["digest_after"] = after
    green = bool(commands) and all(c["status"] == "passed" for c in report["commands"])
    unchanged = after == report["digest"] and current_ref == candidate_ref
    passed = green and unchanged and (bootstrap or not protected) and not report["previous_failures"]
    report["status"] = ("preflight-blocked" if report.get("preflight_error") else
                        "changed-tree" if not unchanged else
                        "protected-edits" if protected and not bootstrap else
                        "bootstrap-pass" if bootstrap and passed else "passed" if passed else "failed")
    report["would_block"] = not passed
    if args.action != "commit-msg":
        return 0 if passed else 1
    reason = skip_reason(report["message_original"])
    if reason:
        report["skips"].append({"reason": reason, "source": "Verification-skip"})
    verified = passed and report["mode"] == "enforce" and not reason and not bootstrap
    allowed = verified or report["mode"] in ("observe", "off") or bool(reason) or passed or escape
    if not allowed:
        report["session_blocks"] = record_failure(state, "session", session)
    report["escape_decision"] = ("two-block-escape" if escape else
                                 "explained-skip" if reason else "off" if off_commit else
                                 "observe-advisory" if report["mode"] == "observe" else
                                 "verified" if verified else "blocked")
    report.update(allowed=allowed, status="verified" if verified else "unverified")
    trailer = "goosie-gate:" + base[:12] + ":" + report["digest"][:12] if verified else "none"
    write_message(Path(args.message), report["message_original"], trailer, report["evidence"])
    return 0 if allowed else 1


def parser():
    cli = argparse.ArgumentParser(description=__doc__)
    actions = cli.add_subparsers(dest="action", required=True)
    for name in ("check", "task", "pr", "install", "commit-msg", "audit"):
        action = actions.add_parser(name)
        action.add_argument("--base", default="main")
        if name == "check":
            action.add_argument("--suite", choices=("smoke", "full"), default="smoke")
            action.add_argument("--bootstrap", action="store_true", help="Development only: no trusted claim")
        if name in ("check", "task"):
            action.add_argument("--staged", action="store_true")
        if name == "commit-msg":
            action.add_argument("message")
    return cli


def new_report(args, evidence):
    return {"version": 1, "action": args.action, "base": None, "digest": None,
            "mode": None, "status": "error", "exit": 1, "trusted": False,
            "commands": [], "paths": [], "protected_edits": [], "skips": [],
            "matched_triggers": [], "attempts": 0, "escape_decision": "none",
            "evidence": str(evidence / "report.json"), "runner": str(Path(__file__).resolve()),
            "would_block": True, "allowed": False}


def emit_report(report, evidence):
    report.pop("message_original", None)
    text = json.dumps(report, indent=2, sort_keys=True) + "\n"
    (evidence / "report.json").write_text(text)
    for path in evidence.rglob("*"):
        if not path.is_symlink():
            path.chmod(0o555 if path.is_dir() else 0o444)
    evidence.chmod(0o555)
    print(text, end="", flush=True)


def interrupted(signum, frame):
    raise InterruptedError("Verification interrupted by signal " + str(signum))


def main(argv=None):
    args = parser().parse_args(argv)
    evidence = Path(tempfile.mkdtemp(prefix="goosie-gate-evidence-"))
    report = new_report(args, evidence)
    delegated = False
    original = None
    try:
        if args.action == "commit-msg":
            message = Path(args.message).absolute()
            args.message = str(message)
            if message.is_symlink() or not message.is_file():
                raise ValueError("Commit message must be a regular file")
            original = message.read_text()
            # Remove forged claims even when policy loading subsequently fails.
            write_message(message, original, "none", report["evidence"])
            report["message_original"] = original
        root = Path(git(Path.cwd(), "rev-parse", "--show-toplevel").decode().strip()).resolve()
        base = git(root, "rev-parse", "--verify", "--end-of-options", args.base + "^{commit}").decode().strip()
        report["base"] = base
        bootstrap = getattr(args, "bootstrap", False)
        if not bootstrap:
            runner = git(root, "show", base + ":" + RUNNER_PATH)
            runner_blob = git(root, "rev-parse", base + ":" + RUNNER_PATH).decode().strip()
            report["runner_blob"] = runner_blob
            current = Path(__file__).resolve()
            selected = current.name == "runner.py" and current.parent.name.startswith("goosie-gate-runner-") and current.read_bytes() == runner
            if not selected:
                with tempfile.TemporaryDirectory(prefix="goosie-gate-runner-") as directory:
                    path = Path(directory) / "runner.py"
                    path.write_bytes(runner)
                    command = [sys.executable, "-I", str(path), *(argv if argv is not None else sys.argv[1:]), "--base", base]
                    # Preserve the original message for the authoritative runner.
                    if original is not None:
                        Path(args.message).write_text(original)
                    process = subprocess.Popen(command, cwd=str(root), start_new_session=True)
                    delegated = True
                    return wait_delegated(process)
        baseline = base_tree(root, base)
        if args.action in ("install", "audit"):
            policy = policy_from_tree(baseline)
            report.update(mode=policy.get("default_mode", "observe"), trusted=True)
            if args.action == "install":
                code = install(root, base, report["runner_blob"], report)
            else:
                due = review_due(policy)
                report.update(status="review-due" if due else "review-current", review_due=due,
                              last_reviewed=policy["last_reviewed"], review_interval_days=policy["review_interval_days"])
                code = 1 if due else 0
        else:
            code = execute(root, args, base, baseline, report, evidence)
        report["exit"] = code
    except (ValueError, OSError, KeyError, TypeError, subprocess.SubprocessError, KeyboardInterrupt) as exc:
        report.update(status="error", error=str(exc), exit=1, allowed=False, would_block=True)
        if original is not None:
            write_message(Path(args.message), original, "none", report["evidence"])
    finally:
        if delegated:
            shutil.rmtree(str(evidence))
        else:
            emit_report(report, evidence)
    return report["exit"]


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    sys.exit(main())
