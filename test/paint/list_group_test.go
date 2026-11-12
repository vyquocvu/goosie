package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func lgFill(x0, y0, x1, y1 int32, z uint32) paint.DisplayCmd {
	return paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(x0, y0, x1, y1), Color: frame.RGB(0, 0, 255), Z: z}
}

// A clean group recorded over the last two commands survives a z-sort that moves
// a higher-Z sibling ahead of it: the group's Start/End are recomputed from the
// sorted list, so they describe where the members actually landed.
func TestGroupRangeSurvivesSort(t *testing.T) {
	l := paint.NewList(4)
	// Append a lifted sibling FIRST so the sort reorders the slab, then the two
	// group members, then register the group over the trailing two commands.
	l.Append(lgFill(0, 0, 10, 10, 5))
	l.Append(lgFill(10, 0, 30, 10, 0))
	l.Append(lgFill(12, 0, 28, 10, 0))
	l.AddGroup(0.5, 2)
	l.SortStable()
	dl := l.Build(1).Publish()

	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1", len(gs))
	}
	g := gs[0]
	if g.Start != 0 || g.End != 2 {
		t.Errorf("group range = [%d,%d), want [0,2): the two Z=0 members sort to the front", g.Start, g.End)
	}
	if g.OwnOpacity != 0.5 {
		t.Errorf("OwnOpacity = %v, want 0.5", g.OwnOpacity)
	}
	if _, ok := dl.GroupAt(g.Start); !ok {
		t.Errorf("GroupAt(%d) = false, want the group starting there", g.Start)
	}
	if _, ok := dl.GroupAt(g.End); ok {
		t.Errorf("GroupAt(%d) = true, want false just past the group", g.End)
	}
}

// A group is only what AddGroup declared: an opacity-less list has none.
func TestNoGroupsDeclaredNoneRecorded(t *testing.T) {
	l := paint.NewList(2)
	l.Append(lgFill(0, 0, 10, 10, 0))
	l.Append(lgFill(10, 0, 20, 10, 0))
	dl := l.Build(1).Publish()
	if len(dl.Groups()) != 0 {
		t.Fatalf("Groups() = %d, want 0", len(dl.Groups()))
	}
	if _, ok := dl.GroupAt(0); ok {
		t.Errorf("GroupAt(0) = true on a group-less list")
	}
}
