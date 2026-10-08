package engine_test

import (
	"os"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestDbgClipBodyDL(t *testing.T) {
	raw, _ := os.ReadFile("/tmp/goosie-wpt/css/css-backgrounds/background-clip/clip-border-area-on-body-propagated-to-root.html")
	s, err := engine.NewSession(string(raw), nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("canvas=%+v", s.BackgroundColor())
	dl, err := s.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range dl.All() {
		if cmd.Kind == paint.CmdFill {
			t.Logf("fill rect=%v r=%d g=%d b=%d a=%d", cmd.Rect, cmd.Color.R(), cmd.Color.G(), cmd.Color.B(), cmd.Color.A())
		} else {
			t.Logf("cmd kind=%v rect=%v", cmd.Kind, cmd.Rect)
		}
	}
}
