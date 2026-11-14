package engine_test

import (
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

const cjkIdeographs = "中文文本需要换行并且继续写入更多字符以填满这个宽度"

func cjkFonts(t *testing.T) *raster.Fonts {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	return fonts
}

func cjkSession(t *testing.T, fonts *raster.Fonts, body string) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(
		`<html><body style="margin: 0">`+body+`</body></html>`,
		nil, 400, engine.WithMetrics(fonts))
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func cjkLines(t *testing.T, sess *engine.Session) [][]string {
	t.Helper()
	type frag struct {
		x    int32
		text string
	}
	byY := map[int32][]frag{}
	var ys []int32
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node == nil || o.Node.Type != dom.NodeText {
			continue
		}
		y := int32(o.Y * 64)
		if _, seen := byY[y]; !seen {
			ys = append(ys, y)
		}
		byY[y] = append(byY[y], frag{x: int32(o.X * 64), text: o.Node.DataContent})
	}
	sort.Slice(ys, func(i, j int) bool { return ys[i] < ys[j] })
	out := make([][]string, 0, len(ys))
	for _, y := range ys {
		line := byY[y]
		sort.Slice(line, func(i, j int) bool { return line[i].x < line[j].x })
		texts := make([]string, 0, len(line))
		for _, f := range line {
			texts = append(texts, f.text)
		}
		out = append(out, texts)
	}
	return out
}

func cjkGlyphs(t *testing.T, sess *engine.Session) []paint.GlyphRun {
	t.Helper()
	var out []paint.GlyphRun
	for _, cmd := range sess.Paint(1).Build(1).All() {
		if cmd.Kind == paint.CmdText {
			out = append(out, cmd.Text.Glyphs...)
		}
	}
	return out
}

func lineCounts(lines [][]string) []int {
	out := make([]int, 0, len(lines))
	for _, l := range lines {
		out = append(out, len(l))
	}
	return out
}

// qc:BR-087
func TestCjkIdeographsBreakAfterEachRune(t *testing.T) {
	fonts := cjkFonts(t)
	sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">`+cjkIdeographs+`</div>`)
	lines := cjkLines(t, sess)
	if got := lineCounts(lines); len(got) != 3 || got[0] != 12 || got[1] != 12 || got[2] != 1 {
		t.Fatalf("CJK line structure = %v, want [12 12 1] (Chromium: 12, 12, 1 in a 200px box)", got)
	}
	if strings.Join(lines[0], "") != string([]rune(cjkIdeographs)[:12]) {
		t.Errorf("line 0 = %q, want the first 12 ideographs", strings.Join(lines[0], ""))
	}
	if n := len(cjkGlyphs(t, sess)); n != 25 {
		t.Errorf("painted glyphs = %d, want 25: wrapping must not drop characters", n)
	}
}

// qc:BR-087
func TestCjkKeepAllSuppressesBreakOpportunities(t *testing.T) {
	fonts := cjkFonts(t)
	sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px; word-break: keep-all">`+cjkIdeographs+`</div>`)
	lines := cjkLines(t, sess)
	if len(lines) != 1 {
		t.Fatalf("keep-all line count = %d, want 1 (Chromium: one 408.016px line)", len(lines))
	}
	if strings.Join(lines[0], "") != cjkIdeographs {
		t.Errorf("keep-all line 0 = %q, want the whole run unbroken", strings.Join(lines[0], ""))
	}
}

// qc:BR-087
func TestBreakAfterIdeographBeforeLatinWord(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">中` + strings.Repeat("a", 57) + `</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2 (Chromium: 中 on line 0, the Latin run on line 1)", len(lines))
	}
	if got := strings.Join(lines[0], ""); got != "中" {
		t.Errorf("line 0 = %q, want \"中\"", got)
	}
	if got := strings.Join(lines[1], ""); got != strings.Repeat("a", 57) {
		t.Errorf("line 1 = %q, want the 57-letter run unbroken", got)
	}
}

// qc:BR-087
func TestNoBreakBeforeIdeographFromLatinSide(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">` + strings.Repeat("a", 57) + `中</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1 (Chromium: one 507.219px overflowing line)", len(lines))
	}
}

// qc:BR-087
func TestCombiningMarkNeverLeadsABreakFragment(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">中` + "\u0301" + strings.Repeat("a", 57) + `</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	if len(lines) < 2 {
		t.Fatalf("line count = %d, want the run to break", len(lines))
	}
	for _, line := range lines {
		for _, frag := range line {
			for _, r := range frag {
				if unicode.IsMark(r) {
					t.Errorf("fragment %q begins with U+%04X: a mark must stay with its base", frag, r)
				}
				break
			}
		}
	}
	if got := strings.Join(lines[0], ""); got != "\u4e2d\u0301" {
		t.Errorf("line 0 = %q, want the ideograph and its mark kept together", got)
	}
}

// qc:BR-087
func TestKanaAndHangulBreakBetweenRunes(t *testing.T) {
	fonts := cjkFonts(t)
	t.Run("kana", func(t *testing.T) {
		const kana = "カタカナの文章がここで折り返されるかどうかを確かめます"
		sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">`+kana+`</div>`)
		lines := cjkLines(t, sess)
		if len(lines) != 3 {
			t.Fatalf("kana line count = %d, want 3 (Chromium: 12, 12, 3 in a 200px box)", len(lines))
		}
		got := 0
		for _, l := range lines {
			got += len([]rune(strings.Join(l, "")))
		}
		if got != 27 {
			t.Errorf("laid-out kana runes = %d, want 27", got)
		}
		if n := len(cjkGlyphs(t, sess)); n != 27 {
			t.Errorf("painted glyphs = %d, want 27", n)
		}
	})
	t.Run("hangul", func(t *testing.T) {
		const hangul = "한국어텍스트가이 상자안에서 줄바꿈되는지 확인합니다"
		sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">`+hangul+`</div>`)
		lines := cjkLines(t, sess)
		if len(lines) < 2 {
			t.Fatalf("hangul line count = %d, want > 1 (Chromium: 2 lines, breaks between syllables)", len(lines))
		}
		want := 0
		for _, r := range hangul {
			if r >= 0xAC00 && r <= 0xD7AF {
				want++
			}
		}
		got := 0
		for i := range sess.Arena.Objects {
			o := &sess.Arena.Objects[i]
			if o.Node == nil || o.Node.Type != dom.NodeText {
				continue
			}
			for _, r := range o.Node.DataContent {
				if r >= 0xAC00 && r <= 0xD7AF {
					got++
				}
			}
		}
		if got != want {
			t.Errorf("laid-out Hangul syllables = %d, want %d: wrapping must not drop characters", got, want)
		}
	})
}

// qc:BR-087
func TestLongUnbrokenLatinWordKeepsEveryGlyph(t *testing.T) {
	fonts := cjkFonts(t)
	sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">`+strings.Repeat("a", 200)+`</div>`)
	lines := cjkLines(t, sess)
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1 (Chromium: one overflowing line, unbroken)", len(lines))
	}
	if n := len(cjkGlyphs(t, sess)); n != 200 {
		t.Errorf("painted glyphs = %d, want 200", n)
	}
}

// qc:BR-087
func TestCombiningMarksAddNoAdvanceButKeepInk(t *testing.T) {
	fonts := cjkFonts(t)
	slot := frame.FontSlot{Family: frame.FontTimes}
	precomposed := sessWidth(t, fonts, "ế")
	decomposed := sessWidth(t, fonts, "e\u0302\u0301")
	if precomposed != decomposed {
		t.Errorf("width(NFC)=%v width(NFD)=%v, want equal: marks must contribute no advance", precomposed, decomposed)
	}
	if lone := sessWidth(t, fonts, "\u0301"); lone != 0 {
		t.Errorf("a lone combining mark measures %v, want 0 (Chromium: advance 0)", lone)
	}
	if adv := fonts.GlyphAdvanceFixed(16, '\u0302', slot); adv != 0 {
		t.Errorf("U+0302 advance = %d, want 0", adv)
	}
	if g := fonts.Glyph(16, '\u0302', slot); g.Bounds.X1 <= g.Bounds.X0 || g.Bounds.Y1 <= g.Bounds.Y0 {
		t.Errorf("U+0302 bounds = %+v, want real accent ink (bounds with area)", g.Bounds)
	}
}

// qc:BR-087
func TestKinsokuFullStopDoesNotLeadLine(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">` +
		`文本文本文本文本文本文本。文本</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	if len(lines) < 2 {
		t.Fatalf("line count = %d, want >= 2", len(lines))
	}
	for i, line := range lines {
		for _, frag := range line {
			if len(frag) > 0 && firstRune(frag) == '。' {
				t.Errorf("line %d begins with 。 (fragment %q): kinsoku forbids it", i, frag)
			}
		}
	}
	if got := strings.Join(lines[0], ""); len([]rune(got)) != 11 {
		t.Errorf("line 0 has %d runes, want 11 (Chromium: 11 ideographs)", len([]rune(got)))
	}
	if got := strings.Join(lines[1], ""); got != "本。文本" {
		t.Errorf("line 1 = %q, want \"本。文本\" (Chromium: stop rides with the previous ideograph)", got)
	}
}

// qc:BR-087
func TestKinsokuCommaDoesNotLeadLine(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">` +
		`文本文本文本文本文本文本、文本</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	for i, line := range lines {
		for _, frag := range line {
			if len(frag) > 0 && firstRune(frag) == '、' {
				t.Errorf("line %d begins with 、 (fragment %q): kinsoku forbids it", i, frag)
			}
		}
	}
}

// qc:BR-087
func TestKinsokuProlongedSoundDoesNotLeadLine(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">` +
		`カタカナカタカナカタカナカタカナカタカナカタカーテキスト</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	for i, line := range lines {
		for _, frag := range line {
			if len(frag) > 0 && firstRune(frag) == 'ー' {
				t.Errorf("line %d begins with ー (fragment %q): kinsoku forbids it", i, frag)
			}
		}
	}
}

// qc:BR-087
func TestKinsokuDoesNotSuppressOrdinaryBreaks(t *testing.T) {
	fonts := cjkFonts(t)
	sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">`+cjkIdeographs+`</div>`)
	lines := cjkLines(t, sess)
	if got := lineCounts(lines); len(got) != 3 || got[0] != 12 || got[1] != 12 || got[2] != 1 {
		t.Fatalf("CJK line structure = %v, want [12 12 1]: kinsoku must not disturb ordinary breaks", got)
	}
}

// qc:BR-087
func TestKinsokuNoForbiddenCharacterLeadsAnyLine(t *testing.T) {
	fonts := cjkFonts(t)
	body := `<div style="width: 200px; font-size: 16px">` +
		`文本文本文本文本文本文本。、ー文本文本文本文本文本文本。文本</div>`
	sess := cjkSession(t, fonts, body)
	lines := cjkLines(t, sess)
	forbidden := map[rune]bool{'。': true, '、': true, 'ー': true}
	for i, line := range lines {
		for _, frag := range line {
			if r := firstRune(frag); forbidden[r] {
				t.Errorf("line %d begins with U+%04X (fragment %q): kinsoku forbids it", i, r, frag)
			}
		}
	}
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func sessWidth(t *testing.T, fonts *raster.Fonts, text string) float32 {
	t.Helper()
	sess := cjkSession(t, fonts, `<p>`+text+`</p>`)
	var total float32
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node == nil || o.Node.Type != dom.NodeText {
			continue
		}
		total += o.W
	}
	return total
}
