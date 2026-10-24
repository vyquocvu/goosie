package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// A generated ::before/::after box is an anonymous text node in the arena, not
// an element. The inline pass used to accept only element nodes as atomic
// inline-level boxes, so an inline-block pseudo-element vanished along with its
// background. Google's header logo is exactly that construct.
func TestInlineBlockPseudoElementPaints(t *testing.T) {
	html := `<!doctype html><html><head><style>
.logo::before{content:"";display:inline-block;width:74px;height:24px;background-color:rgb(255,0,0)}
</style></head><body style="margin:0"><span class="logo"></span></body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	for _, cmd := range list.Build(1).All() {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		if cmd.Color.R() == 255 && cmd.Color.G() == 0 && cmd.Color.B() == 0 &&
			cmd.Rect.X1-cmd.Rect.X0 == 74 && cmd.Rect.Y1-cmd.Rect.Y0 == 24 {
			return
		}
	}
	t.Fatal("74x24 red fill for the inline-block ::before box not found")
}

func TestInlineBlockPseudoElementPaintsBackgroundImage(t *testing.T) {
	html := `<!doctype html><html><head><style>
.logo::before{content:"";display:inline-block;width:74px;height:24px;background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='74' height='24'%3E%3Crect width='74' height='24' fill='%2300ff00'/%3E%3C/svg%3E")}
</style></head><body style="margin:0"><span class="logo"></span></body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	for _, cmd := range list.Build(1).All() {
		if cmd.Kind == paint.CmdImage && !cmd.Rect.Empty() {
			return
		}
	}
	t.Fatal("image command for the ::before background not found")
}
