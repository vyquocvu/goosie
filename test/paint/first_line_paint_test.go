package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestFirstLineColorReachesPaint(t *testing.T) {
	cmds := controlCommands(t, `<style>
		p { width: 40px; font-size: 16px; color: black; }
		p::first-line { color: #ff0000; font-weight: bold; }
	</style><p>Hello world again</p>`)
	var red, black int
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdText || len(cmd.Text.Glyphs) == 0 {
			continue
		}
		switch cmd.Text.Color {
		case frame.RGB(255, 0, 0):
			red += len(cmd.Text.Glyphs)
		case frame.RGB(0, 0, 0):
			black += len(cmd.Text.Glyphs)
		}
	}
	if red == 0 {
		t.Fatal("::first-line painted no red glyphs")
	}
	if black == 0 {
		t.Fatal("a later line painted no black glyphs")
	}
}
