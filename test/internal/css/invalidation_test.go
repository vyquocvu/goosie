package css_test

import (
	"testing"
	"github.com/vyquocvu/goosie/internal/css"
)

// --- test helpers ---

// testElement is defined in specificity_test.go and reused here.

func newTestElem(tag string) *testElement {
	return &testElement{tag: tag, attrs: make(map[string]string)}
}

// --- MutationType tests ---

func TestMutationTypeConstants(t *testing.T) {
	// Verify all mutation types are distinct
	types := []css.MutationType{
		css.MutationClassChange,
		css.MutationIDChange,
		css.MutationAttributeChange,
		css.MutationInlineStyleChange,
		css.MutationTextChange,
		css.MutationInsertion,
		css.MutationRemoval,
	}
	seen := make(map[css.MutationType]bool)
	for _, mt := range types {
		if seen[mt] {
			t.Errorf("duplicate mutation type: %v", mt)
		}
		seen[mt] = true
	}
}

// --- Mutation tests ---

func TestMutationCreation(t *testing.T) {
	elem := newTestElem("div")
	elem.id = "main"
	elem.classes = []string{"container"}

	m := css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "container",
		NewAttr: "wrapper",
	}

	if m.Type != css.MutationClassChange {
		t.Errorf("expected MutationClassChange, got %v", m.Type)
	}
	if m.Target != elem {
		t.Error("target mismatch")
	}
}

// --- StyleInvalidator construction ---

func TestNewStyleInvalidator(t *testing.T) {
	sheet := &css.CompiledStyleSheet{
		IDBucket:    make(map[string][]int),
		ClassBucket: make(map[string][]int),
		TagBucket:   make(map[string][]int),
		AttrBucket:  make(map[string][]int),
	}
	inv := css.NewStyleInvalidator(sheet)
	if inv == nil {
		t.Fatal("NewStyleInvalidator returned nil")
	}
	if inv.Sheet != sheet {
		t.Error("sheet not set correctly")
	}
}

func TestNewStyleInvalidatorNilSheet(t *testing.T) {
	inv := css.NewStyleInvalidator(nil)
	if inv == nil {
		t.Fatal("should handle nil sheet")
	}
}

// --- Class change invalidation ---

func TestClassChangeInvalidatesMatchingRules(t *testing.T) {
	// Create a stylesheet with .highlight { color: red }
	sheet := makeTestStyleSheet(t, `.highlight { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("p")
	elem.classes = []string{"highlight"}

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "highlight",
		NewAttr: "highlight",
	})

	if !result.Self {
		t.Error("class change should invalidate self")
	}
}

func TestClassRemovalInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `.active { color: green; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{}

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "active",
		NewAttr: "",
	})

	if !result.Self {
		t.Error("removing a class should invalidate self")
	}
}

func TestClassAdditionInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `.new-class { font-size: 20px; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"new-class"}

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "",
		NewAttr: "new-class",
	})

	if !result.Self {
		t.Error("adding a class should invalidate self")
	}
}

// --- ID change invalidation ---

func TestIDChangeInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `#main { color: blue; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.id = "main"

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationIDChange,
		Target:  elem,
		OldAttr: "old-id",
		NewAttr: "main",
	})

	if !result.Self {
		t.Error("ID change should invalidate self")
	}
}

// --- Attribute change invalidation ---

func TestAttributeChangeInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `[type="text"] { border: 1px solid; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("input")
	elem.attrs["type"] = "text"

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationAttributeChange,
		Target:  elem,
		OldAttr: "type",
		NewAttr: "text",
	})

	if !result.Self {
		t.Error("attribute change should invalidate self")
	}
}

// --- Inline style change ---

func TestInlineStyleChangeInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `div { color: black; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationInlineStyleChange,
		Target: elem,
	})

	if !result.Self {
		t.Error("inline style change should invalidate self")
	}
}

// --- Text change invalidation ---

func TestTextChangeInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `p { color: black; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("p")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationTextChange,
		Target: elem,
	})

	// Text change does not affect style, only layout
	if result.Self {
		t.Error("text change should not invalidate style on self")
	}
	if !result.LayoutDirty {
		t.Error("text change should mark layout dirty")
	}
}

// --- Insertion invalidation ---

func TestInsertionInvalidatesSelfAndRelatives(t *testing.T) {
	sheet := makeTestStyleSheet(t, `
		p + span { color: red; }
		div > p { color: blue; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	parent := newTestElem("div")
	child := newTestElem("p")
	child.parent = parent
	parent.children = append(parent.children, child)

	newElem := newTestElem("span")
	newElem.parent = parent

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationInsertion,
		Target: newElem,
		Parent: parent,
	})

	if !result.Self {
		t.Error("insertion should invalidate self")
	}
}

// --- Removal invalidation ---

func TestRemovalInvalidatesSelf(t *testing.T) {
	sheet := makeTestStyleSheet(t, `p { color: black; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("p")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationRemoval,
		Target: elem,
	})

	if !result.Self {
		t.Error("removal should invalidate self")
	}
}

// --- Descendant invalidation for inherited property changes ---

func TestInheritedPropertyChangeInvalidatesDescendants(t *testing.T) {
	// A class change that affects an inherited property (color) should
	// invalidate all descendants
	sheet := makeTestStyleSheet(t, `.theme { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"theme"}
	child := newTestElem("p")
	child.parent = elem
	elem.children = append(elem.children, child)
	grandchild := newTestElem("span")
	grandchild.parent = child
	child.children = append(child.children, grandchild)

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "theme",
		NewAttr: "theme",
	})

	if !result.InvalidateDescendants {
		t.Error("inherited property change should invalidate descendants")
	}
}

func TestNonInheritedPropertyChangeDoesNotInvalidateDescendants(t *testing.T) {
	sheet := makeTestStyleSheet(t, `.box { margin: 10px; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"box"}
	child := newTestElem("p")
	child.parent = elem
	elem.children = append(elem.children, child)

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "box",
		NewAttr: "box",
	})

	if result.InvalidateDescendants {
		t.Error("non-inherited property change should not invalidate descendants")
	}
}

// --- Sibling invalidation for relational selectors ---

func TestAdjacentSiblingCombinatorInvalidatesSibling(t *testing.T) {
	sheet := makeTestStyleSheet(t, `h1 + p { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("h1")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationInsertion,
		Target: elem,
	})

	if !result.InvalidateNextSibling {
		t.Error("insertion with adjacent sibling selector should invalidate next sibling")
	}
}

func TestGeneralSiblingCombinatorInvalidatesSiblings(t *testing.T) {
	sheet := makeTestStyleSheet(t, `h1 ~ p { color: blue; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("h1")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationInsertion,
		Target: elem,
	})

	if !result.InvalidateFollowingSiblings {
		t.Error("insertion with general sibling selector should invalidate following siblings")
	}
}

// --- Mutation batching ---

func TestBatchMutations(t *testing.T) {
	sheet := makeTestStyleSheet(t, `
		.highlight { color: red; }
		.active { font-weight: bold; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	elem1 := newTestElem("p")
	elem1.classes = []string{"highlight"}
	elem2 := newTestElem("div")
	elem2.classes = []string{"active"}

	inv.BeginBatch()
	inv.RecordMutation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem1,
		OldAttr: "highlight",
		NewAttr: "highlight",
	})
	inv.RecordMutation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem2,
		OldAttr: "active",
		NewAttr: "active",
	})

	result := inv.FlushBatch()

	if !result.Self {
		t.Error("batched mutations should produce combined invalidation")
	}
}

func TestBatchMutationsEmpty(t *testing.T) {
	sheet := makeTestStyleSheet(t, `div { color: black; }`)
	inv := css.NewStyleInvalidator(sheet)

	inv.BeginBatch()
	result := inv.FlushBatch()

	if result.Self || result.InvalidateDescendants {
		t.Error("empty batch should produce no invalidation")
	}
}

func TestBatchMutationsDeduplicatesTargets(t *testing.T) {
	sheet := makeTestStyleSheet(t, `.a { color: red; } .b { color: blue; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"a", "b"}

	inv.BeginBatch()
	// Same element mutated twice
	inv.RecordMutation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "a",
		NewAttr: "a",
	})
	inv.RecordMutation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "b",
		NewAttr: "b",
	})

	result := inv.FlushBatch()
	// Should have exactly one target entry for elem
	if len(result.Targets) != 1 {
		t.Errorf("expected 1 deduplicated target, got %d", len(result.Targets))
	}
}

// --- InvalidationResult ---

func TestInvalidationResultIsEmpty(t *testing.T) {
	r := css.InvalidationResult{}
	if !r.IsEmpty() {
		t.Error("zero-value result should be empty")
	}

	r.Self = true
	if r.IsEmpty() {
		t.Error("result with Self=true should not be empty")
	}
}

// --- Edge cases ---

func TestNilTargetMutation(t *testing.T) {
	sheet := makeTestStyleSheet(t, `div { color: black; }`)
	inv := css.NewStyleInvalidator(sheet)

	result := inv.ComputeInvalidation(css.Mutation{
		Type:   css.MutationClassChange,
		Target: nil,
	})

	if !result.IsEmpty() {
		t.Error("nil target should produce empty invalidation")
	}
}

func TestUnknownClassChangeNoInvalidation(t *testing.T) {
	sheet := makeTestStyleSheet(t, `.known { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "unknown-class",
		NewAttr: "unknown-class",
	})

	if result.Self {
		t.Error("unknown class change should not invalidate self style")
	}
}

func TestUniversalSelectorAlwaysInvalidates(t *testing.T) {
	sheet := makeTestStyleSheet(t, `* { box-sizing: border-box; }`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")

	result := inv.ComputeInvalidation(css.Mutation{
		Type:    css.MutationAttributeChange,
		Target:  elem,
		OldAttr: "data-x",
		NewAttr: "val",
	})

	// Universal selectors match everything
	if !result.Self {
		t.Error("universal selector rules should cause self invalidation")
	}
}

// --- Affected rules analysis ---

func TestAffectedRulesByClassChange(t *testing.T) {
	sheet := makeTestStyleSheet(t, `
		.red { color: red; }
		.blue { color: blue; }
		div { display: block; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"red"}

	affected := inv.AffectedRuleIndices(css.Mutation{
		Type:    css.MutationClassChange,
		Target:  elem,
		OldAttr: "red",
		NewAttr: "red",
	})

	// Should find rules for .red and div (tag match)
	if len(affected) == 0 {
		t.Error("should find affected rules")
	}
}

// --- Inherited property detection ---

func TestDeclarationsContainInheritedProperty(t *testing.T) {
	tests := []struct {
		css       string
		inherited bool
	}{
		{`div { color: red; }`, true},
		{`div { font-size: 16px; }`, true},
		{`div { margin: 10px; }`, false},
		{`div { padding: 5px; }`, false},
		{`div { display: block; }`, false},
		{`div { visibility: hidden; }`, true},
		{`div { opacity: 0.5; }`, true},
	}

	for _, tt := range tests {
		t.Run(tt.css, func(t *testing.T) {
			sheet := parseTestSheet(t, tt.css)
			if len(sheet.Rules) == 0 {
				t.Fatal("no rules parsed")
			}
			got := css.DeclarationsContainInherited(sheet.Rules[0].Declarations)
			if got != tt.inherited {
				t.Errorf("expected inherited=%v, got %v", tt.inherited, got)
			}
		})
	}
}

// --- Relational selector detection ---

func TestHasAdjacentSiblingCombinator(t *testing.T) {
	sheet := makeTestStyleSheet(t, `h1 + p { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	if !inv.HasSiblingCombinator() {
		t.Error("should detect adjacent sibling combinator")
	}
}

func TestHasGeneralSiblingCombinator(t *testing.T) {
	sheet := makeTestStyleSheet(t, `h1 ~ p { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	if !inv.HasSiblingCombinator() {
		t.Error("should detect general sibling combinator")
	}
}

func TestNoSiblingCombinator(t *testing.T) {
	sheet := makeTestStyleSheet(t, `div p { color: red; }`)
	inv := css.NewStyleInvalidator(sheet)

	if inv.HasSiblingCombinator() {
		t.Error("descendant combinator should not be detected as sibling")
	}
}

// --- helpers ---

func makeTestStyleSheet(t *testing.T, cssText string) *css.CompiledStyleSheet {
	t.Helper()
	sheet := parseTestSheet(t, cssText)
	return css.CompileStyleSheet(sheet)
}

func parseTestSheet(t *testing.T, cssText string) *css.StyleSheet {
	t.Helper()
	p := css.NewParser(cssText)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("failed to parse CSS: %v", err)
	}
	return sheet
}

// --- Benchmarks ---

func BenchmarkComputeInvalidation(b *testing.B) {
	sheet := makeBenchStyleSheet(b, `
		.container { max-width: 1200px; margin: 0 auto; }
		.highlight { background-color: yellow; }
		.active { color: red; font-weight: bold; }
		#main { display: block; }
		p { color: black; line-height: 1.5; }
		h1 + p { margin-top: 0; }
		* { box-sizing: border-box; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.id = "main"
	elem.classes = []string{"container", "highlight"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inv.ComputeInvalidation(css.Mutation{
			Type:    css.MutationClassChange,
			Target:  elem,
			OldAttr: "highlight",
			NewAttr: "active",
		})
	}
}

func BenchmarkComputeInvalidationInherited(b *testing.B) {
	sheet := makeBenchStyleSheet(b, `
		.theme { color: red; font-size: 16px; }
		.dark { color: white; background-color: black; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"theme"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inv.ComputeInvalidation(css.Mutation{
			Type:    css.MutationClassChange,
			Target:  elem,
			OldAttr: "theme",
			NewAttr: "dark",
		})
	}
}

func BenchmarkBatchMutations(b *testing.B) {
	sheet := makeBenchStyleSheet(b, `
		.a { color: red; }
		.b { color: blue; }
		.c { font-size: 14px; }
		p { margin: 0; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("p")
	elem.classes = []string{"a", "b", "c"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inv.BeginBatch()
		inv.RecordMutation(css.Mutation{Type: css.MutationClassChange, Target: elem, OldAttr: "a", NewAttr: "a"})
		inv.RecordMutation(css.Mutation{Type: css.MutationClassChange, Target: elem, OldAttr: "b", NewAttr: "b"})
		inv.RecordMutation(css.Mutation{Type: css.MutationInlineStyleChange, Target: elem})
		inv.FlushBatch()
	}
}

func BenchmarkAffectedRuleIndices(b *testing.B) {
	sheet := makeBenchStyleSheet(b, `
		.container { max-width: 1200px; }
		.highlight { background: yellow; }
		.active { color: red; }
		#main { display: block; }
		p { color: black; }
		div { margin: 0; }
		* { box-sizing: border-box; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	elem := newTestElem("div")
	elem.classes = []string{"container"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inv.AffectedRuleIndices(css.Mutation{
			Type:    css.MutationClassChange,
			Target:  elem,
			OldAttr: "container",
			NewAttr: "container",
		})
	}
}

func BenchmarkHasSiblingCombinator(b *testing.B) {
	sheet := makeBenchStyleSheet(b, `
		h1 + p { color: red; }
		h2 ~ p { color: blue; }
		div p { color: black; }
	`)
	inv := css.NewStyleInvalidator(sheet)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		inv.HasSiblingCombinator()
	}
}

func makeBenchStyleSheet(b *testing.B, cssText string) *css.CompiledStyleSheet {
	b.Helper()
	p := css.NewParser(cssText)
	sheet, err := p.Parse()
	if err != nil {
		b.Fatalf("failed to parse CSS: %v", err)
	}
	return css.CompileStyleSheet(sheet)
}
