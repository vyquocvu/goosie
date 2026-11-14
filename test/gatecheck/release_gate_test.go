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
	if !strings.Contains(got, "go test -count=1 ./...") {
		t.Errorf("the full suite must run with -count=1 so cached results cannot pass a release; got:\n%s", got)
	}
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
