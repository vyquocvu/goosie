package paint_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func imageInputCommands(t *testing.T, html string, opts ...engine.Option) []paint.DisplayCmd {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800, opts...)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return list.All()
}

func commandText(cmd paint.DisplayCmd) string {
	if cmd.Kind != paint.CmdText {
		return ""
	}
	runes := make([]rune, 0, len(cmd.Text.Glyphs))
	for _, glyph := range cmd.Text.Glyphs {
		runes = append(runes, glyph.Rune)
	}
	return string(runes)
}

func imageInputIconFills(commands []paint.DisplayCmd, objX, objY, objW, objH float32) int {
	icon := frame.Rect4(int32(objX), int32(objY), int32(objX+16), int32(objY+16))
	count := 0
	for _, cmd := range commands {
		if cmd.Kind != paint.CmdFill || cmd.Color.A() == 0 || cmd.Rect.Empty() {
			continue
		}
		if cmd.Rect.X0 >= icon.X0 && cmd.Rect.Y0 >= icon.Y0 && cmd.Rect.X1 <= icon.X1 && cmd.Rect.Y1 <= icon.Y1 &&
			cmd.Rect.X0 >= int32(objX) && cmd.Rect.X1 <= int32(objX+objW) && cmd.Rect.Y1 <= int32(objY+objH) {
			count++
		}
	}
	return count
}

func TestImageInputValidPixelsPaintImageAndObjectFit(t *testing.T) {
	src := "data:image/svg+xml," + url.PathEscape(`<svg xmlns="http://www.w3.org/2000/svg" width="60" height="30"><rect width="60" height="30" fill="rgb(200,30,120)"/></svg>`)
	t.Run("natural", func(t *testing.T) {
		commands := imageInputCommands(t, `<!doctype html><html><body style="margin:0"><input id="pic" type="image" src="`+src+`" alt="Go"></body></html>`)
		images, texts := 0, 0
		for _, cmd := range commands {
			switch cmd.Kind {
			case paint.CmdImage:
				images++
				if cmd.Image.Src == nil || cmd.Image.Src.Bounds().Dx() != 60 || cmd.Image.Src.Bounds().Dy() != 30 {
					t.Errorf("image source = %v, want 60x30 pixels", cmd.Image.Src)
				}
				if cmd.Rect.W() != 60 || cmd.Rect.H() != 30 {
					t.Errorf("image rect = %v, want 60x30", cmd.Rect)
				}
			case paint.CmdText:
				texts++
			}
		}
		if images != 1 {
			t.Errorf("CmdImage count = %d, want 1", images)
		}
		if texts != 0 {
			t.Errorf("CmdText count = %d, want 0 for a loaded image", texts)
		}
	})

	t.Run("contain", func(t *testing.T) {
		commands := imageInputCommands(t, `<!doctype html><html><body style="margin:0"><input id="pic" type="image" src="`+src+`" style="width:80px;height:25px;object-fit:contain"></body></html>`)
		var got *paint.DisplayCmd
		for i := range commands {
			if commands[i].Kind == paint.CmdImage {
				got = &commands[i]
				break
			}
		}
		if got == nil {
			t.Fatal("loaded image input emitted no CmdImage")
		}
		if got.Rect.X0 != 15 || got.Rect.Y0 != 0 || got.Rect.W() != 50 || got.Rect.H() != 25 {
			t.Errorf("contain image rect = %v, want x=15 y=0 50x25", got.Rect)
		}
	})
}

func TestImageInputMissingAndBrokenPaintFallback(t *testing.T) {
	fetch := func(base, raw string) ([]byte, error) {
		return nil, errors.New("broken fixture")
	}
	commands := imageInputCommands(t, `<!doctype html><html><head><style>
		body { margin: 0; }
		input { display: block; }
	</style></head><body>
		<input id="missing" type="image">
		<input id="broken" type="image" src="/broken.png" alt="Broken">
	</body></html>`, engine.WithImages("https://example.test/page", fetch))

	texts := make(map[string]bool)
	for _, cmd := range commands {
		if text := commandText(cmd); text != "" {
			texts[text] = true
		}
		if cmd.Kind == paint.CmdImage {
			t.Error("missing or broken image input emitted CmdImage")
		}
	}
	if !texts["Submit"] {
		t.Errorf("fallback text = %v, want Submit for a missing alt", texts)
	}
	if !texts["Broken"] {
		t.Errorf("fallback text = %v, want Broken for a broken source", texts)
	}

	sess, err := engine.NewSession(`<!doctype html><html><head><style>
		body { margin: 0; }
		input { display: block; }
	</style></head><body>
		<input id="missing" type="image">
		<input id="broken" type="image" src="/broken.png" alt="Broken">
	</body></html>`, nil, 800, engine.WithImages("https://example.test/page", fetch))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	for _, id := range []string{"missing", "broken"} {
		n := sess.Doc.ElementByID(id)
		oid, ok := sess.Arena.ForNode(n.ID)
		if !ok {
			t.Fatalf("#%s has no layout object", id)
		}
		obj := sess.Arena.Get(oid)
		if fills := imageInputIconFills(commands, obj.X, obj.Y, obj.W, obj.H); fills < 4 {
			t.Errorf("#%s emitted %d icon fills, want at least 4", id, fills)
		}
	}
}

func TestImageInputEmptyAltPaintsNoFallback(t *testing.T) {
	commands := imageInputCommands(t, `<!doctype html><html><body style="margin:0"><input id="empty" type="image" alt=""></body></html>`)
	for _, cmd := range commands {
		switch cmd.Kind {
		case paint.CmdImage, paint.CmdText, paint.CmdFill, paint.CmdBorder:
			t.Errorf("alt=\"\" emitted %s at %v", cmd.Kind, cmd.Rect)
		}
	}
}
