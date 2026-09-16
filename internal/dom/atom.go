package dom

import "strings"

// Atom is an interned string for element and attribute names. Comparing atoms
// is a single integer comparison rather than a string walk, which is what keeps
// selector matching fast across a large tree.
type Atom uint32

const (
	AtomUnknown Atom = iota
	AtomHTML
	AtomHead
	AtomBody
	AtomTitle
	AtomMeta
	AtomLink
	AtomStyle
	AtomScript
	AtomDiv
	AtomSpan
	AtomP
	AtomA
	AtomImg
	AtomUl
	AtomOl
	AtomLi
	AtomH1
	AtomH2
	AtomH3
	AtomH4
	AtomH5
	AtomH6
	AtomTable
	AtomThead
	AtomTbody
	AtomTr
	AtomTd
	AtomTh
	AtomForm
	AtomInput
	AtomButton
	AtomSelect
	AtomOption
	AtomTextarea
	AtomLabel
	AtomHeader
	AtomFooter
	AtomNav
	AtomMain
	AtomSection
	AtomArticle
	AtomAside
	AtomFigure
	AtomFigcaption
	AtomBr
	AtomHr
	AtomPre
	AtomCode
	AtomStrong
	AtomEm
	AtomB
	AtomI
	AtomU
	AtomSmall
	AtomSub
	AtomSup
	AtomBlockquote
	AtomQ
	AtomDl
	AtomDt
	AtomDd
	AtomAddress
	AtomDetails
	AtomSummary
	AtomDialog
	AtomTemplate
	AtomSlot
	AtomVideo
	AtomAudio
	AtomSource
	AtomCanvas
	AtomSvg
	AtomPath
	AtomCircle
	AtomRect
	AtomLine
	AtomPolygon
	AtomPolyline
	AtomEllipse
	AtomG
	AtomDefs
	AtomUse
	AtomText_
	AtomTspan
	AtomIFrame
	AtomObject
	AtomEmbed
	AtomArea
	AtomMap
	AtomBase
	AtomNoscript
	AtomPicture
	AtomData
	AtomTime
	AtomMark
	AtomProgress
	AtomMeter
	AtomFieldset
	AtomLegend
	AtomDatalist
	AtomOutput
	AtomOptgroup
	AtomCol
	AtomColgroup
	AtomCaption

	atomSentinel
)

var atomNames = [...]string{
	AtomHTML:    "html",
	AtomHead:    "head",
	AtomBody:    "body",
	AtomTitle:   "title",
	AtomMeta:    "meta",
	AtomLink:    "link",
	AtomStyle:   "style",
	AtomScript:  "script",
	AtomDiv:     "div",
	AtomSpan:    "span",
	AtomP:       "p",
	AtomA:       "a",
	AtomImg:     "img",
	AtomUl:      "ul",
	AtomOl:      "ol",
	AtomLi:      "li",
	AtomH1:      "h1",
	AtomH2:      "h2",
	AtomH3:      "h3",
	AtomH4:      "h4",
	AtomH5:      "h5",
	AtomH6:      "h6",
	AtomTable:   "table",
	AtomThead:   "thead",
	AtomTbody:   "tbody",
	AtomTr:      "tr",
	AtomTd:      "td",
	AtomTh:      "th",
	AtomForm:    "form",
	AtomInput:   "input",
	AtomButton:  "button",
	AtomSelect:  "select",
	AtomOption:  "option",
	AtomTextarea: "textarea",
	AtomLabel:   "label",
	AtomHeader:  "header",
	AtomFooter:  "footer",
	AtomNav:     "nav",
	AtomMain:    "main",
	AtomSection: "section",
	AtomArticle: "article",
	AtomAside:   "aside",
	AtomFigure:  "figure",
	AtomFigcaption: "figcaption",
	AtomBr:      "br",
	AtomHr:      "hr",
	AtomPre:     "pre",
	AtomCode:    "code",
	AtomStrong:  "strong",
	AtomEm:      "em",
	AtomB:       "b",
	AtomI:       "i",
	AtomU:       "u",
	AtomSmall:   "small",
	AtomSub:     "sub",
	AtomSup:     "sup",
	AtomBlockquote: "blockquote",
	AtomQ:       "q",
	AtomDl:      "dl",
	AtomDt:      "dt",
	AtomDd:      "dd",
	AtomAddress: "address",
	AtomDetails: "details",
	AtomSummary: "summary",
	AtomDialog:  "dialog",
	AtomTemplate: "template",
	AtomSlot:    "slot",
	AtomVideo:   "video",
	AtomAudio:   "audio",
	AtomSource:  "source",
	AtomCanvas:  "canvas",
	AtomSvg:     "svg",
	AtomPath:    "path",
	AtomCircle:  "circle",
	AtomRect:    "rect",
	AtomLine:    "line",
	AtomPolygon: "polygon",
	AtomPolyline: "polyline",
	AtomEllipse: "ellipse",
	AtomG:       "g",
	AtomDefs:    "defs",
	AtomUse:     "use",
	AtomText_:   "text",
	AtomTspan:   "tspan",
	AtomIFrame:  "iframe",
	AtomObject:  "object",
	AtomEmbed:   "embed",
	AtomArea:    "area",
	AtomMap:     "map",
	AtomBase:    "base",
	AtomNoscript: "noscript",
	AtomPicture: "picture",
	AtomData:    "data",
	AtomTime:    "time",
	AtomMark:    "mark",
	AtomProgress: "progress",
	AtomMeter:   "meter",
	AtomFieldset: "fieldset",
	AtomLegend:  "legend",
	AtomDatalist: "datalist",
	AtomOutput:  "output",
	AtomOptgroup: "optgroup",
	AtomCol:     "col",
	AtomColgroup: "colgroup",
	AtomCaption: "caption",
}

var atomLookup map[string]Atom

func init() {
	atomLookup = make(map[string]Atom, len(atomNames))
	for i, name := range atomNames {
		if name != "" {
			atomLookup[name] = Atom(i)
		}
	}
}

// Lookup returns the atom for name, or AtomUnknown if it is not interned.
func Lookup(name string) Atom {
	if a, ok := atomLookup[strings.ToLower(name)]; ok {
		return a
	}
	return AtomUnknown
}

// String returns the element or attribute name for the atom.
func (a Atom) String() string {
	if a > 0 && int(a) < len(atomNames) {
		return atomNames[a]
	}
	return ""
}

// AttrAtom is an interned attribute name.
type AttrAtom uint32

const (
	AttrUnknown AttrAtom = iota
	AttrID
	AttrClass
	AttrStyle
	AttrSrc
	AttrHref
	AttrAlt
	AttrWidth
	AttrHeight
	AttrType
	AttrValue
	AttrName
	AttrAction
	AttrMethod
	AttrTarget
	AttrRel
	AttrContent
	AttrCharset
	AttrLang
	AttrTitle
	AttrPlaceholder
	AttrDisabled
	AttrChecked
	AttrSelected
	AttrReadonly
	AttrRequired
	AttrMaxlength
	AttrMinlength
	AttrPattern
	AttrAutofocus
	AttrAutocomplete
	AttrFor
	AttrRole
	AttrAriaLabel
	AttrDataPrefix
)

var attrNames = [...]string{
	AttrID:          "id",
	AttrClass:       "class",
	AttrStyle:       "style",
	AttrSrc:         "src",
	AttrHref:        "href",
	AttrAlt:         "alt",
	AttrWidth:       "width",
	AttrHeight:      "height",
	AttrType:        "type",
	AttrValue:       "value",
	AttrName:        "name",
	AttrAction:      "action",
	AttrMethod:      "method",
	AttrTarget:      "target",
	AttrRel:         "rel",
	AttrContent:     "content",
	AttrCharset:     "charset",
	AttrLang:        "lang",
	AttrTitle:       "title",
	AttrPlaceholder: "placeholder",
	AttrDisabled:    "disabled",
	AttrChecked:     "checked",
	AttrSelected:    "selected",
	AttrReadonly:    "readonly",
	AttrRequired:    "required",
	AttrMaxlength:   "maxlength",
	AttrMinlength:   "minlength",
	AttrPattern:     "pattern",
	AttrAutofocus:   "autofocus",
	AttrAutocomplete: "autocomplete",
	AttrFor:         "for",
	AttrRole:        "role",
	AttrAriaLabel:   "aria-label",
	AttrDataPrefix:  "data-",
}

var attrLookup map[string]AttrAtom

func init() {
	attrLookup = make(map[string]AttrAtom, len(attrNames))
	for i, name := range attrNames {
		if name != "" {
			attrLookup[name] = AttrAtom(i)
		}
	}
}

// LookupAttr returns the attribute atom for name, or AttrUnknown.
func LookupAttr(name string) AttrAtom {
	if a, ok := attrLookup[strings.ToLower(name)]; ok {
		return a
	}
	return AttrUnknown
}

// String returns the attribute name.
func (a AttrAtom) String() string {
	if a > 0 && int(a) < len(attrNames) {
		return attrNames[a]
	}
	return ""
}
