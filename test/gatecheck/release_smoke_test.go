package gatecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

const smokeReport = `goosie: run backend=headless requested="headless" interactive=false frames=$n stamped=$n elapsed_s=0.08 fps=199.3 scene=checkerboard size=2880x1800 window=2880x1800 dpr=2
goosie: timings mean_ms=23.423 p50_ms=25.147 p99_ms=33.564 max_ms=33.564 present_mean_ms=1.285 present_p99_ms=2.499 zero_work_frames=3
goosie: counters rasterized=264 reused=499 failed=0 refused=0 deferred=0 panics=0 writes=19669248 bytes=72351744 budget=785383424 evictions=0 miss_pct=34.60 presents=16 idle=0 dropped_vsyncs=2968 plans_published=1 plans_superseded=0
goosie: env goos=darwin goarch=arm64 go=go1.26.5 cpu=10 workers=4
goosie: present queued=16 dropped=0 committed=16
`

const writeReport = `printf '{"report":{"frames":16}}\n' > "$out"`

func smokeScript(t *testing.T) string {
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
	path := filepath.Join(root, "scripts", "release-smoke.sh")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("smoke script: %v", err)
	}
	return path
}

func stubScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goosie")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func goodStub(t *testing.T) string {
	return stubScript(t, `
out=""
n=16
[ -n "${ARGV_LOG:-}" ] && printf '%s\n' "$@" > "$ARGV_LOG"
while [ $# -gt 0 ]; do
  case "$1" in
    -out) out="$2"; shift 2 ;;
    -frames) n="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ -n "$out" ] && `+writeReport+`
cat >&2 <<REPORT
`+strings.TrimRight(smokeReport, "\n")+`
REPORT
exit 0
`)
}

func editedStub(t *testing.T, old, new string) string {
	t.Helper()
	path := goodStub(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stub: %v", err)
	}
	body := strings.TrimPrefix(string(raw), "#!/usr/bin/env bash\n")
	if n := strings.Count(body, old); n != 1 {
		t.Fatalf("%q appears %d times in the stub; a first-occurrence replace would break the wrong line", old, n)
	}
	out := strings.Replace(body, old, new, 1)
	return stubScript(t, out)
}

func runSmoke(t *testing.T, binary string, args ...string) (string, int) {
	t.Helper()
	argv := append([]string{binary}, args...)
	cmd := exec.Command("bash", append([]string{smokeScript(t)}, argv...)...)
	if os.Getenv("ARGV_LOG") == "" {
		cmd.Env = append(os.Environ(), "ARGV_LOG="+filepath.Join(t.TempDir(), "argv.txt"))
	}
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		return string(out), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("run smoke check: %v\n%s", err, out)
	}
	return string(out), 0
}

func wantSmokeExit(t *testing.T, want, code int, out string) {
	t.Helper()
	if code != want {
		t.Fatalf("exit = %d, want %d\noutput:\n%s", code, want, out)
	}
}

func TestReleaseSmokePassesOnAHealthyBinary(t *testing.T) {
	out, code := runSmoke(t, goodStub(t))
	wantSmokeExit(t, 0, code, out)
	if !strings.Contains(out, "release-smoke: PASS") {
		t.Errorf("missing PASS verdict:\n%s", out)
	}
}

func TestReleaseSmokeFailsWhenTheBinaryExitsNonZero(t *testing.T) {
	out, code := runSmoke(t, stubScript(t, `exit 3`))
	wantSmokeExit(t, 1, code, out)
	if !strings.Contains(out, "exit 3") {
		t.Errorf("the binary's exit status should be named:\n%s", out)
	}
}

func TestReleaseSmokeMissingBinaryExitsTwo(t *testing.T) {
	out, code := runSmoke(t, filepath.Join(t.TempDir(), "not-built"))
	wantSmokeExit(t, 2, code, out)
	if !strings.Contains(out, "not-built") {
		t.Errorf("the missing path should be named:\n%s", out)
	}
}

func TestReleaseSmokeUnrunnableBinaryExitsTwo(t *testing.T) {
	dir := t.TempDir()
	notExecutable := filepath.Join(dir, "plain")
	if err := os.WriteFile(notExecutable, []byte("text"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, code := runSmoke(t, dir)
	wantSmokeExit(t, 2, code, out)
	out, code = runSmoke(t, notExecutable)
	wantSmokeExit(t, 2, code, out)
}

func TestReleaseSmokeSilentBinaryExitsTwo(t *testing.T) {
	out, code := runSmoke(t, stubScript(t, `exit 0`))
	wantSmokeExit(t, 2, code, out)
	if !strings.Contains(out, "run line") {
		t.Errorf("the missing run line should be named:\n%s", out)
	}
}

func TestReleaseSmokeFieldsMustBeNumbers(t *testing.T) {
	for _, pair := range [][2]string{
		{"failed=0", "failed=unknown"},
		{"panics=0", "panics=n/a"},
		{"stamped=$n", "stamped=-"},
	} {
		out, code := runSmoke(t, editedStub(t, pair[0], pair[1]))
		wantSmokeExit(t, 2, code, out)
	}
}

func TestReleaseSmokeReportsFailures(t *testing.T) {
	for _, pair := range [][2]string{
		{"failed=0", "failed=7"},
		{"panics=0", "panics=2"},
		{"stamped=$n", "stamped=0"},
	} {
		out, code := runSmoke(t, editedStub(t, pair[0], pair[1]))
		wantSmokeExit(t, 1, code, out)
	}
}

func TestReleaseSmokeRejectsANonHeadlessRun(t *testing.T) {
	out, code := runSmoke(t, editedStub(t, "backend=headless", "backend=native"))
	wantSmokeExit(t, 1, code, out)
	if !strings.Contains(out, "headless") {
		t.Errorf("the backend mismatch should be explained:\n%s", out)
	}
}

func TestReleaseSmokeRejectsAForeignFrameCount(t *testing.T) {
	out, code := runSmoke(t, editedStub(t, `-frames) n="$2"; shift 2 ;;`, `-frames) shift 2 ;;`), "-frames", "23")
	wantSmokeExit(t, 2, code, out)
	if !strings.Contains(out, "frames=16") {
		t.Errorf("the disagreeing counts should be shown:\n%s", out)
	}
}

func TestReleaseSmokeRequiresTheReportFile(t *testing.T) {
	out, code := runSmoke(t, editedStub(t, writeReport, "true"))
	wantSmokeExit(t, 1, code, out)
	if !strings.Contains(out, "report") {
		t.Errorf("the missing report should be named:\n%s", out)
	}
}

func TestReleaseSmokeRejectsAnEmptyReport(t *testing.T) {
	out, code := runSmoke(t, editedStub(t, writeReport, `: > "$out"`))
	wantSmokeExit(t, 1, code, out)
}

func TestReleaseSmokePassesHeadlessAndFramesToTheBinary(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "argv.txt")
	t.Setenv("ARGV_LOG", logPath)
	out, code := runSmoke(t, goodStub(t), "-frames", "23")
	wantSmokeExit(t, 0, code, out)
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("the binary never recorded its argv: %v", err)
	}
	argv := string(raw)
	for _, want := range []string{"-bench", "-backend", "headless", "-frames", "23", "-scene", "checkerboard", "-out"} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv missing %q:\n%s", want, argv)
		}
	}
}

func TestReleaseSmokeBadInvocationExitsTwo(t *testing.T) {
	out, code := runSmoke(t, "")
	wantSmokeExit(t, 2, code, out)
	if !strings.Contains(out, "Usage") {
		t.Errorf("usage not printed:\n%s", out)
	}
	for _, args := range [][]string{{"-nonsense"}, {"-frames"}, {"-frames", "0"}, {"-frames", "abc"}, {"-frames", "-3"}} {
		out, code = runSmoke(t, goodStub(t), args...)
		wantSmokeExit(t, 2, code, out)
	}
}

func TestReleaseSmokeHelpExitsZero(t *testing.T) {
	out, code := runSmoke(t, "--help")
	wantSmokeExit(t, 0, code, out)
	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "-frames") {
		t.Errorf("help should document the flags:\n%s", out)
	}
}
