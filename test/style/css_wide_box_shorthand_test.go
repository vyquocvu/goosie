package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

const cssWideBoxHTML = `<!doctype html><html><body>
<div id="marginParent" style="margin:20px">
  <div id="marginInherit" style="margin:inherit"></div>
  <div id="marginInitial" style="margin:9px;margin:initial"></div>
  <div id="marginUnset" style="margin:9px;margin:unset"></div>
  <div id="marginMixedInherit" style="margin:1px 2px 3px 4px;margin:inherit 9px"></div>
  <div id="marginMixedInitial" style="margin:1px 2px 3px 4px;margin:9px initial"></div>
  <div id="marginMixedUnset" style="margin:1px 2px 3px 4px;margin:unset 9px"></div>
  <div id="marginKeywordThenLonghand" style="margin:inherit;margin-left:7px"></div>
  <div id="marginLonghandThenKeyword" style="margin-left:7px;margin:inherit"></div>
  <div id="marginKeywordThenLogical" style="margin:inherit;margin-inline-start:7px"></div>
  <div id="marginLogicalThenKeyword" style="margin-inline-start:7px;margin:inherit"></div>
  <div id="marginInitialThenLonghand" style="margin:initial;margin-left:7px"></div>
  <div id="marginLonghandThenInitial" style="margin-left:7px;margin:initial"></div>
  <div id="marginUnsetThenLonghand" style="margin:unset;margin-left:7px"></div>
  <div id="marginLonghandThenUnset" style="margin-left:7px;margin:unset"></div>
</div>
<div id="paddingParent" style="padding:1px 2px 3px 4px">
  <div id="paddingInherit" style="padding:inherit"></div>
  <div id="paddingInitial" style="padding:9px;padding:initial"></div>
  <div id="paddingUnset" style="padding:9px;padding:unset"></div>
  <div id="paddingMixedInherit" style="padding:1px 2px 3px 4px;padding:inherit 9px"></div>
  <div id="paddingMixedInitial" style="padding:1px 2px 3px 4px;padding:9px initial"></div>
  <div id="paddingMixedUnset" style="padding:1px 2px 3px 4px;padding:unset 9px"></div>
  <div id="paddingKeywordThenLonghand" style="padding:inherit;padding-left:7px"></div>
  <div id="paddingLonghandThenKeyword" style="padding-left:7px;padding:inherit"></div>
  <div id="paddingKeywordThenLogical" style="padding:inherit;padding-inline-start:7px"></div>
  <div id="paddingLogicalThenKeyword" style="padding-inline-start:7px;padding:inherit"></div>
  <div id="paddingInitialThenLonghand" style="padding:initial;padding-left:7px"></div>
  <div id="paddingLonghandThenInitial" style="padding-left:7px;padding:initial"></div>
  <div id="paddingUnsetThenLonghand" style="padding:unset;padding-left:7px"></div>
  <div id="paddingLonghandThenUnset" style="padding-left:7px;padding:unset"></div>
</div>
<div id="insetParent" style="inset:10px 20px 30px 40px">
  <div id="insetInherit" style="inset:inherit"></div>
  <div id="insetInitial" style="inset:9px;inset:initial"></div>
  <div id="insetUnset" style="inset:9px;inset:unset"></div>
  <div id="insetMixedInherit" style="inset:1px 2px 3px 4px;inset:inherit 9px"></div>
  <div id="insetMixedInitial" style="inset:1px 2px 3px 4px;inset:9px initial"></div>
  <div id="insetMixedUnset" style="inset:1px 2px 3px 4px;inset:unset 9px"></div>
  <div id="insetKeywordThenLonghand" style="inset:inherit;top:7px"></div>
  <div id="insetLonghandThenKeyword" style="top:7px;inset:inherit"></div>
  <div id="insetKeywordThenLogical" style="inset:inherit;inset-block-start:7px"></div>
  <div id="insetLogicalThenKeyword" style="inset-block-start:7px;inset:inherit"></div>
  <div id="insetInitialThenLonghand" style="inset:initial;top:7px"></div>
  <div id="insetLonghandThenInitial" style="top:7px;inset:initial"></div>
  <div id="insetUnsetThenLonghand" style="inset:unset;top:7px"></div>
  <div id="insetLonghandThenUnset" style="top:7px;inset:unset"></div>
</div>
<div id="marginValueParent" style="width:400px;margin:5% auto 7% 11%">
  <div id="marginValueInherit" style="margin:inherit"></div>
  <div id="marginValueInitial" style="margin:initial"></div>
  <div id="marginValueUnset" style="margin:unset"></div>
</div>
</body></html>`

type cssWideMargin struct {
	Top, Right, Bottom, Left float32
	LeftAuto, RightAuto      bool
}

type cssWidePadding struct {
	Top, Right, Bottom, Left float32
}

type cssWideInset struct {
	Top, Right, Bottom, Left float32
	HasTop, HasRight         bool
	HasBottom, HasLeft       bool
}

func cssWideBoxStyles(t *testing.T) map[string]*style.ComputedStyle {
	t.Helper()
	doc := domtest.Parse(cssWideBoxHTML)
	sheet := css.Parse("html, body { margin: 0; padding: 0; }")
	resolved := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, style.Viewport{W: 800, H: 600}, nil)
	out := make(map[string]*style.ComputedStyle)
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		for ; n != nil; n = n.NextSibling {
			if id := n.GetAttribute("id"); id != "" {
				cs := resolved[n.ID]
				if cs == nil {
					t.Fatalf("no computed style for #%s", id)
				}
				out[id] = cs
			}
			walk(n.FirstChild)
		}
	}
	walk(doc.Node.FirstChild)
	return out
}

func cssWideMarginOf(cs *style.ComputedStyle) cssWideMargin {
	return cssWideMargin{cs.MarginTop, cs.MarginRight, cs.MarginBottom, cs.MarginLeft, cs.MarginLeftAuto, cs.MarginRightAuto}
}

func cssWidePaddingOf(cs *style.ComputedStyle) cssWidePadding {
	return cssWidePadding{cs.PaddingTop, cs.PaddingRight, cs.PaddingBottom, cs.PaddingLeft}
}

func cssWideInsetOf(cs *style.ComputedStyle) cssWideInset {
	return cssWideInset{cs.Top, cs.Right, cs.Bottom, cs.Left, cs.HasTop, cs.HasRight, cs.HasBottom, cs.HasLeft}
}

func TestCSSWideMarginShorthandMatchesChromium(t *testing.T) {
	styles := cssWideBoxStyles(t)
	inherited := cssWideMargin{20, 20, 20, 20, false, false}
	initial := cssWideMargin{}
	for _, c := range []struct {
		id   string
		want cssWideMargin
	}{
		{"marginParent", inherited},
		{"marginInherit", inherited},
		{"marginInitial", initial},
		{"marginUnset", initial},
	} {
		if got := cssWideMarginOf(styles[c.id]); got != c.want {
			t.Errorf("#%s margin = %+v, want Chromium's %+v", c.id, got, c.want)
		}
	}
}

func TestCSSWidePaddingShorthandMatchesChromium(t *testing.T) {
	styles := cssWideBoxStyles(t)
	inherited := cssWidePadding{1, 2, 3, 4}
	initial := cssWidePadding{}
	for _, c := range []struct {
		id   string
		want cssWidePadding
	}{
		{"paddingParent", inherited},
		{"paddingInherit", inherited},
		{"paddingInitial", initial},
		{"paddingUnset", initial},
	} {
		if got := cssWidePaddingOf(styles[c.id]); got != c.want {
			t.Errorf("#%s padding = %+v, want Chromium's %+v", c.id, got, c.want)
		}
	}
}

func TestCSSWideInsetShorthandMatchesChromium(t *testing.T) {
	styles := cssWideBoxStyles(t)
	inherited := cssWideInset{10, 20, 30, 40, true, true, true, true}
	initial := cssWideInset{}
	for _, c := range []struct {
		id   string
		want cssWideInset
	}{
		{"insetParent", inherited},
		{"insetInherit", inherited},
		{"insetInitial", initial},
		{"insetUnset", initial},
	} {
		if got := cssWideInsetOf(styles[c.id]); got != c.want {
			t.Errorf("#%s inset = %+v, want Chromium's %+v", c.id, got, c.want)
		}
	}
}

func TestCSSWideBoxShorthandsRejectMixedKeywordsLikeChromium(t *testing.T) {
	styles := cssWideBoxStyles(t)
	margin := cssWideMargin{1, 2, 3, 4, false, false}
	padding := cssWidePadding{1, 2, 3, 4}
	inset := cssWideInset{1, 2, 3, 4, true, true, true, true}
	for _, c := range []struct {
		id   string
		got  func(*style.ComputedStyle) any
		want any
	}{
		{"marginMixedInherit", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, margin},
		{"marginMixedInitial", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, margin},
		{"marginMixedUnset", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, margin},
		{"paddingMixedInherit", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, padding},
		{"paddingMixedInitial", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, padding},
		{"paddingMixedUnset", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, padding},
		{"insetMixedInherit", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, inset},
		{"insetMixedInitial", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, inset},
		{"insetMixedUnset", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, inset},
	} {
		if got := c.got(styles[c.id]); got != c.want {
			t.Errorf("#%s = %+v, want Chromium's unchanged %+v after the invalid mixed declaration", c.id, got, c.want)
		}
	}
}

func TestCSSWideBoxShorthandsKeepSourceOrderWithLonghandsLikeChromium(t *testing.T) {
	styles := cssWideBoxStyles(t)
	margin := cssWideMargin{20, 20, 20, 20, false, false}
	marginLeft := cssWideMargin{20, 20, 20, 7, false, false}
	marginInitial := cssWideMargin{}
	marginInitialLeft := cssWideMargin{Left: 7}
	padding := cssWidePadding{1, 2, 3, 4}
	paddingLeft := cssWidePadding{1, 2, 3, 7}
	paddingInitial := cssWidePadding{}
	paddingInitialLeft := cssWidePadding{Left: 7}
	inset := cssWideInset{10, 20, 30, 40, true, true, true, true}
	insetTop := cssWideInset{7, 20, 30, 40, true, true, true, true}
	insetInitial := cssWideInset{}
	insetInitialTop := cssWideInset{Top: 7, HasTop: true}
	for _, c := range []struct {
		id   string
		got  func(*style.ComputedStyle) any
		want any
	}{
		{"marginKeywordThenLonghand", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, marginLeft},
		{"marginLonghandThenKeyword", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, margin},
		{"marginKeywordThenLogical", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, marginLeft},
		{"marginLogicalThenKeyword", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, margin},
		{"marginInitialThenLonghand", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, marginInitialLeft},
		{"marginLonghandThenInitial", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, marginInitial},
		{"marginUnsetThenLonghand", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, marginInitialLeft},
		{"marginLonghandThenUnset", func(cs *style.ComputedStyle) any { return cssWideMarginOf(cs) }, marginInitial},
		{"paddingKeywordThenLonghand", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, paddingLeft},
		{"paddingLonghandThenKeyword", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, padding},
		{"paddingKeywordThenLogical", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, paddingLeft},
		{"paddingLogicalThenKeyword", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, padding},
		{"paddingInitialThenLonghand", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, paddingInitialLeft},
		{"paddingLonghandThenInitial", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, paddingInitial},
		{"paddingUnsetThenLonghand", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, paddingInitialLeft},
		{"paddingLonghandThenUnset", func(cs *style.ComputedStyle) any { return cssWidePaddingOf(cs) }, paddingInitial},
		{"insetKeywordThenLonghand", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, insetTop},
		{"insetLonghandThenKeyword", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, inset},
		{"insetKeywordThenLogical", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, insetTop},
		{"insetLogicalThenKeyword", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, inset},
		{"insetInitialThenLonghand", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, insetInitialTop},
		{"insetLonghandThenInitial", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, insetInitial},
		{"insetUnsetThenLonghand", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, insetInitialTop},
		{"insetLonghandThenUnset", func(cs *style.ComputedStyle) any { return cssWideInsetOf(cs) }, insetInitial},
	} {
		if got := c.got(styles[c.id]); got != c.want {
			t.Errorf("#%s = %+v, want Chromium's source-ordered %+v", c.id, got, c.want)
		}
	}
}

func TestCSSWideMarginInheritPreservesPercentagesAndAutoLikeChromium(t *testing.T) {
	styles := cssWideBoxStyles(t)
	inherited := cssWideMargin{-7, style.MarginAuto, -9, -13, false, true}
	initial := cssWideMargin{}
	for _, c := range []struct {
		id   string
		want cssWideMargin
	}{
		{"marginValueParent", inherited},
		{"marginValueInherit", inherited},
		{"marginValueInitial", initial},
		{"marginValueUnset", initial},
	} {
		if got := cssWideMarginOf(styles[c.id]); got != c.want {
			t.Errorf("#%s margin = %+v, want Chromium's percentage/auto-preserving %+v", c.id, got, c.want)
		}
	}
}
