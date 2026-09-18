package frame

// FontFamily names a face family Goosie can draw with. It is a resolved value,
// not a CSS string: the family list a document asks for is narrowed to one of
// these slots during style resolution, so nothing downstream parses names.
//
// FontGo is the embedded face and every family's fallback, which is what keeps
// rendering defined on a host with no system fonts at all.
type FontFamily uint8

const (
	FontGo FontFamily = iota
	FontTimes
	FontArial
	FontCourier
	FontGeorgia
	FontVerdana
)

func (f FontFamily) String() string {
	switch f {
	case FontTimes:
		return "Times New Roman"
	case FontArial:
		return "Arial"
	case FontCourier:
		return "Courier New"
	case FontGeorgia:
		return "Georgia"
	case FontVerdana:
		return "Verdana"
	}
	return "Go"
}

// FontSlot is one face: a family at a weight and a slant.
//
// It is comparable and pointer-free on purpose. A slot keys the rasterizer's
// face cache and travels inside a GlyphRun, which is serialized into a display
// list, so a name or an interface would put a dereference in the tile loop.
//
// The zero slot is the embedded face at normal weight and upright slant, which
// makes every GlyphRun built without a style - the synthetic scene, the
// toolbar - draw what v2 has always drawn.
type FontSlot struct {
	Family FontFamily
	Bold   bool
	Italic bool
}
