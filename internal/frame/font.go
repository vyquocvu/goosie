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
	FontHelvetica
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
	case FontHelvetica:
		return "Helvetica Neue"
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
	// Light marks a CSS weight below 400. Only the families with a real light
	// face on the host serve it; everywhere else it collapses back to the
	// regular face during the source lookup.
	Light bool
	// CustomIdx is a 1-based index into the rasterizer's custom font table,
	// set when a @font-face rule registers a family the built-in set does not
	// know. Zero means no custom font, so the zero FontSlot is still the
	// embedded Go face.
	CustomIdx uint16
}
