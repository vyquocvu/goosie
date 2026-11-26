package paint_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

type decorationEvidence struct {
	run      frame.Rect
	baseline int32
	fills    []paint.DisplayCmd
	commands []paint.DisplayCmd
}

func paintDecorationEvidence(t *testing.T, body string, authorCSS []string, text string) decorationEvidence {
	t.Helper()
	html := `<!doctype html><html><body style="margin:0">` + body + `</body></html>`
	sess, err := engine.NewSession(html, authorCSS, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	commands := dl.Build(1).All()
	var out decorationEvidence
	found := false
	for _, cmd := range commands {
		if cmd.Kind == paint.CmdText && !found {
			var sb strings.Builder
			for _, glyph := range cmd.Text.Glyphs {
				sb.WriteRune(glyph.Rune)
			}
			if sb.String() == text {
				out.run = cmd.Rect
				out.baseline = cmd.Text.Glyphs[0].Y
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("text run %q was not painted", text)
	}
	for _, cmd := range commands {
		if cmd.Kind == paint.CmdFill && cmd.Rect.H() <= 8 {
			out.fills = append(out.fills, cmd)
		}
	}
	out.commands = commands
	return out
}

func decorationFillsOfColour(fills []paint.DisplayCmd, colour frame.Color) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, cmd := range fills {
		if cmd.Color == colour {
			out = append(out, cmd)
		}
	}
	return out
}

func decorationRowSpans(fills []paint.DisplayCmd) map[int32][][2]int32 {
	rows := make(map[int32][][2]int32)
	for _, cmd := range fills {
		for y := cmd.Rect.Y0; y < cmd.Rect.Y1; y++ {
			rows[y] = append(rows[y], [2]int32{cmd.Rect.X0, cmd.Rect.X1})
		}
	}
	return rows
}

func TestTextDecorationLineBitsCompose(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	for _, tc := range []struct {
		name string
		line string
		rows []int32
	}{
		{"underline", "underline", []int32{1}},
		{"overline", "overline", []int32{-1}},
		{"line-through", "line-through", []int32{-5}},
		{"all", "underline overline line-through", []int32{-5, -1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := paintDecorationEvidence(t, `<p style="font-size:16px">HELLOWORLD</p>`,
				[]string{`p { text-decoration-line: ` + tc.line + `; text-decoration-style: solid; text-decoration-color: rgb(255, 0, 0); }`}, "HELLOWORLD")
			fills := decorationFillsOfColour(ev.fills, red)
			if len(fills) != len(tc.rows) {
				t.Fatalf("decoration fills = %d, want %d: %+v", len(fills), len(tc.rows), fills)
			}
			rows := make(map[int32]bool)
			for _, fill := range fills {
				rows[fill.Rect.Y0] = true
			}
			for i, row := range tc.rows {
				wantY := ev.baseline + row
				if row == -1 {
					wantY = ev.run.Y0 - 1
				}
				if !rows[wantY] {
					t.Errorf("missing decoration row %d at y=%d: %+v", i, wantY, fills)
				}
				for _, fill := range fills {
					if fill.Rect.Y0 != wantY {
						continue
					}
					if fill.Rect.Y1 != wantY+1 {
						t.Errorf("fill %v is not one device row", fill.Rect)
					}
					if fill.Rect.X0 != ev.run.X0 || fill.Rect.X1 != ev.run.X1 {
						t.Errorf("fill spans x%d..%d, want the run x%d..%d", fill.Rect.X0, fill.Rect.X1, ev.run.X0, ev.run.X1)
					}
				}
			}
		})
	}
}

func TestTextDecorationPatternStyles(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	for _, tc := range []struct {
		name     string
		style    string
		top      int32
		height   int32
		on, off  int32
		fullRows []int32
	}{
		{"dotted", "dotted", 0, 2, 2, 2, []int32{0, 1}},
		{"dashed", "dashed", 0, 2, 6, 4, []int32{0, 1}},
		{"double", "double", 1, 4, 0, 0, []int32{1, 4}},
		{"wavy", "wavy", 2, 4, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := paintDecorationEvidence(t, `<p style="font-size:16px">HELLOWORLD</p>`,
				[]string{`p { text-decoration: underline ` + tc.style + ` rgb(255, 0, 0); }`}, "HELLOWORLD")
			fills := decorationFillsOfColour(ev.fills, red)
			if len(fills) == 0 {
				t.Fatal("no decoration fills reached the display list")
			}
			rows := decorationRowSpans(fills)
			if tc.on > 0 {
				for row := ev.baseline + tc.top; row < ev.baseline+tc.top+tc.height; row++ {
					spans := rows[row]
					if len(spans) < 3 {
						t.Fatalf("row %d has %d spans, want a repeating pattern: %v", row, len(spans), spans)
					}
					for i := 0; i < 3; i++ {
						wantX0 := ev.run.X0 + int32(i)*(tc.on+tc.off)
						if spans[i] != [2]int32{wantX0, wantX0 + tc.on} {
							t.Errorf("row %d span %d = %v, want [%d %d]", row, i, spans[i], wantX0, wantX0+tc.on)
						}
					}
				}
				return
			}
			if tc.style == "double" {
				for _, dy := range tc.fullRows {
					spans := rows[ev.baseline+dy]
					if len(spans) != 1 || spans[0] != [2]int32{ev.run.X0, ev.run.X1} {
						t.Errorf("double row baseline%+d = %v, want one full-width bar", dy, spans)
					}
				}
				if len(rows) != 2 {
					t.Errorf("double painted rows %v, want two bars separated by two rows", rows)
				}
				return
			}
			if len(rows) != int(tc.height) {
				t.Fatalf("wavy painted %d rows, want %d: %v", len(rows), tc.height, rows)
			}
			for dy := int32(0); dy < tc.height; dy++ {
				if len(rows[ev.baseline+tc.top+dy]) == 0 {
					t.Errorf("wavy row baseline%+d is empty", tc.top+dy)
				}
			}
			if spans := rows[ev.baseline+tc.top+3]; len(spans) == 0 || spans[0][0] != ev.run.X0 {
				t.Errorf("wavy bottom row = %v, want ink starting at the run origin", spans)
			}
			if spans := rows[ev.baseline+tc.top]; len(spans) > 0 && spans[0][0] == ev.run.X0 {
				t.Errorf("wavy top row starts at the run origin, so the rows do not form a wave: %v", spans)
			}
		})
	}
}

func TestTextDecorationAutoDoubleGeometryPerLine(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	for _, tc := range []struct {
		name string
		line string
		rows func(frame.Rect, int32) []int32
	}{
		{"underline", "underline", func(_ frame.Rect, baseline int32) []int32 {
			return []int32{baseline + 1, baseline + 4}
		}},
		{"overline", "overline", func(run frame.Rect, _ int32) []int32 {
			return []int32{run.Y0 - 4, run.Y0 - 1}
		}},
		{"line-through", "line-through", func(_ frame.Rect, baseline int32) []int32 {
			return []int32{baseline - 5, baseline - 3}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := paintDecorationEvidence(t, `<p style="font-size:16px">HELLOWORLD</p>`, []string{
				`p { text-decoration-line: ` + tc.line + `; text-decoration-style: double; text-decoration-color: rgb(255, 0, 0); }`,
			}, "HELLOWORLD")
			rows := decorationRowSpans(decorationFillsOfColour(ev.fills, red))
			want := tc.rows(ev.run, ev.baseline)
			if len(rows) != len(want) {
				t.Fatalf("double painted rows %v, want %v", rows, want)
			}
			for _, y := range want {
				spans := rows[y]
				if len(spans) != 1 || spans[0] != [2]int32{ev.run.X0, ev.run.X1} {
					t.Errorf("double row y=%d = %v, want one full-width bar", y, spans)
				}
			}
		})
	}
}

func TestTextDecorationThicknessAndOffsetGeometry(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	for _, tc := range []struct {
		name string
		css  string
		top  int32
		h    int32
	}{
		{"auto", `text-decoration: underline solid rgb(255, 0, 0)`, 1, 1},
		{"3px", `text-decoration: underline solid rgb(255, 0, 0); text-decoration-thickness: 3px`, 2, 3},
		{"5px", `text-decoration: underline solid rgb(255, 0, 0); text-decoration-thickness: 5px`, 3, 5},
		{"offset 2", `text-decoration: underline solid rgb(255, 0, 0); text-underline-offset: 2px`, 2, 1},
		{"offset 4", `text-decoration: underline solid rgb(255, 0, 0); text-underline-offset: 4px`, 4, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := paintDecorationEvidence(t, `<p style="font-size:16px">HELLOWORLD</p>`, []string{`p { ` + tc.css + `; }`}, "HELLOWORLD")
			fills := decorationFillsOfColour(ev.fills, red)
			if len(fills) != 1 {
				t.Fatalf("decoration fills = %d, want one solid bar: %+v", len(fills), fills)
			}
			want := frame.Rect4(ev.run.X0, ev.baseline+tc.top, ev.run.X1, ev.baseline+tc.top+tc.h)
			if fills[0].Rect != want {
				t.Errorf("bar = %v, want %v", fills[0].Rect, want)
			}
		})
	}
}

func TestTextDecorationPatternThickness(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	for _, tc := range []struct {
		name    string
		style   string
		on, off int32
		height  int32
	}{
		{"dotted", "dotted", 3, 3, 3},
		{"dashed", "dashed", 6, 4, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev := paintDecorationEvidence(t, `<p style="font-size:16px">HELLOWORLD</p>`, []string{
				`p { text-decoration: underline ` + tc.style + ` rgb(255, 0, 0); text-decoration-thickness: 3px; }`,
			}, "HELLOWORLD")
			rows := decorationRowSpans(decorationFillsOfColour(ev.fills, red))
			for dy := int32(2); dy < 2+tc.height; dy++ {
				spans := rows[ev.baseline+dy]
				if len(spans) < 3 {
					t.Fatalf("row baseline%+d has %d spans, want a thick repeating pattern: %v", dy, len(spans), spans)
				}
				for i := 0; i < 3; i++ {
					wantX0 := ev.run.X0 + int32(i)*(tc.on+tc.off)
					if spans[i] != [2]int32{wantX0, wantX0 + tc.on} {
						t.Errorf("row baseline%+d span %d = %v, want [%d %d]", dy, i, spans[i], wantX0, wantX0+tc.on)
					}
				}
			}
		})
	}
}

func TestTextDecorationShadowCopiesAreNotDecorated(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	ev := paintDecorationEvidence(t, `<p style="font-size:16px">HELLOWORLD</p>`, []string{
		`p { text-decoration: underline solid rgb(255, 0, 0); text-shadow: 2px 2px 0 rgb(0, 0, 255), -2px -2px 0 rgb(0, 255, 0); }`,
	}, "HELLOWORLD")
	textRuns := 0
	for _, cmd := range ev.commands {
		if cmd.Kind == paint.CmdText {
			textRuns++
		}
	}
	if textRuns != 3 {
		t.Fatalf("text runs = %d, want the main run and two shadow copies", textRuns)
	}
	if fills := decorationFillsOfColour(ev.fills, red); len(fills) != 1 {
		t.Errorf("decoration fills = %d, want only the main run's bar: %+v", len(fills), fills)
	}
}

func TestControlTextDecorationUsesEveryLineAndStyle(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	ev := paintDecorationEvidence(t, `<input value="HELLOWORLD">`, []string{
		`input { width: 240px; font-size: 16px; text-decoration-line: underline overline line-through; text-decoration-style: dashed; text-decoration-color: rgb(255, 0, 0); }`,
	}, "HELLOWORLD")
	fills := decorationFillsOfColour(ev.fills, red)
	rows := decorationRowSpans(fills)
	for _, y := range []int32{ev.run.Y0 - 1, ev.baseline - 6, ev.baseline - 5, ev.baseline, ev.baseline + 1} {
		if len(rows[y]) == 0 {
			t.Errorf("no dashed control decoration on row %d: %v", y, rows)
		}
	}
	for y, spans := range rows {
		for _, span := range spans {
			if span[0] < ev.run.X0 || span[1] > ev.run.X1 || span[1]-span[0] > 6 {
				t.Errorf("control decoration row %d span %v escaped the text advance x%d..%d", y, span, ev.run.X0, ev.run.X1)
			}
		}
	}
}

func TestPlaceholderHostAndPseudoDecorationStylesCompose(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	blue := frame.RGB(0, 0, 255)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`, []string{
		`input { text-decoration-line: underline; text-decoration-style: dashed; text-decoration-color: rgb(0, 0, 255); }`,
		`input::placeholder { text-decoration-line: line-through; text-decoration-style: wavy; text-decoration-color: rgb(255, 0, 0); }`,
	})
	blueRows := decorationRowSpans(decorationFillsOfColour(ev.fills, blue))
	if len(blueRows[ev.baseline]) == 0 || len(blueRows[ev.baseline+1]) == 0 {
		t.Errorf("host dashed underline is missing: %v", blueRows)
	}
	redRows := decorationRowSpans(decorationFillsOfColour(ev.fills, red))
	if len(redRows) != 4 {
		t.Fatalf("pseudo wavy line-through painted %d rows, want a four-row wave: %v", len(redRows), redRows)
	}
	minY, maxY := int32(1<<30), int32(-1<<30)
	for y := range redRows {
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	if maxY-minY != 3 || maxY >= ev.baseline {
		t.Errorf("pseudo wavy line-through spans y%d..%d, want four contiguous rows above baseline %d", minY, maxY, ev.baseline)
	}
}

func TestPlaceholderDecorationComparisonIncludesStyle(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`, []string{
		`input { text-decoration: underline solid rgb(255, 0, 0); }`,
		`input::placeholder { text-decoration: underline dotted rgb(255, 0, 0); }`,
	})
	rows := decorationRowSpans(decorationFillsOfColour(ev.fills, red))
	if len(rows[ev.baseline+1]) < 2 {
		t.Errorf("same-colour host solid and pseudo dotted decorations did not both paint: %v", rows)
	}
	foundFull, foundDot := false, false
	for _, span := range rows[ev.baseline+1] {
		if span == [2]int32{ev.run.X0, ev.run.X1} {
			foundFull = true
		}
		if span[1]-span[0] <= 2 {
			foundDot = true
		}
	}
	if !foundFull || !foundDot {
		t.Errorf("row baseline+1 = %v, want both the host's full bar and the pseudo's 2px dots", rows[ev.baseline+1])
	}
}

func TestPlaceholderDecorationWaveIsClippedToTheField(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`, []string{
		`input::placeholder { text-decoration: underline wavy rgb(255, 0, 0); text-underline-offset: 16px; }`,
	})
	if fills := decorationFillsOfColour(ev.fills, red); len(fills) != 0 {
		t.Errorf("offset wave escaped the field clip: %+v", fills)
	}
	for _, cmd := range ev.fills {
		if cmd.Rect.Y0 < ev.field.Y0 || cmd.Rect.Y1 > ev.field.Y1 {
			t.Errorf("fill %v escaped field %v", cmd.Rect, ev.field)
		}
	}
}
