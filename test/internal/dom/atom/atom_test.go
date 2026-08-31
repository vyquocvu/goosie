package atom_test

import (
	"strings"
	"sync"
	"testing"
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// ---------------------------------------------------------------------------
// Static atom tests
// ---------------------------------------------------------------------------

func TestStaticAtomConstants(t *testing.T) {
	// Verify well-known tag atoms resolve to their expected strings.
	tests := []struct {
		atom atom.Atom
		want string
	}{
		{atom.AtomDiv, "div"},
		{atom.AtomSpan, "span"},
		{atom.AtomP, "p"},
		{atom.AtomA, "a"},
		{atom.AtomH1, "h1"},
		{atom.AtomH2, "h2"},
		{atom.AtomH3, "h3"},
		{atom.AtomBody, "body"},
		{atom.AtomHtml, "html"},
		{atom.AtomHead, "head"},
		{atom.AtomTitle, "title"},
		{atom.AtomMeta, "meta"},
		{atom.AtomLink, "link"},
		{atom.AtomStyle, "style"},
		{atom.AtomScript, "script"},
		{atom.AtomImg, "img"},
		{atom.AtomUl, "ul"},
		{atom.AtomOl, "ol"},
		{atom.AtomLi, "li"},
		{atom.AtomTable, "table"},
		{atom.AtomTr, "tr"},
		{atom.AtomTd, "td"},
		{atom.AtomTh, "th"},
		{atom.AtomThead, "thead"},
		{atom.AtomTbody, "tbody"},
		{atom.AtomTfoot, "tfoot"},
		{atom.AtomForm, "form"},
		{atom.AtomInput, "input"},
		{atom.AtomButton, "button"},
		{atom.AtomTextarea, "textarea"},
		{atom.AtomSelect, "select"},
		{atom.AtomOption, "option"},
		{atom.AtomLabel, "label"},
		{atom.AtomBr, "br"},
		{atom.AtomHr, "hr"},
		{atom.AtomPre, "pre"},
		{atom.AtomCode, "code"},
		{atom.AtomStrong, "strong"},
		{atom.AtomEm, "em"},
		{atom.AtomI, "i"},
		{atom.AtomB, "b"},
		{atom.AtomNav, "nav"},
		{atom.AtomHeader, "header"},
		{atom.AtomFooter, "footer"},
		{atom.AtomMain, "main"},
		{atom.AtomSection, "section"},
		{atom.AtomArticle, "article"},
		{atom.AtomAside, "aside"},
	}
	for _, tt := range tests {
		got := tt.atom.String()
		if got != tt.want {
			t.Errorf("Atom(%d).String() = %q, want %q", tt.atom, got, tt.want)
		}
	}
}

func TestStaticAttrConstants(t *testing.T) {
	tests := []struct {
		atom atom.Atom
		want string
	}{
		{atom.AttrId, "id"},
		{atom.AttrClass, "class"},
		{atom.AttrHref, "href"},
		{atom.AttrSrc, "src"},
		{atom.AttrAlt, "alt"},
		{atom.AttrStyle, "style"},
		{atom.AttrType, "type"},
		{atom.AttrName, "name"},
		{atom.AttrValue, "value"},
		{atom.AttrAction, "action"},
		{atom.AttrMethod, "method"},
		{atom.AttrRel, "rel"},
		{atom.AttrCharset, "charset"},
		{atom.AttrContent, "content"},
		{atom.AttrLang, "lang"},
		{atom.AttrWidth, "width"},
		{atom.AttrHeight, "height"},
		{atom.AttrTitle, "title"},
		{atom.AttrPlaceholder, "placeholder"},
		{atom.AttrDisabled, "disabled"},
		{atom.AttrChecked, "checked"},
		{atom.AttrRole, "role"},
		{atom.AttrAriaLabel, "aria-label"},
	}
	for _, tt := range tests {
		got := tt.atom.String()
		if got != tt.want {
			t.Errorf("Atom(%d).String() = %q, want %q", tt.atom, got, tt.want)
		}
	}
}

func TestStaticAtomIsPermanent(t *testing.T) {
	// Verify that every registered static atom has a non-empty string
	// and reports IsStatic() == true. We check only the known ranges
	// (tags: 1–112, attrs: 200–247) rather than the full [1, staticEnd)
	// range which contains gaps.
	for a := atom.Atom(1); a <= atom.AtomWbr; a++ {
		s := a.String()
		if s == "" {
			t.Errorf("static atom %d has empty string", a)
		}
		if !a.IsStatic() {
			t.Errorf("atom %d (%q) should be static", a, s)
		}
	}
	for a := atom.AttrId; a <= atom.AttrHidden; a++ {
		s := a.String()
		if s == "" {
			t.Errorf("static attr atom %d has empty string", a)
		}
		if !a.IsStatic() {
			t.Errorf("atom %d (%q) should be static", a, s)
		}
	}
}

func TestAtomZeroIsEmpty(t *testing.T) {
	var a atom.Atom
	if s := a.String(); s != "" {
		t.Errorf("zero Atom.String() = %q, want empty", s)
	}
	if a.IsStatic() {
		t.Error("zero Atom should not be static")
	}
}

// ---------------------------------------------------------------------------
// LookupStatic tests
// ---------------------------------------------------------------------------

func TestLookupStaticKnownTags(t *testing.T) {
	known := []string{"div", "span", "p", "a", "h1", "body", "html", "img", "table", "form"}
	for _, tag := range known {
		a, ok := atom.LookupStatic(tag)
		if !ok {
			t.Errorf("LookupStatic(%q) returned not ok", tag)
		}
		if a.String() != tag {
			t.Errorf("LookupStatic(%q) = %q", tag, a.String())
		}
	}
}

func TestLookupStaticUnknown(t *testing.T) {
	_, ok := atom.LookupStatic("custom-element")
	if ok {
		t.Error("LookupStatic should return false for unknown tag")
	}
}

func TestLookupStaticCaseSensitive(t *testing.T) {
	// HTML tags are lowercase in our atom table.
	_, ok := atom.LookupStatic("DIV")
	if ok {
		t.Error("LookupStatic should be case-sensitive (uppercase should miss)")
	}
}

func TestLookupStaticAttr(t *testing.T) {
	known := []string{"id", "class", "href", "src", "style", "type", "name"}
	for _, attr := range known {
		a, ok := atom.LookupStatic(attr)
		if !ok {
			t.Errorf("LookupStatic(%q) returned not ok for attribute", attr)
		}
		if a.String() != attr {
			t.Errorf("LookupStatic(%q) = %q", attr, a.String())
		}
	}
}

// ---------------------------------------------------------------------------
// Dynamic table tests (TDD — these will fail until we implement)
// ---------------------------------------------------------------------------

func TestNewTable(t *testing.T) {
	tbl := atom.NewTable(64, 1024)
	if tbl == nil {
		t.Fatal("NewTable returned nil")
	}
	if tbl.Len() != 0 {
		t.Errorf("new table Len() = %d, want 0", tbl.Len())
	}
}

func TestTableInternAndLookup(t *testing.T) {
	tbl := atom.NewTable(64, 4096)

	a := tbl.Intern("my-class")
	if a == 0 {
		t.Fatal("Intern returned zero atom")
	}
	if a.IsStatic() {
		t.Error("dynamic intern should not return a static atom")
	}
	if got := tbl.Lookup("my-class"); got != a {
		t.Errorf("Lookup(%q) = %d, want %d", "my-class", got, a)
	}
	// Use table's LookupByAtom for dynamic atoms (String() uses default table).
	if got := tbl.LookupByAtom(a); got != "my-class" {
		t.Errorf("LookupByAtom(%d) = %q, want %q", a, got, "my-class")
	}
}

func TestTableInternDeduplication(t *testing.T) {
	tbl := atom.NewTable(64, 4096)

	a1 := tbl.Intern("shared-class")
	a2 := tbl.Intern("shared-class")
	if a1 != a2 {
		t.Errorf("Intern should deduplicate: got %d and %d for same string", a1, a2)
	}
}

func TestTableInternStaticReturnsStaticAtom(t *testing.T) {
	tbl := atom.NewTable(64, 4096)

	// Interning a known static string should return the static atom.
	a := tbl.Intern("div")
	if a != atom.AtomDiv {
		t.Errorf("Intern(%q) = %d, want static AtomDiv=%d", "div", a, atom.AtomDiv)
	}
}

func TestTableInternEmptyString(t *testing.T) {
	tbl := atom.NewTable(64, 4096)
	a := tbl.Intern("")
	if a != 0 {
		t.Errorf("Intern(%q) = %d, want 0", "", a)
	}
}

func TestTableInternOversized(t *testing.T) {
	tbl := atom.NewTable(64, 100) // max string bytes = 100

	big := strings.Repeat("x", 101)
	a := tbl.Intern(big)
	if a != 0 {
		t.Errorf("Intern of oversized string = %d, want 0 (rejected)", a)
	}
	// Lookup should also return zero.
	if got := tbl.Lookup(big); got != 0 {
		t.Errorf("Lookup of oversized string = %d, want 0", got)
	}
}

func TestTableInternExactSizeLimit(t *testing.T) {
	tbl := atom.NewTable(64, 100)
	exact := strings.Repeat("y", 100)
	a := tbl.Intern(exact)
	if a == 0 {
		t.Error("Intern of string at exact size limit should succeed")
	}
	if got := tbl.LookupByAtom(a); got != exact {
		t.Errorf("LookupByAtom mismatch for exact-limit string")
	}
}

func TestTableEviction(t *testing.T) {
	// Create a small table that can hold only a few entries.
	tbl := atom.NewTable(4, 1024) // maxEntries=4

	// Fill the table.
	atoms := make([]atom.Atom, 4)
	for i := 0; i < 4; i++ {
		atoms[i] = tbl.Intern(string(rune('a' + rune(i))))
	}

	// All should be present.
	for i := 0; i < 4; i++ {
		s := string(rune('a' + rune(i)))
		if got := tbl.Lookup(s); got != atoms[i] {
			t.Errorf("Lookup(%q) = %d, want %d before eviction", s, got, atoms[i])
		}
	}

	// Adding one more should evict the LRU entry.
	_ = tbl.Intern("extra")

	if tbl.Len() > 4 {
		t.Errorf("table Len() = %d after eviction, want <= 4", tbl.Len())
	}
}

func TestTableEvictionOrder(t *testing.T) {
	tbl := atom.NewTable(3, 4096) // maxEntries=3

	_ = tbl.Intern("first")
	_ = tbl.Intern("second")
	_ = tbl.Intern("third")

	// Access "first" to make it recently used.
	_ = tbl.Lookup("first")

	// Adding "fourth" should evict "second" (least recently used).
	_ = tbl.Intern("fourth")

	if got := tbl.Lookup("first"); got == 0 {
		t.Error("\"first\" should survive eviction (recently accessed)")
	}
	if got := tbl.Lookup("second"); got != 0 {
		t.Error("\"second\" should have been evicted (LRU)")
	}
	if got := tbl.Lookup("third"); got == 0 {
		t.Error("\"third\" should survive eviction")
	}
	if got := tbl.Lookup("fourth"); got == 0 {
		t.Error("\"fourth\" should be present")
	}
}

func TestTableLookupMiss(t *testing.T) {
	tbl := atom.NewTable(64, 4096)
	if got := tbl.Lookup("nonexistent"); got != 0 {
		t.Errorf("Lookup of missing key = %d, want 0", got)
	}
}

func TestTableBytesUsed(t *testing.T) {
	tbl := atom.NewTable(64, 4096)
	if tbl.BytesUsed() != 0 {
		t.Errorf("empty table BytesUsed() = %d, want 0", tbl.BytesUsed())
	}

	_ = tbl.Intern("hello") // 5 bytes
	if got := tbl.BytesUsed(); got != 5 {
		t.Errorf("BytesUsed() = %d, want 5", got)
	}

	_ = tbl.Intern("world!") // 6 bytes
	if got := tbl.BytesUsed(); got != 11 {
		t.Errorf("BytesUsed() = %d, want 11", got)
	}
}

func TestTableReset(t *testing.T) {
	tbl := atom.NewTable(64, 4096)
	_ = tbl.Intern("alpha")
	_ = tbl.Intern("beta")

	tbl.Reset()
	if tbl.Len() != 0 {
		t.Errorf("after Reset, Len() = %d, want 0", tbl.Len())
	}
	if tbl.BytesUsed() != 0 {
		t.Errorf("after Reset, BytesUsed() = %d, want 0", tbl.BytesUsed())
	}
	// Dynamic atoms should be gone.
	if got := tbl.Lookup("alpha"); got != 0 {
		t.Errorf("after Reset, Lookup(%q) = %d, want 0", "alpha", got)
	}
}

// ---------------------------------------------------------------------------
// Concurrency tests
// ---------------------------------------------------------------------------

func TestTableConcurrentIntern(t *testing.T) {
	tbl := atom.NewTable(256, 65536)
	const goroutines = 8
	const perGoroutine = 100

	var wg sync.WaitGroup
	results := make([][]atom.Atom, goroutines)

	for g := 0; g < goroutines; g++ {
		g := g
		results[g] = make([]atom.Atom, perGoroutine)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				s := "shared-key"
				results[g][i] = tbl.Intern(s)
			}
		}()
	}
	wg.Wait()

	// All goroutines interning the same string must get the same atom.
	expected := results[0][0]
	for g := 0; g < goroutines; g++ {
		for i := 0; i < perGoroutine; i++ {
			if results[g][i] != expected {
				t.Fatalf("goroutine %d iter %d: got atom %d, want %d", g, i, results[g][i], expected)
			}
		}
	}
}

func TestTableConcurrentMixed(t *testing.T) {
	tbl := atom.NewTable(256, 65536)
	var wg sync.WaitGroup

	// Writers.
	for g := 0; g < 4; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s := string(rune('A'+rune(g))) + strings.Repeat("x", i%10)
				tbl.Intern(s)
			}
		}()
	}
	// Readers.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				tbl.Lookup("shared-key")
				tbl.Len()
				tbl.BytesUsed()
			}
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// DefaultTable tests
// ---------------------------------------------------------------------------

func TestDefaultTableInternAndLookup(t *testing.T) {
	// Reset default table to avoid cross-test pollution.
	atom.ResetDefault()

	a := atom.Intern("test-class")
	if a == 0 {
		t.Fatal("default Intern returned zero")
	}
	if got := atom.Lookup("test-class"); got != a {
		t.Errorf("default Lookup = %d, want %d", got, a)
	}
}

func TestDefaultTableReset(t *testing.T) {
	atom.ResetDefault()
	_ = atom.Intern("before-reset")
	atom.ResetDefault()
	if got := atom.Lookup("before-reset"); got != 0 {
		t.Errorf("after ResetDefault, Lookup = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestAtomStringOutOfBounds(t *testing.T) {
	// An atom with an invalid index should return empty string.
	bad := atom.Atom(999999)
	if got := bad.String(); got != "" {
		t.Errorf("out-of-bounds Atom.String() = %q, want empty", got)
	}
}

func TestTableInternNilTable(t *testing.T) {
	var tbl *atom.Table
	// Methods on nil Table should not panic.
	if got := tbl.Intern("x"); got != 0 {
		t.Errorf("nil Table.Intern = %d, want 0", got)
	}
	if got := tbl.Lookup("x"); got != 0 {
		t.Errorf("nil Table.Lookup = %d, want 0", got)
	}
	if got := tbl.Len(); got != 0 {
		t.Errorf("nil Table.Len = %d, want 0", got)
	}
	if got := tbl.BytesUsed(); got != 0 {
		t.Errorf("nil Table.BytesUsed = %d, want 0", got)
	}
	tbl.Reset() // should not panic
}

func TestStaticCountReasonable(t *testing.T) {
	// Sanity check: we should have a reasonable number of static atoms.
	count := int(atom.StaticEnd) - 1
	if count < 50 {
		t.Errorf("expected at least 50 static atoms, got %d", count)
	}
	if count > 1000 {
		t.Errorf("too many static atoms: %d", count)
	}
}
