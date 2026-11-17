import datetime
import importlib.util
import json
import os
from pathlib import Path
import socket
import signal
import subprocess
import sys
import tempfile
import unittest


RUNNER = Path(__file__).with_name("verification_gate.py")
IDENTITY = ["-c", "user.name=Gate Fixture", "-c", "user.email=gate@example.invalid"]


class VerificationGateTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(RUNNER.is_file(), "verification_gate.py is not implemented")
        self.temp = tempfile.TemporaryDirectory(prefix="gate-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.home = self.root / "caller-home"
        self.home.mkdir()
        self.env = {k: v for k, v in os.environ.items()
                    if not k.startswith(("GIT_", "PYTHON", "XDG_"))}
        self.env.pop("GOOSIE_VERIFICATION_DISPOSABLE_HOST", None)
        self.env.pop("GOOSIE_VERIFICATION_SESSION", None)
        self.env.update(HOME=str(self.home), GIT_CONFIG_NOSYSTEM="1",
                        GIT_CONFIG_GLOBAL=os.devnull, PYTHONDONTWRITEBYTECODE="1")
        self.git("init", "-b", "main")
        self.write("scripts/verification_gate.py", RUNNER.read_text())
        self.write("REVIEW.md", "Human reviewed fixture policy.\n")
        self.write("app.data", "pass")
        self.write("tests/assertion.py", "baseline assertion\n")
        self.write(".gitignore", "ignored.bin\nnode_modules/\n")
        self.fake_go = self.root / "go"
        self.fake_go.write_text(
            "#!" + sys.executable + "\n"
            "import json, pathlib, sys\n"
            "assert '-json' in sys.argv and '-count=1' in sys.argv\n"
            "action = pathlib.Path('app.data').read_text().strip()\n"
            "if action == 'empty':\n"
            "    print(json.dumps({'Action': 'pass', 'Package': 'fixture'}))\n"
            "else:\n"
            "    print(json.dumps({'Action': action, 'Test': 'TestFixture'}))\n"
            "print('retained stderr', file=sys.stderr)\n")
        self.fake_go.chmod(0o755)
        self.policy = {
            "version": 1, "default_mode": "enforce", "review_interval_days": 30,
            "last_reviewed": "2026-11-17",
            "protected": ["scripts/**", "tests/**", "fixtures/**"],
            "suites": {"smoke": [self.py("smoke", "print('smoke ran')")],
                       "full": [{"name": "go-tests", "command": [str(self.fake_go), "test", "-json", "./..."], "timeout": 10}]},
            "triggers": [{"id": "app-trigger", "paths": ["app.data"],
                          "command": ["{python}", "-c", "print('trigger ran')"], "timeout": 10}]}
        self.save_policy()
        self.commit_fixture()
        self.base = self.git("rev-parse", "HEAD").stdout.strip()

    def write(self, name, content):
        path = self.repo / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def py(self, name, code, timeout=10):
        return {"name": name, "command": ["{python}", "-c", code], "timeout": timeout}

    def save_policy(self):
        self.write("docs/verification/gate.json", json.dumps(self.policy))

    def git(self, *args, check=True, extra_env=None):
        env = dict(self.env)
        env.update(extra_env or {})
        result = subprocess.run(["git", *args], cwd=str(self.repo), env=env,
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if check:
            self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def commit_fixture(self):
        self.git("add", "--all")
        self.git(*IDENTITY, "commit", "-m", "Fixture policy")

    def update_base(self):
        self.save_policy()
        self.commit_fixture()
        self.base = self.git("rev-parse", "HEAD").stdout.strip()

    def cli(self, *args, expected=0, script=None, extra_env=None):
        env = dict(self.env)
        env.update(extra_env or {})
        result = subprocess.run([sys.executable, "-I", str(script or RUNNER), *args],
                                cwd=str(self.repo), env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=40)
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        try:
            report = json.loads(result.stdout)
        except ValueError:
            self.fail("Gate did not return JSON: " + result.stdout + result.stderr)
        evidence = Path(report["evidence"])
        self.assertTrue(evidence.is_file())
        self.assertEqual(json.loads(evidence.read_text()), report)
        self.assertEqual(evidence.stat().st_mode & 0o222, 0)
        print("evidence: " + str(evidence), flush=True)
        return report

    def check(self, *args, expected=0):
        return self.cli("check", "--base", self.base, *args, expected=expected)

    def message(self, text="Fixture change\n"):
        path = self.root / "message"
        path.write_text(text)
        return path

    def commit_message(self, path, expected=0, **kwargs):
        return self.cli("commit-msg", str(path), "--base", self.base, expected=expected, **kwargs)

    def test_base_policy_cannot_be_weakened(self):
        self.write("app.data", "fail")
        self.policy["suites"]["full"] = [self.py("fake-pass", "print('pass')")]
        self.policy["default_mode"] = "off"
        self.policy["protected"] = []
        self.save_policy()
        report = self.check("--suite", "full", expected=1)
        self.assertEqual(report["mode"], "enforce")
        self.assertIn("docs/verification/gate.json", report["protected_edits"])
        self.assertEqual([c["name"] for c in report["commands"]],
                         ["smoke", "go-tests", "app-trigger"])
        self.assertEqual(report["commands"][1]["status"], "failed")

    def test_protected_modified_deleted_and_added_are_restored(self):
        self.write("fixtures/old.txt", "old")
        self.policy["suites"]["smoke"] = [self.py("restored", "from pathlib import Path; "
            "assert Path('tests/assertion.py').read_text() == 'baseline assertion\\n'; "
            "assert Path('fixtures/old.txt').read_text() == 'old'; "
            "assert not Path('tests/new.py').exists()")]
        self.update_base()
        self.write("tests/assertion.py", "weakened")
        (self.repo / "fixtures/old.txt").unlink()
        self.write("tests/new.py", "self grading")
        report = self.check("--suite", "full", expected=1)
        self.assertEqual(report["protected_edits"], ["fixtures/old.txt", "tests/assertion.py", "tests/new.py"])
        self.assertTrue(all(c["status"] == "passed" for c in report["commands"]))
        self.assertEqual(report["status"], "protected-edits")

    def test_staged_snapshot_uses_index_and_digest_ignores_unstaged(self):
        self.write("app.data", "pass\n")
        self.git("add", "app.data")
        first = self.check("--suite", "full", "--staged")
        self.write("app.data", "fail")
        self.write("untracked.txt", "not in index")
        second = self.check("--suite", "full", "--staged")
        worktree = self.check("--suite", "full", expected=1)
        self.assertEqual(first["digest"], second["digest"])
        self.assertNotEqual(first["digest"], worktree["digest"])
        self.assertNotEqual(first["evidence"], second["evidence"])
        self.git("add", "app.data")
        third = self.check("--suite", "full", "--staged", expected=1)
        self.assertNotEqual(first["digest"], third["digest"])

    def test_worktree_untracked_and_executable_modes_are_hashed(self):
        first = self.check("--suite", "smoke")
        path = self.write("extra.txt", "included")
        second = self.check("--suite", "smoke")
        path.chmod(0o755)
        third = self.check("--suite", "smoke")
        self.assertNotEqual(first["digest"], second["digest"])
        self.assertNotEqual(second["digest"], third["digest"])
        self.assertIn("extra.txt", third["paths"])

    def test_snapshot_has_head_private_home_and_no_ignored_files(self):
        self.policy["suites"]["smoke"] = [self.py("isolation", "import os, pathlib, subprocess; "
            "assert os.environ['HOME'] != " + repr(str(self.home)) + "; "
            "assert os.environ['XDG_CONFIG_HOME'].startswith(os.environ['HOME']); "
            "assert os.environ['GOCACHE'] and os.environ['GOMODCACHE']; "
            "assert not pathlib.Path('ignored.bin').exists(); "
            "assert not pathlib.Path('node_modules').exists(); "
            "assert not pathlib.Path('.git/user-secret').exists(); "
            "subprocess.run(['git', 'rev-parse', 'HEAD'], check=True)")]
        self.update_base()
        self.write("ignored.bin", "binary")
        self.write("node_modules/thing", "dependency")
        self.write(".git/user-secret", "private metadata")
        report = self.check("--suite", "smoke")
        self.assertFalse(Path(report["snapshot"]).exists())
        self.assertFalse(Path(report["home"]).exists())
        self.assertNotIn("ignored.bin", report["paths"])

    def test_candidate_cli_reexecutes_base_runner(self):
        candidate = self.repo / "scripts/verification_gate.py"
        candidate.write_text(candidate.read_text().replace('"goosie-gate:"', '"candidate-gate:"'))
        report = self.cli("check", "--base", self.base, "--suite", "smoke", expected=1, script=candidate)
        self.assertEqual(report["runner_blob"], self.git("rev-parse", self.base + ":scripts/verification_gate.py").stdout.strip())
        self.assertNotEqual(Path(report["runner"]), candidate)
        self.assertFalse(Path(report["runner"]).exists())
        self.assertIn("scripts/verification_gate.py", report["protected_edits"])

    def test_candidate_modules_are_not_imported(self):
        self.write("json.py", "raise RuntimeError('candidate import')")
        self.write("scripts/json.py", "raise RuntimeError('candidate import')")
        self.check("--suite", "smoke", expected=1)

    def test_missing_policy_or_runner_refuses_check_and_install(self):
        self.git("rm", "docs/verification/gate.json")
        self.commit_fixture()
        self.base = self.git("rev-parse", "HEAD").stdout.strip()
        report = self.check("--suite", "smoke", expected=1)
        self.assertEqual(report["status"], "error")
        self.cli("install", "--base", self.base, expected=1)
        self.assertFalse((self.repo / ".git/hooks/goosie-verify").exists())
        self.git("rm", "scripts/verification_gate.py")
        self.commit_fixture()
        self.cli("install", "--base", "main", expected=1)

    def test_go_json_requires_pass_and_rejects_fail_skip_and_empty(self):
        for action, expected in [("pass", 0), ("fail", 1), ("skip", 1), ("empty", 1)]:
            with self.subTest(action=action):
                self.write("app.data", action)
                report = self.check("--suite", "full", expected=expected)
                result = report["commands"][1]
                self.assertIn("retained stderr", Path(result["stderr"]).read_text())
                self.assertIn('"Action"', Path(result["stdout"]).read_text())
                if action == "skip":
                    self.assertTrue(report["skips"])
                self.assertIn("-count=1", result["command"])

    def test_go_test_without_json_is_invalid_policy(self):
        self.policy["suites"]["full"][0]["command"].remove("-json")
        self.update_base()
        report = self.check("--suite", "full", expected=1)
        self.assertIn("-json", report["error"])

    def test_all_commands_continue_after_failure_and_triggers_only_full(self):
        self.policy["suites"]["smoke"] = [self.py("failure", "raise SystemExit(3)")]
        self.policy["suites"]["full"].append(self.py("lint", "print('lint ran')"))
        self.update_base()
        self.write("app.data", "pass\n")
        smoke = self.check("--suite", "smoke", expected=1)
        self.assertEqual(smoke["matched_triggers"], [])
        full = self.check("--suite", "full", expected=1)
        self.assertEqual(full["matched_triggers"], ["app-trigger"])
        self.assertEqual([c["name"] for c in full["commands"]], ["failure", "go-tests", "lint", "app-trigger"])
        self.assertEqual(full["commands"][0]["exit"], 3)

    def test_default_observe_is_advisory_only_for_commits(self):
        del self.policy["default_mode"]
        self.update_base()
        self.write("app.data", "fail")
        self.git("add", "app.data")
        path = self.message("Change\n\nVerified-by: forged\nVerification-evidence: stale\n")
        report = self.commit_message(path)
        self.assertEqual(report["mode"], "observe")
        self.assertTrue(report["would_block"])
        self.assertIn("Verified-by: none", path.read_text())
        self.assertNotIn("forged", path.read_text())
        self.cli("task", "--base", self.base, expected=1)
        self.cli("pr", "--base", self.base)
        self.write("app.data", "pass")
        self.git("add", "app.data")
        report = self.commit_message(path)
        self.assertIn("Verified-by: none", path.read_text())
        self.assertFalse(report["would_block"])

    def test_enforce_blocks_twice_then_allows_next_commit_in_session(self):
        self.write("app.data", "fail")
        self.git("add", "app.data")
        path = self.message()
        first = self.commit_message(path, expected=1)
        self.write("app.data", "fail\n")
        self.git("add", "app.data")
        second = self.commit_message(path, expected=1)
        third = self.commit_message(path)
        self.assertEqual([first["session_blocks"], second["session_blocks"]], [1, 2])
        self.assertEqual(third["escape_decision"], "two-block-escape")
        self.assertEqual(third["status"], "unverified")
        self.assertEqual(third["commands"], [])
        self.assertIn("Verified-by: none", path.read_text())

    def test_failed_candidate_cannot_retry_into_verified(self):
        external = self.root / "external-result"
        external.write_text("fail")
        self.policy["suites"]["smoke"] = [self.py("unstable", "from pathlib import Path; raise SystemExit(0 if Path(" + repr(str(external)) + ").read_text() == 'pass' else 1)")]
        self.update_base()
        path = self.message()
        first = self.commit_message(path, expected=1)
        external.write_text("pass")
        second = self.commit_message(path, expected=1)
        self.assertEqual(first["digest"], second["digest"])
        self.assertTrue(second["previous_failures"])
        self.assertEqual(second["status"], "unverified")
        self.assertIn("Verified-by: none", path.read_text())

    def test_pr_uses_committed_head_not_unstaged_fix(self):
        self.write("app.data", "fail")
        self.commit_fixture()
        self.write("app.data", "pass")
        report = self.cli("pr", "--base", self.base, expected=1)
        self.assertEqual(report["candidate_ref"], self.git("rev-parse", "HEAD").stdout.strip())
        self.assertEqual(report["commands"][1]["status"], "failed")

    def test_explicit_explanation_allows_first_but_generic_does_not(self):
        self.write("app.data", "fail")
        self.git("add", "app.data")
        path = self.message("Change\n\nVerification-skip: Infrastructure outage ticket QA-12\n")
        report = self.commit_message(path)
        self.assertEqual(report["attempts"], 1)
        for reason in ("skip", "later", "skip skip skip skip", "    "):
            path.write_text("Change\n\nVerification-skip: " + reason + "\n")
            self.commit_message(path, expected=1, extra_env={"GOOSIE_VERIFICATION_SESSION": "invalid-reason-" + reason})
        self.write("app.data", "fail\n")
        self.git("add", "app.data")
        path.write_text("Change\n")
        report = self.commit_message(path, expected=1)
        self.assertEqual(report["attempts"], 1)

    def test_exact_staged_success_replaces_forged_trailers(self):
        self.write("app.data", "pass\n")
        self.git("add", "app.data")
        self.write("app.data", "fail")
        path = self.message("Change\n\nVerified-by: forged\n continuation\nverified-by: stale\nVerification-evidence: stale\n")
        report = self.commit_message(path)
        text = path.read_text()
        self.assertIn("Verified-by: goosie-gate:" + self.base[:12] + ":" + report["digest"][:12], text)
        self.assertIn("Verification-evidence: " + report["evidence"], text)
        self.assertEqual(text.lower().count("verified-by:"), 1)
        self.assertNotIn("continuation", text)
        self.assertEqual(report["status"], "verified")

    def test_off_never_mints_verified_and_explicit_checks_still_fail(self):
        self.policy["default_mode"] = "off"
        self.update_base()
        self.write("app.data", "fail")
        self.git("add", "app.data")
        path = self.message("Change\n\nVerified-by: forged\n")
        report = self.commit_message(path)
        self.assertEqual(report["status"], "unverified")
        self.assertEqual(report["commands"], [])
        self.assertIn("Verified-by: none", path.read_text())
        self.cli("task", "--base", self.base, expected=1)

    def test_bootstrap_keeps_proposal_without_trusted_claim(self):
        self.policy["suites"]["smoke"] = [self.py("proposal", "from pathlib import Path; assert Path('tests/assertion.py').read_text() == 'proposal'")]
        self.save_policy()
        self.write("tests/assertion.py", "proposal")
        report = self.check("--suite", "smoke", "--bootstrap")
        self.assertEqual(report["status"], "bootstrap-pass")
        self.assertFalse(report["trusted"])
        self.assertIn("tests/assertion.py", report["protected_edits"])
        self.assertFalse((self.repo / ".git/hooks/goosie-verify").exists())

    def test_changed_tree_invalidates_even_green_execution(self):
        target = self.repo / "app.data"
        self.policy["suites"]["smoke"] = [self.py("concurrent-change", "from pathlib import Path; Path(" + repr(str(target)) + ").write_text('changed')")]
        self.update_base()
        report = self.check("--suite", "smoke", expected=1)
        self.assertEqual(report["status"], "changed-tree")
        self.assertNotEqual(report["digest"], report["digest_after"])

    def test_timeout_retains_logs_kills_process_group_and_cleans_workspace(self):
        code = "import os, signal; pid=os.fork(); print('pid', os.getpid(), flush=True); signal.pause()"
        self.policy["suites"]["smoke"] = [self.py("timeout", code, timeout=0.4)]
        self.update_base()
        report = self.check("--suite", "full", expected=1)
        result = report["commands"][0]
        self.assertEqual(result["status"], "timeout")
        self.assertEqual(len(Path(result["stdout"]).read_text().splitlines()), 2)
        self.assertEqual(report["commands"][1]["status"], "passed")
        self.assertFalse(Path(report["snapshot"]).exists())
        with self.assertRaises(ProcessLookupError):
            os.killpg(result["pid"], 0)

    def test_symlinks_and_submodules_are_rejected(self):
        (self.repo / "link").symlink_to(self.home, target_is_directory=True)
        report = self.check("--suite", "smoke", expected=1)
        self.assertIn("symlink", report["error"].lower())
        (self.repo / "link").unlink()
        self.git("update-index", "--add", "--cacheinfo", "160000," + self.base + ",submodule")
        report = self.check("--suite", "smoke", "--staged", expected=1)
        self.assertIn("submodule", report["error"].lower())

    def test_real_installed_commit_hook_success_and_block(self):
        config_before = (self.repo / ".git/config").read_bytes()
        report = self.cli("install", "--base", self.base)
        self.assertEqual(report["status"], "installed")
        launcher = self.repo / ".git/hooks/goosie-verify"
        self.assertIn(self.base, launcher.read_text())
        self.assertIn(self.git("rev-parse", self.base + ":scripts/verification_gate.py").stdout.strip(), launcher.read_text())
        self.write("scripts/verification_gate.py", "raise RuntimeError('untrusted runner')")
        self.write("app.data", "pass\n")
        self.git("add", "app.data")
        result = self.git(*IDENTITY, "commit", "-m", "Hook success")
        message = self.git("log", "-1", "--format=%B").stdout
        self.assertIn("Verified-by: goosie-gate:", message)
        print("hook success output: " + result.stdout + result.stderr, flush=True)
        before = self.git("rev-parse", "HEAD").stdout
        self.write("app.data", "fail")
        self.git("add", "app.data")
        result = self.git(*IDENTITY, "commit", "-m", "Hook block", check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.git("rev-parse", "HEAD").stdout, before)
        self.assertEqual((self.repo / ".git/config").read_bytes(), config_before)
        print("hook block output: " + result.stdout + result.stderr, flush=True)

    def test_install_preserves_unrelated_hooks(self):
        hook = self.write(".git/hooks/post-commit", "#!/bin/sh\n")
        self.cli("install", "--base", self.base)
        self.assertEqual(hook.read_text(), "#!/bin/sh\n")

    def test_full_suite_requires_disposable_host_when_policy_demands_it(self):
        self.policy["requires_disposable_host"] = True
        self.update_base()
        report = self.check("--suite", "full", expected=1)
        self.assertEqual(report["status"], "preflight-blocked")
        self.assertEqual(report["commands"], [])
        self.check("--suite", "smoke")
        report = self.cli("check", "--suite", "full", "--base", self.base,
                          extra_env={"GOOSIE_VERIFICATION_DISPOSABLE_HOST": "1"})
        self.assertEqual(report["status"], "passed")

    def test_nested_evidence_survives_snapshot_cleanup(self):
        self.policy["suites"]["smoke"] = [{"name": "artifact", "command": ["{python}", "-c",
            "from pathlib import Path; import sys; p=Path(sys.argv[1]); p.mkdir(); (p/'proof.txt').write_text('retained')",
            "{evidence}/artifact"], "timeout": 10}]
        self.update_base()
        report = self.check("--suite", "smoke")
        proof = Path(report["evidence"]).parent / "artifact" / "proof.txt"
        self.assertEqual(proof.read_text(), "retained")
        self.assertTrue(proof.parent.stat().st_mode & 0o111)
        self.assertEqual(proof.stat().st_mode & 0o222, 0)

    def test_install_refuses_nonempty_hooks_and_foreign_hookspath(self):
        hook = self.write(".git/hooks/commit-msg", "#!/bin/sh\nexit 0\n")
        self.cli("install", "--base", self.base, expected=1)
        self.assertEqual(hook.read_text(), "#!/bin/sh\nexit 0\n")
        hook.unlink()
        self.cli("install", "--base", self.base, expected=1,
                 extra_env={"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.hooksPath", "GIT_CONFIG_VALUE_0": str(self.root / "elsewhere")})
        self.assertFalse((self.repo / ".git/hooks/goosie-verify").exists())
        self.cli("install", "--base", self.base,
                 extra_env={"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "core.hooksPath", "GIT_CONFIG_VALUE_0": str(self.repo / ".git/hooks")})

    def test_audit_uses_base_policy_date_and_is_read_only(self):
        self.policy["last_reviewed"] = "2000-01-01"
        self.update_base()
        self.policy["last_reviewed"] = datetime.date.today().isoformat()
        self.save_policy()
        before = self.git("status", "--porcelain").stdout
        report = self.cli("audit", "--base", self.base, expected=1)
        self.assertTrue(report["review_due"])
        self.assertEqual(report["last_reviewed"], "2000-01-01")
        self.assertEqual(before, self.git("status", "--porcelain").stdout)

    def test_audit_due_boundary_and_paths(self):
        spec = importlib.util.spec_from_file_location("gate_under_test", RUNNER)
        gate = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(gate)
        policy = dict(self.policy)
        self.assertFalse(gate.review_due(policy, datetime.date(2026, 12, 16)))
        self.assertTrue(gate.review_due(policy, datetime.date(2026, 12, 17)))
        for path in ("../escape", "/absolute", ".git/config", "dir/.GIT/config", "dir\\escape"):
            with self.subTest(path=path), self.assertRaises(ValueError):
                gate.validate_path(path)

    def test_interrupted_delegation_cleans_runner_snapshot_and_process_group(self):
        self.assert_interrupted_delegation()

    def test_installed_launcher_interruption_cleans_children(self):
        self.assert_interrupted_delegation(installed=True)

    def test_interrupted_real_smoke_reaps_separate_child_session(self):
        self.assert_interrupted_delegation(nested=True)

    def test_interrupted_run_preserves_earlier_failure_history(self):
        self.assert_interrupted_delegation(prior_failure=True)

    def assert_interrupted_delegation(self, installed=False, nested=False, prior_failure=False):
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen(1)
            listener.settimeout(10)
            address = listener.getsockname()
            code = ("import os, socket, signal; s=socket.create_connection(" + repr(address) + "); "
                    "s.sendall(str(os.getpid()).encode()); s.close(); signal.pause()")
            self.policy["suites"]["smoke"] = [self.py("interrupt", code, timeout=20)]
            if prior_failure:
                external = self.root / "unstable-result"
                external.write_text("fail")
                read_result = "from pathlib import Path; result=Path(" + repr(str(external)) + ").read_text(); "
                self.policy["suites"]["smoke"] = [
                    self.py("earlier-failure", read_result + "raise SystemExit(1 if result == 'fail' else 0)"),
                    self.py("interrupt", read_result + "\nif result == 'fail':\n    " + code, timeout=20)]
            if nested:
                self.write("scripts/verify.py", RUNNER.with_name("verify.py").read_text())
                self.fake_go.write_text("#!" + sys.executable + "\n" + code + "\n")
                self.env.update(PATH=str(self.root) + os.pathsep + self.env["PATH"],
                                GOCACHE=str(self.root / "go-cache"), GOMODCACHE=str(self.root / "go-mod"))
                self.policy["suites"]["smoke"] = [{"name": "real-smoke", "command": ["{python}",
                    "scripts/verify.py", "--out", "{evidence}/browser"], "timeout": 20}]
            self.update_base()
            command = [sys.executable, "-I", str(RUNNER), "check", "--base", self.base]
            if installed:
                self.cli("install", "--base", self.base)
                command = [str(self.repo / ".git/hooks/goosie-verify"), "check"]
            process = subprocess.Popen(command, cwd=str(self.repo), env=self.env, text=True,
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            pid = None
            try:
                connection, _ = listener.accept()
                with connection:
                    connection.settimeout(10)
                    pid = int(connection.recv(100))
                process.terminate()
                stdout, stderr = process.communicate(timeout=10)
                self.assertNotEqual(process.returncode, 0)
                report = json.loads(stdout)
                self.assertEqual(report["status"], "error", stderr)
                self.assertIn("interrupt", report["error"].lower())
                self.assertFalse(Path(report["runner"]).exists())
                self.assertFalse(Path(report["snapshot"]).exists())
                with self.assertRaises(ProcessLookupError):
                    os.killpg(pid, 0)
                print("interrupt evidence: " + report["evidence"], flush=True)
                if prior_failure:
                    external.write_text("pass")
                    retry = self.commit_message(self.message(), expected=1)
                    self.assertEqual(retry["digest"], report["digest"])
                    self.assertGreater(retry["previous_failures"], 0)
                    self.assertEqual(retry["status"], "unverified")
            finally:
                if pid is not None:
                    try:
                        os.killpg(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                if process.poll() is None:
                    process.kill()
                process.communicate(timeout=10)

    def test_bootstrap_can_develop_policy_absent_from_base(self):
        self.git("rm", "docs/verification/gate.json", "REVIEW.md")
        self.commit_fixture()
        self.base = self.git("rev-parse", "HEAD").stdout.strip()
        self.save_policy()
        self.write("REVIEW.md", "Proposed review")
        report = self.check("--suite", "full", "--bootstrap")
        self.assertEqual(report["status"], "bootstrap-pass")
        self.assertFalse(report["trusted"])
        self.check("--suite", "full", expected=1)

    def test_staged_mutation_during_run_cannot_mint_verified(self):
        code = ("import pathlib, subprocess; p=pathlib.Path(" + repr(str(self.repo / 'app.data')) + "); "
                "p.write_text('changed'); subprocess.run(['git', '-C', " + repr(str(self.repo)) +
                ", 'add', 'app.data'], check=True)")
        self.policy["suites"]["smoke"] = [self.py("change-index", code)]
        self.update_base()
        path = self.message("Change\n\nVerified-by: forged\n")
        report = self.commit_message(path, expected=1)
        self.assertNotEqual(report["digest"], report["digest_after"])
        self.assertIn("Verified-by: none", path.read_text())
        self.assertTrue(report["would_block"])

    def test_shell_string_is_rejected_and_missing_review_clears_forgery(self):
        self.policy["suites"]["smoke"][0]["command"] = "printf pass"
        self.update_base()
        report = self.check(expected=1)
        self.assertIn("argv", report["error"])
        self.git("rm", "REVIEW.md")
        self.commit_fixture()
        self.base = self.git("rev-parse", "HEAD").stdout.strip()
        path = self.message("Change\n\nVerified-by: forged\n")
        report = self.commit_message(path, expected=1)
        self.assertIn("REVIEW.md", report["error"])
        self.assertIn("Verified-by: none", path.read_text())

    def test_attempt_counter_is_concurrency_safe(self):
        spec = importlib.util.spec_from_file_location("gate_under_test", RUNNER)
        gate = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(gate)
        from concurrent.futures import ThreadPoolExecutor
        state = self.repo / ".git" / "goosie-verification"
        with ThreadPoolExecutor(max_workers=8) as pool:
            attempts = list(pool.map(lambda _: gate.record_failure(state, self.base, "digest"), range(12)))
        self.assertEqual(sorted(attempts), list(range(1, 13)))


if __name__ == "__main__":
    unittest.main(verbosity=2)
