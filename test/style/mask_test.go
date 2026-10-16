package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func styleForClass(t *testing.T, doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle, cls string) *style.ComputedStyle {
	t.Helper()
	nodes := doc.ElementsByClassName(cls)
	if len(nodes) == 0 {
		t.Fatalf("no .%s node", cls)
	}
	st, ok := styles[nodes[0].ID]
	if !ok || st == nil {
		t.Fatalf("no style for .%s", cls)
	}
	return st
}

func TestMaskImageParse(t *testing.T) {
	doc := domtest.Parse(`<html><body><div class="icon">x</div></body></html>`)
	sheet := css.ParseForViewport(`.icon{mask-image:url(menu.svg);background-color:#202122;}`, 800)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	st := styleForClass(t, doc, styles, "icon")
	if st.MaskImage != "menu.svg" {
		t.Errorf("MaskImage = %q, want menu.svg", st.MaskImage)
	}
}

func TestWebkitMaskImageAlias(t *testing.T) {
	doc := domtest.Parse(`<html><body><div class="icon">x</div></body></html>`)
	sheet := css.ParseForViewport(`.icon{-webkit-mask-image:url(menu.svg);mask-image:url(other.svg);}`, 800)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	st := styleForClass(t, doc, styles, "icon")
	// Both spellings feed one field in cascade order; the later rule wins.
	if st.MaskImage != "other.svg" {
		t.Errorf("MaskImage = %q, want other.svg", st.MaskImage)
	}
}

func TestMaskImageNone(t *testing.T) {
	doc := domtest.Parse(`<html><body><div class="icon">x</div></body></html>`)
	sheet := css.ParseForViewport(`.icon{mask-image:none;}`, 800)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	st := styleForClass(t, doc, styles, "icon")
	if st.MaskImage != "" {
		t.Errorf("MaskImage = %q, want empty", st.MaskImage)
	}
}
