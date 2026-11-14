package gatecheck

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

var permittedLicenses = map[string]bool{
	"MIT":          true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"Apache-2.0":   true,
	"ISC":          true,
}

var releaseBinaries = []string{"./cmd/goosie", "./cmd/goosie-headless"}

const mainModulePath = "github.com/vyquocvu/goosie"

type buildTarget struct {
	goos   string
	goarch string
}

func (b buildTarget) String() string { return b.goos + "/" + b.goarch }

func (b buildTarget) subtest() string { return b.goos + "-" + b.goarch }

var releaseTargets = []buildTarget{
	{goos: "darwin", goarch: "arm64"},
	{goos: "darwin", goarch: "amd64"},
	{goos: "linux", goarch: "amd64"},
	{goos: "linux", goarch: "arm64"},
	{goos: "windows", goarch: "amd64"},
}

var linkedModulesByTarget = map[buildTarget][]string{
	{goos: "darwin", goarch: "arm64"}: {
		"github.com/dlclark/regexp2/v2@v2.5.2",
		"github.com/dop251/goja@v0.0.0-20260926152631-39ec2650adc9",
		"github.com/go-sourcemap/sourcemap@v2.1.3+incompatible",
		"github.com/google/pprof@v0.0.0-20230207041349-798e818bf904",
		"golang.org/x/image@v0.45.0",
		"golang.org/x/net@v0.62.0",
		"golang.org/x/text@v0.43.0",
	},
	{goos: "darwin", goarch: "amd64"}: {
		"github.com/dlclark/regexp2/v2@v2.5.2",
		"github.com/dop251/goja@v0.0.0-20260926152631-39ec2650adc9",
		"github.com/go-sourcemap/sourcemap@v2.1.3+incompatible",
		"github.com/google/pprof@v0.0.0-20230207041349-798e818bf904",
		"golang.org/x/image@v0.45.0",
		"golang.org/x/net@v0.62.0",
		"golang.org/x/sys@v0.49.0",
		"golang.org/x/text@v0.43.0",
	},
	{goos: "linux", goarch: "amd64"}: {
		"github.com/dlclark/regexp2/v2@v2.5.2",
		"github.com/dop251/goja@v0.0.0-20260926152631-39ec2650adc9",
		"github.com/go-sourcemap/sourcemap@v2.1.3+incompatible",
		"github.com/google/pprof@v0.0.0-20230207041349-798e818bf904",
		"golang.org/x/image@v0.45.0",
		"golang.org/x/net@v0.62.0",
		"golang.org/x/sys@v0.49.0",
		"golang.org/x/text@v0.43.0",
	},
	{goos: "linux", goarch: "arm64"}: {
		"github.com/dlclark/regexp2/v2@v2.5.2",
		"github.com/dop251/goja@v0.0.0-20260926152631-39ec2650adc9",
		"github.com/go-sourcemap/sourcemap@v2.1.3+incompatible",
		"github.com/google/pprof@v0.0.0-20230207041349-798e818bf904",
		"golang.org/x/image@v0.45.0",
		"golang.org/x/net@v0.62.0",
		"golang.org/x/text@v0.43.0",
	},
	{goos: "windows", goarch: "amd64"}: {
		"github.com/dlclark/regexp2/v2@v2.5.2",
		"github.com/dop251/goja@v0.0.0-20260926152631-39ec2650adc9",
		"github.com/go-sourcemap/sourcemap@v2.1.3+incompatible",
		"github.com/google/pprof@v0.0.0-20230207041349-798e818bf904",
		"golang.org/x/image@v0.45.0",
		"golang.org/x/net@v0.62.0",
		"golang.org/x/sys@v0.49.0",
		"golang.org/x/text@v0.43.0",
	},
}

// qc:BR-194
func TestLinkedDependenciesPinned(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	allowPath := filepath.Join(root, "test", "gatecheck", "testdata", "dependency-licenses.txt")
	allowed, err := loadAllowlist(allowPath)
	if err != nil {
		t.Fatalf("allowlist: %v", err)
	}
	if len(allowed) == 0 {
		t.Fatalf("allowlist %s has no pin lines; the gate would audit nothing", allowPath)
	}
	if len(releaseTargets) < 2 {
		t.Fatalf("releaseTargets has %d entries; a one-target contract is the bug this test exists to prevent", len(releaseTargets))
	}

	sets := make(map[buildTarget][]string, len(releaseTargets))
	computable := true
	for _, target := range releaseTargets {
		want, ok := linkedModulesByTarget[target]
		if !ok {
			t.Errorf("target %s is in releaseTargets but has no audited module list in linkedModulesByTarget", target)
			computable = false
			continue
		}
		modules, err := linkedModules(root, target, releaseBinaries)
		if err != nil {
			t.Errorf("target %s: cannot compute the linked dependency set, failing closed instead of skipping: %v", target, err)
			computable = false
			continue
		}
		sets[target] = modules
		if diff := stringDiff(want, modules); diff != "" {
			t.Errorf("target %s links a different module set than audited (%s):\n%s", target, target.subtest(), diff)
		}
	}

	for _, target := range releaseTargets {
		modules, ok := sets[target]
		if !ok {
			continue
		}
		t.Run(target.subtest(), func(t *testing.T) {
			for _, mod := range modules {
				lic, pinned := allowed[mod]
				if !pinned {
					t.Errorf("linked module %s is not in dependency-licenses.txt (re-audit and amend) on target %s", mod, target)
					continue
				}
				if !permittedLicenses[lic] {
					t.Errorf("linked module %s has non-permitted license %q on target %s", mod, lic, target)
				}
			}
		})
	}

	if !computable {
		t.Errorf("staleness direction not evaluated: at least one target had no dependency set")
		return
	}
	linked := map[string]bool{}
	for _, target := range releaseTargets {
		for _, mod := range sets[target] {
			linked[mod] = true
		}
	}
	for mod := range allowed {
		if !linked[mod] {
			t.Errorf("allowlist entry %s is linked into no release target (%s) (stale pin?)", mod, targetNames(releaseTargets))
		}
	}
}

// qc:BR-194
func TestReleaseTargetsMatchTheReleaseMatrix(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	path := filepath.Join(root, ".github", "workflows", "release.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the release matrix, which gate 8 has to track: %v", err)
	}
	got, err := releaseMatrixRows(string(data))
	if err != nil {
		t.Fatalf("%v", err)
	}
	want := map[buildTarget]bool{}
	for _, target := range releaseTargets {
		want[target] = true
	}
	for _, target := range got {
		if !want[target] {
			t.Errorf("release.yml builds %s but releaseTargets does not audit its licences", target)
		}
		delete(want, target)
	}
	for target := range want {
		t.Errorf("releaseTargets audits %s but release.yml publishes no artifact for it", target)
	}
}

// qc:BR-194
func TestTargetResolutionFailsClosed(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	target := buildTarget{goos: "linux", goarch: "amd64"}
	for _, tc := range []struct {
		name string
		pkgs []string
	}{
		{"unresolvable-package", []string{"./cmd/not-a-real-package-xyz"}},
		{"zero-module-package", []string{"./internal/frame"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modules, err := linkedModules(root, target, tc.pkgs)
			if err == nil {
				t.Fatalf("linkedModules(%v) returned no error with %d modules; an unverified target must fail the gate, not empty it", tc.pkgs, len(modules))
			}
			if len(modules) != 0 {
				t.Errorf("linkedModules(%v) returned %d modules alongside error %v, want none", tc.pkgs, len(modules), err)
			}
			t.Logf("failed closed as required: %v", err)
		})
	}
}

var matrixRow = regexp.MustCompile(`\{\s*goos:\s*([A-Za-z0-9]+)\s*,\s*goarch:\s*([A-Za-z0-9]+)\s*,`)

func releaseMatrixRows(workflow string) ([]buildTarget, error) {
	var out []buildTarget
	for _, m := range matrixRow.FindAllStringSubmatch(workflow, -1) {
		out = append(out, buildTarget{goos: m[1], goarch: m[2]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no `{goos: ..., goarch: ...}` matrix rows found in .github/workflows/release.yml; refusing to audit a target list that could not be read")
	}
	return out, nil
}

func targetNames(targets []buildTarget) string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.String())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func linkedModules(root string, target buildTarget, pkgs []string) ([]string, error) {
	args := append([]string{"list", "-f", "{{if not .Standard}}{{.ImportPath}}|{{.Module.Path}}@{{.Module.Version}}{{end}}", "-deps"}, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GOOS="+target.goos,
		"GOARCH="+target.goarch,
		"CGO_ENABLED=0",
		"GOFLAGS=",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps for %s: %v: %s", target, err, strings.TrimSpace(stderr.String()))
	}
	seen := map[string]bool{}
	var modules []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 || parts[1] == "" {
			return nil, fmt.Errorf("go list -deps for %s printed an unparseable line %q", target, line)
		}
		mod := parts[1]
		at := strings.LastIndex(mod, "@")
		if at <= 0 {
			return nil, fmt.Errorf("go list -deps for %s printed module %q with no pinned version", target, mod)
		}
		if mod[:at] == mainModulePath || seen[mod] {
			continue
		}
		seen[mod] = true
		modules = append(modules, mod)
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("go list -deps for %s resolved no third-party module for %v: the release binary links at least one, so this is a broken measurement, not an empty one", target, pkgs)
	}
	sort.Strings(modules)
	return modules, nil
}

func loadAllowlist(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed allowlist line %q (want '<module>@<version> <license>')", line)
		}
		out[fields[0]] = fields[1]
	}
	return out, sc.Err()
}

func stringDiff(want, got []string) string {
	inWant := map[string]bool{}
	inGot := map[string]bool{}
	for _, w := range want {
		inWant[w] = true
	}
	for _, g := range got {
		inGot[g] = true
	}
	var missing, extra []string
	for _, w := range want {
		if !inGot[w] {
			missing = append(missing, w)
		}
	}
	for _, g := range got {
		if !inWant[g] {
			extra = append(extra, g)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	var b strings.Builder
	if len(missing) > 0 {
		fmt.Fprintf(&b, "audited but not linked: %s", strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "linked but not audited: %s", strings.Join(extra, ", "))
	}
	return b.String()
}
