package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

const urlNumberCSS = `
.bg { background-image: url(/w/images/sort_both.svg?0e440); }
.outer .inner { position: absolute; top: 0; left: 0; width: 10px; height: 10px; }`

const urlNumberHTML = `<html><head><link rel="stylesheet" href="/s.css"></head><body>
<div class="outer" style="position: relative;"><div class="inner">x</div><div class="after">y</div></div>
</body></html>`

func absposApplied(t *testing.T, sess *engine.Session) {
	t.Helper()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	var ys []int32
	for _, c := range list.Build(1).All() {
		if c.Kind.String() == "text" {
			ys = append(ys, c.Rect.Y0)
		}
	}
	if len(ys) != 2 {
		t.Fatalf("want 2 text runs (x, y), got %d at %v", len(ys), ys)
	}
	if ys[1] != 9 {
		t.Fatalf("second text at y=%d, want 9 (abspos .inner takes no slot)", ys[1])
	}
}

func TestInlineSheetWithURLNumberSurvives(t *testing.T) {
	html := `<html><head><style>` + urlNumberCSS + `</style></head><body>
<div class="outer" style="position: relative;"><div class="inner">x</div><div class="after">y</div></div>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("sheet with url() number must not fail the document: %v", err)
	}
	defer sess.Close()
	absposApplied(t, sess)
}

func TestRemoteSheetWithURLNumberSurvives(t *testing.T) {
	linker := func(base, href string) (string, error) {
		return urlNumberCSS, nil
	}
	sess, err := engine.NewSession(urlNumberHTML, nil, 800,
		engine.WithLinkedCSS("https://example.test/", linker))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	absposApplied(t, sess)
}
