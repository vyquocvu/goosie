// Package atom provides string interning for the Goosie engine.
//
// Static atoms are pre-assigned uint32 constants for common HTML tag names
// and attribute names. They are never evicted and require zero allocation
// for lookup.
//
// Dynamic atoms are interned into a bounded LRU table with configurable
// entry count and byte limits. Strings exceeding the byte limit are
// rejected (return zero Atom) to prevent unbounded memory growth.
//
// The package is designed as foundation infrastructure for M2.2 (atom and
// string interning) and will be consumed by M2.3 (compact DOM store) and
// M3.1 (CSS pipeline).
package atom

import (
	"container/list"
	"sync"
)

// Atom is a compact uint32 handle representing an interned string.
// Zero is the invalid/empty atom. Values below StaticEnd are pre-assigned
// static atoms for common HTML tags and attributes.
type Atom uint32

const (
	// AtomNone is the zero/invalid atom.
	AtomNone Atom = 0

	// --- HTML tag names (alphabetical) ---
	AtomA          Atom = 1
	AtomAbbr       Atom = 2
	AtomAddress    Atom = 3
	AtomArea       Atom = 4
	AtomArticle    Atom = 5
	AtomAside      Atom = 6
	AtomAudio      Atom = 7
	AtomB          Atom = 8
	AtomBase       Atom = 9
	AtomBdi        Atom = 10
	AtomBdo        Atom = 11
	AtomBlockquote Atom = 12
	AtomBody       Atom = 13
	AtomBr         Atom = 14
	AtomButton     Atom = 15
	AtomCanvas     Atom = 16
	AtomCaption    Atom = 17
	AtomCite       Atom = 18
	AtomCode       Atom = 19
	AtomCol        Atom = 20
	AtomColgroup   Atom = 21
	AtomData       Atom = 22
	AtomDatalist   Atom = 23
	AtomDd         Atom = 24
	AtomDel        Atom = 25
	AtomDetails    Atom = 26
	AtomDfn        Atom = 27
	AtomDialog     Atom = 28
	AtomDiv        Atom = 29
	AtomDl         Atom = 30
	AtomDt         Atom = 31
	AtomEm         Atom = 32
	AtomEmbed      Atom = 33
	AtomFieldset   Atom = 34
	AtomFigcaption Atom = 35
	AtomFigure     Atom = 36
	AtomFooter     Atom = 37
	AtomForm       Atom = 38
	AtomH1         Atom = 39
	AtomH2         Atom = 40
	AtomH3         Atom = 41
	AtomH4         Atom = 42
	AtomH5         Atom = 43
	AtomH6         Atom = 44
	AtomHead       Atom = 45
	AtomHeader     Atom = 46
	AtomHgroup     Atom = 47
	AtomHr         Atom = 48
	AtomHtml       Atom = 49
	AtomI          Atom = 50
	AtomIframe     Atom = 51
	AtomImg        Atom = 52
	AtomInput      Atom = 53
	AtomIns        Atom = 54
	AtomKbd        Atom = 55
	AtomLabel      Atom = 56
	AtomLegend     Atom = 57
	AtomLi         Atom = 58
	AtomLink       Atom = 59
	AtomMain       Atom = 60
	AtomMap        Atom = 61
	AtomMark       Atom = 62
	AtomMenu       Atom = 63
	AtomMeta       Atom = 64
	AtomMeter      Atom = 65
	AtomNav        Atom = 66
	AtomNoscript   Atom = 67
	AtomObject     Atom = 68
	AtomOl         Atom = 69
	AtomOptgroup   Atom = 70
	AtomOption     Atom = 71
	AtomOutput     Atom = 72
	AtomP          Atom = 73
	AtomParam      Atom = 74
	AtomPicture    Atom = 75
	AtomPre        Atom = 76
	AtomProgress   Atom = 77
	AtomQ          Atom = 78
	AtomRp         Atom = 79
	AtomRt         Atom = 80
	AtomRuby       Atom = 81
	AtomS          Atom = 82
	AtomSamp       Atom = 83
	AtomScript     Atom = 84
	AtomSection    Atom = 85
	AtomSelect     Atom = 86
	AtomSlot       Atom = 87
	AtomSmall      Atom = 88
	AtomSource     Atom = 89
	AtomSpan       Atom = 90
	AtomStrong     Atom = 91
	AtomStyle      Atom = 92
	AtomSub        Atom = 93
	AtomSummary    Atom = 94
	AtomSup        Atom = 95
	AtomTable      Atom = 96
	AtomTbody      Atom = 97
	AtomTd         Atom = 98
	AtomTemplate   Atom = 99
	AtomTextarea   Atom = 100
	AtomTfoot      Atom = 101
	AtomTh         Atom = 102
	AtomThead      Atom = 103
	AtomTime       Atom = 104
	AtomTitle      Atom = 105
	AtomTr         Atom = 106
	AtomTrack      Atom = 107
	AtomU          Atom = 108
	AtomUl         Atom = 109
	AtomVar        Atom = 110
	AtomVideo      Atom = 111
	AtomWbr        Atom = 112

	// --- HTML attribute names ---
	AttrId           Atom = 200
	AttrClass        Atom = 201
	AttrHref         Atom = 202
	AttrSrc          Atom = 203
	AttrAlt          Atom = 204
	AttrStyle        Atom = 205
	AttrType         Atom = 206
	AttrName         Atom = 207
	AttrValue        Atom = 208
	AttrAction       Atom = 209
	AttrMethod       Atom = 210
	AttrRel          Atom = 211
	AttrCharset      Atom = 212
	AttrContent      Atom = 213
	AttrLang         Atom = 214
	AttrWidth        Atom = 215
	AttrHeight       Atom = 216
	AttrTitle        Atom = 217
	AttrPlaceholder  Atom = 218
	AttrDisabled     Atom = 219
	AttrChecked      Atom = 220
	AttrRole         Atom = 221
	AttrAriaLabel    Atom = 222
	AttrFor          Atom = 223
	AttrTabindex     Atom = 224
	AttrTarget       Atom = 225
	AttrDataPrefix   Atom = 226
	AttrSrcset       Atom = 227
	AttrSizes        Atom = 228
	AttrMedia        Atom = 229
	AttrDatetime     Atom = 230
	AttrOpen         Atom = 231
	AttrMax          Atom = 232
	AttrMin          Atom = 233
	AttrStep         Atom = 234
	AttrPattern      Atom = 235
	AttrRequired     Atom = 236
	AttrReadonly     Atom = 237
	AttrAutofocus    Atom = 238
	AttrAutocomplete Atom = 239
	AttrEnctype      Atom = 240
	AttrColspan      Atom = 241
	AttrRowspan      Atom = 242
	AttrScope        Atom = 243
	AttrSpan         Atom = 244
	AttrDir          Atom = 245
	AttrDraggable    Atom = 246
	AttrHidden       Atom = 247

	// StaticEnd marks the boundary between static and dynamic atoms.
	StaticEnd Atom = 500
)

// staticStrings maps static Atom values to their string representations.
// Index 0 is empty (AtomNone). We use a flat slice for O(1) lookup.
var staticStrings [StaticEnd]string

// staticLookup maps static strings to their Atom values.
var staticLookup map[string]Atom

func init() {
	staticLookup = make(map[string]Atom, 256)

	// Register all static atoms.
	register := func(a Atom, s string) {
		staticStrings[a] = s
		staticLookup[s] = a
	}

	// HTML tags
	register(AtomA, "a")
	register(AtomAbbr, "abbr")
	register(AtomAddress, "address")
	register(AtomArea, "area")
	register(AtomArticle, "article")
	register(AtomAside, "aside")
	register(AtomAudio, "audio")
	register(AtomB, "b")
	register(AtomBase, "base")
	register(AtomBdi, "bdi")
	register(AtomBdo, "bdo")
	register(AtomBlockquote, "blockquote")
	register(AtomBody, "body")
	register(AtomBr, "br")
	register(AtomButton, "button")
	register(AtomCanvas, "canvas")
	register(AtomCaption, "caption")
	register(AtomCite, "cite")
	register(AtomCode, "code")
	register(AtomCol, "col")
	register(AtomColgroup, "colgroup")
	register(AtomData, "data")
	register(AtomDatalist, "datalist")
	register(AtomDd, "dd")
	register(AtomDel, "del")
	register(AtomDetails, "details")
	register(AtomDfn, "dfn")
	register(AtomDialog, "dialog")
	register(AtomDiv, "div")
	register(AtomDl, "dl")
	register(AtomDt, "dt")
	register(AtomEm, "em")
	register(AtomEmbed, "embed")
	register(AtomFieldset, "fieldset")
	register(AtomFigcaption, "figcaption")
	register(AtomFigure, "figure")
	register(AtomFooter, "footer")
	register(AtomForm, "form")
	register(AtomH1, "h1")
	register(AtomH2, "h2")
	register(AtomH3, "h3")
	register(AtomH4, "h4")
	register(AtomH5, "h5")
	register(AtomH6, "h6")
	register(AtomHead, "head")
	register(AtomHeader, "header")
	register(AtomHgroup, "hgroup")
	register(AtomHr, "hr")
	register(AtomHtml, "html")
	register(AtomI, "i")
	register(AtomIframe, "iframe")
	register(AtomImg, "img")
	register(AtomInput, "input")
	register(AtomIns, "ins")
	register(AtomKbd, "kbd")
	register(AtomLabel, "label")
	register(AtomLegend, "legend")
	register(AtomLi, "li")
	register(AtomLink, "link")
	register(AtomMain, "main")
	register(AtomMap, "map")
	register(AtomMark, "mark")
	register(AtomMenu, "menu")
	register(AtomMeta, "meta")
	register(AtomMeter, "meter")
	register(AtomNav, "nav")
	register(AtomNoscript, "noscript")
	register(AtomObject, "object")
	register(AtomOl, "ol")
	register(AtomOptgroup, "optgroup")
	register(AtomOption, "option")
	register(AtomOutput, "output")
	register(AtomP, "p")
	register(AtomParam, "param")
	register(AtomPicture, "picture")
	register(AtomPre, "pre")
	register(AtomProgress, "progress")
	register(AtomQ, "q")
	register(AtomRp, "rp")
	register(AtomRt, "rt")
	register(AtomRuby, "ruby")
	register(AtomS, "s")
	register(AtomSamp, "samp")
	register(AtomScript, "script")
	register(AtomSection, "section")
	register(AtomSelect, "select")
	register(AtomSlot, "slot")
	register(AtomSmall, "small")
	register(AtomSource, "source")
	register(AtomSpan, "span")
	register(AtomStrong, "strong")
	register(AtomStyle, "style")
	register(AtomSub, "sub")
	register(AtomSummary, "summary")
	register(AtomSup, "sup")
	register(AtomTable, "table")
	register(AtomTbody, "tbody")
	register(AtomTd, "td")
	register(AtomTemplate, "template")
	register(AtomTextarea, "textarea")
	register(AtomTfoot, "tfoot")
	register(AtomTh, "th")
	register(AtomThead, "thead")
	register(AtomTime, "time")
	register(AtomTitle, "title")
	register(AtomTr, "tr")
	register(AtomTrack, "track")
	register(AtomU, "u")
	register(AtomUl, "ul")
	register(AtomVar, "var")
	register(AtomVideo, "video")
	register(AtomWbr, "wbr")

	// HTML attributes
	register(AttrId, "id")
	register(AttrClass, "class")
	register(AttrHref, "href")
	register(AttrSrc, "src")
	register(AttrAlt, "alt")
	register(AttrStyle, "style")
	register(AttrType, "type")
	register(AttrName, "name")
	register(AttrValue, "value")
	register(AttrAction, "action")
	register(AttrMethod, "method")
	register(AttrRel, "rel")
	register(AttrCharset, "charset")
	register(AttrContent, "content")
	register(AttrLang, "lang")
	register(AttrWidth, "width")
	register(AttrHeight, "height")
	register(AttrTitle, "title")
	register(AttrPlaceholder, "placeholder")
	register(AttrDisabled, "disabled")
	register(AttrChecked, "checked")
	register(AttrRole, "role")
	register(AttrAriaLabel, "aria-label")
	register(AttrFor, "for")
	register(AttrTabindex, "tabindex")
	register(AttrTarget, "target")
	register(AttrDataPrefix, "data-")
	register(AttrSrcset, "srcset")
	register(AttrSizes, "sizes")
	register(AttrMedia, "media")
	register(AttrDatetime, "datetime")
	register(AttrOpen, "open")
	register(AttrMax, "max")
	register(AttrMin, "min")
	register(AttrStep, "step")
	register(AttrPattern, "pattern")
	register(AttrRequired, "required")
	register(AttrReadonly, "readonly")
	register(AttrAutofocus, "autofocus")
	register(AttrAutocomplete, "autocomplete")
	register(AttrEnctype, "enctype")
	register(AttrColspan, "colspan")
	register(AttrRowspan, "rowspan")
	register(AttrScope, "scope")
	register(AttrSpan, "span")
	register(AttrDir, "dir")
	register(AttrDraggable, "draggable")
	register(AttrHidden, "hidden")
}

// String returns the interned string for the atom.
// Returns empty string for AtomNone or out-of-bounds dynamic atoms.
func (a Atom) String() string {
	if a == AtomNone {
		return ""
	}
	if a < StaticEnd {
		return staticStrings[a]
	}
	// Dynamic atoms are resolved via the default table.
	return defaultTable.LookupByAtom(a)
}

// IsStatic reports whether the atom is a pre-assigned static atom.
func (a Atom) IsStatic() bool {
	return a > AtomNone && a < StaticEnd
}

// LookupStatic returns the static atom for s, or (0, false) if s is not
// a pre-assigned static atom.
func LookupStatic(s string) (Atom, bool) {
	a, ok := staticLookup[s]
	return a, ok
}

// ---------------------------------------------------------------------------
// Dynamic atom table with bounded LRU eviction
// ---------------------------------------------------------------------------

// Table is a bounded LRU-evicted string interning table.
// It is safe for concurrent use.
type Table struct {
	mu          sync.RWMutex
	maxEntries  int
	maxBytes    int
	bytesUsed   int
	nextAtom    Atom
	strToAtom   map[string]Atom
	atomToStr   map[Atom]string
	lruElements map[Atom]*list.Element
	lru         *list.List // front = most recently used
}

type lruEntry struct {
	atom Atom
}

// NewTable creates a new bounded interning table.
// maxEntries is the maximum number of dynamic entries.
// maxBytes is the maximum total bytes of interned strings.
func NewTable(maxEntries int, maxBytes int) *Table {
	if maxEntries <= 0 {
		maxEntries = 64
	}
	if maxBytes <= 0 {
		maxBytes = 4096
	}
	return &Table{
		maxEntries:  maxEntries,
		maxBytes:    maxBytes,
		nextAtom:    StaticEnd,
		strToAtom:   make(map[string]Atom, maxEntries),
		atomToStr:   make(map[Atom]string, maxEntries),
		lruElements: make(map[Atom]*list.Element, maxEntries),
		lru:         list.New(),
	}
}

// Intern returns the atom for s. If s is a known static string, the static
// atom is returned. If s is already in the dynamic table, the existing atom
// is returned (promoting it in the LRU). Otherwise a new atom is allocated.
// Strings exceeding the byte limit or empty strings return AtomNone (0).
func (t *Table) Intern(s string) Atom {
	if s == "" {
		return AtomNone
	}
	// Check static table first (lock-free).
	if a, ok := staticLookup[s]; ok {
		return a
	}
	if t == nil {
		return AtomNone
	}
	if len(s) > t.maxBytes {
		return AtomNone
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Double-check under lock.
	if a, ok := t.strToAtom[s]; ok {
		t.promote(a)
		return a
	}

	// Check byte budget.
	if len(s) > t.maxBytes-t.bytesUsed && t.lru.Len() == 0 {
		// Single string exceeds entire budget and table is empty.
		return AtomNone
	}

	// Evict until both entry count and byte budget are satisfied.
	for t.lru.Len() >= t.maxEntries || t.bytesUsed+len(s) > t.maxBytes {
		if t.lru.Len() == 0 {
			return AtomNone
		}
		t.evictLRU()
	}

	a := t.nextAtom
	t.nextAtom++
	t.strToAtom[s] = a
	t.atomToStr[a] = s
	t.bytesUsed += len(s)
	elem := t.lru.PushFront(&lruEntry{atom: a})
	t.lruElements[a] = elem

	return a
}

// Lookup returns the atom for s, or AtomNone if not interned.
// Accessing an entry promotes it in the LRU order.
func (t *Table) Lookup(s string) Atom {
	if s == "" {
		return AtomNone
	}
	if a, ok := staticLookup[s]; ok {
		return a
	}
	if t == nil {
		return AtomNone
	}

	t.mu.Lock()
	a, ok := t.strToAtom[s]
	if ok {
		t.promote(a)
	}
	t.mu.Unlock()
	return a
}

// LookupByAtom returns the string for a dynamic atom, or "" if not found.
func (t *Table) LookupByAtom(a Atom) string {
	if a < StaticEnd {
		return staticStrings[a]
	}
	if t == nil {
		return ""
	}
	t.mu.RLock()
	s := t.atomToStr[a]
	t.mu.RUnlock()
	return s
}

// Len returns the number of dynamic entries.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	n := t.lru.Len()
	t.mu.RUnlock()
	return n
}

// BytesUsed returns the total bytes of dynamically interned strings.
func (t *Table) BytesUsed() int {
	if t == nil {
		return 0
	}
	t.mu.RLock()
	n := t.bytesUsed
	t.mu.RUnlock()
	return n
}

// Reset clears all dynamic entries. Static atoms are unaffected.
func (t *Table) Reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.strToAtom = make(map[string]Atom, 64)
	t.atomToStr = make(map[Atom]string, 64)
	t.lruElements = make(map[Atom]*list.Element, 64)
	t.lru.Init()
	t.bytesUsed = 0
	t.nextAtom = StaticEnd
	t.mu.Unlock()
}

func (t *Table) bytesUsedLocked() int {
	return t.bytesUsed
}

func (t *Table) promote(a Atom) {
	if elem, ok := t.lruElements[a]; ok {
		t.lru.MoveToFront(elem)
	}
}

func (t *Table) evictLRU() {
	back := t.lru.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*lruEntry)
	s := t.atomToStr[entry.atom]
	t.bytesUsed -= len(s)
	delete(t.strToAtom, s)
	delete(t.atomToStr, entry.atom)
	delete(t.lruElements, entry.atom)
	t.lru.Remove(back)
}

// ---------------------------------------------------------------------------
// Default global table
// ---------------------------------------------------------------------------

var defaultTable = NewTable(1024, 65536)

// Intern interns s into the default global table.
func Intern(s string) Atom {
	return defaultTable.Intern(s)
}

// Lookup returns the atom for s from the default global table.
func Lookup(s string) Atom {
	return defaultTable.Lookup(s)
}

// ResetDefault resets the default global table, clearing all dynamic entries.
func ResetDefault() {
	defaultTable.Reset()
}
