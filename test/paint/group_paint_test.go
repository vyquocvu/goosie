package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func gpLayer(t *testing.T, html string) *paint.LayerDL {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	list, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return list.Build(1).Publish()
}

func TestCleanFadedBoxRecordsOneGroup(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.g{opacity:0.5;width:100px;height:50px;background:rgb(200,0,0)}
	</style></head><body><div class="g"></div></body></html>`
	dl := gpLayer(t, html)
	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1 (one clean faded declaration)", len(gs))
	}
	if gs[0].OwnOpacity != 0.5 {
		t.Errorf("group OwnOpacity = %v, want 0.5", gs[0].OwnOpacity)
	}
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Opacity > 0 && c.Opacity < 1 {
			t.Errorf("member fill carried a partial per-command opacity %v, want opaque (the group fade is withheld; applied once at composite)", c.Opacity)
		}
	}
}

func TestFadedBoxWithZDescendantIsNotGrouped(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.g{opacity:0.5;width:120px;height:80px;background:rgb(200,0,0)}
	.k{position:relative;z-index:3;width:40px;height:20px;background:rgb(0,0,200)}
	</style></head><body><div class="g"><div class="k"></div></div></body></html>`
	dl := gpLayer(t, html)
	if len(dl.Groups()) != 0 {
		t.Fatalf("Groups() = %d, want 0 (a z-lifted descendant makes the box non-clean)", len(dl.Groups()))
	}
	saw := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Opacity == 0.5 {
			saw = true
		}
	}
	if !saw {
		t.Errorf("fallback did not thread opacity per-command: no fill carried 0.5")
	}
}

func TestTwoChildrenOneContainerGroup(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.c{opacity:0.5;width:120px;height:60px}
	.a{position:absolute;left:0;top:0;width:60px;height:60px;background:rgb(0,0,255)}
	.b{position:absolute;left:30px;top:0;width:60px;height:60px;background:rgb(0,0,255)}
	</style></head><body><div class="c"><div class="a"></div><div class="b"></div></div></body></html>`
	dl := gpLayer(t, html)
	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1 (the container only; the children have no opacity of their own)", len(gs))
	}
}

func TestNestedFadedChildSharingHeadFadesOnce(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	</style></head><body><div style="opacity:0.5"><div style="opacity:0.5;background:rgb(0,0,255);width:60px;height:60px"></div></div></body></html>`
	dl := gpLayer(t, html)
	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1 (the container; the head-sharing child is folded in)", len(gs))
	}
	if gs[0].OwnOpacity != 0.5 {
		t.Errorf("container group OwnOpacity = %v, want 0.5", gs[0].OwnOpacity)
	}
	saw := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill {
			saw = true
			if c.Opacity != 0.5 {
				t.Errorf("head-sharing child fill carried opacity %v, want 0.5 (its withheld group fade must fold back per-command, not vanish)", c.Opacity)
			}
		}
	}
	if !saw {
		t.Fatal("no fill command found to check")
	}
}
