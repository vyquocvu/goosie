package frame

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

type FontSlot struct {
	Family    FontFamily
	Bold      bool
	Italic    bool
	Light     bool
	CustomIdx uint16
}
