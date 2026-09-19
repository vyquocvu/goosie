package engine

import (
	"errors"
	"strings"
	"testing"
)

func TestExpandCSSImportsSplicesUnconditionedSheetsInPlace(t *testing.T) {
	fetched := []string{}
	linker := func(base, href string) (string, error) {
		fetched = append(fetched, href)
		switch href {
		case "https://e/x/theme.css":
			return ":root { --c: red }\n@import url(\"deep.css\");", nil
		case "https://e/x/deep.css":
			return "a { color: var(--c) }", nil
		}
		return "", errors.New("missing")
	}
	in := "@import url(\"theme.css\");\nbody { color: blue }"
	out := expandCSSImports(linker, "https://e/x/page.css", in, map[string]bool{"https://e/x/page.css": true}, 1)
	if !strings.Contains(out, "--c: red") {
		t.Fatalf("imported sheet missing from:\n%s", out)
	}
	if !strings.Contains(out, "color: var(--c)") {
		t.Fatalf("nested import missing from:\n%s", out)
	}
	if i, j := strings.Index(out, "--c: red"), strings.Index(out, "color: blue"); i > j {
		t.Fatalf("import must cascade before the rules that follow the statement:\n%s", out)
	}
	if strings.Contains(out, "@import") {
		t.Errorf("the statement itself should be gone:\n%s", out)
	}
}

func TestExpandCSSImportsSkipsConditionedAndCyclicStatements(t *testing.T) {
	linker := func(base, href string) (string, error) {
		if href == "https://e/a.css" {
			// a.css imports itself: the cycle must break on the first repeat.
			return `@import url("a.css"); b { color: green }`, nil
		}
		return "", errors.New("missing")
	}
	in := "@import url(\"dark.css\") (prefers-color-scheme: dark);\n@import url(\"print.css\") print;\n@import url(\"a.css\");"
	out := expandCSSImports(linker, "https://e/page.css", in, map[string]bool{}, 1)
	if strings.Contains(out, "dark") || strings.Contains(out, "print") {
		t.Errorf("conditioned imports must not be fetched:\n%s", out)
	}
	if !strings.Contains(out, "color: green") {
		t.Errorf("unconditioned import missing:\n%s", out)
	}
	if n := strings.Count(out, "color: green"); n != 1 {
		t.Errorf("cycle guard let the sheet in %d times", n)
	}
}
