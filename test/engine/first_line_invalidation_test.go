package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestFirstLineMetricChangeRebuildsLayout(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)
	sess, err := engine.NewSession(`<html><head><style>
		#target { width: 150px; font: 16px/20px Arial, sans-serif; }
		#target::first-line { font-size: 16px; }
		#target.hot::first-line { font-size: 32px; }
	</style></head><body><p id="target">Alpha beta gamma delta</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	target := sess.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("target not found")
	}
	key := style.PseudoKey{NodeID: target.ID, Pseudo: "first-line"}
	if got := sess.PseudoStyles[key]; got == nil || got.FontSize != 16 {
		t.Fatalf("initial first-line style = %+v, want font-size 16", got)
	}
	before := sess.WorkCounts()
	target.SetAttribute("class", "hot")
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	if got := sess.WorkCounts().LayoutBuilds; got != before.LayoutBuilds+1 {
		t.Fatalf("layout builds = %d, want %d", got, before.LayoutBuilds+1)
	}
	if got := sess.PseudoStyles[key]; got == nil || got.FontSize != 32 {
		t.Fatalf("updated first-line style = %+v, want font-size 32", got)
	}
}
