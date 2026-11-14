package ax

type Role uint8

const (
	RoleDocument Role = iota
	RoleGroup
	RoleStaticText
	RoleLink
	RoleButton
	RoleImage
	RoleTextField
	RoleSearch
	RoleProgressBar
	RoleMeter
	RoleTabGroup
	RoleTab
	RoleComboBox
	RoleOption
	RoleHeading
	RoleListBox
)

func (r Role) String() string {
	switch r {
	case RoleDocument:
		return "document"
	case RoleGroup:
		return "group"
	case RoleStaticText:
		return "text"
	case RoleLink:
		return "link"
	case RoleButton:
		return "button"
	case RoleImage:
		return "image"
	case RoleTextField:
		return "textbox"
	case RoleSearch:
		return "search"
	case RoleProgressBar:
		return "progressbar"
	case RoleMeter:
		return "meter"
	case RoleTabGroup:
		return "tablist"
	case RoleTab:
		return "tab"
	case RoleComboBox:
		return "combobox"
	case RoleOption:
		return "option"
	case RoleHeading:
		return "heading"
	case RoleListBox:
		return "listbox"
	default:
		return "generic"
	}
}

type Prop struct {
	Name  string
	Value string
	Ref   uint64
}

func BoolProp(name string, v bool) Prop {
	if v {
		return Prop{Name: name, Value: "true"}
	}
	return Prop{Name: name, Value: "false"}
}

func StringProp(name, value string) Prop {
	return Prop{Name: name, Value: value}
}

func RefProp(name string, target uint64) Prop {
	return Prop{Name: name, Ref: target}
}

type Node struct {
	Role       Role
	RoleName   string
	DOMNodeID  uint64
	ID         string
	Label      string
	Value      string
	Href       string
	Selected   bool
	Disabled   bool
	AddressBar bool
	Focused    bool
	Level      int
	Props      []Prop

	X0, Y0, X1, Y1 float32

	Children []Node
}
