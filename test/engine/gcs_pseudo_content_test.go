package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

const gcsContentPageBase = "https://example.test/dir/page.html"

const gcsContentPage = `<!DOCTYPE html><html><head>
<style>
  body { font-size: 16px; color: rgb(10, 20, 30); counter-reset: c 3; }
  #curl::before { content: url(none.png); }
  #curlabs::before { content: url(https://example.test/x.png); }
  #curlquoted::before { content: url("a b.png"); }
  #curlroot::before { content: url(/none.png); }
  #curlmix::before { content: url(none.png) "T"; }
  #cattr::before { content: attr(data-x); }
  #cattrnum::before { content: attr(data-num); }
  #cattrsp::before { content: attr(data-sp); }
  #cattrq::before { content: attr(data-q); }
  #cattrmiss::before { content: attr(data-nope); }
  #cattrfb::before { content: attr(data-nope, "FB"); }
  #cattrintfb::before { content: attr(data-nope, 42); }
  #ccount::before { content: counter(c); }
  #ccountstyle::before { content: counter(c, lower-roman); }
  #ccounters::before { content: counters(c, "-"); }
  #copenq::before { content: open-quote; }
  #cmixtext::before { content: "A" counter(c) "B"; }
  #cmixattr::before { content: "x" attr(data-x) "y"; }
  #cmixattrcounter::before { content: attr(data-x) counter(c); }
  #cmixempty::before { content: "" attr(data-x); }
  #cnormal::before { content: normal; }
  #cnonekw::before { content: none; }
  #cemptystr::before { content: ""; }
  #cstr::before { content: "PRE"; }
  #cslash::before { content: "a" / "b"; }
  #cafter::after { content: attr(data-x); }
  #cmarkerr::before { content: attr(data-x); }
  #cfl::first-line { content: attr(data-x); }
  #cfl::first-letter { content: attr(data-x); }
  #cph { color: rgb(9, 9, 9); }
  #cph::placeholder { content: attr(data-x); }
  #cesctab::before { content: "a\9 b"; }
  #cesccr::before { content: "a\d b"; }
  #cescone::before { content: "a\1 b"; }
  #cescuni::before { content: "a\e9 b"; }
  #cescqt::before { content: "a\"b"; }
  #cescbk::before { content: "a\\b"; }
  #cesc6::before { content: "a\000041 b"; }
  #cescsur::before { content: "a\d800b"; }
  #cesczero::before { content: "a\0 b"; }
  #cattrupper::before { content: attr(DATA-X); }
  #cattrmissupper::before { content: attr(DATA-NOPE); }
  #cinval::before { content: bogus(1); }
  #cquotes::before { content: quotes("«" "»"); }
  #cleader::before { content: leader(".") "x"; }
  #cstring::before { content: string(x); }
  #celement::before { content: element(#curl); }
  #cmixall::before { content: "a" counter(c) url(none.png) attr(data-x) open-quote; }
  #cimgset::before { content: image-set(url(none.png) 1x); }
  #cctl0::before { content: attr(data-c); }
  #cctl1::before { content: attr(data-c); }
  #cctl2::before { content: attr(data-c); }
  #cctl3::before { content: attr(data-c); }
  #cctl4::before { content: attr(data-c); }
  #cctl5::before { content: attr(data-c); }
</style>
</head><body>
<div id="curl">a</div>
<div id="curlabs">b</div>
<div id="curlquoted">c</div>
<div id="curlroot">d</div>
<div id="curlmix">e</div>
<div id="cattr" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>f</div>
<div id="cattrnum" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>g</div>
<div id="cattrsp" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>h</div>
<div id="cattrq" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>i</div>
<div id="cattrmiss" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>j</div>
<div id="cattrfb" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>k</div>
<div id="cattrintfb" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>l</div>
<div id="ccount">m</div>
<div id="ccountstyle">n</div>
<div id="ccounters">o</div>
<div id="copenq">p</div>
<div id="cmixtext" data-x="ATTR">q</div>
<div id="cmixattr" data-x="ATTR">r</div>
<div id="cmixattrcounter" data-x="ATTR">s</div>
<div id="cmixempty" data-x="ATTR">t</div>
<div id="cnormal">u</div>
<div id="cnonekw">v</div>
<div id="cemptystr">w</div>
<div id="cstr">x</div>
<div id="cslash">y</div>
<div id="cafter" data-x="ATTR">z</div>
<ul id="cmarkerr" data-x="ATTR"><li>mark</li></ul>
<div id="cfl">alpha beta gamma delta</div>
<div id="cesctab">e1</div>
<div id="cesccr">e2</div>
<div id="cescone">e3</div>
<div id="cescuni">e4</div>
<div id="cescqt">e5</div>
<div id="cescbk">e6</div>
<div id="cesc6">e6a</div>
<div id="cescsur">e6b</div>
<div id="cesczero">e6c</div>
<div id="cattrupper" data-x="ATTR">e7</div>
<div id="cattrmissupper" data-x="ATTR">e8</div>
<div id="cinval">e15</div>
<div id="cquotes">e16</div>
<div id="cleader">e17</div>
<div id="cstring">e18</div>
<div id="celement">e19</div>
<div id="cmixall" data-x="ATTR">e20</div>
<div id="cimgset" data-x="ATTR">e21</div>
<div id="cctl0">e9</div>
<div id="cctl1">e10</div>
<div id="cctl2">e11</div>
<div id="cctl3">e12</div>
<div id="cctl4">e13</div>
<div id="cctl5">e14</div>
<input id="cph" placeholder="type here">
</body></html>`

const gcsContentCtlSetup = `(function () {
  var v = ['a\tb', 'a\nb', 'a\rb', 'a\\b', 'a"b', 'a\u00e9b\u0001c\u00a0d'];
  for (var i = 0; i < v.length; i++) document.getElementById('cctl' + i).setAttribute('data-c', v[i]);
  return v.length;
})()`

func gcsContentSession(t *testing.T) *js.Runtime {
	t.Helper()
	rt, err := js.New(js.Options{URL: gcsContentPageBase})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	sess, err := engine.NewSession(gcsContentPage, nil, 800,
		engine.WithJS(rt), engine.WithImages(gcsContentPageBase, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return rt
}

func gcsContentRead(t *testing.T, rt *js.Runtime, id, pseudo string) string {
	t.Helper()
	src := `getComputedStyle(document.getElementById("` + id + `"), "::` + pseudo + `").getPropertyValue("content")`
	v, err := rt.Evaluate(src)
	if err != nil {
		t.Fatalf("evaluate %s: %v", src, err)
	}
	return v
}

type gcsContentCase struct{ id, pseudo, want string }

func gcsContentCheck(t *testing.T, rt *js.Runtime, cases []gcsContentCase) {
	t.Helper()
	for _, c := range cases {
		if got := gcsContentRead(t, rt, c.id, c.pseudo); got != c.want {
			t.Errorf("#%s::%s content: got %q want %q", c.id, c.pseudo, got, c.want)
		}
	}
}

// qc:BR-326
func TestGetComputedStyleContentQuotesAndResolvesURLs(t *testing.T) {
	rt := gcsContentSession(t)
	gcsContentCheck(t, rt, []gcsContentCase{
		{"curl", "before", `url("https://example.test/dir/none.png")`},
		{"curlabs", "before", `url("https://example.test/x.png")`},
		{"curlquoted", "before", `url("https://example.test/dir/a%20b.png")`},
		{"curlroot", "before", `url("https://example.test/none.png")`},
		{"curlmix", "before", `url("https://example.test/dir/none.png") "T"`},
	})
}

// qc:BR-326
func TestGetComputedStyleContentEvaluatesAttr(t *testing.T) {
	rt := gcsContentSession(t)
	gcsContentCheck(t, rt, []gcsContentCase{
		{"cattr", "before", `"ATTR"`},
		{"cattrnum", "before", `"42"`},
		{"cattrsp", "before", `"a b"`},
		{"cattrq", "before", `"say \"hi\""`},
		{"cattrmiss", "before", `""`},
		{"cattrfb", "before", `"FB"`},
		{"cattrintfb", "before", "none"},
		{"cafter", "after", `"ATTR"`},
		{"cmarkerr", "before", `"ATTR"`},
		{"cph", "placeholder", `""`},
	})
}

// qc:BR-326
func TestGetComputedStyleContentEchoesCounterAndQuoteForms(t *testing.T) {
	rt := gcsContentSession(t)
	gcsContentCheck(t, rt, []gcsContentCase{
		{"ccount", "before", "counter(c)"},
		{"ccountstyle", "before", "counter(c, lower-roman)"},
		{"ccounters", "before", `counters(c, "-")`},
		{"copenq", "before", "open-quote"},
		{"cmixtext", "before", `"A" counter(c) "B"`},
	})
}

// qc:BR-326
func TestGetComputedStyleContentEscapesAndMatchesAttributeCase(t *testing.T) {
	rt := gcsContentSession(t)
	if _, err := rt.Evaluate(gcsContentCtlSetup); err != nil {
		t.Fatalf("set attribute script: %v", err)
	}
	gcsContentCheck(t, rt, []gcsContentCase{
		{"cesctab", "before", `"a\9 b"`},
		{"cesccr", "before", `"a\d b"`},
		{"cescone", "before", `"a\1 b"`},
		{"cescuni", "before", `"aéb"`},
		{"cescqt", "before", `"a\"b"`},
		{"cescbk", "before", `"a\\b"`},
		{"cesc6", "before", `"aAb"`},
		{"cescsur", "before", "\"a\U000d800b\""},
		{"cesczero", "before", "\"a\ufffdb\""},
		{"cattrupper", "before", `"ATTR"`},
		{"cattrmissupper", "before", `""`},
		{"cctl0", "before", `"a\9 b"`},
		{"cctl1", "before", `"a\a b"`},
		{"cctl2", "before", `"a\d b"`},
		{"cctl3", "before", `"a\\b"`},
		{"cctl4", "before", `"a\"b"`},
		{"cctl5", "before", "\"aéb\\1 c\u00a0d\""},
	})
}

// qc:BR-326
func TestGetComputedStyleContentInvalidatesUnsupportedFunctions(t *testing.T) {
	rt := gcsContentSession(t)
	gcsContentCheck(t, rt, []gcsContentCase{
		{"cinval", "before", "none"},
		{"cquotes", "before", "none"},
		{"cleader", "before", "none"},
		{"cstring", "before", "none"},
		{"celement", "before", "none"},
		{"cmixall", "before", `"a" counter(c) url("https://example.test/dir/none.png") "ATTR" open-quote`},
	})
	if got := gcsContentRead(t, rt, "cimgset", "before"); got != "none" {
		t.Errorf("#cimgset::before content: got %q, want the registered divergence: this engine answers the initial keyword while Chromium answers image-set(url(\"https://example.test/dir/none.png\") 1dppx) — GOOSIE-GCSPSEUDO-001 residual (vi)", got)
	}
}

// qc:BR-326
func TestGetComputedStyleContentMergesAdjacentStringAndAttrRuns(t *testing.T) {
	rt := gcsContentSession(t)
	gcsContentCheck(t, rt, []gcsContentCase{
		{"cmixattr", "before", `"xATTRy"`},
		{"cmixattrcounter", "before", `"ATTR" counter(c)`},
		{"cmixempty", "before", `"ATTR"`},
	})
}

// qc:BR-326
func TestGetComputedStyleContentKeepsThePinnedBR322Spellings(t *testing.T) {
	rt := gcsContentSession(t)
	gcsContentCheck(t, rt, []gcsContentCase{
		{"cstr", "before", `"PRE"`},
		{"cemptystr", "before", `""`},
		{"cnormal", "before", "none"},
		{"cnonekw", "before", "none"},
		{"cslash", "before", `"a" / "b"`},
		{"cfl", "first-line", "normal"},
		{"cfl", "first-letter", "normal"},
	})
}
