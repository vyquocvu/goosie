package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// textObject finds the laid-out word object whose content is exactly text.
func textObject(arena *layout.Arena, text string) *layout.Object {
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Node != nil && o.Node.Text() && o.Node.DataContent == text {
			return o
		}
	}
	return nil
}

func TestRTLWordPlacement(t *testing.T) {
	// "First" and "Second" are both LTR stretches: UAX #9 keeps a run of
	// Latin words reading left-to-right even inside an RTL paragraph, so
	// the bidi-aware placement keeps their logical order on screen.
	arena := session(t, `<html><body style="margin: 0;">
<div id="target" style="width: 800px; direction: rtl; font-size: 16px;">
  <span>First</span>
  <span>Second</span>
</div>
</body></html>`, 800)

	first := textObject(arena, "First")
	second := textObject(arena, "Second")
	if first == nil || second == nil {
		t.Fatalf("first=%v second=%v text objects not found", first, second)
	}

	if first.X >= second.X {
		t.Errorf("expected first word (%v) to be to the left of second word (%v): LTR runs keep logical order in RTL layout", first.X, second.X)
	}
}

func TestRTLHebrewWordPlacement(t *testing.T) {
	// Pure Hebrew words form one level-1 stretch: the first logical word is
	// placed rightmost, the way an RTL line reads.
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 800px; direction: rtl; font-size: 16px;">ראשון שני</div>
</body></html>`, 800)

	first := textObject(arena, "ראשון")
	second := textObject(arena, "שני")
	if first == nil || second == nil {
		t.Fatalf("hebrew word objects not found")
	}

	if first.X <= second.X {
		t.Errorf("expected ראשון (%v) to the right of שני (%v) in RTL layout", first.X, second.X)
	}
}

// In an RTL line the logically first word is the rightmost one, whatever
// script it is written in. Measured (scripts/verify-br087-bidi.js
// ad_latin_logical_first, fixture box x=600..800):
//
//	visual ="םולש hello"
//	LA "hello" x=[766.188 775.078 783.984 787.531 791.094] ascending=true
//	HE "שלום"  x=[750.625 743.219 739.281 729.703] descending=true
//
// The old version of this test claimed the Hebrew went rightmost because it is
// the RTL stretch; the reference reverses the whole line's run order, so being
// RTL only decides which way the word itself reads.
// qc:BR-087
func TestBidiMixedPlacementRTL(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 800px; direction: rtl; font-size: 16px;">hello שלום</div>
</body></html>`, 800)

	latin := textObject(arena, "hello")
	hebrew := textObject(arena, "שלום")
	if latin == nil || hebrew == nil {
		t.Fatalf("mixed-run text objects not found")
	}

	if latin.X <= hebrew.X {
		t.Errorf("expected hello (%v) placed rightmost, to the right of שלום (%v): the logically first word of an RTL line is the rightmost", latin.X, hebrew.X)
	}
}

// "abc 123 שלום" in an RTL line gives visual ="םולש abc 123" (measured in
// scripts/verify-br087-bidi.js fixture ae_latin_digits_logical_first, box
// x=600..800): the Latin and the digits are one ascending island, and being
// logically first it is the rightmost thing on the line - abc=743.063
// 1=773.297 3=791.094 - while the Hebrew sits to its left reading reversed
// (ש=727.5 … ם=706.578).
// qc:BR-087
func TestBidiDigitsStayLTRInRTL(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 800px; direction: rtl; font-size: 16px;">abc 123 שלום</div>
</body></html>`, 800)

	abc := textObject(arena, "abc")
	digits := textObject(arena, "123")
	hebrew := textObject(arena, "שלום")
	if abc == nil || digits == nil || hebrew == nil {
		t.Fatalf("digit-run text objects not found")
	}

	if digits.X <= abc.X {
		t.Errorf("expected digits (%v) to the right of abc (%v): LTR runs keep logical order", digits.X, abc.X)
	}
	if hebrew.X >= abc.X {
		t.Errorf("expected שלום (%v) to the LEFT of the Latin island starting at abc (%v): the logically first run is rightmost", hebrew.X, abc.X)
	}
}

func TestBidiPlaintextResolvesParagraphLevel(t *testing.T) {
	// unicode-bidi: plaintext takes the paragraph level from the first
	// strong character: the line starts with "hello", so the RTL-declared
	// element resolves to an LTR line and places runs in logical order.
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 800px; direction: rtl; unicode-bidi: plaintext; font-size: 16px;">hello שלום</div>
</body></html>`, 800)

	latin := textObject(arena, "hello")
	hebrew := textObject(arena, "שלום")
	if latin == nil || hebrew == nil {
		t.Fatalf("plaintext text objects not found")
	}

	if latin.X >= hebrew.X {
		t.Errorf("expected hello (%v) left of שלום (%v): plaintext resolves the paragraph to LTR", latin.X, hebrew.X)
	}
}
