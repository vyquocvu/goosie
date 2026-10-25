// BR-179 asks for a release gate: run the full suite, go vet -all, archtest and the gate
// suite on the candidate commit, require every command to exit 0, and keep the logs.
// Until scripts/release-gate.sh existed, that instruction was carried out by whoever was
// doing the release, from memory - which is not a test, and produces no evidence either.
//
// These are the tests for the checker, not for the browser: the gate is CI code with no
// assertions of its own, so it is tested the way scripts/v2-gate-check.sh and
// scripts/release-smoke.sh are. The exit-status contract is the same as theirs - 0 pass,
// 1 a command failed, 2 no verdict - and the interesting cases are the ones where a gate
// could report the wrong thing: a cached test run, a command that never started, a list
// someone quietly shortened.
package gatecheck

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

type gateReport struct {
	Status    string `json:"status"`
	Commands  int    `json:"commands"`
	Failed    int    `json:"failed"`
	NoVerdict int    `json:"no_verdict"`
	Results   []struct {
		Index   int    `json:"index"`
		Command string `json:"command"`
		Exit    int    `json:"exit"`
		Log     string `json:"log"`
	} `json:"results"`
}

func gateScript(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	path := filepath.Join(root, "scripts", "release-gate.sh")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("gate script: %v", err)
	}
	return path
}

// runGate returns the script's combined output and exit code. A non-zero code is an
// expected result here, so it is returned rather than treated as a test failure.
func runGate(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{gateScript(t)}, args...)...)
	out, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run release-gate.sh: %v", err)
	}
	return string(out), code
}

func writeGateCommands(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "commands.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write command list: %v", err)
	}
	return path
}

func readGateReport(t *testing.T, outDir string) gateReport {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(outDir, "release-gate.json"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var rep gateReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("parse report %q: %v", raw, err)
	}
	return rep
}

// The point of BR-179 is the set of commands, so the set is asserted rather than trusted:
// shorten scripts/release-gate.sh's built-in list by one line and this fails.
// qc:BR-179
func TestReleaseGateDefaultListIsTheQCGate(t *testing.T) {
	out, code := runGate(t, "-list")
	if code != 0 {
		t.Fatalf("-list exit = %d, want 0; output:\n%s", code, out)
	}
	var parsed struct {
		Commands []string `json:"commands"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &parsed); err != nil {
		t.Fatalf("parse -list output %q: %v", out, err)
	}
	got := strings.Join(parsed.Commands, "\n")
	for _, want := range []string{
		"go test -count=1 ./...",
		"go vet -all ./...",
		"./internal/archtest/",
		"./test/gate/",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("built-in gate list is missing %q; got:\n%s", want, got)
		}
	}
	// -count=1 is not decoration: a release run that reuses cached "ok" lines from an
	// earlier tree has not tested this commit.
	if !strings.Contains(got, "go test -count=1 ./...") {
		t.Errorf("the full suite must run with -count=1 so cached results cannot pass a release; got:\n%s", got)
	}
	// Same reasoning for the gate suite: without -run TestGate the package's other tests
	// are not the gate, and a stale binary could otherwise be blamed on the gate.
	if !strings.Contains(got, "-run TestGate ./test/gate/") {
		t.Errorf("gate suite command must select TestGate; got:\n%s", got)
	}
}

func TestReleaseGatePassesWhenEveryCommandPasses(t *testing.T) {
	out := t.TempDir()
	list := writeGateCommands(t, "echo gate-one-ok", "echo gate-two-ok")
	stdout, code := runGate(t, "-commands", list, "-out", out)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; output:\n%s", code, stdout)
	}
	rep := readGateReport(t, out)
	if rep.Status != "pass" || rep.Failed != 0 || rep.Commands != 2 || len(rep.Results) != 2 {
		t.Errorf("report = %+v, want status pass, 2 commands, 0 failed", rep)
	}
}

// A failing command has to make the gate fail *and* stay on disk: BR-179 asks for the logs
// as evidence, and a gate that exits 1 without saying which command broke is not much help
// at 2am.
// qc:BR-179
func TestReleaseGateFailsWhenOneCommandFails(t *testing.T) {
	out := t.TempDir()
	list := writeGateCommands(t, "echo gate-marker-pass", "sh -c 'echo gate-marker-fail >&2; exit 3'")
	stdout, code := runGate(t, "-commands", list, "-out", out)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; output:\n%s", code, stdout)
	}
	rep := readGateReport(t, out)
	if rep.Status != "fail" || rep.Failed != 1 || len(rep.Results) != 2 {
		t.Fatalf("report = %+v, want status fail with one failing command still recorded", rep)
	}
	if rep.Results[1].Exit != 3 {
		t.Errorf("failing command exit = %d, want the script's 3 to be preserved", rep.Results[1].Exit)
	}
	if !strings.Contains(stdout, "gate-marker-fail") && !strings.Contains(stdout, "exit 3") {
		t.Errorf("output does not name the failing command; got:\n%s", stdout)
	}
}

// Later commands must still run after an early failure: "vet was clean on this commit" is
// worth knowing even when a test failed, and re-running the whole gate to find out is how
// people start skipping it.
func TestReleaseGateKeepsRunningAfterAFailure(t *testing.T) {
	out := t.TempDir()
	list := writeGateCommands(t, "exit 1", "echo gate-third-ran")
	stdout, code := runGate(t, "-commands", list, "-out", out)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; output:\n%s", code, stdout)
	}
	rep := readGateReport(t, out)
	if len(rep.Results) != 2 || rep.Results[1].Exit != 0 {
		t.Fatalf("report = %+v, want both commands attempted", rep)
	}
	if _, err := os.ReadFile(rep.Results[0].Log); err != nil {
		t.Fatalf("failed command log %q missing: %v", rep.Results[0].Log, err)
	}
	log2, err := os.ReadFile(rep.Results[1].Log)
	if err != nil {
		t.Fatalf("second command log %q missing: %v", rep.Results[1].Log, err)
	}
	if !strings.Contains(string(log2), "gate-third-ran") {
		t.Errorf("second log = %q, want the command's own output", log2)
	}
}

// 127 means the command never started, which is not a browser bug and must not be reported
// as a pass either. This is the case a hand-run gate gets wrong most often.
func TestReleaseGateHasNoVerdictWhenACommandCannotRun(t *testing.T) {
	out := t.TempDir()
	list := writeGateCommands(t, "definitely-not-a-real-binary-xyz")
	stdout, code := runGate(t, "-commands", list, "-out", out)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (no verdict); output:\n%s", code, stdout)
	}
	rep := readGateReport(t, out)
	if rep.NoVerdict != 1 {
		t.Errorf("report = %+v, want no_verdict 1", rep)
	}
}

func TestReleaseGateRejectsAListWithNoCommands(t *testing.T) {
	out := t.TempDir()
	list := writeGateCommands(t, "# only a comment", "")
	stdout, code := runGate(t, "-commands", list, "-out", out)
	if code != 2 {
		t.Fatalf("exit = %d, want 2; output:\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "release-gate.json")); err == nil {
		t.Error("a gate that attempted nothing must not leave a report behind")
	}
}

func TestReleaseGateBadInvocationExitsTwo(t *testing.T) {
	for _, args := range [][]string{
		{"-commands", filepath.Join(t.TempDir(), "missing.txt")},
		{"-out"},
		{"-nonsense"},
	} {
		out, code := runGate(t, args...)
		if code != 2 {
			t.Errorf("release-gate.sh %s: exit = %d, want 2; output:\n%s", strings.Join(args, " "), code, out)
		}
	}
}

func TestReleaseGateHelpExitsZero(t *testing.T) {
	out, code := runGate(t, "-h")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; output:\n%s", code, out)
	}
	for _, want := range []string{"-commands", "-out", "-list"} {
		if !strings.Contains(out, want) {
			t.Errorf("help does not document %s; got:\n%s", want, out)
		}
	}
}
