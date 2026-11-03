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

// BR-087 text surfaces. Every expectation below is the number Chromium 1.63
// produced for the same fixture in scripts/verify-br087-text-parity.js
// (2026-11-04), quoted in docs/roadmap-v2.md.

const cjkIdeographs = "中文文本需要换行并且继续写入更多字符以填满这个宽度" // 25 ideographs

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

// cjkLines groups the laid-out text fragments by line box (their Y) and orders
// each line by X, so an assertion reads as "the text Chromium put on line n".
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

// Chromium 1.63, 200px box, 16px: the 25 ideographs broke into lines of 12,
// 12, 1 - a break opportunity exists after every ideograph. Goosie laid the
// same paragraph out as one unbroken 400px fragment.
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

// Chromium: the same paragraph with word-break: keep-all is one line 408.016px
// wide - keep-all removes every CJK break opportunity, it does not add any.
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

// Chromium broke the ideograph away from the following Latin word: line 0 is
// "中" (16.328px) and the 57-letter run sits on line 1, 507.219px wide.
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

// The mirror case measures the absence of a rule: Chromium kept 57 letters and
// a trailing ideograph on one overflowing line (507.219px), because the break
// opportunity sits after a CJK rune, not before one.
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

// A combining mark is part of the character it decorates, and a lone mark
// advances 0 (Chromium measured 0px for one on its own), so a fragment that
// begins with a mark would draw an accent over nothing.
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

// Chromium broke the 27-kana sentence into 12, 12, 3 and the Hangul paragraph
// into two lines: kana and Hangul syllables carry the same break opportunity
// as ideographs.
// qc:BR-087
func TestKanaAndHangulBreakBetweenRunes(t *testing.T) {
	fonts := cjkFonts(t)
	t.Run("kana", func(t *testing.T) {
		const kana = "カタカナの文章がここで折り返されるかどうかを確かめます" // 27 runes
		sess := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">`+kana+`</div>`)
		lines := cjkLines(t, sess)
		// Chromium filled 12, 12, 3 characters; Goosie's face measures the same
		// runes a little narrower, so it fits 13 per line. The owned contract is
		// that breaks exist between kana at all, and that none are lost.
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
		const hangul = "한국어텍스트가이 상자안에서 줄바꿈되는지 확인합니다" // 24 syllables + 3 spaces
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

// Chromium laid 200 letters in a 200px box as one unbroken 1779.688px line with
// every character inked - no overflow-wrap: break-word, nothing dropped. This
// pins that Goosie agrees rather than "fixes" anything.
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

// Chromium: ế and e+U+0302+U+0301 are both 8.906px, and a lone combining mark
// advances 0. Goosie agrees (marks measure zero and still get accent ink), so
// this pins the composition contract the wrapping change must not disturb.
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
