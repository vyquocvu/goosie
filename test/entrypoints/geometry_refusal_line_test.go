package entrypoints

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoosieGeometryRefusalLineIsWellFormed drives the real binary over a
// document whose only CSS length is 8e8e8px - the form TASK-040 had to keep
// refusing after it stopped refusing cafef.vn - and reads the line an operator
// actually gets. It pins three things at once: the run still fails closed (no
// PNG, nonzero exit), the refusal names the property, the value, the threshold
// and the branch that fired, and the program label appears exactly once so the
// message does not start with the twice-wrapped `goosie: goosie:` that made the
// useful half of the line stop being its start.
// qc:BR-164
func TestGoosieGeometryRefusalLineIsWellFormed(t *testing.T) {
	bin := buildBinary(t, "goosie")

	tmp := t.TempDir()
	html := filepath.Join(tmp, "oversized.html")
	doc := `<!doctype html><html><head><style>.a{width:8e8e8px}</style></head>` +
		`<body><div class="a">hi</div></body></html>`
	if err := os.WriteFile(html, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "out.png")

	cmd := exec.Command(bin, "-screenshot", "-out", png, "-url", "file://"+filepath.ToSlash(html))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("oversized CSS length succeeded, want refusal\nstderr: %s", stderr.String())
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Errorf("exit code = %v, want 1 (a usage error would be 2): %v", exit, err)
	}
	if _, statErr := os.Stat(png); statErr == nil {
		t.Errorf("refused document still wrote a PNG at %s", png)
	}

	lines := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n")
	var refusal string
	for _, l := range lines {
		if strings.Contains(l, "outside safe geometry range") {
			refusal = l
			break
		}
	}
	if refusal == "" {
		t.Fatalf("no geometry refusal on stderr:\n%s", stderr.String())
	}
	for _, want := range []string{
		`property "width"`, // which declaration
		`value "8e8"`,      // which token
		`unit "e8px"`,      // and what unit it carried
		"limit 2097152 CSS pixels",
		"branch magnitude",
	} {
		if !strings.Contains(refusal, want) {
			t.Errorf("refusal line omits %s: %q", want, refusal)
		}
	}
	if strings.HasPrefix(refusal, "goosie: goosie:") {
		t.Errorf("doubled program label on the line an operator reads: %q", refusal)
	}
	if !strings.HasPrefix(refusal, "goosie: ") {
		t.Errorf("refusal line lost the program label: %q", refusal)
	}
	if n := strings.Count(refusal, "goosie: "); n != 1 {
		t.Errorf("goosie: label count = %d, want 1 in %q", n, refusal)
	}
}

// TestGoosieDigitLeadingCustomPropertyIsAdmitted is the other half of
// GOOSIE-LIVE-002, measured on cafef.vn: a legal custom property whose name
// begins with digits (--8e8e8e, a colour name) is an ident, not the dimension
// -8e8, so the document must open. Before the tokenizer fix this run exited 1
// and wrote no PNG at all.
// qc:BR-164
func TestGoosieDigitLeadingCustomPropertyIsAdmitted(t *testing.T) {
	bin := buildBinary(t, "goosie")

	tmp := t.TempDir()
	html := filepath.Join(tmp, "custom-property.html")
	doc := `<!doctype html><html><head><style>:root{--8e8e8e:#8e8e8e}` +
		`.a{color:var(--8e8e8e);background:#fff;width:200px;height:40px}</style></head>` +
		`<body><div class="a">hi</div></body></html>`
	if err := os.WriteFile(html, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "out.png")

	cmd := exec.Command(bin, "-screenshot", "-out", png, "-url", "file://"+filepath.ToSlash(html))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("digit-leading custom property refused: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "outside safe geometry range") {
		t.Errorf("admission guard fired on a legal custom property:\n%s", stderr.String())
	}
	st, statErr := os.Stat(png)
	if statErr != nil {
		t.Fatalf("no PNG written: %v", statErr)
	}
	if st.Size() < 1024 {
		t.Errorf("PNG is %d bytes, too small to be a painted page", st.Size())
	}
}
