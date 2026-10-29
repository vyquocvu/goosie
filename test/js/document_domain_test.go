package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestDocumentDomainGetterAndRelaxation(t *testing.T) {
	rt := newRuntime(t, js.Options{
		URL: "https://shop.example.test:8443/path",
		DOM: domtest.Parse(`<html><body></body></html>`),
	})
	if err := rt.Run(`
		var initialDomain = document.domain;
		document.domain = "example.test";
		var relaxedDomain = document.domain;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `initialDomain`); got != "shop.example.test" {
		t.Errorf("initial document.domain = %q, want shop.example.test", got)
	}
	if got := evalString(t, rt, `relaxedDomain`); got != "example.test" {
		t.Errorf("document.domain after relaxation = %q, want example.test", got)
	}
	if err := rt.Run(`
		try { document.domain = "shop.example.test"; } catch (error) { var narrowerDomainError = error.name; }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `narrowerDomainError`); got != "SecurityError" {
		t.Errorf("setting document.domain back to a narrower host = %q, want SecurityError", got)
	}
}

func TestDocumentDomainRejectsInvalidSuffixes(t *testing.T) {
	rt := newRuntime(t, js.Options{
		URL: "https://shop.example.test/path",
		DOM: domtest.Parse(`<html><body></body></html>`),
	})
	if err := rt.Run(`
		var publicSuffixError;
		try { document.domain = "test"; } catch (error) { publicSuffixError = error.name; }
		var unrelatedDomainError;
		try { document.domain = "attacker.test"; } catch (error) { unrelatedDomainError = error.name; }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"publicSuffixError", "unrelatedDomainError"} {
		if got := evalString(t, rt, name); got != "SecurityError" {
			t.Errorf("%s = %q, want SecurityError", name, got)
		}
	}
}
