package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

const gcsPseudoPage = `<!DOCTYPE html><html><head>
<style>
  body { font-size: 16px; color: rgb(10, 20, 30); }
  #gen { color: rgb(1, 2, 3); font-size: 20px; }
  #gen::before { content: "PRE"; color: rgb(200, 0, 0); font-size: 12px; display: inline-block; width: 40px; height: 15px; margin-right: 7px; }
  #gen::after { content: ""; color: rgb(7, 8, 9); }
  #fl { width: 200px; }
  #fl::first-line { color: rgb(40, 50, 60); text-transform: uppercase; width: 99px; margin-left: 8px; }
  #fl::first-letter { color: rgb(60, 70, 80); font-size: 32px; }
  #none { display: none; }
  #none::before { content: "HIDDEN"; color: rgb(30, 30, 30); }
  #cnt::before { content: none; color: rgb(11, 12, 13); }
  #auth::placeholder { color: rgb(180, 30, 30); }
  #inh { color: rgb(70, 70, 70); font-size: 13px; }
</style>
</head><body>
<div id="plain">plain text</div>
<div id="gen">host text</div>
<div id="fl">alpha beta gamma delta</div>
<div id="none">hidden text</div>
<div id="cnt">count text</div>
<div id="inh">inherited host</div>
<input id="pha" placeholder="type here">
<input id="phb">
<input id="auth" placeholder="auth">
</body></html>`

func gcsPseudoSession(t *testing.T) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	sess, err := engine.NewSession(gcsPseudoPage, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, rt
}

func gcsRead(t *testing.T, rt *js.Runtime, id, pseudo, property string) string {
	t.Helper()
	src := `getComputedStyle(document.getElementById("` + id + `"), `
	if pseudo == "" {
		src += ")"
	} else {
		src += `"::` + pseudo + `")`
	}
	src += `.getPropertyValue("` + property + `")`
	v, err := rt.Evaluate(src)
	if err != nil {
		t.Fatalf("evaluate %s: %v", src, err)
	}
	return v
}

// qc:BR-322
func TestGetComputedStyleResolvesTheGeneratedPseudoStyle(t *testing.T) {
	_, rt := gcsPseudoSession(t)

	cases := []struct{ id, pseudo, property, want string }{
		{"gen", "before", "content", `"PRE"`},
		{"gen", "before", "color", "rgb(200, 0, 0)"},
		{"gen", "before", "font-size", "12px"},
		{"gen", "before", "display", "inline-block"},
		{"gen", "before", "width", "40px"},
		{"gen", "before", "height", "15px"},
		{"gen", "before", "margin-right", "7px"},
		{"gen", "before", "position", "static"},
		{"gen", "before", "visibility", "visible"},
		{"gen", "after", "content", `""`},
		{"gen", "after", "color", "rgb(7, 8, 9)"},
		{"gen", "after", "font-size", "20px"},
		{"gen", "after", "display", "inline"},
		{"gen", "after", "width", "auto"},
		{"gen", "after", "height", "auto"},
	}
	for _, c := range cases {
		if got := gcsRead(t, rt, c.id, c.pseudo, c.property); got != c.want {
			t.Errorf("#%s::%s %s: got %q want %q", c.id, c.pseudo, c.property, got, c.want)
		}
	}
}

// qc:BR-322
func TestGetComputedStyleAnswersUngeneratedPseudoElements(t *testing.T) {
	_, rt := gcsPseudoSession(t)

	cases := []struct{ id, pseudo, property, want string }{
		{"plain", "before", "content", "none"},
		{"plain", "before", "color", "rgb(10, 20, 30)"},
		{"plain", "before", "font-size", "16px"},
		{"plain", "before", "display", "inline"},
		{"plain", "after", "content", "none"},
		{"plain", "first-line", "content", "normal"},
		{"plain", "first-line", "color", "rgb(10, 20, 30)"},
		{"plain", "first-letter", "color", "rgb(10, 20, 30)"},
		{"plain", "selection", "content", "normal"},
		{"plain", "selection", "display", "inline"},
		{"plain", "marker", "display", "inline-block"},
		{"plain", "marker", "content", "normal"},
		{"plain", "backdrop", "display", "block"},
		{"plain", "before", "width", "auto"},
		{"plain", "before", "height", "auto"},
		{"gen", "highlight(x)", "display", "inline"},
		{"inh", "before", "color", "rgb(70, 70, 70)"},
		{"inh", "before", "font-size", "13px"},
		{"cnt", "before", "content", "none"},
		{"cnt", "before", "color", "rgb(11, 12, 13)"},
		{"none", "before", "content", `"HIDDEN"`},
		{"none", "before", "color", "rgb(30, 30, 30)"},
	}
	for _, c := range cases {
		if got := gcsRead(t, rt, c.id, c.pseudo, c.property); got != c.want {
			t.Errorf("#%s::%s %s: got %q want %q", c.id, c.pseudo, c.property, got, c.want)
		}
	}
}

// qc:BR-322
func TestGetComputedStyleFirstLineDropsThePropertiesItCannotCarry(t *testing.T) {
	_, rt := gcsPseudoSession(t)

	cases := []struct{ property, want string }{
		{"color", "rgb(40, 50, 60)"},
		{"text-transform", "uppercase"},
		{"width", "auto"},
		{"height", "auto"},
		{"margin-left", "0px"},
		{"opacity", "1"},
		{"visibility", "visible"},
		{"display", "inline"},
	}
	for _, c := range cases {
		if got := gcsRead(t, rt, "fl", "first-line", c.property); got != c.want {
			t.Errorf("::first-line %s: got %q want %q", c.property, got, c.want)
		}
	}
	if got := gcsRead(t, rt, "fl", "first-letter", "color"); got != "rgb(60, 70, 80)" {
		t.Errorf("::first-letter color: got %q", got)
	}
	if got := gcsRead(t, rt, "fl", "first-letter", "font-size"); got != "32px" {
		t.Errorf("::first-letter font-size: got %q", got)
	}
	if got := gcsRead(t, rt, "fl", "first-letter", "content"); got != "normal" {
		t.Errorf("::first-letter content: got %q want normal", got)
	}
}

// qc:BR-322
func TestGetComputedStylePlaceholderBaselineFollowsTheShownState(t *testing.T) {
	_, rt := gcsPseudoSession(t)

	if got := gcsRead(t, rt, "pha", "placeholder", "color"); got != "rgb(117, 117, 117)" {
		t.Errorf("shown placeholder colour: got %q want the measured baseline", got)
	}
	if got := gcsRead(t, rt, "phb", "placeholder", "color"); got != "rgb(10, 20, 30)" {
		t.Errorf("placeholder colour without a placeholder attribute: got %q want the element colour", got)
	}
	if got := gcsRead(t, rt, "auth", "placeholder", "color"); got != "rgb(180, 30, 30)" {
		t.Errorf("authored placeholder colour: got %q", got)
	}
}

// qc:BR-322
func TestGetComputedStylePseudoArgumentLeavesTheElementPathAlone(t *testing.T) {
	_, rt := gcsPseudoSession(t)

	cases := []struct{ id, property, want string }{
		{"gen", "color", "rgb(1, 2, 3)"},
		{"gen", "font-size", "20px"},
		{"gen", "display", "block"},
		{"plain", "color", "rgb(10, 20, 30)"},
	}
	for _, c := range cases {
		if got := gcsRead(t, rt, c.id, "", c.property); got != c.want {
			t.Errorf("element #%s %s: got %q want %q", c.id, c.property, got, c.want)
		}
	}
	if got := gcsRead(t, rt, "gen", "", "content"); got != "" {
		t.Errorf("element content: got %q want the unmodelled empty answer", got)
	}
}

// qc:BR-322
func TestGetComputedStylePartAndSlottedAnswerTheElementStyle(t *testing.T) {
	_, rt := gcsPseudoSession(t)

	for _, pseudo := range []string{"part(widget)", "slotted(div)"} {
		v, err := rt.Evaluate(`getComputedStyle(document.getElementById("gen"), "::` + pseudo + `").getPropertyValue("color")`)
		if err != nil {
			t.Fatalf("%s: %v", pseudo, err)
		}
		if v != "rgb(1, 2, 3)" {
			t.Errorf("%s colour: got %q want the element's own cascade", pseudo, v)
		}
	}
}

// qc:BR-322
func TestReadingAPseudoStyleDoesNotGenerateItsBox(t *testing.T) {
	sess, rt := gcsPseudoSession(t)

	before := visiblePaintText(t, sess)
	if !strings.Contains(before, "PRE") {
		t.Fatalf("the authored ::before content disappeared from the paint list: %q", before)
	}
	if strings.Contains(before, "HIDDEN") {
		t.Errorf("a display:none host generated pseudo ink: %q", before)
	}
	for _, id := range []string{"gen", "none", "cnt", "plain", "inh"} {
		if got := gcsRead(t, rt, id, "before", "content"); got == "" {
			t.Errorf("#%s::before content came back empty", id)
		}
	}
	after := visiblePaintText(t, sess)
	if after != before {
		t.Errorf("reading the pseudo style changed the paint list:\nbefore %q\nafter  %q", before, after)
	}
}
