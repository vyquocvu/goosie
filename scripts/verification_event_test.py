import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch


class EventContract(unittest.TestCase):
    def setUp(self):
        path = Path(__file__).with_name("verification_event.py")
        self.assertTrue(path.exists(), "event adapter is missing")
        spec = importlib.util.spec_from_file_location("verification_event", path)
        self.event = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.event)

    def test_stop_and_pr_events_select_checks(self):
        self.assertEqual(self.event.action_for({"hook_event_name": "Stop"}), "task")
        self.assertEqual(self.event.action_for({"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "gh pr create --title example"}}), "pr")
        self.assertEqual(self.event.action_for({"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "git push && gh pr ready"}}), "reject")
        self.assertIsNone(self.event.action_for({"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "go test ./..."}}))

    def test_leading_assignments_keep_current_branch_creation_gated(self):
        for command in [
            "GH_PROMPT_DISABLED=1 gh pr create",
            "GH_PROMPT_DISABLED=1 NO_COLOR=1 gh pr create --fill",
        ]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.action_for(event), "pr")

    def test_explicit_pr_targets_are_rejected(self):
        for command in [
            "gh --repo owner/repo pr create",
            "GH_PROMPT_DISABLED=1 gh --repo owner/repo pr create",
            "gh --repo=owner/repo pr create",
            "gh -R owner/repo pr create",
            "gh -Rowner/repo pr create",
            "gh pr --repo owner/repo create",
            "gh pr create --repo owner/repo",
            "gh pr create -Rowner/repo",
            "gh pr create --head other",
            "gh pr create --head=other",
            "gh pr create -H other",
            "gh pr create -Hother",
            "gh pr create --head HEAD",
            "gh pr create 123",
            "gh pr create https://github.com/owner/repo/pull/123",
            "GH_REPO=owner/repo gh pr create",
            "GH_HOST=other.example gh pr create",
            "GIT_DIR=/other/.git gh pr create",
            "GIT_WORK_TREE=/other gh pr create",
        ]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.action_for(event), "reject")

    def test_ready_cannot_verify_the_remote_pr_from_local_head(self):
        for command in ["gh pr ready", "gh pr ready 123", "gh pr ready other", "GH_PROMPT_DISABLED=1 gh pr ready"]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.action_for(event), "reject")

    def test_directory_changes_before_pr_creation_are_rejected(self):
        for command in [
            "cd /other && gh pr create",
            "cd /other; GH_PROMPT_DISABLED=1 gh pr create",
            "pushd /other\ngh pr create",
            "popd && gh pr create",
            "chdir /other && gh pr create",
            "gh pr create && cd /other && gh pr create",
            "gh pr create && gh pr ready 123",
        ]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.action_for(event), "reject")

    def test_non_target_option_values_are_allowed(self):
        for command in [
            "gh pr create --base release --title '--head' --body '--repo'",
            "gh pr create -B release -t '--head' -b 'cd /other; gh pr ready'",
            "gh pr create --title='gh pr ready' --body='--repo other'",
        ]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.action_for(event), "pr")

    def test_pr_creation_must_be_standalone_to_bind_verified_head(self):
        for command in ["git switch other && gh pr create --fill", "git commit -m fix && gh pr create", "gh pr create && git switch other"]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.action_for(event), "reject")

    def test_quoted_mentions_and_comments_are_not_pr_commands(self):
        for command in [
            "git log --grep='gh pr create'",
            "printf '%s' 'gh pr create'",
            "go test ./... # gh pr ready",
            "printf '%s' 'GH_PROMPT_DISABLED=1 gh pr create'",
            "printf '%s' 'gh --repo owner/repo pr create'",
            "go test ./... # GH_PROMPT_DISABLED=1 gh pr create",
            "GH_PROMPT_DISABLED=1 printf '%s' 'gh pr create'",
            "gh alias set publish 'pr create'",
        ]:
            with self.subTest(command=command):
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertIsNone(self.event.action_for(event), command)

    def test_rejected_targets_block_without_invoking_the_local_gate(self):
        for command in ["gh pr ready 123", "gh --repo owner/repo pr create", "cd /other && gh pr create"]:
            with self.subTest(command=command), patch.object(self.event, "invoke_gate", return_value=0) as gate, patch("sys.stderr", new_callable=io.StringIO) as stderr:
                event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.handle(event), 2)
                gate.assert_not_called()
                self.assertIn("HEAD", stderr.getvalue())

    def test_event_session_reaches_the_launcher_for_task_and_pr_checks(self):
        for kind, command, action in [("Stop", "", "task"), ("PreToolUse", "GH_PROMPT_DISABLED=1 gh pr create", "pr")]:
            with self.subTest(action=action), patch.dict(os.environ, {"GOOSIE_VERIFICATION_SESSION": "stale-session", "KEEP_FOR_LAUNCHER": "present"}), patch.object(self.event.subprocess, "check_output", return_value="/approved/hooks/goosie-verify\n") as resolve, patch.object(self.event.Path, "is_file", return_value=True), patch.object(self.event.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run:
                event = {"hook_event_name": kind, "session_id": "event-session", "tool_name": "Bash", "tool_input": {"command": command}}
                self.assertEqual(self.event.handle(event), 0)
                resolve.assert_called_once_with(["git", "rev-parse", "--git-path", "hooks/goosie-verify"], text=True)
                run.assert_called_once()
                self.assertEqual(run.call_args.args[0], ["/approved/hooks/goosie-verify", action])
                self.assertEqual(run.call_args.kwargs.get("env", {}).get("GOOSIE_VERIFICATION_SESSION"), "event-session")
                self.assertEqual(run.call_args.kwargs["env"]["KEEP_FOR_LAUNCHER"], "present")
                self.assertEqual(os.environ["GOOSIE_VERIFICATION_SESSION"], "stale-session")

    def test_settings_run_from_a_subdirectory(self):
        root = Path(__file__).resolve().parent.parent
        settings = json.loads((root / ".claude/settings.json").read_text())
        command = settings["hooks"]["PreToolUse"][0]["hooks"][0]["command"]
        event = {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "go test ./..."}}
        result = subprocess.run(["/bin/sh", "-c", command], cwd=root / "scripts", input=json.dumps(event), text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_recursive_stop_does_not_loop(self):
        self.assertIsNone(self.event.action_for({"hook_event_name": "Stop", "stop_hook_active": True}))

    def test_malformed_payload_is_rejected(self):
        result = subprocess.run([sys.executable, str(Path(self.event.__file__))], input="not-json", capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn("invalid hook input", result.stderr)

    def test_failed_gate_blocks_and_pass_allows(self):
        with patch.object(self.event, "invoke_gate", return_value=1):
            self.assertEqual(self.event.handle({"hook_event_name": "Stop"}), 2)
        with patch.object(self.event, "invoke_gate", return_value=0):
            self.assertEqual(self.event.handle({"hook_event_name": "Stop"}), 0)


if __name__ == "__main__":
    unittest.main()
