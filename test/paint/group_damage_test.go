package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// Two identical command lists whose only difference is a group's OWN opacity must
// still report damage over the group's range, because OwnOpacity lives outside
// the command fields and sameCommand never sees it.
func TestContentDamageGroupOpacityOnlyChange(t *testing.T) {
	before := paint.NewList(2)
	before.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: frame.RGB(0, 0, 255)})
	before.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: frame.RGB(0, 0, 255)})
	before.AddGroup(0.5, 2)

	after := paint.NewList(2)
	after.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: frame.RGB(0, 0, 255)})
	after.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: frame.RGB(0, 0, 255)})
	after.AddGroup(0.25, 2)

	bo, ao := before.Build(1), after.Build(1)
	got := paint.ContentDamage(bo, ao)
	want := frame.Rect4(10, 10, 100, 70)
	if got != want {
		t.Fatalf("damage = %v, want the group's command-range union %v (opacity-only change)", got, want)
	}
}
