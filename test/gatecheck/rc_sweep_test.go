package gatecheck

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

type rcReport struct {
	Status      string `json:"status"`
	Fingerprint struct {
		SHA       string `json:"sha"`
		OS        string `json:"os"`
		BinarySHA string `json:"binary_sha256"`
	} `json:"fingerprint"`
	Steps []struct {
		Index    int      `json:"index"`
		Name     string   `json:"name"`
		Command  string   `json:"command"`
		Exit     int      `json:"exit"`
		Duration *float64 `json:"duration_seconds"`
		Log      string   `json:"log"`
	} `json:"steps"`
}

func rcSweepScript(t *testing.T) string {
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
	path := filepath.Join(root, "scripts", "rc-sweep.sh")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("rc sweep script: %v", err)
	}
	return path
}

func runRCSweep(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{rcSweepScript(t)}, args...)...)
	out, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run rc-sweep.sh: %v", err)
	}
	return string(out), code
}

func rcFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-goosie")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	steps := filepath.Join(dir, "steps.txt")
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return bin, steps
}

func writeRCJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readRCReport(t *testing.T, dir string) rcReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "rc-sweep.json"))
	if err != nil {
		t.Fatalf("read rc-sweep report: %v", err)
	}
	var report rcReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse rc-sweep report: %v", err)
	}
	return report
}

// qc:BR-180
func TestRCSweepReportContainsSevenEvidenceRowsAndFingerprint(t *testing.T) {
	bin, steps := rcFixture(t)
	out := t.TempDir()
	stdout, code := runRCSweep(t, "-binary", bin, "-steps", steps, "-out", out)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; output:\n%s", code, stdout)
	}
	report := readRCReport(t, out)
	if report.Status != "pass" || len(report.Steps) != 7 {
		t.Fatalf("report = %+v, want pass and 7 step rows", report)
	}
	if report.Fingerprint.SHA == "" || report.Fingerprint.OS == "" || report.Fingerprint.BinarySHA == "" {
		t.Fatalf("incomplete fingerprint: %+v", report.Fingerprint)
	}
	for i, step := range report.Steps {
		if step.Index != i+1 || step.Name == "" || step.Command == "" || step.Exit != 0 || step.Duration == nil || step.Log == "" {
			t.Errorf("step %d incomplete: %+v", i+1, step)
		}
		if _, err := os.Stat(step.Log); err != nil {
			t.Errorf("step %d log %q missing: %v", i+1, step.Log, err)
		}
	}
}

// qc:BR-180
func TestRCSweepUsesFailClosedZeroOneTwoAndRejectsMissingSteps(t *testing.T) {
	bin, steps := rcFixture(t)
	bad := filepath.Join(t.TempDir(), "bad-steps.txt")
	_ = os.WriteFile(bad, []byte("true\n"), 0o644)
	if out, code := runRCSweep(t, "-binary", bin, "-steps", bad, "-out", t.TempDir()); code != 2 || !strings.Contains(out, "exactly seven") {
		t.Fatalf("missing steps: exit=%d output=%q, want 2 with missing-list reason", code, out)
	}
	failedSteps := filepath.Join(t.TempDir(), "fail.txt")
	_ = os.WriteFile(failedSteps, []byte("true\nexit 7\ntrue\ntrue\ntrue\ntrue\ntrue\n"), 0o644)
	if out, code := runRCSweep(t, "-binary", bin, "-steps", failedSteps, "-out", t.TempDir()); code != 1 || !strings.Contains(out, "FAIL") {
		t.Fatalf("failing step: exit=%d output=%q, want 1", code, out)
	}
	missing := filepath.Join(t.TempDir(), "missing.txt")
	if out, code := runRCSweep(t, "-binary", bin, "-steps", missing, "-out", t.TempDir()); code != 2 || !strings.Contains(out, "cannot read steps") {
		t.Fatalf("unreadable steps: exit=%d output=%q, want 2", code, out)
	}
	_ = steps
}

func writePerCaseFixture(t *testing.T, dir, name string, priority string, testName string) (string, string) {
	t.Helper()
	golden := filepath.Join(dir, "golden.json")
	ratchet := filepath.Join(dir, "ratchet.json")
	writeRCJSON(t, golden, []map[string]any{{"id": name, "priority": priority, "tests": []string{"test/gatecheck/rc_sweep_test.go:" + testName}}})
	writeRCJSON(t, ratchet, map[string]int{"p0_tagged": 0, "p0p1_tagged": 0})
	return golden, ratchet
}

func fakeGo(t *testing.T, output string, exit int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go")
	script := "#!/bin/sh\nprintf '%s\\n' '" + strings.ReplaceAll(output, "'", "'\\''") + "'\nexit " + string(rune('0'+exit)) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runPerCaseWithFakeGo(t *testing.T, fakePath, testName string) (string, int, string) {
	t.Helper()
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\n-\ntrue\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	golden, ratchet := writePerCaseFixture(t, dir, "BR-180", "P0", testName)
	out := t.TempDir()
	cmd := exec.Command("bash", rcSweepScript(t), "-binary", bin, "-steps", steps, "-golden", golden, "-ratchet", ratchet, "-out", out)
	cmd.Env = append(os.Environ(), "PATH="+fakePath+":"+os.Getenv("PATH"))
	data, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(data), code, out
}

// qc:BR-180
func TestRCSweepRejectsTaggedTestWhenRunPatternMatchesNothing(t *testing.T) {
	fake := fakeGo(t, `{"Action":"output","Output":"ok"}`, 0)
	stdout, code, out := runPerCaseWithFakeGo(t, fake, "TestThisNameDoesNotExist")
	if code != 2 || !strings.Contains(stdout, "NO VERDICT") {
		t.Fatalf("exit=%d output=%q, want 2 because no tagged test ran", code, stdout)
	}
	report := readRCReport(t, out)
	if !strings.Contains(report.Steps[4].Command, "-count=1") {
		t.Fatalf("per-case command = %q, want uncached -count=1 invocation", report.Steps[4].Command)
	}
}

// qc:BR-180
func TestRCSweepMapsFailingP0TaggedTestToFailure(t *testing.T) {
	fake := fakeGo(t, "{\"Action\":\"run\",\"Test\":\"TestKnownP0\"}\n{\"Action\":\"fail\",\"Test\":\"TestKnownP0\"}", 1)
	stdout, code, out := runPerCaseWithFakeGo(t, fake, "TestKnownP0")
	if code != 1 || !strings.Contains(stdout, "FAIL") {
		t.Fatalf("exit=%d output=%q, want 1 for failing P0", code, stdout)
	}
	report := readRCReport(t, out)
	if report.Steps[4].Exit != 1 {
		t.Fatalf("per-case step = %+v, want the tagged P0 test failure", report.Steps[4])
	}
}

// qc:BR-180
func TestRCSweepRejectsUntaggedP0Case(t *testing.T) {
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\n-\ntrue\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	golden := filepath.Join(dir, "golden.json")
	ratchet := filepath.Join(dir, "ratchet.json")
	writeRCJSON(t, golden, []map[string]any{{"id": "BR-180", "priority": "P0", "tests": []string{}}})
	writeRCJSON(t, ratchet, map[string]int{"p0_tagged": 0, "p0p1_tagged": 0})
	out := t.TempDir()
	stdout, code := runRCSweep(t, "-binary", bin, "-steps", steps, "-golden", golden, "-ratchet", ratchet, "-out", out)
	if code != 1 || !strings.Contains(stdout, "FAIL") {
		t.Fatalf("untagged P0: exit=%d output=%q, want 1", code, stdout)
	}
	report := readRCReport(t, out)
	if report.Steps[4].Exit != 1 {
		t.Fatalf("per-case step = %+v, want exit 1 for untagged P0", report.Steps[4])
	}
	log, err := os.ReadFile(report.Steps[4].Log)
	if err != nil || !strings.Contains(string(log), "P0 cases have no tagged test") {
		t.Fatalf("per-case log = %q, error=%v; want the missing P0 tag reason", log, err)
	}
}

// qc:BR-180
func TestRCSweepEnforcesTaggedCaseRatchetFloor(t *testing.T) {
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\n-\ntrue\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	golden := filepath.Join(dir, "golden.json")
	writeRCJSON(t, golden, []map[string]any{{"id": "BR-180", "priority": "P0", "tests": []string{"test/gatecheck/rc_sweep_test.go:TestKnownP0"}}})
	ratchet := filepath.Join(dir, "ratchet.json")
	writeRCJSON(t, ratchet, map[string]int{"p0_tagged": 1, "p0p1_tagged": 2})
	fake := fakeGo(t, "{\"Action\":\"run\",\"Test\":\"TestKnownP0\"}", 0)
	out := t.TempDir()
	cmd := exec.Command("bash", rcSweepScript(t), "-binary", bin, "-steps", steps, "-golden", golden, "-ratchet", ratchet, "-out", out)
	cmd.Env = append(os.Environ(), "PATH="+fake+":"+os.Getenv("PATH"))
	data, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	stdout := string(data)
	if code != 1 || !strings.Contains(stdout, "FAIL") {
		t.Fatalf("ratchet decrease: exit=%d output=%q, want 1", code, stdout)
	}
	report := readRCReport(t, out)
	if report.Steps[4].Exit != 1 {
		t.Fatalf("per-case step = %+v, want exit 1 for ratchet decrease", report.Steps[4])
	}
	log, err := os.ReadFile(report.Steps[4].Log)
	if err != nil || !strings.Contains(string(log), "fell below ratchet") {
		t.Fatalf("per-case log = %q, error=%v; want tagged-case floor reason", log, err)
	}
}

// qc:BR-180
func TestRCSweepChecksSuppliedBuildSHAAgainstHEAD(t *testing.T) {
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("-\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := runRCSweep(t, "-binary", bin, "-steps", steps, "-sha", strings.Repeat("0", 40), "-out", t.TempDir()); code != 1 || !strings.Contains(out, "FAIL") {
		t.Fatalf("SHA mismatch: exit=%d output=%q, want 1", code, out)
	}
	if out, code := runRCSweep(t, "-binary", bin, "-steps", steps, "-sha", strings.Repeat("0", 40), "-allow-sha-mismatch", "-out", t.TempDir()); code != 0 {
		t.Fatalf("explicitly allowed SHA mismatch: exit=%d output=%q, want 0", code, out)
	}
}

// qc:BR-180
func TestRCSweepFailsWhenAnOpenP0P1DefectExists(t *testing.T) {
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\ntrue\ntrue\n-\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	dir := t.TempDir()
	defects := filepath.Join(dir, "defects.json")
	signoffs := filepath.Join(dir, "signoffs")
	_ = os.Mkdir(signoffs, 0o755)
	writeRCJSON(t, defects, []map[string]string{{"id": "TEST-OPEN", "priority": "P1", "status": "open"}})
	stdout, code := runRCSweep(t, "-binary", bin, "-steps", steps, "-defects", defects, "-signoffs", signoffs, "-out", out)
	if code != 1 || !strings.Contains(stdout, "FAIL") {
		t.Fatalf("exit=%d output=%q, want 1 for open P1", code, stdout)
	}
	if report := readRCReport(t, out); report.Steps[6].Exit != 1 {
		t.Fatalf("defect/sign-off step = %+v, want exit 1", report.Steps[6])
	} else if data, err := os.ReadFile(report.Steps[6].Log); err != nil || !strings.Contains(string(data), "TEST-OPEN") {
		t.Fatalf("defect log = %q, error=%v; want the blocking defect ID", data, err)
	}
}

// qc:BR-180
func TestRCSweepRequiresHumanSignoffForExactBuildSHA(t *testing.T) {
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\ntrue\ntrue\n-\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	defects := filepath.Join(dir, "defects.json")
	signoffs := filepath.Join(dir, "signoffs")
	_ = os.Mkdir(signoffs, 0o755)
	writeRCJSON(t, defects, []any{})
	args := []string{"-binary", bin, "-steps", steps, "-defects", defects, "-signoffs", signoffs, "-out", t.TempDir()}
	missingOut, code := runRCSweep(t, args...)
	if code != 1 || !strings.Contains(missingOut, "FAIL") {
		t.Fatalf("missing sign-off: exit=%d output=%q, want 1", code, missingOut)
	}
	shaCmd := exec.Command("git", "rev-parse", "HEAD")
	shaCmd.Dir = filepath.Dir(filepath.Dir(rcSweepScript(t)))
	shaBytes, err := shaCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.TrimSpace(string(shaBytes))
	writeRCJSON(t, filepath.Join(signoffs, sha+".json"), map[string]any{
		"reviewer": "QA Reviewer", "date": "2026-10-25", "build_sha": strings.Repeat("0", len(sha)),
		"cua_cases": []string{"BR-001"}, "statement": "human reviewed",
	})
	mismatchOut, code := runRCSweep(t, args...)
	if code != 1 || !strings.Contains(mismatchOut, "FAIL") {
		t.Fatalf("mismatched sign-off SHA: exit=%d output=%q, want 1", code, mismatchOut)
	}
	report := readRCReport(t, args[len(args)-1])
	log, err := os.ReadFile(report.Steps[6].Log)
	if err != nil || !strings.Contains(string(log), "sign-off") {
		t.Fatalf("sign-off rejection log = %q, error=%v; want missing/mismatched human sign-off", log, err)
	}
}

// qc:BR-180
func TestRCSweepOfflineLiveSmokeHasNoPassVerdictAndLocalURLIsSupported(t *testing.T) {
	bin, steps := rcFixture(t)
	if err := os.WriteFile(steps, []byte("true\ntrue\ntrue\ntrue\ntrue\n-\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Closed loopback port models an offline host without ever contacting the internet.
	offlineOut := t.TempDir()
	if out, code := runRCSweep(t, "-binary", bin, "-steps", steps, "-live-url", "http://127.0.0.1:1/", "-out", offlineOut); code != 2 || strings.Contains(out, "live_smoke: pass") || !strings.Contains(out, "live_smoke: NO VERDICT") {
		t.Fatalf("offline live smoke: exit=%d output=%q, want only a step-level no verdict", code, out)
	}
	if report := readRCReport(t, offlineOut); report.Steps[5].Exit != 2 || report.Status == "pass" {
		t.Fatalf("offline report = %+v, want live step 2 and no sweep pass", report)
	}
	withFailure := filepath.Join(t.TempDir(), "mixed.txt")
	if err := os.WriteFile(withFailure, []byte("exit 1\ntrue\ntrue\ntrue\ntrue\n-\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mixedOut := t.TempDir()
	if out, code := runRCSweep(t, "-binary", bin, "-steps", withFailure, "-live-url", "http://127.0.0.1:1/", "-out", mixedOut); code != 1 || !strings.Contains(out, "FAIL") || strings.Contains(out, "live_smoke: PASS") {
		t.Fatalf("offline plus real failing step: exit=%d output=%q, want overall FAIL and live no verdict", code, out)
	}
	if report := readRCReport(t, mixedOut); report.Status != "fail" || report.Steps[0].Exit != 1 || report.Steps[5].Exit != 2 {
		t.Fatalf("mixed offline report = %+v, want failure plus step-level live no verdict", report)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on local IPv4 loopback: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	server.Listener = listener
	server.Start()
	defer server.Close()
	localBin := filepath.Join(t.TempDir(), "fake-goosie")
	localProgram := `#!/bin/sh
url=
out=
while [ $# -gt 0 ]; do case "$1" in -url) url=$2; shift 2;; -out) out=$2; shift 2;; *) shift;; esac; done
printf '{"report":{"frames":1,"document":"%s","doc_height":10},"frames":[{}]}\n' "$url" > "$out"
`
	if err := os.WriteFile(localBin, []byte(localProgram), 0o755); err != nil {
		t.Fatal(err)
	}
	stepsLocal := filepath.Join(t.TempDir(), "local.txt")
	if err := os.WriteFile(stepsLocal, []byte("true\ntrue\ntrue\ntrue\ntrue\n-\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	localOut := t.TempDir()
	localDefects := filepath.Join(t.TempDir(), "defects.json")
	writeRCJSON(t, localDefects, []any{})
	localSignoffs := t.TempDir()
	localURL := server.URL + "/products"
	if out, code := runRCSweep(t, "-binary", localBin, "-steps", stepsLocal, "-live-url", localURL, "-defects", localDefects, "-signoffs", localSignoffs, "-out", localOut); code != 0 || !strings.Contains(out, "live_smoke: PASS") {
		t.Fatalf("local live smoke should pass its local preflight and frame report: exit=%d output=%q", code, out)
	}
	if report := readRCReport(t, localOut); report.Steps[5].Exit != 0 {
		t.Fatalf("local live-smoke step = %+v, want exit 0", report.Steps[5])
	}
}
