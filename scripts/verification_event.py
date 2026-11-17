import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys


def gh_pr_action(words):
    target_options = {"--repo", "-R", "--head", "-H", "--hostname"}
    explicit_target = False
    subcommands = []
    index = 1
    while index < len(words) and len(subcommands) < 2:
        token = words[index]
        option = token.split("=", 1)[0] if token.startswith("--") else token[:2]
        if option in target_options:
            explicit_target = True
            index += 2 if token == option else 1
        elif token in {"--help", "-h", "--version"}:
            index += 1
        elif token.startswith("-"):
            return None
        else:
            subcommands.append(token)
            index += 1
    if subcommands not in (["pr", "create"], ["pr", "ready"]):
        return None
    if explicit_target or subcommands[1] == "ready":
        return "reject"
    value_options = {
        "--assignee", "-a", "--base", "-B", "--body", "-b",
        "--body-file", "-F", "--label", "-l", "--milestone", "-m",
        "--project", "-p", "--recover", "--reviewer", "-r",
        "--template", "-T", "--title", "-t",
    }
    boolean_options = {
        "--draft", "-d", "--dry-run", "--editor", "-e", "--fill",
        "--fill-first", "--fill-verbose", "--web", "-w", "--help", "-h",
    }
    while index < len(words):
        token = words[index]
        option = token.split("=", 1)[0] if token.startswith("--") else token[:2]
        if option in value_options:
            index += 2 if token == option else 1
        elif token in boolean_options or (token.startswith("--") and option in boolean_options):
            index += 1
        else:
            return "reject"
    return "pr"


def action_for(event):
    """Recognize common direct shell commands, not aliases, expansion or a shell sandbox."""
    kind = event.get("hook_event_name")
    if kind == "Stop" and not event.get("stop_hook_active"):
        return "task"
    if kind == "PreToolUse" and event.get("tool_name") == "Bash":
        command = event.get("tool_input", {}).get("command", "")
        lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|()\n")
        lexer.whitespace = " \t\r"
        lexer.whitespace_split = True
        commands = [[]]
        for token in lexer:
            if token and all(char in ";&|()\n" for char in token):
                commands.append([])
            else:
                commands[-1].append(token)
        action = None
        changed_directory = False
        for words in commands:
            index = 0
            target_assignment = False
            while index < len(words) and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", words[index]):
                name = words[index].split("=", 1)[0]
                target_assignment |= name in {"GH_REPO", "GH_HOST", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR"}
                index += 1
            words = words[index:]
            if not words:
                continue
            if words[0] in {"cd", "chdir", "pushd", "popd"}:
                changed_directory = True
            if words[0] == "gh":
                candidate = gh_pr_action(words)
                if candidate and (candidate == "reject" or changed_directory or target_assignment):
                    return "reject"
                if candidate:
                    action = candidate
        if action and sum(bool(words) for words in commands) != 1:
            return "reject"
        return action
    return None


def invoke_gate(action, session_id=None):
    try:
        path = subprocess.check_output(["git", "rev-parse", "--git-path", "hooks/goosie-verify"], text=True).strip()
        if not Path(path).is_file():
            print("Verification inactive: install a human-approved base policy first; no verified claim is available.", file=sys.stderr)
            return 1
        env = os.environ.copy()
        if session_id is not None:
            env["GOOSIE_VERIFICATION_SESSION"] = session_id
        return subprocess.run([str(Path(path).resolve()), action], env=env).returncode
    except (OSError, subprocess.SubprocessError) as error:
        print(f"Verification unavailable: {error}", file=sys.stderr)
        return 1


def handle(event):
    action = action_for(event)
    if action == "reject":
        print("PR publication blocked: local HEAD verification requires a standalone current-branch gh pr create command, without ready or explicit targets.", file=sys.stderr)
        return 2
    if action and invoke_gate(action, event.get("session_id")):
        print("Required verification did not pass; inspect its evidence before claiming completion or publishing a PR.", file=sys.stderr)
        return 2
    return 0


def main():
    try:
        event = json.load(sys.stdin)
        if not isinstance(event, dict):
            raise ValueError("expected object")
        return handle(event)
    except (ValueError, TypeError, AttributeError) as error:
        print(f"invalid hook input: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
