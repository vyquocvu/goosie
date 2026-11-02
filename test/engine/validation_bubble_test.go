package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

func paintedText(list *paint.List) string {
	var b strings.Builder
	for _, cmd := range list.All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		for _, g := range cmd.Text.Glyphs {
			b.WriteRune(g.Rune)
		}
	}
	return b.String()
}

func validationBubbleCommand(list *paint.List, message string) (paint.DisplayCmd, bool) {
	for _, cmd := range list.All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		var b strings.Builder
		for _, g := range cmd.Text.Glyphs {
			b.WriteRune(g.Rune)
		}
		if b.String() == message {
			return cmd, true
		}
	}
	return paint.DisplayCmd{}, false
}

func findAXNode(nodes []ax.Node, id string) *ax.Node {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
		if found := findAXNode(nodes[i].Children, id); found != nil {
			return found
		}
	}
	return nil
}

// qc:BR-105
func TestBlockedSubmitPaintsTheConstraintMessage(t *testing.T) {
	cases := []struct {
		name    string
		control string
		want    string
	}{
		{"required text", `<input name="q" required>`, "Please fill out this field."},
		{"required checkbox", `<input type="checkbox" name="c" required>`, "Please check this box if you want to proceed."},
		{"required radio", `<input type="radio" name="r" required>`, "Please select one of these options."},
		{"required select", `<select name="s" required><option value="" selected>Pick</option><option value="a">A</option></select>`, "Please select an item in the list."},
		{"required file", `<input type="file" name="f" required>`, "Please select a file."},
		{"email", `<input type="email" name="e" value="nope">`, "Please include an '@' in the email address."},
		{"number", `<input type="number" name="n" value="abc">`, "Please enter a number."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newFormSession(t, `<form id="f" action="/send">`+tc.control+`</form>`)
			s.SubmitForm(s.Doc.ElementByID("f"), nil)
			assertNoForm(t, s)
			if got := paintedText(s.Paint(1)); !strings.Contains(got, tc.want) {
				t.Fatalf("painted text %q does not contain %q", got, tc.want)
			}
		})
	}
}

func TestValidationBubbleAnchorsToTheFirstInvalidControl(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send">
		<input id="empty" name="empty" required>
		<input id="mail" name="mail" type="email" value="nope">
	</form>`)
	first := s.Doc.ElementByID("empty")
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	assertNoForm(t, s)
	if s.Focused() != first {
		t.Fatal("the blocked submission should focus the first invalid control")
	}
	list := s.Paint(1)
	if !strings.Contains(paintedText(list), "Please fill out this field.") {
		t.Fatalf("first control's message missing from painted text %q", paintedText(list))
	}
	if strings.Contains(paintedText(list), "email address") {
		t.Fatal("only the first invalid control's message should be shown")
	}
	cmd, ok := validationBubbleCommand(list, "Please fill out this field.")
	if !ok {
		t.Fatal("bubble text command not found")
	}
	bounds, _ := cmd.Bounds()
	obj := objectForNode(s, first)
	if obj == nil {
		t.Fatal("first invalid control has no layout box")
	}
	x0, y0, x1, y1 := obj.BorderRect()
	if bounds.X0 == 0 && bounds.Y0 == 0 {
		t.Fatal("bubble should be positioned, not left at the origin")
	}
	if cmd.Rect.X1 < int32(x0) || cmd.Rect.X0 > int32(x1) {
		t.Fatalf("bubble span %v..%v does not overlap control %v..%v", cmd.Rect.X0, cmd.Rect.X1, x0, x1)
	}
	if !(bounds.Y1 <= int32(y0) || bounds.Y0 >= int32(y1)) {
		t.Fatalf("bubble bounds %v should sit outside control [%v,%v]", bounds, y0, y1)
	}
}

func objectForNode(s *engine.Session, n *dom.Node) *layout.Object {
	if s.Arena == nil {
		return nil
	}
	for i := range s.Arena.Objects {
		if s.Arena.Objects[i].Node == n {
			return &s.Arena.Objects[i]
		}
	}
	return nil
}

func TestValidationBubbleClearsOnUserInteraction(t *testing.T) {
	t.Run("typing", func(t *testing.T) {
		s := newFormSession(t, `<form id="f" action="/send"><input name="q" required></form>`)
		s.SubmitForm(s.Doc.ElementByID("f"), nil)
		assertNoForm(t, s)
		if !strings.Contains(paintedText(s.Paint(1)), "Please fill out this field.") {
			t.Fatal("bubble should be painted after the blocked submission")
		}
		if !s.Edit(engine.EditRune, 'x') {
			t.Fatal("typing into the focused control should succeed")
		}
		if got := paintedText(s.Paint(1)); strings.Contains(got, "Please fill out this field.") {
			t.Fatalf("typing should dismiss the bubble, painted %q", got)
		}
	})

	t.Run("click", func(t *testing.T) {
		s := newFormSession(t, `<form id="f" action="/send"><input name="q" required></form>`)
		s.SubmitForm(s.Doc.ElementByID("f"), nil)
		assertNoForm(t, s)
		s.ClickAt(0, 500)
		if got := paintedText(s.Paint(1)); strings.Contains(got, "Please fill out this field.") {
			t.Fatalf("a click should dismiss the bubble, painted %q", got)
		}
	})

	t.Run("successful submit", func(t *testing.T) {
		s := newFormSession(t, `<form id="f" action="/send"><input id="q" name="q" required></form>`)
		form := s.Doc.ElementByID("f")
		s.SubmitForm(form, nil)
		assertNoForm(t, s)
		s.Doc.ElementByID("q").SetAttribute("value", "ready")
		s.SubmitForm(form, nil)
		if _, ok := s.PopSubmittedForm(); !ok {
			t.Fatal("the corrected form should submit")
		}
		if got := paintedText(s.Paint(1)); strings.Contains(got, "Please fill out this field.") {
			t.Fatalf("a successful submission should dismiss the bubble, painted %q", got)
		}
	})
}

func TestValidationBubbleIsExposedToAccessibility(t *testing.T) {
	s := newFormSession(t, `<form id="f" action="/send"><input id="q" name="q" required></form>`)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	assertNoForm(t, s)
	node := findAXNode(s.AccessibilityTree(), "goosie.validation-message")
	if node == nil {
		t.Fatal("the validation message should appear in the accessibility tree")
	}
	if node.Label != "Please fill out this field." {
		t.Fatalf("AX label = %q, want the constraint message", node.Label)
	}
	if node.DOMNodeID != uint64(s.Doc.ElementByID("q").ID) {
		t.Fatalf("AX message should be anchored to the invalid control, DOMNodeID=%d", node.DOMNodeID)
	}
}
