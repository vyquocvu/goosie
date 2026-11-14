package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

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
