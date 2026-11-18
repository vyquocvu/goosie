package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

const fontWideSheet = `
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.inh input, .inh select, .inh button, .inh textarea { font: inherit; }
.ini input, .ini select, .ini button, .ini textarea { font: initial; }
.un input, .un button { font: unset; }
.deep { font: 20px Georgia, serif; }
.deep input { font: inherit; }
.loud { font: italic 700 20px/30px Georgia, serif; }
#pseudoHost::before { content: "P"; font: inherit; }
#pseudoHost::after { content: "A"; font: initial; }`

const fontWideHTML = `<!DOCTYPE html><html><head><style>` + fontWideSheet + `</style></head><body>
<div class="g plain"><input id="plainInput" value="Hello"><select id="plainSelect"><option>Go</option></select><button id="plainButton">Go</button><textarea id="plainTextarea">Hello</textarea></div>
<div class="g inh"><input id="inhInput" value="Hello"><select id="inhSelect"><option>Go</option></select><button id="inhButton">Go</button><textarea id="inhTextarea">Hello</textarea></div>
<div class="g ini"><input id="iniInput" value="Hello"><select id="iniSelect"><option>Go</option></select><button id="iniButton">Go</button><textarea id="iniTextarea">Hello</textarea></div>
<div class="g un"><input id="unInput" value="Hello"><button id="unButton">Go</button></div>
<div class="g deep"><input id="deepInput" value="Hello"></div>
<div class="g loud"><input id="loudInherit" style="font: inherit" value="Hello"><input id="loudInitial" style="font: initial" value="Hello"><input id="loudUnset" style="font: unset" value="Hello"></div>
<input id="inheritThenLonghand" style="font: inherit; font-size: 12px" value="Hello">
<input id="longhandThenInherit" style="font-size: 12px; font: inherit" value="Hello">
<input id="invalidMix" style="font: inherit bold 12px Arial" value="Hello">
<input id="trailingInheritIdent" style="font: 12px Arial inherit" value="Hello">
<input id="quotedFamilyNamedInherit" style="font: 12px Arial, 'inherit'" value="Hello">
<input id="initialThenLonghand" style="font: initial; font-family: sans-serif" value="Hello">
<div id="divInitial" style="font: initial"><span id="divInitialKid">text</span></div>
<input id="colourKept" style="color: rgb(1, 2, 3); font: inherit" value="Hello">
<div class="loud" id="pseudoHost">text</div>
</body></html>`

type fontGroup struct {
	Size       float32
	Family     frame.FontFamily
	Weight     style.FontWeight
	Face       style.FontStyle
	LineHeight float32
}

func fontWideStyles(t *testing.T) map[string]*style.ComputedStyle {
	t.Helper()
	doc := domtest.Parse(fontWideHTML)
	styles := style.Resolve(doc, []*css.Stylesheet{css.ParseForViewport(fontWideSheet, 800)}, nil)
	out := map[string]*style.ComputedStyle{}
	for _, id := range []string{"plainInput", "plainSelect", "plainButton", "plainTextarea",
		"inhInput", "inhSelect", "inhButton", "inhTextarea",
		"iniInput", "iniSelect", "iniButton", "iniTextarea",
		"unInput", "unButton", "deepInput",
		"loudInherit", "loudInitial", "loudUnset",
		"inheritThenLonghand", "longhandThenInherit", "invalidMix", "trailingInheritIdent",
		"quotedFamilyNamedInherit", "initialThenLonghand", "divInitial", "divInitialKid", "colourKept"} {
		el := doc.ElementByID(id)
		if el == nil {
			t.Fatalf("no #%s in the fixture", id)
		}
		st := styles[el.ID]
		if st == nil {
			t.Fatalf("no computed style for #%s", id)
		}
		out[id] = st
	}
	return out
}

func fontWideGroups(t *testing.T) map[string]fontGroup {
	t.Helper()
	out := map[string]fontGroup{}
	for id, st := range fontWideStyles(t) {
		out[id] = fontGroup{Size: st.FontSize, Family: st.FontFamily, Weight: st.FontWeight,
			Face: st.FontStyle, LineHeight: st.LineHeight}
	}
	return out
}

// qc:BR-323
func TestFontShorthandInheritCopiesTheParentsWholeGroup(t *testing.T) {
	got := fontWideGroups(t)
	normal := fontGroup{16, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}
	for _, c := range []struct {
		id, why string
		want    fontGroup
	}{
		{"inhInput", "a text field takes the document's 16px sans-serif instead of the UA control face", normal},
		{"inhSelect", "select", normal},
		{"inhButton", "button", normal},
		{"inhTextarea", "textarea, whose UA face is 13.3333px monospace", normal},
		{"unInput", "unset on this group is inherit because every longhand of it inherits", normal},
		{"unButton", "unset on the button", normal},
		{"deepInput", "the parent's own family and size, not the document's", fontGroup{20, frame.FontGeorgia, style.WeightNormal, style.FontStyleNormal, -1}},
		{"loudInherit", "weight, italic face and the parent's 30px line-height travel with the group", fontGroup{20, frame.FontGeorgia, style.WeightBold, style.FontStyleItalic, 30}},
		{"loudUnset", "unset reaches the same group as inherit", fontGroup{20, frame.FontGeorgia, style.WeightBold, style.FontStyleItalic, 30}},
	} {
		if got[c.id] != c.want {
			t.Errorf("#%s = %+v, want %+v (%s)", c.id, got[c.id], c.want, c.why)
		}
	}
}

// qc:BR-323
func TestFontShorthandInitialResetsTheGroupToTheCssInitialValues(t *testing.T) {
	got := fontWideGroups(t)
	initial := fontGroup{16, frame.FontTimes, style.WeightNormal, style.FontStyleNormal, -1}
	for _, c := range []struct {
		id, why string
		want    fontGroup
	}{
		{"iniInput", "the initial group is 16px Times, which is the reference's standard family, not the document's sans-serif", initial},
		{"iniSelect", "select", initial},
		{"iniButton", "button", initial},
		{"iniTextarea", "textarea, whose UA face is monospace", initial},
		{"loudInitial", "initial discards the parent's bold italic 30px group", initial},
		{"divInitial", "a non-control resets the same way", initial},
		{"divInitialKid", "and the child inherits the reset group", initial},
	} {
		if got[c.id] != c.want {
			t.Errorf("#%s = %+v, want %+v (%s)", c.id, got[c.id], c.want, c.why)
		}
	}
}

// qc:BR-323
func TestFontShorthandWideKeywordKeepsItsPlaceInTheCascade(t *testing.T) {
	got := fontWideGroups(t)
	for _, c := range []struct {
		id, why string
		want    fontGroup
	}{
		{"plainInput", "with no keyword declared the measured UA face stays", fontGroup{13.3333, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}},
		{"plainButton", "and the same on the button", fontGroup{13.3333, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}},
		{"plainTextarea", "and the monospace field", fontGroup{13.3333, frame.FontCourier, style.WeightNormal, style.FontStyleNormal, -1}},
		{"inheritThenLonghand", "a longhand declared after the keyword still wins inside the group", fontGroup{12, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}},
		{"longhandThenInherit", "the keyword declared after the longhand replaces it", fontGroup{16, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}},
		{"initialThenLonghand", "initial then an author family", fontGroup{16, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}},
	} {
		if got[c.id] != c.want {
			t.Errorf("#%s = %+v, want %+v (%s)", c.id, got[c.id], c.want, c.why)
		}
	}
}

// qc:BR-323
func TestFontShorthandRejectsAWholeDeclarationThatMixesAWideKeywordWithComponents(t *testing.T) {
	got := fontWideGroups(t)
	if want := (fontGroup{13.3333, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}); got["invalidMix"] != want {
		t.Errorf("#invalidMix = %+v, want %+v: `font: inherit bold 12px Arial` is invalid, so none of it may land", got["invalidMix"], want)
	}
	if want := (fontGroup{12, frame.FontTimes, style.WeightNormal, style.FontStyleNormal, -1}); got["trailingInheritIdent"] != want {
		t.Errorf("#trailingInheritIdent = %+v, want %+v: `font: 12px Arial inherit` is a two-word family name, not the keyword - the reference computes the family \"Arial inherit\", which is an unknown face", got["trailingInheritIdent"], want)
	}
	if want := (fontGroup{12, frame.FontArial, style.WeightNormal, style.FontStyleNormal, -1}); got["quotedFamilyNamedInherit"] != want {
		t.Errorf("#quotedFamilyNamedInherit = %+v, want %+v: a quoted family that spells a keyword is a family name", got["quotedFamilyNamedInherit"], want)
	}
	if c := fontWideStyles(t)["colourKept"].Color; c != (css.Color{R: 1, G: 2, B: 3, A: 255}) {
		t.Errorf("#colourKept colour = %v, want rgb(1,2,3): the font group must not reach outside itself", c)
	}
}

// qc:BR-323
func TestPseudoElementFontShorthandWideKeywordsSeeTheHostGroup(t *testing.T) {
	doc := domtest.Parse(fontWideHTML)
	sheet := css.ParseForViewport(fontWideSheet, 800)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	pseudoStyles := style.ResolvePseudoElements(doc, []*css.Stylesheet{sheet}, styles, nil)
	host := doc.ElementByID("pseudoHost")
	if host == nil {
		t.Fatal("no #pseudoHost in the fixture")
	}
	for _, c := range []struct {
		pseudo string
		want   fontGroup
		why    string
	}{
		{"before", fontGroup{20, frame.FontGeorgia, style.WeightBold, style.FontStyleItalic, 30}, "`font: inherit` on a pseudo takes the host's whole group, which is the reference's 20px Georgia italic 700 on a 30px line"},
		{"after", fontGroup{16, frame.FontTimes, style.WeightNormal, style.FontStyleNormal, -1}, "`font: initial` on a pseudo is the initial group, not the host's"},
	} {
		cs, ok := pseudoStyles[style.PseudoKey{NodeID: host.ID, Pseudo: c.pseudo}]
		if !ok {
			t.Errorf("#pseudoHost::%s has no resolved pseudo style", c.pseudo)
			continue
		}
		got := fontGroup{Size: cs.FontSize, Family: cs.FontFamily, Weight: cs.FontWeight,
			Face: cs.FontStyle, LineHeight: cs.LineHeight}
		if got != c.want {
			t.Errorf("#pseudoHost::%s = %+v, want %+v (%s)", c.pseudo, got, c.want, c.why)
		}
	}
}
