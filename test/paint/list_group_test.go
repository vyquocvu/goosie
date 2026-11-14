package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func lgFill(x0, y0, x1, y1 int32, z uint32) paint.DisplayCmd {
	return paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(x0, y0, x1, y1), Color: frame.RGB(0, 0, 255), Z: z}
}

func TestGroupRangeSurvivesSort(t *testing.T) {
	l := paint.NewList(4)
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
