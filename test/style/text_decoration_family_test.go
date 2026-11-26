package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestTextDecorationStyleDefaultsToSolid(t *testing.T) {
	if style.TextDecorationStyleSolid != 0 {
		t.Fatalf("solid enum = %d, want the zero value", style.TextDecorationStyleSolid)
	}
	styles, p, _ := decoDoc(t, `#p1 { text-decoration: underline; }`)
	if got := styles[p.ID].TextDecorationStyle; got != style.TextDecorationStyleSolid {
		t.Fatalf("default decoration style = %v, want solid", got)
	}
}

func TestTextDecorationShorthandParsesEveryLineAndStyle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		style style.TextDecorationStyle
	}{
		{"solid", "underline overline line-through solid rgb(1, 2, 3) 3px", style.TextDecorationStyleSolid},
		{"double", "underline overline line-through double rgb(1, 2, 3) 3px", style.TextDecorationStyleDouble},
		{"dotted", "underline overline line-through dotted rgb(1, 2, 3) 3px", style.TextDecorationStyleDotted},
		{"dashed", "underline overline line-through dashed rgb(1, 2, 3) 3px", style.TextDecorationStyleDashed},
		{"wavy", "underline overline line-through wavy rgb(1, 2, 3) 3px", style.TextDecorationStyleWavy},
		{"case and order", "3px RGB(1, 2, 3) WAVY line-through underline overline", style.TextDecorationStyleWavy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			styles, p, _ := decoDoc(t, `#p1 { text-decoration: `+tc.value+`; }`)
			cs := styles[p.ID]
			wantLines := style.TextDecorationUnderline | style.TextDecorationOverline | style.TextDecorationLineThrough
			if cs.TextDecoration != wantLines {
				t.Errorf("line bits = %d, want %d", cs.TextDecoration, wantLines)
			}
			if cs.TextDecorationStyle != tc.style {
				t.Errorf("style = %v, want %v", cs.TextDecorationStyle, tc.style)
			}
			if got := rgba(cs.TextDecorationColor); got != [4]uint8{1, 2, 3, 255} {
				t.Errorf("colour = %v, want {1 2 3 255}", got)
			}
			if cs.TextDecorationThickness != 3 {
				t.Errorf("thickness = %v, want 3", cs.TextDecorationThickness)
			}
		})
	}
}

func TestTextDecorationLonghandsUpdateOnlyTheirOwnComponent(t *testing.T) {
	styles, p, _ := decoDoc(t, `
		#p1 {
			text-decoration: underline dashed rgb(1, 2, 3) 3px;
			text-underline-offset: 4px;
			text-decoration-line: overline line-through;
		}
	`)
	cs := styles[p.ID]
	wantLines := style.TextDecorationOverline | style.TextDecorationLineThrough
	if cs.TextDecoration != wantLines {
		t.Errorf("line bits = %d, want %d", cs.TextDecoration, wantLines)
	}
	if cs.TextDecorationStyle != style.TextDecorationStyleDashed {
		t.Errorf("line longhand changed style to %v", cs.TextDecorationStyle)
	}
	if got := rgba(cs.TextDecorationColor); got != [4]uint8{1, 2, 3, 255} {
		t.Errorf("line longhand changed colour to %v", got)
	}
	if cs.TextDecorationThickness != 3 || cs.TextUnderlineOffset != 4 {
		t.Errorf("line longhand changed thickness/offset to %v/%v", cs.TextDecorationThickness, cs.TextUnderlineOffset)
	}

	styles, p, _ = decoDoc(t, `
		#p1 {
			text-decoration: underline dashed rgb(1, 2, 3) 3px;
			text-decoration-style: wavy;
		}
	`)
	cs = styles[p.ID]
	if cs.TextDecoration != style.TextDecorationUnderline {
		t.Errorf("style longhand changed line bits to %d", cs.TextDecoration)
	}
	if cs.TextDecorationStyle != style.TextDecorationStyleWavy {
		t.Errorf("style = %v, want wavy", cs.TextDecorationStyle)
	}
	if got := rgba(cs.TextDecorationColor); got != [4]uint8{1, 2, 3, 255} || cs.TextDecorationThickness != 3 {
		t.Errorf("style longhand changed colour/thickness to %v/%v", cs.TextDecorationColor, cs.TextDecorationThickness)
	}
}

func TestTextDecorationShorthandResetsTheWholeFamily(t *testing.T) {
	styles, p, _ := decoDoc(t, `
		#p1 {
			text-decoration-line: overline line-through;
			text-decoration-style: wavy;
			text-decoration-color: rgb(1, 2, 3);
			text-decoration-thickness: 5px;
			text-underline-offset: 4px;
			text-decoration: underline;
		}
	`)
	cs := styles[p.ID]
	if cs.TextDecoration != style.TextDecorationUnderline {
		t.Errorf("line bits = %d, want underline", cs.TextDecoration)
	}
	if cs.TextDecorationStyle != style.TextDecorationStyleSolid {
		t.Errorf("style = %v, want solid", cs.TextDecorationStyle)
	}
	if cs.TextDecorationColor.A != 0 {
		t.Errorf("colour = %v, want the auto sentinel", cs.TextDecorationColor)
	}
	if cs.TextDecorationThickness != 0 {
		t.Errorf("thickness = %v, want the auto sentinel", cs.TextDecorationThickness)
	}
	if cs.TextUnderlineOffset != 4 {
		t.Errorf("offset = %v, want 4 preserved", cs.TextUnderlineOffset)
	}
}

func TestTextDecorationFamilyFollowsSourceOrder(t *testing.T) {
	for _, tc := range []struct {
		name      string
		decls     string
		line      style.TextDecoration
		style     style.TextDecorationStyle
		colour    [4]uint8
		thickness float32
	}{
		{"line before shorthand", `text-decoration-line: overline; text-decoration: underline dashed rgb(1, 2, 3) 3px`, style.TextDecorationUnderline, style.TextDecorationStyleDashed, [4]uint8{1, 2, 3, 255}, 3},
		{"line after shorthand", `text-decoration: underline dashed rgb(1, 2, 3) 3px; text-decoration-line: overline`, style.TextDecorationOverline, style.TextDecorationStyleDashed, [4]uint8{1, 2, 3, 255}, 3},
		{"style before shorthand", `text-decoration-style: wavy; text-decoration: underline dashed rgb(1, 2, 3) 3px`, style.TextDecorationUnderline, style.TextDecorationStyleDashed, [4]uint8{1, 2, 3, 255}, 3},
		{"style after shorthand", `text-decoration: underline dashed rgb(1, 2, 3) 3px; text-decoration-style: wavy`, style.TextDecorationUnderline, style.TextDecorationStyleWavy, [4]uint8{1, 2, 3, 255}, 3},
		{"colour before shorthand", `text-decoration-color: rgb(4, 5, 6); text-decoration: underline dashed rgb(1, 2, 3) 3px`, style.TextDecorationUnderline, style.TextDecorationStyleDashed, [4]uint8{1, 2, 3, 255}, 3},
		{"colour after shorthand", `text-decoration: underline dashed rgb(1, 2, 3) 3px; text-decoration-color: rgb(4, 5, 6)`, style.TextDecorationUnderline, style.TextDecorationStyleDashed, [4]uint8{4, 5, 6, 255}, 3},
		{"thickness before shorthand", `text-decoration-thickness: 5px; text-decoration: underline dashed rgb(1, 2, 3) 3px`, style.TextDecorationUnderline, style.TextDecorationStyleDashed, [4]uint8{1, 2, 3, 255}, 3},
		{"thickness after shorthand", `text-decoration: underline dashed rgb(1, 2, 3) 3px; text-decoration-thickness: 5px`, style.TextDecorationUnderline, style.TextDecorationStyleDashed, [4]uint8{1, 2, 3, 255}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			styles, p, _ := decoDoc(t, `#p1 { `+tc.decls+`; }`)
			cs := styles[p.ID]
			if cs.TextDecoration != tc.line || cs.TextDecorationStyle != tc.style {
				t.Errorf("line/style = %d/%v, want %d/%v", cs.TextDecoration, cs.TextDecorationStyle, tc.line, tc.style)
			}
			if got := rgba(cs.TextDecorationColor); got != tc.colour {
				t.Errorf("colour = %v, want %v", got, tc.colour)
			}
			if cs.TextDecorationThickness != tc.thickness {
				t.Errorf("thickness = %v, want %v", cs.TextDecorationThickness, tc.thickness)
			}
		})
	}
}

func TestTextDecorationFamilyInherits(t *testing.T) {
	styles, p, span := decoDoc(t, `
		#p1 {
			text-decoration: overline line-through wavy rgb(1, 2, 3) 5px;
			text-underline-offset: 4px;
		}
	`)
	parent, child := styles[p.ID], styles[span.ID]
	if child.TextDecoration != parent.TextDecoration || child.TextDecorationStyle != parent.TextDecorationStyle ||
		child.TextDecorationColor != parent.TextDecorationColor || child.TextDecorationThickness != parent.TextDecorationThickness ||
		child.TextUnderlineOffset != parent.TextUnderlineOffset {
		t.Errorf("child decoration = %d/%v/%v/%v/%v, want parent %d/%v/%v/%v/%v",
			child.TextDecoration, child.TextDecorationStyle, child.TextDecorationColor, child.TextDecorationThickness, child.TextUnderlineOffset,
			parent.TextDecoration, parent.TextDecorationStyle, parent.TextDecorationColor, parent.TextDecorationThickness, parent.TextUnderlineOffset)
	}
}

func TestTextDecorationShorthandNoneStandsAlone(t *testing.T) {
	styles, p, _ := decoDoc(t, `#p1 { text-decoration: none; }`)
	cs := styles[p.ID]
	if cs.TextDecoration != style.TextDecorationNone || cs.TextDecorationStyle != style.TextDecorationStyleSolid ||
		cs.TextDecorationColor.A != 0 || cs.TextDecorationThickness != 0 {
		t.Errorf("text-decoration: none computed as %d/%v/%v/%v, want none/solid/auto/auto",
			cs.TextDecoration, cs.TextDecorationStyle, cs.TextDecorationColor, cs.TextDecorationThickness)
	}
}

func TestTextDecorationInvalidValuesLeaveTheFamilyUntouched(t *testing.T) {
	for _, tc := range []struct {
		name  string
		decls string
	}{
		{"unknown line", `text-decoration-line: underline nonsense`},
		{"none with line", `text-decoration-line: none underline`},
		{"duplicate line", `text-decoration-line: underline underline`},
		{"unknown style", `text-decoration-style: groove`},
		{"multiple styles", `text-decoration-style: dashed wavy`},
		{"unknown shorthand token", `text-decoration: underline nonsense`},
		{"none with shorthand line", `text-decoration: none underline`},
		{"none with shorthand style", `text-decoration: none solid`},
		{"none with shorthand colour", `text-decoration: none red`},
		{"none with shorthand thickness", `text-decoration: none 3px`},
		{"none with shorthand auto", `text-decoration: none auto`},
		{"shorthand style before none", `text-decoration: solid none`},
		{"shorthand colour before none", `text-decoration: red none`},
		{"shorthand thickness before none", `text-decoration: 3px none`},
		{"multiple shorthand styles", `text-decoration: underline dashed wavy`},
		{"multiple shorthand colours", `text-decoration: underline red blue`},
		{"negative shorthand thickness", `text-decoration: underline -1px`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			styles, p, _ := decoDoc(t, `
				#p1 {
					text-decoration: overline dashed rgb(1, 2, 3);
					text-decoration-thickness: 4px;
					text-underline-offset: 2px;
					`+tc.decls+`;
				}
			`)
			cs := styles[p.ID]
			if cs.TextDecoration != style.TextDecorationOverline || cs.TextDecorationStyle != style.TextDecorationStyleDashed {
				t.Errorf("invalid value changed line/style to %d/%v", cs.TextDecoration, cs.TextDecorationStyle)
			}
			if got := rgba(cs.TextDecorationColor); got != [4]uint8{1, 2, 3, 255} {
				t.Errorf("invalid value changed colour to %v", got)
			}
			if cs.TextDecorationThickness != 4 || cs.TextUnderlineOffset != 2 {
				t.Errorf("invalid value changed thickness/offset to %v/%v", cs.TextDecorationThickness, cs.TextUnderlineOffset)
			}
		})
	}
}
