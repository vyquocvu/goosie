package gatecheck

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

// permittedLicenses is the closed set of licenses acceptable in a distributed
// binary. Permissive only: copyleft would change what distribution means and
// needs product/legal review, never a silent test update.
var permittedLicenses = map[string]bool{
	"MIT":          true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"Apache-2.0":   true,
	"ISC":          true,
}

// releaseBinaries are the packages whose linked dependencies constitute the
// distributed provenance. Dev-only tools (wpt-runner, diagnostics) are out
// of scope: they never leave the repository.
var releaseBinaries = []string{"./cmd/goosie", "./cmd/goosie-headless"}

// TestLinkedDependenciesPinned fails on any distribution drift: a module
// linked into a release binary that is not in dependency-licenses.txt, a
// version that moved without re-audit, or a license outside the permitted
// set. The fix is never to relax the test: re-audit the license text and
// amend the allowlist in the same commit as the go.mod change.
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

	cmd := exec.Command("go", append([]string{"list", "-f", "{{if not .Standard}}{{.ImportPath}}|{{.Module.Path}}@{{.Module.Version}}{{end}}", "-deps"}, releaseBinaries...)...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) != 2 || parts[1] == "@" || strings.HasPrefix(parts[1], "github.com/vyquocvu/goosie@") {
			continue
		}
		mod := parts[1]
		if seen[mod] {
			continue
		}
		seen[mod] = true
		lic, ok := allowed[mod]
		if !ok {
			t.Errorf("linked module %s is not in dependency-licenses.txt (re-audit and amend)", mod)
			continue
		}
		if !permittedLicenses[lic] {
			t.Errorf("linked module %s has non-permitted license %q", mod, lic)
		}
	}
	for mod := range allowed {
		if !seen[mod] {
			t.Errorf("allowlist entry %s is linked into no release binary (stale pin?)", mod)
		}
	}
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
