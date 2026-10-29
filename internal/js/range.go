package js

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// This file implements the DOM Range and Selection APIs.
//
// Range represents a contiguous region in a DOM tree defined by a start
// boundary point (container + offset) and an end boundary point. It provides
// methods to manipulate those boundary points and to extract, clone, delete,
// or insert content within the range.
//
// Selection represents the text selection in a document. It holds a list of
// Ranges (usually 0 or 1) and provides methods to inspect and modify the
// selection.

// ---------------------------------------------------------------------------
// Range
// ---------------------------------------------------------------------------

// rangeBoundary is one endpoint of a Range: a node and an integer offset into
// that node. For element nodes the offset is a child index; for text/comment
// nodes it is a character offset.
type rangeBoundary struct {
	container *dom.Node
	offset    int
}

// jsRange is the Go state behind one JS Range object. The JS wrapper stores a
// pointer to this struct in its __range__ field.
type jsRange struct {
	start rangeBoundary
	end   rangeBoundary
}

func (r *Runtime) registerLiveRange(ran *jsRange) {
	if ran == nil {
		return
	}
	if r.liveRanges == nil {
		r.liveRanges = make(map[*jsRange]struct{})
	}
	r.liveRanges[ran] = struct{}{}
}

func (r *Runtime) adjustLiveRangesForCharacterData(node *dom.Node, offset, end, insertedLength int) {
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	visited := make(map[*Runtime]bool)
	var visit func(*Runtime)
	visit = func(runtime *Runtime) {
		if runtime == nil || visited[runtime] {
			return
		}
		visited[runtime] = true
		for ran := range runtime.liveRanges {
			adjust := func(boundary *rangeBoundary) {
				if boundary.container != node {
					return
				}
				switch {
				case boundary.offset > offset && boundary.offset <= end:
					boundary.offset = offset
				case boundary.offset > end:
					boundary.offset += insertedLength - (end - offset)
				}
			}
			adjust(&ran.start)
			adjust(&ran.end)
		}
		for _, child := range runtime.iframeChildRuntimes {
			visit(child)
		}
	}
	visit(root)
}

func (r *Runtime) adjustLiveRangesForNodeRemoval(parent, removed *dom.Node, index int) {
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	visited := make(map[*Runtime]bool)
	var visit func(*Runtime)
	visit = func(runtime *Runtime) {
		if runtime == nil || visited[runtime] {
			return
		}
		visited[runtime] = true
		for ran := range runtime.liveRanges {
			adjust := func(boundary *rangeBoundary) {
				if boundary.container == parent {
					if boundary.offset > index {
						boundary.offset--
					}
					return
				}
				for node := boundary.container; node != nil; node = node.Parent {
					if node == removed {
						boundary.container = parent
						boundary.offset = index
						return
					}
				}
			}
			adjust(&ran.start)
			adjust(&ran.end)
		}
		for _, child := range runtime.iframeChildRuntimes {
			visit(child)
		}
	}
	visit(root)
}

// setupRange installs document.createRange() and the Range prototype methods
// on the document object. Called from setupDOM.
func (r *Runtime) setupRange() {
	if r.domDoc == nil {
		return
	}
	r.installStaticRangeConstructor()
	rangeProto := r.vm.NewObject()
	constructor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		return r.jsCreateRangeForDocument(r.domDoc)
	}).(*goja.Object)
	if ok {
		_ = constructor.Set("prototype", rangeProto)
		_ = rangeProto.Set("constructor", constructor)
		for name, value := range map[string]int{
			"START_TO_START": 0, "START_TO_END": 1, "END_TO_END": 2, "END_TO_START": 3,
		} {
			_ = constructor.Set(name, value)
			_ = rangeProto.Set(name, value)
		}
		_ = r.vm.Set("Range", constructor)
	}

	doc := r.docObj()

	// document.createRange()
	_ = doc.Set("createRange", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateRange()
	})
}

// jsCreateRange constructs a new Range object whose start and end are both
// collapsed to the document node, matching the DOM Standard's "create a range"
// algorithm (startContainer = endContainer = document, startOffset = endOffset = 0).
func (r *Runtime) jsCreateRange() *goja.Object {
	return r.jsCreateRangeForDocument(r.domDoc)
}

func (r *Runtime) jsCreateRangeForDocument(doc *dom.Document) *goja.Object {
	if doc == nil {
		return r.vm.NewObject()
	}
	ran := &jsRange{
		start: rangeBoundary{container: &doc.Node, offset: 0},
		end:   rangeBoundary{container: &doc.Node, offset: 0},
	}
	r.registerLiveRange(ran)
	obj := r.vm.NewObject()
	if constructor := r.vm.Get("Range"); constructor != nil && !goja.IsUndefined(constructor) {
		if proto := constructor.ToObject(r.vm).Get("prototype"); proto != nil {
			if prototype, ok := proto.(*goja.Object); ok {
				_ = obj.SetPrototype(prototype)
			}
		}
	}
	_ = obj.Set("__range__", ran)
	r.installRangeMethods(obj, ran)
	return obj
}

// getRange extracts the *jsRange from a JS Range wrapper. Returns nil if the
// object is not a Range.
func getRange(obj *goja.Object) *jsRange {
	if obj == nil {
		return nil
	}
	v := obj.Get("__range__")
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	if exp := v.Export(); exp != nil {
		if ran, ok := exp.(*jsRange); ok {
			return ran
		}
	}
	return nil
}

// installRangeMethods sets the accessors and methods on one Range wrapper.
func (r *Runtime) installRangeMethods(obj *goja.Object, ran *jsRange) {
	// startContainer
	_ = obj.Set("__getStartContainer__", func() goja.Value {
		return r.wrapNode(ran.start.container)
	})
	// startOffset
	_ = obj.Set("__getStartOffset__", func() int {
		return ran.start.offset
	})
	// endContainer
	_ = obj.Set("__getEndContainer__", func() goja.Value {
		return r.wrapNode(ran.end.container)
	})
	// endOffset
	_ = obj.Set("__getEndOffset__", func() int {
		return ran.end.offset
	})
	// collapsed
	_ = obj.Set("__getCollapsed__", func() bool {
		return ran.start.container == ran.end.container && ran.start.offset == ran.end.offset
	})
	// commonAncestorContainer
	_ = obj.Set("__getCommonAncestorContainer__", func() goja.Value {
		return r.wrapNode(commonAncestor(ran.start.container, ran.end.container))
	})

	// Methods
	_ = obj.Set("setStart", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSetStart(call, ran)
	})
	_ = obj.Set("setEnd", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSetEnd(call, ran)
	})
	_ = obj.Set("setStartBefore", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSetStartBefore(call, ran)
	})
	_ = obj.Set("setStartAfter", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSetStartAfter(call, ran)
	})
	_ = obj.Set("setEndBefore", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSetEndBefore(call, ran)
	})
	_ = obj.Set("setEndAfter", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSetEndAfter(call, ran)
	})
	_ = obj.Set("collapse", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeCollapse(call, ran)
	})
	_ = obj.Set("selectNode", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSelectNode(call, ran)
	})
	_ = obj.Set("selectNodeContents", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSelectNodeContents(call, ran)
	})
	_ = obj.Set("cloneRange", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeCloneRange(ran)
	})
	_ = obj.Set("detach", func(call goja.FunctionCall) goja.Value {
		// detach() is a no-op in modern browsers but must exist.
		return goja.Undefined()
	})
	_ = obj.Set("deleteContents", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeDeleteContents(ran)
	})
	_ = obj.Set("extractContents", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeExtractContents(ran)
	})
	_ = obj.Set("cloneContents", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeCloneContents(ran)
	})
	_ = obj.Set("createContextualFragment", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeCreateContextualFragment(call, ran)
	})
	_ = obj.Set("insertNode", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeInsertNode(call, ran)
	})
	_ = obj.Set("surroundContents", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeSurroundContents(call, ran)
	})
	_ = obj.Set("compareBoundaryPoints", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeCompareBoundaryPoints(call, ran)
	})
	_ = obj.Set("isPointInRange", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeIsPointInRange(call, ran)
	})
	_ = obj.Set("comparePoint", func(call goja.FunctionCall) goja.Value {
		return r.jsRangeComparePoint(call, ran)
	})
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(rangeToString(ran))
	})

	// Define JS getters for the read-only properties.
	_ = r.vm.Set("__rangeTarget__", obj)
	_, _ = r.vm.RunString(`
		(function() {
			var o = __rangeTarget__;
			Object.defineProperty(o, 'startContainer', {
				get: function() { return o.__getStartContainer__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'startOffset', {
				get: function() { return o.__getStartOffset__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'endContainer', {
				get: function() { return o.__getEndContainer__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'endOffset', {
				get: function() { return o.__getEndOffset__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'collapsed', {
				get: function() { return o.__getCollapsed__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'commonAncestorContainer', {
				get: function() { return o.__getCommonAncestorContainer__(); },
				configurable: true, enumerable: true
			});
		})();
	`)
	_ = r.vm.Set("__rangeTarget__", nil)
}

// jsRangeCreateContextualFragment parses markup using the Range's start
// container as the HTML fragment context, falling back to the owner document's
// body when the boundary is not inside an element.
func (r *Runtime) jsRangeCreateContextualFragment(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'createContextualFragment': 1 argument required"))
	}
	if ran == nil || ran.start.container == nil || ran.start.container.Doc == nil {
		panic(r.vm.NewTypeError("Failed to execute 'createContextualFragment': Range has no owner document"))
	}
	context := ran.start.container
	if !context.Element() {
		context = context.Parent
	}
	if context == nil || !context.Element() {
		doc := ran.start.container.Doc
		context = doc.Body
		if context == nil {
			context = doc.HTML
		}
	}
	if context == nil || !context.Element() {
		panic(r.domError("InvalidStateError", "The Range has no element context for fragment parsing"))
	}
	parseContext := *context
	isXML := context.Doc != nil && context.Doc.IsXML
	if !isXML {
		// The HTML fragment algorithm treats the document's <html> and <body>
		// contexts equivalently. A foreign-namespace element with one of those
		// names follows the generic fragment path instead.
		if parseContext.Namespace == dom.NSHTML && parseContext.Data == "html" {
			parseContext.Data = "body"
		} else if parseContext.Namespace != dom.NSHTML && parseContext.Data == "html" {
			parseContext.Data = "div"
		}
		if parseContext.Namespace == dom.NSSVG && parseContext.Data == "div" {
			parseContext.Namespace = dom.NSHTML
			parseContext.NamespaceURI = ""
		}
	}
	limits := dom.ParseLimits{
		Nodes: 4096, Depth: 64, Attributes: 256, AttributeBytes: 8192,
	}
	var fragment *dom.Node
	var err error
	if isXML {
		fragment, err = ParseXMLFragmentBounded(&parseContext, call.Arguments[0].String(), limits)
	} else {
		fragment, err = dom.ParseFragmentBounded(&parseContext, call.Arguments[0].String(), limits)
	}
	if err != nil || fragment == nil {
		panic(r.domError("SyntaxError", "The contextual fragment could not be parsed"))
	}
	fragment.AdoptInto(context.Doc)
	registry := (*CustomElementRegistry)(nil)
	preserveNullRegistry := true
	if !context.IsHTMLTemplate() {
		registry = r.customElementRegistryForNode(context)
		r.associateScopedRegistryWithDocument(registry, context.Doc)
	}
	r.associateParsedSubtreeWithRegistry(fragment, registry, preserveNullRegistry)
	return r.wrapNode(fragment)
}

// ---------------------------------------------------------------------------
// Range boundary setters
// ---------------------------------------------------------------------------

func (r *Runtime) jsRangeSetStart(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'setStart': 2 arguments required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setStart': parameter 1 is not of type 'Node'"))
	}
	offset := int(call.Arguments[1].ToInteger())
	ran.start = rangeBoundary{container: node, offset: offset}
	// If the new start is after the end, collapse to start.
	if compareBoundaryPoints(ran.start, ran.end) > 0 {
		ran.end = ran.start
	}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSetEnd(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'setEnd': 2 arguments required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setEnd': parameter 1 is not of type 'Node'"))
	}
	offset := int(call.Arguments[1].ToInteger())
	ran.end = rangeBoundary{container: node, offset: offset}
	// If the new end is before the start, collapse to end.
	if compareBoundaryPoints(ran.end, ran.start) < 0 {
		ran.start = ran.end
	}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSetStartBefore(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'setStartBefore': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setStartBefore': parameter 1 is not of type 'Node'"))
	}
	if node.Parent == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setStartBefore': Node has no parent"))
	}
	offset := childIndex(node.Parent, node)
	ran.start = rangeBoundary{container: node.Parent, offset: offset}
	if compareBoundaryPoints(ran.start, ran.end) > 0 {
		ran.end = ran.start
	}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSetStartAfter(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'setStartAfter': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setStartAfter': parameter 1 is not of type 'Node'"))
	}
	if node.Parent == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setStartAfter': Node has no parent"))
	}
	offset := childIndex(node.Parent, node) + 1
	ran.start = rangeBoundary{container: node.Parent, offset: offset}
	if compareBoundaryPoints(ran.start, ran.end) > 0 {
		ran.end = ran.start
	}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSetEndBefore(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'setEndBefore': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setEndBefore': parameter 1 is not of type 'Node'"))
	}
	if node.Parent == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setEndBefore': Node has no parent"))
	}
	offset := childIndex(node.Parent, node)
	ran.end = rangeBoundary{container: node.Parent, offset: offset}
	if compareBoundaryPoints(ran.end, ran.start) < 0 {
		ran.start = ran.end
	}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSetEndAfter(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'setEndAfter': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setEndAfter': parameter 1 is not of type 'Node'"))
	}
	if node.Parent == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setEndAfter': Node has no parent"))
	}
	offset := childIndex(node.Parent, node) + 1
	ran.end = rangeBoundary{container: node.Parent, offset: offset}
	if compareBoundaryPoints(ran.end, ran.start) < 0 {
		ran.start = ran.end
	}
	return goja.Undefined()
}

// ---------------------------------------------------------------------------
// Range collapse / select
// ---------------------------------------------------------------------------

func (r *Runtime) jsRangeCollapse(call goja.FunctionCall, ran *jsRange) goja.Value {
	toStart := true
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
		toStart = call.Arguments[0].ToBoolean()
	}
	if toStart {
		ran.end = ran.start
	} else {
		ran.start = ran.end
	}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSelectNode(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'selectNode': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'selectNode': parameter 1 is not of type 'Node'"))
	}
	if node.Parent == nil {
		panic(r.vm.NewTypeError("Failed to execute 'selectNode': Node has no parent"))
	}
	idx := childIndex(node.Parent, node)
	ran.start = rangeBoundary{container: node.Parent, offset: idx}
	ran.end = rangeBoundary{container: node.Parent, offset: idx + 1}
	return goja.Undefined()
}

func (r *Runtime) jsRangeSelectNodeContents(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'selectNodeContents': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'selectNodeContents': parameter 1 is not of type 'Node'"))
	}
	length := nodeChildCount(node)
	ran.start = rangeBoundary{container: node, offset: 0}
	ran.end = rangeBoundary{container: node, offset: length}
	return goja.Undefined()
}

func (r *Runtime) jsRangeCloneRange(ran *jsRange) *goja.Object {
	clone := &jsRange{
		start: rangeBoundary{container: ran.start.container, offset: ran.start.offset},
		end:   rangeBoundary{container: ran.end.container, offset: ran.end.offset},
	}
	r.registerLiveRange(clone)
	obj := r.vm.NewObject()
	_ = obj.Set("__range__", clone)
	r.installRangeMethods(obj, clone)
	return obj
}

// ---------------------------------------------------------------------------
// Range content manipulation
// ---------------------------------------------------------------------------

func (r *Runtime) jsRangeDeleteContents(ran *jsRange) goja.Value {
	if ran.start.container == ran.end.container && ran.start.offset == ran.end.offset {
		return goja.Undefined() // collapsed
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	root := commonAncestor(start.container, end.container)
	if root == nil {
		return goja.Undefined()
	}
	if isRangeCharacterData(root) {
		units := characterDataCodeUnits(root)
		from, to := start.offset, end.offset
		if from < 0 {
			from = 0
		}
		if from > len(units) {
			from = len(units)
		}
		if to < from {
			to = from
		}
		if to > len(units) {
			to = len(units)
		}
		r.replaceCharacterData(root, from, to-from, "")
		collapse := rangeBoundary{container: root, offset: from}
		ran.start, ran.end = collapse, collapse
		return goja.Undefined()
	}
	collapse := rangeContentsCollapsePoint(start, end)
	ran.start, ran.end = collapse, collapse
	var fullyContained []*dom.Node
	type partialCharacterData struct {
		node     *dom.Node
		from, to int
	}
	var textNodes []partialCharacterData
	var collect func(*dom.Node)
	collect = func(node *dom.Node) {
		if node == nil {
			return
		}
		if fullyContainedByRange(node, start, end) {
			fullyContained = append(fullyContained, node)
			return
		}
		if isRangeCharacterData(node) {
			units := characterDataCodeUnits(node)
			textStart := rangeBoundary{container: node, offset: 0}
			textEnd := rangeBoundary{container: node, offset: len(units)}
			if compareBoundaryPoints(textEnd, start) > 0 && compareBoundaryPoints(textStart, end) < 0 {
				from, to := 0, len(units)
				if node == start.container {
					from = start.offset
				}
				if node == end.container {
					to = end.offset
				}
				if from < 0 {
					from = 0
				}
				if from > len(units) {
					from = len(units)
				}
				if to < from {
					to = from
				}
				if to > len(units) {
					to = len(units)
				}
				if from < to {
					textNodes = append(textNodes, partialCharacterData{node: node, from: from, to: to})
				}
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		collect(child)
	}
	for _, node := range fullyContained {
		if node.Parent != nil {
			r.rangeRemoveNode(node)
		}
	}
	for _, partial := range textNodes {
		r.replaceCharacterData(partial.node, partial.from, partial.to-partial.from, "")
		if partial.node == start.container {
			collapse = rangeBoundary{container: partial.node, offset: partial.from}
		}
	}
	r.notifyMutation()
	return goja.Undefined()
}

func rangeContentsCollapsePoint(start, end rangeBoundary) rangeBoundary {
	if isAncestorNode(start.container, end.container) {
		return start
	}
	reference := start.container
	for reference != nil && reference.Parent != nil && !isAncestorNode(reference.Parent, end.container) {
		reference = reference.Parent
	}
	if reference == nil || reference.Parent == nil {
		return start
	}
	parent := reference.Parent
	return rangeBoundary{container: parent, offset: childIndex(parent, reference) + 1}
}

func fullyContainedByRange(node *dom.Node, start, end rangeBoundary) bool {
	if node == nil || node.Parent == nil {
		return false
	}
	index := childIndex(node.Parent, node)
	before := rangeBoundary{container: node.Parent, offset: index}
	after := rangeBoundary{container: node.Parent, offset: index + 1}
	return compareBoundaryPoints(start, before) <= 0 && compareBoundaryPoints(after, end) <= 0
}

type selectedCellPlaceholder struct {
	row   *dom.Node
	index int
	tag   string
}

func selectedTableCellPlaceholders(root *dom.Node, start, end rangeBoundary) []selectedCellPlaceholder {
	if root == nil || compareBoundaryPoints(start, end) >= 0 {
		return nil
	}
	var cells []selectedCellPlaceholder
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && (strings.EqualFold(node.Data, "td") || strings.EqualFold(node.Data, "th")) &&
			node.Parent != nil && strings.EqualFold(node.Parent.Data, "tr") && fullyContainedByRange(node, start, end) {
			cells = append(cells, selectedCellPlaceholder{row: node.Parent, index: childIndex(node.Parent, node), tag: strings.ToLower(node.Data)})
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return cells
}

func restoreSelectedTableCellPlaceholders(r *Runtime, cells []selectedCellPlaceholder) {
	if r == nil || len(cells) == 0 {
		return
	}
	sort.SliceStable(cells, func(i, j int) bool {
		return cells[i].index > cells[j].index
	})
	for _, selected := range cells {
		row := selected.row
		if row == nil || !row.IsConnected() || row.Doc == nil {
			continue
		}
		cell := row.Doc.NewElement(selected.tag)
		cell.AppendChild(row.Doc.NewElement("br"))
		index := selected.index
		if count := nodeChildCount(row); index > count {
			index = count
		}
		row.InsertNode(cell, childAt(row, index))
		r.notifyMutation()
	}
}

func (r *Runtime) jsRangeExtractContents(ran *jsRange) goja.Value {
	frag, _, changed := r.rangeContents(ran, true, r.domDoc)
	if changed {
		r.notifyMutation()
	}
	return r.wrapNode(frag)
}

func (r *Runtime) jsRangeCloneContents(ran *jsRange) goja.Value {
	frag, _, _ := r.rangeContents(ran, false, r.domDoc)
	return r.wrapNode(frag)
}

func (r *Runtime) rangeContents(ran *jsRange, extract bool, fallbackDoc *dom.Document) (*dom.Node, rangeBoundary, bool) {
	doc := fallbackDoc
	if ran != nil && ran.start.container != nil && ran.start.container.Doc != nil {
		doc = ran.start.container.Doc
	}
	if doc == nil {
		return nil, rangeBoundary{}, false
	}
	fragment := doc.NewDocumentFragment()
	if ran == nil || ran.start.container == nil || ran.end.container == nil {
		return fragment, rangeBoundary{}, false
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	if compareBoundaryPoints(start, end) == 0 {
		return fragment, start, false
	}
	root := commonAncestor(start.container, end.container)
	if root == nil {
		return fragment, start, false
	}
	collapsed := start
	changed := false
	if isRangeCharacterData(root) {
		part, modified, collapse := r.rangeContentsNode(root, start, end, extract, nil, nil)
		if part != nil {
			fragment.AppendChild(part)
		}
		changed = modified
		if collapse.container != nil {
			collapsed = collapse
		}
		return fragment, collapsed, changed
	}
	if extract {
		collapsed = rangeContentsCollapsePoint(start, end)
		ran.start, ran.end = collapsed, collapsed
	}
	contained, intersects := rangeContentsSelection(root, start, end)
	for child := root.FirstChild; child != nil; {
		next := child.NextSibling
		part, modified, collapse := r.rangeContentsNode(child, start, end, extract, contained, intersects)
		if part != nil {
			fragment.AppendChild(part)
		}
		changed = changed || modified
		if collapse.container != nil {
			collapsed = collapse
		}
		child = next
	}
	return fragment, collapsed, changed
}

func rangeContentsSelection(root *dom.Node, start, end rangeBoundary) (map[*dom.Node]bool, map[*dom.Node]bool) {
	contained := make(map[*dom.Node]bool)
	intersects := make(map[*dom.Node]bool)
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node != root && node.Parent != nil {
			index := childIndex(node.Parent, node)
			before := rangeBoundary{container: node.Parent, offset: index}
			after := rangeBoundary{container: node.Parent, offset: index + 1}
			contained[node] = compareBoundaryPoints(start, before) <= 0 && compareBoundaryPoints(after, end) <= 0
			intersects[node] = compareBoundaryPoints(after, start) > 0 && compareBoundaryPoints(before, end) < 0
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	for _, boundary := range []rangeBoundary{start, end} {
		if !isRangeCharacterData(boundary.container) {
			continue
		}
		for node := boundary.container; node != nil && node != root; node = node.Parent {
			intersects[node] = true
		}
	}
	return contained, intersects
}

func (r *Runtime) rangeContentsNode(node *dom.Node, start, end rangeBoundary, extract bool, contained, intersects map[*dom.Node]bool) (*dom.Node, bool, rangeBoundary) {
	if node == nil {
		return nil, false, rangeBoundary{}
	}
	if node.Parent != nil {
		if intersects != nil && !intersects[node] {
			return nil, false, rangeBoundary{}
		}
		fullyContained := fullyContainedByRange(node, start, end)
		if contained != nil {
			fullyContained = contained[node]
		}
		if fullyContained {
			if node.Type == dom.NodeDoctype {
				panic(r.domError("HierarchyRequestError", "A DocumentType node cannot be cloned into a DocumentFragment"))
			}
			if extract {
				r.rangeRemoveNode(node)
				return node, true, rangeBoundary{}
			}
			cloneValue := r.jsCloneNode(goja.FunctionCall{
				This:      r.wrapNode(node),
				Arguments: []goja.Value{r.vm.ToValue(true)},
			})
			return r.getNode(cloneValue), false, rangeBoundary{}
		}
	} else if !isRangeCharacterData(node) {
		return nil, false, rangeBoundary{}
	}
	if isRangeCharacterData(node) {
		units := characterDataCodeUnits(node)
		from, to := 0, len(units)
		if node == start.container {
			from = start.offset
		}
		if node == end.container {
			to = end.offset
		}
		if from < 0 {
			from = 0
		}
		if from > len(units) {
			from = len(units)
		}
		if to < from {
			to = from
		}
		if to > len(units) {
			to = len(units)
		}
		part := node.CloneNode(false)
		setCharacterDataCodeUnits(part, units[from:to])
		var collapse rangeBoundary
		changed := extract && from < to
		if extract && changed {
			r.replaceCharacterData(node, from, to-from, "")
			if node == start.container {
				collapse = rangeBoundary{container: node, offset: from}
			}
		}
		if !changed && from == to && node != start.container && node != end.container {
			return nil, false, rangeBoundary{}
		}
		return part, changed, collapse
	}
	var partial *dom.Node
	if node.Type == dom.NodeElement {
		// Range clone/extract algorithms use the DOM cloning steps for each
		// partially contained element as well as fully contained subtrees.
		// Route that shallow clone through Node.cloneNode so custom-element
		// registry association and construction happen in tree order.
		value := r.jsCloneNode(goja.FunctionCall{
			This:      r.wrapNode(node),
			Arguments: []goja.Value{r.vm.ToValue(false)},
		})
		partial = r.getNode(value)
	} else {
		partial = node.CloneNode(false)
	}
	changed := false
	var collapse rangeBoundary
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		part, modified, point := r.rangeContentsNode(child, start, end, extract, contained, intersects)
		if part != nil {
			partial.AppendChild(part)
		}
		changed = changed || modified
		if point.container != nil {
			collapse = point
		}
		child = next
	}
	return partial, changed, collapse
}

// rangeRemoveNode applies the same lifecycle and admission accounting as a
// script-visible Node.removeChild() when a Range algorithm detaches a node.
func (r *Runtime) rangeRemoveNode(node *dom.Node) {
	if node == nil || node.Parent == nil {
		return
	}
	parent := node.Parent
	wasLive := r.nodeIsLive(node)
	wasConnected := node.IsConnected()
	removal := snapshotChildRemovals([]*dom.Node{node})[0]
	parent.RemoveChild(node)
	r.queueChildRemoval(parent, node, removal.previous, removal.next)
	r.markRadioGroupsDirtyForNodes([]*dom.Node{node})
	if wasLive {
		r.releaseNodes([]*dom.Node{node})
	}
	r.notifyMutation()
	if wasConnected {
		r.invokeDisconnectedCallbacks(node)
	}
}

func isRangeCharacterData(node *dom.Node) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case dom.NodeText, dom.NodeComment, dom.NodeCDATASection, dom.NodeProcessingInstruction:
		return true
	default:
		return false
	}
}

func (r *Runtime) jsRangeInsertNode(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'insertNode': 1 argument required"))
	}
	newNode := r.getNode(call.Arguments[0])
	if newNode == nil {
		panic(r.vm.NewTypeError("Failed to execute 'insertNode': parameter 1 is not of type 'Node'"))
	}
	parent := ran.start.container
	if parent.Type == dom.NodeText {
		textNode := ran.start.container
		parent = textNode.Parent
		if parent == nil {
			return goja.Undefined()
		}
		offset := ran.start.offset
		units := characterDataCodeUnits(textNode)
		if offset < 0 {
			offset = 0
		}
		if offset > len(units) {
			offset = len(units)
		}
		oldValue := string(utf16.Decode(units))
		newText := textNode.Doc.NewText("")
		setCharacterDataCodeUnits(newText, append([]uint16(nil), units[offset:]...))
		previous, next := textNode, textNode.NextSibling
		r.admitInsert("Range.insertNode", parent, []*dom.Node{newText}, func() {
			parent.InsertBefore(newText, next)
			setCharacterDataCodeUnits(textNode, units[:offset])
		})
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, AddedNodes: []*dom.Node{newText}, PreviousSibling: previous, NextSibling: next})
		r.queueMutationRecord(MutationRecord{Type: "characterData", Target: textNode, OldValue: &oldValue})
		r.adjustLiveRangesForTextSplit(textNode, newText, offset)
		return r.jsInsertBefore(goja.FunctionCall{This: r.wrapNode(parent), Arguments: []goja.Value{call.Arguments[0], r.wrapNode(newText)}})
	}
	ref := childAt(parent, ran.start.offset)
	return r.jsInsertBefore(goja.FunctionCall{
		This:      r.wrapNode(parent),
		Arguments: []goja.Value{call.Arguments[0], nodeValueOrNull(r, ref)},
	})
}

func (r *Runtime) adjustLiveRangesForTextSplit(original, remainder *dom.Node, offset int) {
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	visited := make(map[*Runtime]bool)
	var visit func(*Runtime)
	visit = func(runtime *Runtime) {
		if runtime == nil || visited[runtime] {
			return
		}
		visited[runtime] = true
		for ran := range runtime.liveRanges {
			for _, boundary := range []*rangeBoundary{&ran.start, &ran.end} {
				if boundary.container == original && boundary.offset > offset {
					boundary.container = remainder
					boundary.offset -= offset
				}
			}
		}
		for _, child := range runtime.iframeChildRuntimes {
			visit(child)
		}
	}
	visit(root)
}

func (r *Runtime) jsRangeSurroundContents(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'surroundContents': 1 argument required"))
	}
	newParent := r.getNode(call.Arguments[0])
	if newParent == nil {
		panic(r.vm.NewTypeError("Failed to execute 'surroundContents': parameter 1 is not of type 'Node'"))
	}
	// Collect the nodes in the range.
	nodes := collectRangeNodes(ran)
	if len(nodes) == 0 {
		return goja.Undefined()
	}
	// Check that all nodes share the same parent (simplified check).
	firstParent := nodes[0].Parent
	for _, n := range nodes {
		if n.Parent != firstParent {
			panic(r.vm.NewTypeError("Failed to execute 'surroundContents': the Range has partially selected a non-text node"))
		}
	}
	// Remove nodes from their parent and append to newParent.
	for _, n := range nodes {
		if n.Parent != nil {
			r.rangeRemoveNode(n)
		}
		newParent.AppendChild(n)
	}
	// Insert newParent where the first node was.
	ref := childAt(firstParent, ran.start.offset)
	r.jsInsertBefore(goja.FunctionCall{
		This:      r.wrapNode(firstParent),
		Arguments: []goja.Value{r.wrapNode(newParent), nodeValueOrNull(r, ref)},
	})
	// Update the range to surround the new parent.
	if newParent.Parent != nil {
		idx := childIndex(newParent.Parent, newParent)
		ran.start = rangeBoundary{container: newParent.Parent, offset: idx}
		ran.end = rangeBoundary{container: newParent.Parent, offset: idx + 1}
	}
	r.notifyMutation()
	return goja.Undefined()
}

func nodeValueOrNull(r *Runtime, node *dom.Node) goja.Value {
	if node == nil {
		return goja.Null()
	}
	return r.wrapNode(node)
}

// ---------------------------------------------------------------------------
// Range comparison
// ---------------------------------------------------------------------------

// compareBoundaryPoints returns -1 if a < b, 0 if a == b, 1 if a > b,
// using document order.
func compareBoundaryPoints(a, b rangeBoundary) int {
	if a.container == b.container {
		if a.offset < b.offset {
			return -1
		}
		if a.offset > b.offset {
			return 1
		}
		return 0
	}
	if a.container == nil || b.container == nil {
		return 0
	}
	if isAncestorNode(a.container, b.container) {
		child := childBelowAncestor(a.container, b.container)
		index := childIndex(a.container, child)
		if a.offset <= index {
			return -1
		}
		return 1
	}
	if isAncestorNode(b.container, a.container) {
		child := childBelowAncestor(b.container, a.container)
		index := childIndex(b.container, child)
		if index < b.offset {
			return -1
		}
		return 1
	}
	ancestor := commonAncestor(a.container, b.container)
	if ancestor == nil {
		return 0
	}
	aChild := childBelowAncestor(ancestor, a.container)
	bChild := childBelowAncestor(ancestor, b.container)
	aIndex := childIndex(ancestor, aChild)
	bIndex := childIndex(ancestor, bChild)
	if aIndex < bIndex {
		return -1
	}
	if aIndex > bIndex {
		return 1
	}
	return 0
}

func isAncestorNode(ancestor, node *dom.Node) bool {
	for cur := node; cur != nil; cur = cur.Parent {
		if cur == ancestor {
			return true
		}
	}
	return false
}

func childBelowAncestor(ancestor, node *dom.Node) *dom.Node {
	if ancestor == nil || node == nil || ancestor == node {
		return nil
	}
	cur := node
	for cur.Parent != nil && cur.Parent != ancestor {
		cur = cur.Parent
	}
	if cur.Parent == ancestor {
		return cur
	}
	return nil
}

// treePosition returns a document-order index for a node by walking the tree.
// This is O(n) but correct for comparison purposes.
func treePosition(n *dom.Node) int {
	if n == nil {
		return 0
	}
	pos := 0
	var walk func(node *dom.Node) bool
	walk = func(node *dom.Node) bool {
		if node == n {
			return false // found
		}
		pos++
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	// Find the document root.
	root := n
	for root.Parent != nil {
		root = root.Parent
	}
	walk(root)
	return pos
}

func (r *Runtime) jsRangeCompareBoundaryPoints(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'compareBoundaryPoints': 2 arguments required"))
	}
	how := webIDLUnsignedShort(call.Arguments[0])
	otherObj, ok := call.Arguments[1].(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Failed to execute 'compareBoundaryPoints': parameter 2 is not of type 'Range'"))
	}
	other := getRange(otherObj)
	if other == nil {
		panic(r.vm.NewTypeError("Failed to execute 'compareBoundaryPoints': parameter 2 is not of type 'Range'"))
	}

	var a, b rangeBoundary
	// START_TO_START = 0, START_TO_END = 1, END_TO_END = 2, END_TO_START = 3
	switch how {
	case 0: // START_TO_START
		a, b = ran.start, other.start
	case 1: // START_TO_END
		a, b = ran.end, other.start
	case 2: // END_TO_END
		a, b = ran.end, other.end
	case 3: // END_TO_START
		a, b = ran.start, other.end
	default:
		panic(r.domError("NotSupportedError", "The how parameter must be one of the Range boundary constants"))
	}
	if nodeRoot(ran.start.container) != nodeRoot(other.start.container) {
		panic(r.domError("WrongDocumentError", "The ranges have different roots"))
	}
	return r.vm.ToValue(compareBoundaryPoints(a, b))
}

func (r *Runtime) jsRangeIsPointInRange(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'isPointInRange': 2 arguments required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'isPointInRange': parameter 1 is not of type 'Node'"))
	}
	if nodeRoot(node) != nodeRoot(ran.start.container) {
		return r.vm.ToValue(false)
	}
	offset := webIDLUnsignedLong(call.Arguments[1])
	r.validateRangePoint(node, offset, "isPointInRange")
	point := rangeBoundary{container: node, offset: offset}
	// Point is in range if start <= point <= end.
	return r.vm.ToValue(compareBoundaryPoints(ran.start, point) <= 0 && compareBoundaryPoints(point, ran.end) <= 0)
}

func (r *Runtime) jsRangeComparePoint(call goja.FunctionCall, ran *jsRange) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'comparePoint': 2 arguments required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'comparePoint': parameter 1 is not of type 'Node'"))
	}
	if nodeRoot(node) != nodeRoot(ran.start.container) {
		panic(r.domError("WrongDocumentError", "The point and range have different roots"))
	}
	offset := webIDLUnsignedLong(call.Arguments[1])
	r.validateRangePoint(node, offset, "comparePoint")
	point := rangeBoundary{container: node, offset: offset}
	if compareBoundaryPoints(point, ran.start) < 0 {
		return r.vm.ToValue(-1)
	}
	if compareBoundaryPoints(point, ran.end) > 0 {
		return r.vm.ToValue(1)
	}
	return r.vm.ToValue(0)
}

func (r *Runtime) validateRangePoint(node *dom.Node, offset int, method string) {
	if node.Type == dom.NodeDoctype {
		panic(r.domError("InvalidNodeTypeError", "A DocumentType node cannot be used as a range boundary"))
	}
	if offset > nodeChildCount(node) {
		panic(r.domError("IndexSizeError", "The offset is greater than the node length"))
	}
	_ = method
}

func webIDLUnsignedShort(value goja.Value) int {
	number := value.ToFloat()
	if math.IsNaN(number) || math.IsInf(number, 0) || number == 0 {
		return 0
	}
	wrapped := math.Mod(math.Trunc(number), 65536)
	if wrapped < 0 {
		wrapped += 65536
	}
	return int(wrapped)
}

func webIDLUnsignedLong(value goja.Value) int {
	number := value.ToFloat()
	if math.IsNaN(number) || math.IsInf(number, 0) || number == 0 {
		return 0
	}
	wrapped := math.Mod(math.Trunc(number), 4294967296)
	if wrapped < 0 {
		wrapped += 4294967296
	}
	return int(wrapped)
}

func nodeRoot(node *dom.Node) *dom.Node {
	for node != nil && node.Parent != nil {
		node = node.Parent
	}
	return node
}

// ---------------------------------------------------------------------------
// Range toString
// ---------------------------------------------------------------------------

// rangeToString returns the text contained by the range in tree order.
func rangeToString(ran *jsRange) string {
	if ran.start.container == nil || ran.end.container == nil ||
		compareBoundaryPoints(ran.start, ran.end) == 0 {
		return ""
	}
	var sb strings.Builder
	collectTextInRange(ran.start.container, ran.start.offset, ran.end.container, ran.end.offset, &sb)
	return sb.String()
}

// collectTextInRange visits text descendants of the common ancestor and appends
// only the portions between the range's boundary points.
func collectTextInRange(startContainer *dom.Node, startOffset int, endContainer *dom.Node, endOffset int, sb *strings.Builder) {
	start := rangeBoundary{container: startContainer, offset: startOffset}
	end := rangeBoundary{container: endContainer, offset: endOffset}
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	ancestor := commonAncestor(start.container, end.container)
	if ancestor == nil {
		return
	}
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeText {
			units := characterDataCodeUnits(node)
			textStart := rangeBoundary{container: node, offset: 0}
			textEnd := rangeBoundary{container: node, offset: len(units)}
			if compareBoundaryPoints(textEnd, start) <= 0 || compareBoundaryPoints(textStart, end) >= 0 {
				return
			}
			from, to := 0, len(units)
			if node == start.container {
				from = start.offset
			}
			if node == end.container {
				to = end.offset
			}
			if from < 0 {
				from = 0
			}
			if from > len(units) {
				from = len(units)
			}
			if to < from {
				to = from
			}
			if to > len(units) {
				to = len(units)
			}
			if from < to {
				sb.WriteString(string(utf16.Decode(units[from:to])))
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(ancestor)
}

// ---------------------------------------------------------------------------
// Range helpers
// ---------------------------------------------------------------------------

// commonAncestor returns the deepest node that is an ancestor of both a and b.
func commonAncestor(a, b *dom.Node) *dom.Node {
	if a == nil || b == nil {
		return nil
	}
	if a == b {
		return a
	}
	// Collect ancestors of a.
	ancestors := make(map[*dom.Node]bool)
	for n := a; n != nil; n = n.Parent {
		ancestors[n] = true
	}
	// Walk up from b to find the first common ancestor.
	for n := b; n != nil; n = n.Parent {
		if ancestors[n] {
			return n
		}
	}
	return nil
}

// childIndex returns the index of child among parent's children.
func childIndex(parent, child *dom.Node) int {
	idx := 0
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c == child {
			return idx
		}
		idx++
	}
	return idx
}

// nodeChildCount returns the number of children of node.
func nodeChildCount(node *dom.Node) int {
	if isRangeCharacterData(node) {
		return len(characterDataCodeUnits(node))
	}
	count := 0
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		count++
	}
	return count
}

func (r *Runtime) normalizeNode(node *dom.Node) {
	if node == nil {
		return
	}
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if !child.Text() {
			r.normalizeNode(child)
			child = next
			continue
		}
		if child.DataContent == "" {
			index := childIndex(node, child)
			removal := snapshotChildRemovals([]*dom.Node{child})[0]
			r.adjustLiveRangesForNodeRemoval(node, child, index)
			node.RemoveChild(child)
			r.queueChildRemoval(node, child, removal.previous, removal.next)
			child = next
			continue
		}
		for next != nil && next.Text() {
			following := next.NextSibling
			appendAt := nodeChildCount(child)
			r.replaceCharacterDataUnits(child, appendAt, 0, characterDataCodeUnits(next))
			index := childIndex(node, next)
			removal := snapshotChildRemovals([]*dom.Node{next})[0]
			r.adjustLiveRangesForNodeRemoval(node, next, index)
			node.RemoveChild(next)
			r.queueChildRemoval(node, next, removal.previous, removal.next)
			next = following
		}
		child = next
	}
}

// childAt returns the child at the given index, or nil if out of range.
func childAt(parent *dom.Node, index int) *dom.Node {
	idx := 0
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if idx == index {
			return c
		}
		idx++
	}
	return nil
}

// collectRangeNodes returns the top-level nodes fully or partially contained
// in the range. This is a simplified implementation that collects nodes
// between start and end in document order.
func collectRangeNodes(ran *jsRange) []*dom.Node {
	var nodes []*dom.Node
	start := ran.start.container
	end := ran.end.container

	if start == end {
		// Same container.
		if start.Type == dom.NodeText {
			// For text nodes, we can't really "collect" partial text as nodes.
			// Return the text node itself; the caller handles splitting.
			nodes = append(nodes, start)
		} else {
			// Collect children from startOffset to endOffset.
			idx := 0
			for c := start.FirstChild; c != nil; c = c.NextSibling {
				if idx >= ran.start.offset && idx < ran.end.offset {
					nodes = append(nodes, c)
				}
				idx++
			}
		}
		return nodes
	}

	// Different containers: collect nodes between start and end.
	ancestor := commonAncestor(start, end)
	if ancestor == nil {
		return nil
	}

	// Walk the tree from ancestor and collect nodes in range.
	collecting := false
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == start {
			collecting = true
			if n.Type != dom.NodeText {
				// Collect children from offset onward.
				idx := 0
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if idx >= ran.start.offset {
						nodes = append(nodes, c)
					}
					idx++
				}
			} else {
				nodes = append(nodes, n)
			}
			return
		}
		if n == end {
			if n.Type != dom.NodeText {
				// Collect children up to offset.
				idx := 0
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if idx >= ran.end.offset {
						break
					}
					nodes = append(nodes, c)
					idx++
				}
			} else {
				nodes = append(nodes, n)
			}
			collecting = false
			return
		}
		if collecting {
			nodes = append(nodes, n)
			return // Don't descend into already-collected nodes.
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if !collecting && len(nodes) > 0 {
				// Check if we've reached the end.
				break
			}
		}
	}
	walk(ancestor)
	return nodes
}

// ---------------------------------------------------------------------------
// Selection
// ---------------------------------------------------------------------------

// jsSelection is the Go state behind one JS Selection object.
type jsSelection struct {
	ranges          []*jsRange
	selectedControl *dom.Node
	backward        bool
}

// setupSelection installs window.getSelection() on the window object.
func (r *Runtime) setupSelection() {
	if r.domDoc == nil {
		return
	}

	sel := r.selectionForDocument(r.domDoc)
	r.selection = sel

	win := r.vm.Get("window")
	if win == nil {
		return
	}
	winObj, ok := win.(*goja.Object)
	if !ok {
		return
	}

	_ = winObj.Set("getSelection", func(call goja.FunctionCall) goja.Value {
		return r.wrapSelection(sel)
	})

	// Also install on the global scope.
	_ = r.vm.Set("getSelection", func(call goja.FunctionCall) goja.Value {
		return r.wrapSelection(sel)
	})
}

func (r *Runtime) selectionForDocument(doc *dom.Document) *jsSelection {
	if doc == nil {
		return nil
	}
	if r.documentSelections == nil {
		r.documentSelections = make(map[*dom.Document]*jsSelection)
	}
	if selection := r.documentSelections[doc]; selection != nil {
		return selection
	}
	selection := &jsSelection{}
	r.documentSelections[doc] = selection
	return selection
}

// EnsureContentEditableSelection places a missing or out-of-host Selection at
// the start of an editing host. Focusing a contenteditable host makes it ready
// for keyboard editing even when no script has created a Range first.
func (r *Runtime) EnsureContentEditableSelection(host *dom.Node) bool {
	if host == nil || host.Doc == nil || !dom.IsContentEditable(host) {
		return false
	}
	sel := r.selectionForDocument(host.Doc)
	if sel == nil {
		return false
	}
	if len(sel.ranges) > 0 {
		ran := sel.ranges[0]
		if ran != nil && ran.start.container != nil && ran.end.container != nil &&
			isAncestorNode(host, ran.start.container) && isAncestorNode(host, ran.end.container) {
			return false
		}
	}
	container := host
	var findText func(*dom.Node) *dom.Node
	findText = func(node *dom.Node) *dom.Node {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == dom.NodeText {
				return child
			}
			if child.Element() && child != host && child.HasAttribute("contenteditable") && dom.IsContentEditable(child) {
				continue
			}
			if text := findText(child); text != nil {
				return text
			}
		}
		return nil
	}
	if text := findText(host); text != nil {
		container = text
	}
	caret := rangeBoundary{container: container, offset: 0}
	ran := &jsRange{start: caret, end: caret}
	r.registerLiveRange(ran)
	sel.ranges = []*jsRange{ran}
	sel.selectedControl = nil
	sel.backward = false
	return true
}

// MoveSelectionCaret moves the document Selection through editable text in
// tree order. The direction is one of left, right, home, or end. Left/right
// cross Text node boundaries while remaining within the editing host.
func (r *Runtime) MoveSelectionCaret(host *dom.Node, direction string) bool {
	if host == nil || host.Doc == nil {
		return false
	}
	sel := r.selectionForDocument(host.Doc)
	if sel == nil || len(sel.ranges) == 0 {
		return false
	}
	ran := sel.ranges[0]
	if ran == nil || ran.start.container == nil || !isAncestorNode(host, ran.start.container) {
		return false
	}
	texts := make([]*dom.Node, 0, 8)
	var collect func(*dom.Node)
	collect = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeText {
			texts = append(texts, node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(host)
	if len(texts) == 0 {
		return false
	}
	units := make([]uint16, 0, 32)
	length := func(node *dom.Node) int {
		if node == nil || node.Type != dom.NodeText {
			return 0
		}
		return len(characterDataCodeUnits(node))
	}
	for _, text := range texts {
		units = append(units, characterDataCodeUnits(text)...)
	}
	var countText func(*dom.Node) int
	countText = func(node *dom.Node) int {
		if node == nil {
			return 0
		}
		if node.Type == dom.NodeText {
			return length(node)
		}
		total := 0
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			total += countText(child)
		}
		return total
	}
	pointOffset := func(boundary rangeBoundary) (int, bool) {
		before := 0
		found := false
		var visit func(*dom.Node)
		visit = func(node *dom.Node) {
			if node == nil || found {
				return
			}
			if node == boundary.container {
				if node.Type == dom.NodeText {
					offset := boundary.offset
					if offset < 0 {
						offset = 0
					}
					if n := length(node); offset > n {
						offset = n
					}
					before += offset
				} else {
					offset := boundary.offset
					if offset < 0 {
						offset = 0
					}
					for child, i := node.FirstChild, 0; child != nil && i < offset; child, i = child.NextSibling, i+1 {
						before += countText(child)
					}
				}
				found = true
				return
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
				if found {
					return
				}
				before += countText(child)
			}
		}
		visit(host)
		return before, found
	}
	start, ok := pointOffset(ran.start)
	if !ok {
		return false
	}
	target := start
	if ran.start != ran.end {
		if direction == "left" || direction == "home" {
			target = start
		} else {
			target, ok = pointOffset(ran.end)
			if !ok {
				return false
			}
		}
	} else {
		switch direction {
		case "left":
			target--
			if target > 0 && target < len(units) && units[target] >= 0xDC00 && units[target] <= 0xDFFF && units[target-1] >= 0xD800 && units[target-1] <= 0xDBFF {
				target--
			}
		case "right":
			if target < len(units) {
				target++
				if target < len(units) && units[target-1] >= 0xD800 && units[target-1] <= 0xDBFF && units[target] >= 0xDC00 && units[target] <= 0xDFFF {
					target++
				}
			}
		case "home", "end":
			if ran.start.container.Type == dom.NodeText {
				base, found := pointOffset(rangeBoundary{container: ran.start.container, offset: 0})
				if !found {
					return false
				}
				if direction == "home" {
					target = base
				} else {
					target = base + length(ran.start.container)
				}
			} else if direction == "home" {
				target = 0
			} else {
				target = countText(host)
			}
		default:
			return false
		}
	}
	if target < 0 {
		target = 0
	}
	total := countText(host)
	if target > total {
		target = total
	}
	if target == start && ran.start == ran.end {
		return false
	}
	var container *dom.Node
	var offset int
	cumulative := 0
	for _, text := range texts {
		n := length(text)
		if direction == "left" || direction == "home" {
			if target <= cumulative+n && n > 0 {
				container, offset = text, target-cumulative
				break
			}
		} else if target >= cumulative && target < cumulative+n {
			container, offset = text, target-cumulative
			break
		}
		cumulative += n
	}
	if container == nil {
		for i := len(texts) - 1; i >= 0; i-- {
			if n := length(texts[i]); n > 0 {
				container, offset = texts[i], n
				break
			}
		}
	}
	if container == nil {
		return false
	}
	caret := rangeBoundary{container: container, offset: offset}
	ran.start, ran.end = caret, caret
	return true
}

// SelectionTextCaret returns the collapsed Selection's Text node and UTF-16
// offset when that caret is inside host. It is used for composition rendering,
// where the preview belongs in the layout run without changing the DOM.
func (r *Runtime) SelectionTextCaret(host *dom.Node) (*dom.Node, int, bool) {
	if host == nil || host.Doc == nil {
		return nil, 0, false
	}
	sel := r.selectionForDocument(host.Doc)
	if sel == nil || len(sel.ranges) == 0 {
		return nil, 0, false
	}
	ran := sel.ranges[0]
	if ran == nil || ran.start != ran.end || ran.start.container == nil || ran.start.container.Type != dom.NodeText || !isAncestorNode(host, ran.start.container) {
		return nil, 0, false
	}
	offset := ran.start.offset
	if offset < 0 {
		offset = 0
	}
	if length := len(characterDataCodeUnits(ran.start.container)); offset > length {
		offset = length
	}
	return ran.start.container, offset, true
}

func (r *Runtime) execCommandDeleteSelection(sel *jsSelection) goja.Value {
	return r.execCommandDeleteSelectionWithInputType(sel, "deleteContentBackward")
}

func (r *Runtime) execCommandDeleteSelectionWithInputType(sel *jsSelection, inputType string) goja.Value {
	if sel == nil || len(sel.ranges) == 0 {
		return r.vm.ToValue(false)
	}
	if len(sel.ranges) > 1 {
		allTableCells := true
		for _, selected := range sel.ranges {
			if !selectedTableCellRange(selected) {
				allTableCells = false
				break
			}
		}
		if allTableCells && (inputType == "deleteContentBackward" || inputType == "deleteContentForward" || inputType == "deleteWordBackward" || inputType == "deleteWordForward" || inputType == "deleteByCut") {
			host := editingHostFor(sel.ranges[0].start.container)
			if host != nil {
				valid := true
				for _, selected := range sel.ranges {
					if editingHostFor(selected.start.container) != host || editingHostFor(selected.end.container) != host {
						valid = false
						break
					}
				}
				if valid {
					for index := len(sel.ranges) - 1; index >= 0; index-- {
						selected := sel.ranges[index]
						start, end := selected.start, selected.end
						if compareBoundaryPoints(start, end) > 0 {
							start, end = end, start
						}
						if !deleteTableCellSelection(r, selected, start, end, host, inputType) {
							return r.vm.ToValue(false)
						}
					}
					r.dispatchInputEvent(host, nil, inputType)
					return r.vm.ToValue(true)
				}
			}
		}
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	if compareBoundaryPoints(ran.start, ran.end) == 0 {
		host := editingHostFor(ran.start.container)
		forward := inputType == "deleteContentForward" || inputType == "deleteWordForward"
		backward := inputType == "deleteContentBackward" || inputType == "deleteWordBackward"
		if forward || backward {
			if forward || r.userInput {
				start, end, ok := r.deletionTargetRange(host, ran, inputType)
				if ok && compareBoundaryPoints(start, end) != 0 {
					ran.start, ran.end = start, end
				} else if forward {
					return r.vm.ToValue(false)
				}
			}
			if compareBoundaryPoints(ran.start, ran.end) == 0 {
				if previous, ok := previousEditableTextBoundary(host, ran.start.container, ran.start.offset); ok {
					ran.start = previous
					ran.end = rangeBoundary{container: previous.container, offset: nextCharacterBoundary(previous.container, previous.offset)}
				} else {
					// A caret after an atomic, text-free child (for example an empty
					// custom element or image) deletes that child as one editing unit.
					// Preserve the parent/index boundary so Range deletion uses the
					// normal lifecycle-aware subtree removal path.
					container := ran.start.container
					offset := ran.start.offset
					if container == nil || !container.Element() || offset <= 0 {
						return r.vm.ToValue(false)
					}
					childCount := 0
					for child := container.FirstChild; child != nil; child = child.NextSibling {
						childCount++
					}
					if offset > childCount {
						return r.vm.ToValue(false)
					}
					child := container.FirstChild
					for i := 1; i < offset && child != nil; i++ {
						child = child.NextSibling
					}
					if child == nil || !child.Element() || editingHostFor(child) != host {
						return r.vm.ToValue(false)
					}
					ran.start = rangeBoundary{container: container, offset: offset - 1}
					ran.end = rangeBoundary{container: container, offset: offset}
				}
			}
		}
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	host := editingHostFor(start.container)
	if host == nil || editingHostFor(end.container) != host ||
		!isAncestorNode(host, start.container) || !isAncestorNode(host, end.container) {
		return r.vm.ToValue(false)
	}
	if deleteTableCellSelection(r, ran, start, end, host, inputType) {
		r.dispatchInputEvent(host, nil, inputType)
		return r.vm.ToValue(true)
	}
	if start.container == end.container && start.container.Element() && end.offset == start.offset+1 &&
		(start.container.Data == "li" || start.container.Data == "ul" || start.container.Data == "ol") {
		selected := childAt(start.container, start.offset)
		if selected != nil && selected.Element() && (strings.EqualFold(selected.Data, "ul") || strings.EqualFold(selected.Data, "ol")) {
			if nestedStart, nestedEnd, ok := nestedListTextTargetRange(ran); ok {
				ran.start, ran.end = nestedStart, nestedEnd
				start, end = nestedStart, nestedEnd
			}
		}
	}
	startBlock := nearestEditingBlock(host, start.container)
	endBlock := nearestEditingBlock(host, end.container)
	preserveTrailingBreakOnJoin := inputType == "deleteContentForward" && startBlock != nil && start.container == startBlock &&
		start.offset > 0 && childAt(startBlock, start.offset-1) != nil && childAt(startBlock, start.offset-1).Element() &&
		strings.EqualFold(childAt(startBlock, start.offset-1).Data, "br")
	preservedList := selectedListWithNoneditableItem(ran)
	trailingNestedList := selectedTrailingNestedListOwner(ran)
	definitionItemStartsAt := func(block *dom.Node, boundary rangeBoundary) bool {
		if block == nil {
			return false
		}
		if boundary.container == block {
			return boundary.offset == 0
		}
		return boundary.container == firstEditableTextInSubtree(block, host) && boundary.offset == 0
	}
	discardEmptyDefinitionTerm := startBlock != nil && endBlock != nil && startBlock != endBlock &&
		strings.EqualFold(startBlock.Data, "dt") && strings.EqualFold(endBlock.Data, "dd") &&
		startBlock.Parent != nil && endBlock.Parent != nil && startBlock.Parent != endBlock.Parent &&
		strings.EqualFold(startBlock.Parent.Data, "dl") && strings.EqualFold(endBlock.Parent.Data, "dl") &&
		definitionItemStartsAt(startBlock, start) && definitionItemStartsAt(endBlock, end)
	if preservedList != nil {
		ran.start = rangeBoundary{container: preservedList, offset: 0}
		ran.end = rangeBoundary{container: preservedList, offset: nodeChildCount(preservedList)}
		start, end = ran.start, ran.end
		startBlock, endBlock = preservedList, preservedList
	}
	if item, ok := selectedSoleListItemContents(ran); ok && editingHostFor(item) == host {
		ran.start = rangeBoundary{container: item, offset: 0}
		ran.end = rangeBoundary{container: item, offset: nodeChildCount(item)}
		start, end = ran.start, ran.end
		startBlock, endBlock = item, item
	}
	nestedListsToClean := selectedNestedListsWhoseTextIsDeleted(host, ran)
	nestedListItemsToClean := selectedListItemsAdjacentToNestedContent(host, ran)
	endBlockJoinOffset := 0
	if inputType == "deleteContentForward" && endBlock != nil && end.container == firstEditableTextInSubtree(endBlock, host) && end.container.Type == dom.NodeText {
		endBlockJoinOffset = leadingTargetSpaceEnd(r, endBlock, end.container)
	}
	horizontalRuleParent, horizontalRuleOffset := selectedHorizontalRulePosition(host, start, end)
	selectedCells := selectedTableCellPlaceholders(host, start, end)
	r.jsRangeDeleteContents(ran)
	restoreSelectedTableCellPlaceholders(r, selectedCells)
	mergedInlineBeforeBlock := endBlock != nil && boundaryAtEditingBlockStart(host, endBlock, end) &&
		mergeInlineBeforeListItem(r, start, endBlock, end, host)
	if !mergedInlineBeforeBlock && endBlock != nil && boundaryAtEditingBlockStart(host, endBlock, end) {
		mergedInlineBeforeBlock = mergeInlineBeforeBlock(r, start, endBlock, host)
	}
	if startBlock != nil && boundaryAtEditingBlockEnd(host, startBlock, start) {
		mergedInlineAfterBlock := mergeInlineAfterBlock(r, startBlock, start, end, host)
		if !mergedInlineAfterBlock && startBlock.Element() && strings.EqualFold(startBlock.Data, "li") {
			mergeInlineAfterListItem(r, startBlock, start, end, host)
		}
	}
	if discardEmptyDefinitionTerm && removeEmptyDefinitionListEndBlock(r, endBlock, startBlock, host) {
		startBlock = nil
		caret := rangeBoundary{container: endBlock, offset: 0}
		ran.start, ran.end = caret, caret
	}
	if horizontalRuleParent != nil {
		trimHorizontalRuleBoundaryWhitespace(r, horizontalRuleParent, horizontalRuleOffset)
	}
	if inputType == "deleteContentForward" {
		for current := end.container; current != nil && current != host; {
			parent := current.Parent
			if current.Element() && !isEditingBlockElement(current) && current.TextContent() == "" && parent != nil {
				r.rangeRemoveNode(current)
			}
			current = parent
		}
	}
	if inputType == "deleteContentForward" && endBlock != nil && startBlock != endBlock && end.container == firstEditableTextInSubtree(endBlock, host) && end.offset == endBlockJoinOffset {
		mergeInlineBeforeBlock(r, start, endBlock, host)
	}
	if inputType == "deleteWordBackward" && start.container.Type == dom.NodeText && end.container.Type == dom.NodeText {
		// Preserve the visible separator and remove empty formatting wrappers
		// when a word crosses inline text nodes.
		units := characterDataCodeUnits(start.container)
		if len(units) > 0 && units[len(units)-1] == ' ' {
			r.replaceCharacterDataUnits(start.container, len(units)-1, 1, []uint16{0x00a0})
		}
		for empty := end.container.Parent; empty != nil && empty != host && empty.Element() && empty.TextContent() == "" && !isEditingBlockElement(empty); empty = empty.Parent {
			r.rangeRemoveNode(empty)
		}
	}
	mergedNestedList := false
	if startBlock != nil && endBlock != nil && startBlock != endBlock {
		if !removeEmptyLeadingListRoot(r, startBlock, endBlock, host) {
			mergeAdjacentEditingBlocksWithTrailingBreak(r, startBlock, endBlock, host, preserveTrailingBreakOnJoin)
			mergedNestedList = mergeNestedListIntoPreviousItem(r, startBlock, endBlock, host)
		}
	}
	for _, list := range nestedListsToClean {
		if list.IsConnected() && list.TextContent() == "" {
			if mergedNestedList && isAncestorNode(startBlock, list) {
				ensureNestedListPlaceholder(r, list)
			} else {
				r.rangeRemoveNode(list)
			}
		}
	}
	for _, item := range nestedListItemsToClean {
		if item.IsConnected() && item.TextContent() == "" && !(mergedNestedList && isAncestorNode(startBlock, item)) {
			r.rangeRemoveNode(item)
		}
	}
	removeEmptyDefinitionListEndBlock(r, startBlock, endBlock, host)
	if trailingNestedList != nil && trailingNestedList.IsConnected() {
		item := trailingNestedList.Doc.NewElement("li")
		item.AppendChild(trailingNestedList.Doc.NewElement("br"))
		trailingNestedList.AppendChild(item)
		r.notifyMutation()
	} else if preservedList != nil && preservedList.IsConnected() && preservedList.FirstChild == nil {
		item := preservedList.Doc.NewElement("li")
		item.AppendChild(preservedList.Doc.NewElement("br"))
		preservedList.AppendChild(item)
		r.notifyMutation()
	} else if startBlock != nil && startBlock.IsConnected() {
		r.ensureEditingBlockPlaceholder(startBlock)
	}
	// Keep the editing selection anchored in its original start text node when
	// that node survives deletion. Range.deleteContents() collapses according
	// to the DOM Range boundary algorithm, while editing deletion keeps the
	// caret at the start character offset within surviving text.
	if start.container.Type == dom.NodeText && isAncestorNode(host, start.container) {
		offset := start.offset
		if length := len(characterDataCodeUnits(start.container)); offset > length {
			offset = length
		}
		if offset < 0 {
			offset = 0
		}
		collapsed := rangeBoundary{container: start.container, offset: offset}
		ran.start, ran.end = collapsed, collapsed
	}
	r.dispatchInputEvent(host, nil, inputType)
	return r.vm.ToValue(true)
}

func removeEmptyDefinitionListEndBlock(r *Runtime, start, end, host *dom.Node) bool {
	if r == nil || start == nil || end == nil || start == end || host == nil || !end.IsConnected() ||
		!end.Element() || !(strings.EqualFold(end.Data, "dt") || strings.EqualFold(end.Data, "dd")) ||
		end.Parent == nil || !strings.EqualFold(end.Parent.Data, "dl") || editingHostFor(end) != host {
		return false
	}
	for child := end.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != dom.NodeText || child.Data != "" {
			return false
		}
	}
	list := end.Parent
	r.rangeRemoveNode(end)
	for child := list.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && (strings.EqualFold(child.Data, "dt") || strings.EqualFold(child.Data, "dd")) {
			return true
		}
		if child.Type != dom.NodeText || strings.TrimSpace(child.Data) != "" {
			return true
		}
	}
	r.rangeRemoveNode(list)
	return true
}

func deleteTableCellSelection(r *Runtime, ran *jsRange, start, end rangeBoundary, host *dom.Node, inputType string) bool {
	switch inputType {
	case "deleteContentBackward", "deleteContentForward", "deleteWordBackward", "deleteWordForward", "deleteByCut", "insertText":
	default:
		return false
	}
	tableAncestor := func(node *dom.Node) *dom.Node {
		for current := node; current != nil; current = current.Parent {
			if current.Element() && strings.EqualFold(current.Data, "table") {
				return current
			}
		}
		return nil
	}
	startTable, endTable := tableAncestor(start.container), tableAncestor(end.container)
	if startTable == nil || endTable == nil || !isAncestorNode(host, startTable) || !isAncestorNode(host, endTable) {
		return false
	}
	crossTable := startTable != endTable
	if inputType == "insertText" && !crossTable {
		return false
	}
	type cellRange struct {
		cell          *dom.Node
		before, after rangeBoundary
	}
	var cells []cellRange
	var collect func(*dom.Node)
	collect = func(node *dom.Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && (strings.EqualFold(child.Data, "td") || strings.EqualFold(child.Data, "th")) &&
				(crossTable || tableAncestor(child) == startTable) {
				index := childIndex(child.Parent, child)
				before := rangeBoundary{container: child.Parent, offset: index}
				after := rangeBoundary{container: child.Parent, offset: index + 1}
				if compareBoundaryPoints(end, before) > 0 && compareBoundaryPoints(start, after) < 0 {
					cells = append(cells, cellRange{cell: child, before: before, after: after})
				}
				continue
			}
			collect(child)
		}
	}
	collect(host)
	if len(cells) == 0 {
		return false
	}
	for _, entry := range cells {
		if editingHostFor(entry.cell) != host {
			return false
		}
	}
	for _, entry := range cells {
		cell := entry.cell
		cellStart := start
		if compareBoundaryPoints(start, entry.before) <= 0 {
			cellStart = rangeBoundary{container: cell, offset: 0}
		}
		cellEnd := end
		if compareBoundaryPoints(end, entry.after) >= 0 {
			cellEnd = rangeBoundary{container: cell, offset: nodeChildCount(cell)}
		}
		if compareBoundaryPoints(cellStart, cellEnd) < 0 {
			cellRange := &jsRange{start: cellStart, end: cellEnd}
			r.jsRangeDeleteContents(cellRange)
		}
		if cell.FirstChild == nil {
			cell.AppendChild(cell.Doc.NewElement("br"))
			r.notifyMutation()
		}
	}
	caret := rangeBoundary{container: cells[0].cell, offset: 0}
	ran.start, ran.end = caret, caret
	return true
}

func selectedSoleListItemContents(ran *jsRange) (*dom.Node, bool) {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() || ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return nil, false
	}
	list := ran.start.container
	if !strings.EqualFold(list.Data, "ul") && !strings.EqualFold(list.Data, "ol") {
		return nil, false
	}
	item := childAt(list, ran.start.offset)
	if item == nil || item.NextSibling != nil || !item.Element() || !strings.EqualFold(item.Data, "li") {
		return nil, false
	}
	return item, true
}

func selectedTrailingNestedListOwner(ran *jsRange) *dom.Node {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() || ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return nil
	}
	owner := ran.start.container
	if !strings.EqualFold(owner.Data, "ul") && !strings.EqualFold(owner.Data, "ol") {
		return nil
	}
	selected := childAt(owner, ran.start.offset)
	if selected == nil || selected.NextSibling != nil || !selected.Element() || (!strings.EqualFold(selected.Data, "ul") && !strings.EqualFold(selected.Data, "ol")) {
		return nil
	}
	return owner
}

func selectedNestedListsWhoseTextIsDeleted(host *dom.Node, ran *jsRange) []*dom.Node {
	if host == nil || ran == nil || ran.start.container == nil || ran.end.container == nil ||
		compareBoundaryPoints(ran.start, ran.end) == 0 {
		return nil
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	var candidates []*dom.Node
	var visit func(*dom.Node, bool)
	visit = func(node *dom.Node, insideList bool) {
		if node == nil {
			return
		}
		isList := node.Element() && (strings.EqualFold(node.Data, "ul") || strings.EqualFold(node.Data, "ol"))
		if isList && insideList && isAncestorNode(node, end.container) && !isAncestorNode(node, start.container) {
			allTextSelected := true
			hasText := false
			var inspect func(*dom.Node) bool
			inspect = func(child *dom.Node) bool {
				if child != node && child.Element() && dom.ContentEditableState(child) == "false" {
					return false
				}
				if child.Type == dom.NodeText {
					if child.DataContent == "" {
						return true
					}
					hasText = true
					textStart := rangeBoundary{container: child, offset: 0}
					textEnd := rangeBoundary{container: child, offset: len(characterDataCodeUnits(child))}
					return compareBoundaryPoints(start, textStart) <= 0 && compareBoundaryPoints(textEnd, end) <= 0
				}
				for descendant := child.FirstChild; descendant != nil; descendant = descendant.NextSibling {
					if !inspect(descendant) {
						return false
					}
				}
				return true
			}
			allTextSelected = inspect(node)
			if allTextSelected && hasText {
				candidates = append(candidates, node)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, insideList || isList)
		}
	}
	visit(host, false)
	return candidates
}

func selectedListItemsAdjacentToNestedContent(host *dom.Node, ran *jsRange) []*dom.Node {
	if host == nil || ran == nil || ran.start.container == nil || ran.end.container == nil ||
		compareBoundaryPoints(ran.start, ran.end) == 0 {
		return nil
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	containsList := func(root *dom.Node) bool {
		stack := []*dom.Node{root}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() && (strings.EqualFold(child.Data, "ul") || strings.EqualFold(child.Data, "ol")) {
					return true
				}
				stack = append(stack, child)
			}
		}
		return false
	}
	var selected []*dom.Node
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && strings.EqualFold(node.Data, "li") && node.Parent != nil && node.Parent.Element() &&
			(strings.EqualFold(node.Parent.Data, "ul") || strings.EqualFold(node.Parent.Data, "ol")) {
			hasNestedPredecessor := false
			for previous := node.PrevSibling; previous != nil; previous = previous.PrevSibling {
				if previous.Element() && (strings.EqualFold(previous.Data, "ul") || strings.EqualFold(previous.Data, "ol") ||
					(strings.EqualFold(previous.Data, "li") && containsList(previous))) {
					hasNestedPredecessor = true
					break
				}
			}
			if following := node.NextSibling; following != nil && following.Element() &&
				(strings.EqualFold(following.Data, "ul") || strings.EqualFold(following.Data, "ol")) &&
				end.container != nil && end.container.Element() && end.offset == 0 && isAncestorNode(following, end.container) {
				hasNestedPredecessor = true
			}
			if hasNestedPredecessor {
				allTextSelected := true
				hasText := false
				var inspect func(*dom.Node) bool
				inspect = func(child *dom.Node) bool {
					if child != node && child.Element() && dom.ContentEditableState(child) == "false" {
						return false
					}
					if child.Type == dom.NodeText {
						if child.DataContent == "" {
							return true
						}
						hasText = true
						textStart := rangeBoundary{container: child, offset: 0}
						textEnd := rangeBoundary{container: child, offset: len(characterDataCodeUnits(child))}
						return compareBoundaryPoints(start, textStart) <= 0 && compareBoundaryPoints(textEnd, end) <= 0
					}
					for descendant := child.FirstChild; descendant != nil; descendant = descendant.NextSibling {
						if !inspect(descendant) {
							return false
						}
					}
					return true
				}
				allTextSelected = inspect(node)
				if allTextSelected && hasText {
					selected = append(selected, node)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(host)
	return selected
}

func selectedListWithNoneditableItem(ran *jsRange) *dom.Node {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() || ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return nil
	}
	list := childAt(ran.start.container, ran.start.offset)
	if list == nil || !list.Element() || (!strings.EqualFold(list.Data, "ul") && !strings.EqualFold(list.Data, "ol")) {
		return nil
	}
	for item := list.FirstChild; item != nil; item = item.NextSibling {
		if item.Element() && strings.EqualFold(item.Data, "li") && dom.ContentEditableState(item) == "false" {
			return list
		}
	}
	return nil
}

func (r *Runtime) ensureEditingBlockPlaceholder(block *dom.Node) {
	if r == nil || block == nil || block.Doc == nil {
		return
	}
	if block.FirstChild == nil {
		block.AppendChild(block.Doc.NewElement("br"))
		r.notifyMutation()
		return
	}
	for child := block.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != dom.NodeText || child.DataContent != "" {
			return
		}
	}
	for child := block.FirstChild; child != nil; {
		next := child.NextSibling
		index := childIndex(block, child)
		r.adjustLiveRangesForNodeRemoval(block, child, index)
		block.RemoveChild(child)
		child = next
	}
	block.AppendChild(block.Doc.NewElement("br"))
	r.notifyMutation()
}

func nextCharacterBoundary(text *dom.Node, offset int) int {
	units := characterDataCodeUnits(text)
	if offset < 0 {
		offset = 0
	}
	if offset >= len(units) {
		return len(units)
	}
	if units[offset] >= 0xD800 && units[offset] <= 0xDBFF && offset+1 < len(units) && units[offset+1] >= 0xDC00 && units[offset+1] <= 0xDFFF {
		return offset + 2
	}
	return offset + 1
}

func previousEditableTextBoundary(host, container *dom.Node, offset int) (rangeBoundary, bool) {
	if host == nil || container == nil || !isAncestorNode(host, container) {
		return rangeBoundary{}, false
	}
	if container.Type == dom.NodeText {
		units := characterDataCodeUnits(container)
		if offset > len(units) {
			offset = len(units)
		}
		if offset > 0 {
			start := offset - 1
			if units[start] >= 0xDC00 && units[start] <= 0xDFFF && start > 0 && units[start-1] >= 0xD800 && units[start-1] <= 0xDBFF {
				start--
			}
			return rangeBoundary{container: container, offset: start}, true
		}
	}
	if container.Element() {
		child := container.FirstChild
		var previous *dom.Node
		for i := 0; i < offset && child != nil; i++ {
			previous = child
			child = child.NextSibling
		}
		if previous != nil {
			if text := lastEditableTextInSubtree(previous, host); text != nil {
				if start, ok := previousCharacterStart(text); ok {
					return rangeBoundary{container: text, offset: start}, true
				}
			}
		}
	}
	for node := container; node != nil && node != host; {
		if sibling := node.PrevSibling; sibling != nil {
			node = sibling
			if text := lastEditableTextInSubtree(node, host); text != nil {
				if start, ok := previousCharacterStart(text); ok {
					return rangeBoundary{container: text, offset: start}, true
				}
			}
			continue
		}
		node = node.Parent
	}
	return rangeBoundary{}, false
}

// textChildTargetRange follows the editing target-range rule for a selection
// that selects exactly one Text child through its parent element boundaries.
// The public Range remains element-boundary based; the beforeinput target
// range instead identifies the selected character data.
func textChildTargetRange(ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	parent := ran.start.container
	if ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	child := parent.FirstChild
	for i := 0; i < ran.start.offset && child != nil; i++ {
		child = child.NextSibling
	}
	if child == nil || child.Type != dom.NodeText {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	return rangeBoundary{container: child, offset: 0}, rangeBoundary{container: child, offset: len(characterDataCodeUnits(child))}, true
}

// nonEditableListTargetRange maps a selection of a list with noneditable edge
// items to the editable content at each edge. Editing hosts preserve list
// structure around noneditable items, so those edges remain list boundaries.
func nonEditableListTargetRange(ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	parent := ran.start.container
	if ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	list := childAt(parent, ran.start.offset)
	if list == nil || !list.Element() || (!strings.EqualFold(list.Data, "ul") && !strings.EqualFold(list.Data, "ol")) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	first, last := list.FirstChild, list.LastChild
	if first == nil || last == nil || !first.Element() || !last.Element() || !strings.EqualFold(first.Data, "li") || !strings.EqualFold(last.Data, "li") {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	hasNoneditable := false
	for item := list.FirstChild; item != nil; item = item.NextSibling {
		if item.Element() && strings.EqualFold(item.Data, "li") && dom.ContentEditableState(item) == "false" {
			hasNoneditable = true
			break
		}
	}
	if !hasNoneditable {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	start := rangeBoundary{container: first, offset: 0}
	if dom.ContentEditableState(first) == "false" {
		start = rangeBoundary{container: list, offset: 0}
	} else if boundary, ok := listItemEdgeBoundary(first, true); ok {
		start = boundary
	}
	end := rangeBoundary{container: last, offset: nodeChildCount(last)}
	if dom.ContentEditableState(last) == "false" {
		end = rangeBoundary{container: list, offset: nodeChildCount(list)}
	} else if boundary, ok := listItemEdgeBoundary(last, false); ok {
		end = boundary
	}
	return start, end, true
}

func listItemEdgeBoundary(item *dom.Node, start bool) (rangeBoundary, bool) {
	var visit func(*dom.Node) (rangeBoundary, bool)
	visit = func(node *dom.Node) (rangeBoundary, bool) {
		if node != item && node.Element() && dom.ContentEditableState(node) == "false" {
			return rangeBoundary{}, false
		}
		if node.Type == dom.NodeText {
			if start {
				return rangeBoundary{container: node, offset: 0}, true
			}
			return rangeBoundary{container: node, offset: len(characterDataCodeUnits(node))}, true
		}
		if node.Element() && strings.EqualFold(node.Data, "br") {
			if start {
				return rangeBoundary{container: node, offset: 0}, true
			}
			parent := node.Parent
			if parent != nil {
				return rangeBoundary{container: parent, offset: childIndex(parent, node) + 1}, true
			}
		}
		if start {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if boundary, ok := visit(child); ok {
					return boundary, true
				}
			}
		} else {
			for child := node.LastChild; child != nil; child = child.PrevSibling {
				if boundary, ok := visit(child); ok {
					return boundary, true
				}
			}
		}
		return rangeBoundary{}, false
	}
	return visit(item)
}

// listItemTextTargetRange reports the selected text when a list selection
// encloses one list item. The public selection keeps list boundaries, while
// beforeinput targets the item text that deletion edits.
func listItemTextTargetRange(ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	list := ran.start.container
	if !strings.EqualFold(list.Data, "ul") && !strings.EqualFold(list.Data, "ol") {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	item := childAt(list, ran.start.offset)
	if item == nil || item.NextSibling != nil || !item.Element() || !strings.EqualFold(item.Data, "li") || dom.ContentEditableState(item) == "false" {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	text := item.FirstChild
	if text == nil || text != item.LastChild || text.Type != dom.NodeText {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	return rangeBoundary{container: text, offset: 0}, rangeBoundary{container: text, offset: len(characterDataCodeUnits(text))}, true
}

func nestedListTextTargetRange(ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if ran == nil || ran.start.container == nil || ran.start.container != ran.end.container || !ran.start.container.Element() || ran.start.offset < 0 || ran.end.offset != ran.start.offset+1 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	selected := childAt(ran.start.container, ran.start.offset)
	if selected == nil || !selected.Element() || (!strings.EqualFold(selected.Data, "ul") && !strings.EqualFold(selected.Data, "ol")) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	var text *dom.Node
	stack := []*dom.Node{selected}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node != selected && node.Element() && dom.ContentEditableState(node) == "false" {
			return rangeBoundary{}, rangeBoundary{}, false
		}
		if node.Type == dom.NodeText {
			if text != nil {
				return rangeBoundary{}, rangeBoundary{}, false
			}
			text = node
			continue
		}
		if node.Element() && strings.EqualFold(node.Data, "br") {
			return rangeBoundary{}, rangeBoundary{}, false
		}
		for child := node.LastChild; child != nil; child = child.PrevSibling {
			stack = append(stack, child)
		}
	}
	if text == nil {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	return rangeBoundary{container: text, offset: 0}, rangeBoundary{container: text, offset: len(characterDataCodeUnits(text))}, true
}

func listItemTextBoundaryTargetRange(host *dom.Node, ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if host == nil || ran == nil || ran.start.container == nil || ran.end.container == nil || compareBoundaryPoints(ran.start, ran.end) == 0 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	start, end := ran.start, ran.end
	if start.container.Type == dom.NodeText && end.container.Element() && strings.EqualFold(end.container.Data, "li") && end.offset == 0 {
		if text := firstEditableTextInSubtree(end.container, host); text != nil && separateEditingListRoots(start.container, end.container, host) {
			return start, rangeBoundary{container: text, offset: 0}, true
		}
	}
	if end.container.Type == dom.NodeText && start.container.Element() && strings.EqualFold(start.container.Data, "li") && start.offset == nodeChildCount(start.container) {
		if text := lastEditableTextInSubtree(start.container, host); text != nil && separateEditingListRoots(end.container, start.container, host) {
			return rangeBoundary{container: text, offset: len(characterDataCodeUnits(text))}, end, true
		}
	}
	return rangeBoundary{}, rangeBoundary{}, false
}

func singleDefinitionListSelectionTargetRange(host *dom.Node, ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if host == nil || ran == nil || ran.start.container == nil || ran.end.container == nil ||
		ran.start.container.Type != dom.NodeText || ran.end.container.Type != dom.NodeText ||
		compareBoundaryPoints(ran.start, ran.end) >= 0 ||
		ran.start.offset != len(characterDataCodeUnits(ran.start.container)) ||
		ran.end.offset != len(characterDataCodeUnits(ran.end.container)) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	definitionItemFor := func(node *dom.Node) *dom.Node {
		for current := node; current != nil && current != host; current = current.Parent {
			if current.Element() && (strings.EqualFold(current.Data, "dt") || strings.EqualFold(current.Data, "dd")) &&
				current.Parent != nil && strings.EqualFold(current.Parent.Data, "dl") {
				return current
			}
		}
		return nil
	}
	startItem, endItem := definitionItemFor(ran.start.container), definitionItemFor(ran.end.container)
	if startItem == nil || endItem == nil || startItem == endItem ||
		firstEditableTextInSubtree(startItem, host) != ran.start.container ||
		lastEditableTextInSubtree(endItem, host) != ran.end.container {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	startList, endList := startItem.Parent, endItem.Parent
	definitionItemCount := func(list *dom.Node) int {
		count := 0
		for child := list.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && (strings.EqualFold(child.Data, "dt") || strings.EqualFold(child.Data, "dd")) {
				count++
				continue
			}
			if child.Type != dom.NodeText || strings.TrimSpace(child.Data) != "" {
				return 0
			}
		}
		return count
	}
	startItemCount, endItemCount := definitionItemCount(startList), definitionItemCount(endList)
	if startList == endList || startList.Parent == nil || startList.Parent != endList.Parent ||
		startItemCount == 0 || endItemCount == 0 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	for sibling := startList.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling == endList {
			if startItemCount > 1 && endItemCount == 1 {
				return ran.start, rangeBoundary{container: endItem, offset: 0}, true
			}
			if startItemCount == 1 && endItemCount == 1 {
				start := ran.start
				start.offset = 0
				return start, ran.end, true
			}
			return rangeBoundary{}, rangeBoundary{}, false
		}
		if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.Data) != "" {
			return rangeBoundary{}, rangeBoundary{}, false
		}
	}
	return rangeBoundary{}, rangeBoundary{}, false
}

func separateEditingListRoots(start, end, host *dom.Node) bool {
	rootFor := func(node *dom.Node) *dom.Node {
		var root *dom.Node
		for current := node; current != nil && current != host; current = current.Parent {
			if current.Element() && (strings.EqualFold(current.Data, "ul") || strings.EqualFold(current.Data, "ol")) {
				root = current
			}
		}
		return root
	}
	startRoot, endRoot := rootFor(start), rootFor(end)
	if startRoot == nil || endRoot == nil || startRoot == endRoot {
		return false
	}
	stack := []*dom.Node{startRoot}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && (strings.EqualFold(child.Data, "ul") || strings.EqualFold(child.Data, "ol")) {
				return true
			}
			stack = append(stack, child)
		}
	}
	return false
}

// normalizeEditingTargetRange includes collapsed boundary whitespace when a
// non-collapsed edit selection crosses an editing block or selects an atomic
// editing element. Those spaces would otherwise become visible after deletion
// or insertion at the join point.
func normalizeEditingTargetRange(host *dom.Node, ran *jsRange) (rangeBoundary, rangeBoundary, bool) {
	if host == nil || ran == nil || ran.start.container == nil || ran.end.container == nil {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if ran.start.container == ran.end.container && ran.start.container.Element() && ran.end.offset == ran.start.offset+1 {
		parent := ran.start.container
		child := childAt(parent, ran.start.offset)
		if isAtomicEditingElement(child) && !strings.EqualFold(child.Data, "br") && child.PrevSibling != nil && child.NextSibling != nil &&
			child.PrevSibling.Type == dom.NodeText && child.NextSibling.Type == dom.NodeText {
			previous, next := child.PrevSibling, child.NextSibling
			adjacent := &jsRange{
				start: rangeBoundary{container: previous, offset: len(characterDataCodeUnits(previous))},
				end:   rangeBoundary{container: next, offset: 0},
			}
			return normalizeEditingTargetRange(host, adjacent)
		}
	}
	if ran.start.container.Type != dom.NodeText || ran.end.container.Type != dom.NodeText {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	startBlock := nearestEditingBlock(host, start.container)
	endBlock := nearestEditingBlock(host, end.container)
	structural := startBlock != nil && endBlock != nil && startBlock != endBlock
	if !structural {
		root := commonAncestor(start.container, end.container)
		var findAtomic func(*dom.Node) bool
		findAtomic = func(node *dom.Node) bool {
			if node == nil {
				return false
			}
			if isAtomicEditingElement(node) && fullyContainedByRange(node, start, end) {
				return true
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if findAtomic(child) {
					return true
				}
			}
			return false
		}
		structural = findAtomic(root)
	}
	if !structural {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	startUnits := characterDataCodeUnits(start.container)
	if start.offset >= len(startUnits) {
		start.offset = len(startUnits)
		for start.offset > 0 && isHTMLSpaceCodeUnit(startUnits[start.offset-1]) {
			start.offset--
		}
	}
	endUnits := characterDataCodeUnits(end.container)
	if end.offset <= 0 {
		end.offset = 0
		for end.offset < len(endUnits) && isHTMLSpaceCodeUnit(endUnits[end.offset]) {
			end.offset++
		}
	}
	return start, end, true
}

func (r *Runtime) deletionTargetRange(host *dom.Node, ran *jsRange, inputType string) (rangeBoundary, rangeBoundary, bool) {
	if host == nil || ran == nil || ran.start.container == nil || compareBoundaryPoints(ran.start, ran.end) != 0 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	backward := inputType == "deleteContentBackward" || inputType == "deleteWordBackward"
	forward := inputType == "deleteContentForward" || inputType == "deleteWordForward" || inputType == "deleteSoftLineForward"
	if !backward && !forward {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if forward {
		if inputType == "deleteSoftLineForward" {
			return r.softLineForwardTargetRange(host, ran.start)
		}
		if start, end, ok := r.emptyPlaceholderBeforeNonEditableBlockTargetRange(host, ran.start); ok {
			return start, end, true
		}
	}
	if backward && ran.start.container.Type == dom.NodeText && ran.start.offset == 0 {
		item := ran.start.container.Parent
		for item != nil && item != host && (!item.Element() || !strings.EqualFold(item.Data, "li")) {
			item = item.Parent
		}
		if item != nil && item != host && item.Parent != nil && item.Parent.Element() &&
			(strings.EqualFold(item.Parent.Data, "ul") || strings.EqualFold(item.Parent.Data, "ol")) {
			hasBreak := false
			for child := item.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() && strings.EqualFold(child.Data, "br") {
					hasBreak = true
					break
				}
			}
			previous := item.PrevSibling
			if hasBreak && previous != nil && previous.Element() && strings.EqualFold(previous.Data, "li") && editingHostFor(previous) == host &&
				(previous.LastChild == nil || !previous.LastChild.Element() || !strings.EqualFold(previous.LastChild.Data, "br")) {
				return rangeBoundary{container: previous, offset: nodeChildCount(previous)}, ran.start, true
			}
		}
	}
	if start, end, ok := emptyListItemTargetRange(host, ran.start); ok {
		return start, end, true
	}
	if start, end, ok := r.emptyEditingBlockTargetRange(host, ran.start); ok {
		return start, end, true
	}
	point := ran.start
	container := point.container
	if forward && container.Element() {
		if start, end, ok := r.forwardElementTargetRange(host, point); ok {
			return start, end, true
		}
	}
	if backward && container.Element() && point.offset > 0 {
		previous := childAt(container, point.offset-1)
		if previous != nil && isAtomicEditingElement(previous) && editingHostFor(previous) == host {
			index := childIndex(container, previous)
			return rangeBoundary{container: container, offset: index}, rangeBoundary{container: container, offset: index + 1}, true
		}
	}
	if container.Type == dom.NodeText {
		units := characterDataCodeUnits(container)
		offset := point.offset
		if offset < 0 {
			offset = 0
		}
		if offset > len(units) {
			offset = len(units)
		}
		point.offset = offset
		if backward && (offset == 0 || isLeadingHTMLSpaceRun(units, offset)) {
			var previous *dom.Node
			for current := container; current != nil && current != host && previous == nil; current = current.Parent {
				candidate := current.PrevSibling
				for candidate != nil && candidate.Type == dom.NodeText && strings.TrimSpace(candidate.DataContent) == "" {
					candidate = candidate.PrevSibling
				}
				if candidate != nil && isEditingBlockElement(candidate) {
					previous = candidate
				}
			}
			if previous != nil && editingHostFor(previous) == host &&
				(previous.LastChild == nil || !previous.LastChild.Element() || !strings.EqualFold(previous.LastChild.Data, "br")) {
				if previousText := lastEditableTextInSubtree(previous, host); previousText != nil {
					previousUnits := characterDataCodeUnits(previousText)
					start := len(previousUnits)
					previousWhiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(previous)["white-space"]))
					currentBlock := nearestEditingBlock(host, container)
					currentWhiteSpace := ""
					if currentBlock != nil {
						currentWhiteSpace = strings.ToLower(strings.TrimSpace(r.computedValues(currentBlock)["white-space"]))
					}
					preserveWhitespace := func(value string) bool {
						return value == "pre" || value == "pre-wrap" || value == "break-spaces"
					}
					if !preserveWhitespace(previousWhiteSpace) && !preserveWhitespace(currentWhiteSpace) && !strings.EqualFold(previous.Data, "pre") && (currentBlock == nil || !strings.EqualFold(currentBlock.Data, "pre")) {
						for start > 0 && isHTMLSpaceCodeUnit(previousUnits[start-1]) {
							start--
						}
					}
					end := offset
					if !preserveWhitespace(currentWhiteSpace) && (currentBlock == nil || !strings.EqualFold(currentBlock.Data, "pre")) {
						end = leadingHTMLSpaceEnd(units)
					}
					return rangeBoundary{container: previousText, offset: start}, rangeBoundary{container: container, offset: end}, true
				}
			}
		}
		if backward && (offset == 0 || isLeadingHTMLSpaceRun(units, offset)) {
			for block := nearestEditingBlock(host, container); block != nil; block = nearestEditingBlock(host, block.Parent) {
				if firstEditableTextInSubtree(block, host) != container {
					if block.Parent == nil || block.Parent == host {
						break
					}
					continue
				}
				previous := block.PrevSibling
				for previous != nil && previous.Type == dom.NodeText && strings.TrimSpace(previous.DataContent) == "" {
					previous = previous.PrevSibling
				}
				if previous != nil && !isEditingBlockElement(previous) && editingHostFor(previous) == host {
					if previousText := lastEditableTextInSubtree(previous, host); previousText != nil {
						previousUnits := characterDataCodeUnits(previousText)
						start := len(previousUnits)
						whiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(block)["white-space"]))
						if whiteSpace != "pre" && whiteSpace != "pre-wrap" && whiteSpace != "break-spaces" && !strings.EqualFold(block.Data, "pre") {
							for start > 0 && isHTMLSpaceCodeUnit(previousUnits[start-1]) {
								start--
							}
						}
						end := offset
						if end == 0 {
							end = leadingHTMLSpaceEnd(units)
						}
						return rangeBoundary{container: previousText, offset: start}, rangeBoundary{container: container, offset: end}, true
					}
				}
				if block.Parent == nil || block.Parent == host {
					break
				}
			}
		}
		if backward && container.Parent != nil && len(units) > 0 && isHTMLSpaceCodeUnit(units[0]) {
			if previous := container.PrevSibling; previous != nil && previous.Element() && strings.EqualFold(previous.Data, "br") {
				spaceEnd := leadingHTMLSpaceEnd(units)
				if offset <= spaceEnd {
					end := offset
					if end == 0 {
						end = spaceEnd
					}
					index := childIndex(container.Parent, previous)
					return rangeBoundary{container: container.Parent, offset: index}, rangeBoundary{container: container, offset: end}, true
				}
			}
		}
		if backward && isLeadingHTMLSpaceRun(units, offset) && container.Parent != nil {
			if previous := container.PrevSibling; previous != nil && previous.Element() && strings.EqualFold(previous.Data, "hr") {
				index := childIndex(container.Parent, previous)
				return rangeBoundary{container: container.Parent, offset: index}, point, true
			}
		}
		if backward && offset > 0 && isLeadingHTMLSpaceRun(units, offset) {
			block := nearestEditingBlock(host, container)
			if block != nil {
				previous := block.PrevSibling
				for previous != nil && previous.Type == dom.NodeText && strings.TrimSpace(previous.Data) == "" {
					previous = previous.PrevSibling
				}
				if previous != nil && isEditingBlockElement(previous) {
					if previous.LastChild != nil && previous.LastChild.Element() && strings.EqualFold(previous.LastChild.Data, "br") {
						index := childIndex(previous, previous.LastChild)
						return rangeBoundary{container: previous, offset: index}, rangeBoundary{container: container, offset: leadingHTMLSpaceEnd(units)}, true
					}
					if strings.EqualFold(previous.Data, "pre") || strings.EqualFold(block.Data, "pre") {
						if previousText := lastEditableTextInSubtree(previous, host); previousText != nil {
							return rangeBoundary{container: previousText, offset: len(characterDataCodeUnits(previousText))}, rangeBoundary{container: container, offset: leadingHTMLSpaceEnd(units)}, true
						}
					} else if previousText := lastEditableTextInSubtree(previous, host); previousText != nil {
						previousUnits := characterDataCodeUnits(previousText)
						start := len(previousUnits)
						for start > 0 && isHTMLSpaceCodeUnit(previousUnits[start-1]) {
							start--
						}
						return rangeBoundary{container: previousText, offset: start}, rangeBoundary{container: container, offset: leadingHTMLSpaceEnd(units)}, true
					}
				}
			}
		}
		if forward {
			if start, end, ok := r.forwardBlockWhitespaceTargetRange(host, container, point); ok {
				return start, end, true
			}
			if offset < len(units) && isHTMLSpaceCodeUnit(units[offset]) {
				block := nearestEditingBlock(host, container)
				whiteSpace := ""
				if block != nil {
					whiteSpace = strings.ToLower(strings.TrimSpace(r.computedValues(block)["white-space"]))
				}
				if whiteSpace == "pre-line" {
					end := offset
					for end < len(units) && isHTMLSpaceCodeUnit(units[end]) {
						end++
					}
					return point, rangeBoundary{container: container, offset: end}, true
				}
			}
		}
		if backward && offset > 0 {
			start := offset - 1
			if units[start] >= 0xDC00 && units[start] <= 0xDFFF && start > 0 && units[start-1] >= 0xD800 && units[start-1] <= 0xDBFF {
				start--
			}
			if isHTMLSpaceCodeUnit(units[offset-1]) {
				block := nearestEditingBlock(host, container)
				whiteSpace := ""
				if block != nil {
					whiteSpace = strings.ToLower(r.computedValues(block)["white-space"])
				}
				if whiteSpace == "pre-line" {
					crossedLineBreak := false
					for start > 0 && isHTMLSpaceCodeUnit(units[start-1]) {
						if units[start-1] == '\n' {
							if crossedLineBreak {
								break
							}
							crossedLineBreak = true
						}
						start--
					}
				} else {
					for start > 0 && isHTMLSpaceCodeUnit(units[start-1]) {
						start--
					}
				}
			}
			if inputType == "deleteWordBackward" {
				for start > 0 && isHTMLSpaceCodeUnit(units[start-1]) {
					start--
				}
				for start > 0 && !isHTMLSpaceCodeUnit(units[start-1]) {
					start--
				}
				// A word can span inline formatting boundaries. Continue into
				// preceding editable text nodes until whitespace or a block edge.
				if start == 0 {
					cursor := rangeBoundary{container: container, offset: 0}
					for previous, ok := previousEditableTextBoundary(host, cursor.container, cursor.offset); ok; previous, ok = previousEditableTextBoundary(host, cursor.container, cursor.offset) {
						if previous.container == nil || previous.container.Type != dom.NodeText {
							break
						}
						previousUnits := characterDataCodeUnits(previous.container)
						if previous.offset < len(previousUnits) && isHTMLSpaceCodeUnit(previousUnits[previous.offset]) {
							break
						}
						start = previous.offset
						container = previous.container
						cursor = rangeBoundary{container: container, offset: start}
					}
				}
			}
			// Deleting the last character inside an inline formatting element
			// removes that element as one editing unit in the current UA model.
			if inputType != "deleteWordBackward" && offset == len(units) {
				for ancestor := container.Parent; ancestor != nil && ancestor != host; ancestor = ancestor.Parent {
					if isEditingBlockElement(ancestor) {
						break
					}
					if ancestor.Element() && lastEditableTextInSubtree(ancestor, host) == container && ancestor.Parent != nil {
						index := childIndex(ancestor.Parent, ancestor)
						return rangeBoundary{container: ancestor.Parent, offset: index}, rangeBoundary{container: ancestor.Parent, offset: index + 1}, true
					}
				}
			}
			return rangeBoundary{container: container, offset: start}, point, true
		}
		if forward && offset < len(units) {
			end := offset + 1
			if units[offset] >= 0xD800 && units[offset] <= 0xDBFF && end < len(units) && units[end] >= 0xDC00 && units[end] <= 0xDFFF {
				end++
			}
			if inputType == "deleteWordForward" {
				for end < len(units) && !isHTMLSpaceCodeUnit(units[end]) {
					end++
				}
				spaceStart := end
				for end < len(units) && isHTMLSpaceCodeUnit(units[end]) {
					end++
				}
				block := nearestEditingBlock(host, container)
				whiteSpace := ""
				if block != nil {
					whiteSpace = strings.ToLower(strings.TrimSpace(r.computedValues(block)["white-space"]))
				}
				preservesSpaces := whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" || (block != nil && strings.EqualFold(block.Data, "pre"))
				if !preservesSpaces && end < len(units) && end-spaceStart > 1 {
					end-- // Retain one collapsed space to separate the following word.
				}
			}
			if offset == 0 {
				for ancestor := container.Parent; ancestor != nil && ancestor != host; ancestor = ancestor.Parent {
					if isEditingBlockElement(ancestor) {
						break
					}
					if ancestor.Element() && firstEditableTextInSubtree(ancestor, host) == container && ancestor.Parent != nil {
						index := childIndex(ancestor.Parent, ancestor)
						return rangeBoundary{container: ancestor.Parent, offset: index}, rangeBoundary{container: ancestor.Parent, offset: index + 1}, true
					}
				}
			}
			return point, rangeBoundary{container: container, offset: end}, true
		}
	}
	if backward {
		if container.Type == dom.NodeText && point.offset == 0 {
			units := characterDataCodeUnits(container)
			if parent := container.Parent; parent != nil {
				// A line break immediately before the caret is a deletion unit.
				// Keep the end at the text boundary so consecutive BRs delete one
				// line break at a time instead of collapsing the range before all of
				// them via previousEditableTextBoundary.
				if previous := container.PrevSibling; previous != nil && previous.Element() && strings.EqualFold(previous.Data, "br") {
					index := childIndex(parent, previous)
					return rangeBoundary{container: parent, offset: index}, rangeBoundary{container: parent, offset: index + 1}, true
				}
				if previous := container.PrevSibling; previous != nil && previous.Element() && dom.ContentEditableState(previous) == "false" {
					return point, point, true
				}
				if previous := container.PrevSibling; previous != nil && previous.Element() && isAtomicEditingElement(previous) {
					if strings.EqualFold(previous.Data, "hr") && previous.PrevSibling != nil && previous.PrevSibling.Element() && strings.EqualFold(previous.PrevSibling.Data, "br") {
						index := childIndex(parent, previous.PrevSibling)
						return rangeBoundary{container: parent, offset: index}, rangeBoundary{container: parent, offset: index + 2}, true
					}
					if strings.EqualFold(previous.Data, "hr") && previous.PrevSibling != nil && previous.PrevSibling.Type == dom.NodeText {
						previousText := previous.PrevSibling
						units := characterDataCodeUnits(previousText)
						start := len(units)
						for start > 0 && isHTMLSpaceCodeUnit(units[start-1]) {
							start--
						}
						if start < len(units) {
							index := childIndex(parent, previous)
							return rangeBoundary{container: previousText, offset: start}, rangeBoundary{container: parent, offset: index + 1}, true
						}
					}
					index := childIndex(parent, previous)
					return rangeBoundary{container: parent, offset: index}, rangeBoundary{container: parent, offset: index + 1}, true
				} else if previous != nil && previous.Element() && !isEditingBlockElement(previous) {
					index := childIndex(parent, previous)
					return rangeBoundary{container: parent, offset: index}, point, true
				}
			}
			block := nearestEditingBlock(host, container)
			if previous := previousSiblingEditingBlock(host, block); previous != nil {
				hasLineBreak := false
				if block != nil && strings.EqualFold(block.Data, "li") {
					for child := block.FirstChild; child != nil; child = child.NextSibling {
						if child.Element() && strings.EqualFold(child.Data, "br") {
							hasLineBreak = true
							break
						}
					}
				}
				if hasLineBreak && block != nil && strings.EqualFold(block.Data, "li") && strings.EqualFold(previous.Data, "li") && previous.Parent == block.Parent &&
					(previous.LastChild == nil || !previous.LastChild.Element() || !strings.EqualFold(previous.LastChild.Data, "br")) {
					return rangeBoundary{container: previous, offset: nodeChildCount(previous)}, point, true
				}
				if previous.LastChild != nil && previous.LastChild.Element() && strings.EqualFold(previous.LastChild.Data, "br") {
					lastBreak := previous.LastChild
					firstTrailingBreak := lastBreak
					if block != nil && strings.EqualFold(block.Data, "li") && strings.EqualFold(previous.Data, "li") {
						for sibling := firstTrailingBreak.PrevSibling; sibling != nil && sibling.Element() && strings.EqualFold(sibling.Data, "br"); sibling = sibling.PrevSibling {
							firstTrailingBreak = sibling
						}
					}
					index := childIndex(previous, lastBreak)
					if firstTrailingBreak != lastBreak {
						index = childIndex(previous, firstTrailingBreak)
					}
					return rangeBoundary{container: previous, offset: index}, point, true
				}
				if previousText := lastEditableTextInSubtree(previous, host); previousText != nil {
					start := len(characterDataCodeUnits(previousText))
					end := 0
					if leadingHTMLSpaceEnd(units) > 0 && !strings.EqualFold(previous.Data, "pre") && !strings.EqualFold(block.Data, "pre") {
						previousUnits := characterDataCodeUnits(previousText)
						for start > 0 && isHTMLSpaceCodeUnit(previousUnits[start-1]) {
							start--
						}
						end = leadingHTMLSpaceEnd(units)
					}
					return rangeBoundary{container: previousText, offset: start}, rangeBoundary{container: container, offset: end}, true
				}
			}
			if previous, ok := previousEditableTextBoundary(host, container, point.offset); ok {
				return previous, point, true
			}
		}
		return point, point, true
	}
	if forward && container.Type == dom.NodeText && point.offset >= len(characterDataCodeUnits(container)) {
		if parent := container.Parent; parent != nil {
			if next := container.NextSibling; next != nil && next.Element() && strings.EqualFold(next.Data, "br") {
				index := childIndex(parent, next)
				if next.NextSibling == nil {
					if block := nearestEditingBlock(host, parent); block != nil {
						if following := nextSiblingEditingBlock(host, block); following != nil {
							if text := firstEditableTextInSubtree(following, host); text != nil {
								return point, rangeBoundary{container: text, offset: leadingTargetSpaceEnd(r, following, text)}, true
							}
						}
					}
					return point, point, true
				}
				if nextText := next.NextSibling; nextText != nil && nextText.Type == dom.NodeText {
					end := rangeBoundary{container: nextText, offset: leadingTargetSpaceEnd(r, nearestEditingBlock(host, nextText), nextText)}
					if end.offset > 0 {
						return rangeBoundary{container: parent, offset: index}, end, true
					}
					return rangeBoundary{container: parent, offset: index}, rangeBoundary{container: parent, offset: index + 1}, true
				}
				return rangeBoundary{container: parent, offset: index}, rangeBoundary{container: parent, offset: index + 1}, true
			}
			if next := container.NextSibling; next != nil && isAtomicEditingElement(next) && editingHostFor(next) == host {
				index := childIndex(parent, next)
				return rangeBoundary{container: parent, offset: index}, rangeBoundary{container: parent, offset: index + 1}, true
			}
			if next := container.NextSibling; next != nil && next.Element() && dom.ContentEditableState(next) == "false" {
				return point, point, true
			}
			if parent.Element() && strings.EqualFold(parent.Data, "li") && parent.Parent != nil && parent.Parent.Element() &&
				(strings.EqualFold(parent.Parent.Data, "ul") || strings.EqualFold(parent.Parent.Data, "ol")) {
				trailingBreaks := true
				hasBreak := false
				for sibling := container.NextSibling; sibling != nil; sibling = sibling.NextSibling {
					if sibling.Type == dom.NodeText && strings.TrimSpace(sibling.DataContent) == "" {
						continue
					}
					if sibling.Element() && strings.EqualFold(sibling.Data, "br") {
						hasBreak = true
						continue
					}
					trailingBreaks = false
					break
				}
				if hasBreak && trailingBreaks {
					nextItem := parent.NextSibling
					for nextItem != nil && nextItem.Type == dom.NodeText && strings.TrimSpace(nextItem.DataContent) == "" {
						nextItem = nextItem.NextSibling
					}
					if nextItem != nil && nextItem.Element() && strings.EqualFold(nextItem.Data, "li") && dom.ContentEditableState(nextItem) != "false" {
						if nextText := firstEditableTextInSubtree(nextItem, host); nextText != nil {
							return point, rangeBoundary{container: nextText, offset: 0}, true
						}
					}
				}
			}
			if next := container.NextSibling; next != nil && next.Element() && !isEditingBlockElement(next) {
				index := childIndex(parent, next)
				return point, rangeBoundary{container: parent, offset: index + 1}, true
			}
		}
		block := editingBlockForBoundary(host, container)
		if next := nextSiblingEditingBlock(host, block); next != nil {
			if nextText := firstEditableTextInSubtree(next, host); nextText != nil {
				return point, rangeBoundary{container: nextText, offset: 0}, true
			}
		}
		if next, ok := nextEditableTextBoundary(host, container, point.offset); ok {
			if next.container != nil && next.container.Type == dom.NodeText && editingBlockForBoundary(host, next.container) != block {
				nextBlock := editingBlockForBoundary(host, next.container)
				return point, rangeBoundary{container: next.container, offset: leadingTargetSpaceEnd(r, nextBlock, next.container)}, true
			}
			return point, next, true
		}
	}
	return point, point, true
}

func (r *Runtime) softLineForwardTargetRange(host *dom.Node, point rangeBoundary) (rangeBoundary, rangeBoundary, bool) {
	if r == nil || host == nil || point.container == nil || !isAncestorNode(host, point.container) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	block := editingBlockForBoundary(host, point.container)
	if block == nil {
		block = host
	}
	var breakBoundary rangeBoundary
	foundBreak := false
	var findBreak func(*dom.Node)
	findBreak = func(node *dom.Node) {
		if node == nil || foundBreak {
			return
		}
		if node != block && node.Element() && dom.ContentEditableState(node) == "false" {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && strings.EqualFold(child.Data, "br") {
				before := rangeBoundary{container: node, offset: childIndex(node, child)}
				if compareBoundaryPoints(point, before) < 0 {
					breakBoundary, foundBreak = before, true
					return
				}
				continue
			}
			findBreak(child)
			if foundBreak {
				return
			}
		}
	}
	findBreak(block)
	if foundBreak {
		if compareBoundaryPoints(point, breakBoundary) < 0 {
			return point, breakBoundary, true
		}
		return point, point, true
	}
	text := lastEditableTextInSubtree(block, host)
	if text == nil {
		return point, point, true
	}
	end := rangeBoundary{container: text, offset: len(characterDataCodeUnits(text))}
	if compareBoundaryPoints(point, end) >= 0 {
		return point, point, true
	}
	return point, end, true
}

func (r *Runtime) emptyPlaceholderBeforeNonEditableBlockTargetRange(host *dom.Node, point rangeBoundary) (rangeBoundary, rangeBoundary, bool) {
	block := point.container
	if host == nil || block == nil || !block.Element() || point.offset != 0 || editingHostFor(block) != host || !isEditingBlockElement(block) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	previous := block.PrevSibling
	for previous != nil && previous.Type == dom.NodeText && strings.TrimSpace(previous.DataContent) == "" {
		previous = previous.PrevSibling
	}
	if previous == nil || !previous.Element() || dom.ContentEditableState(previous) != "false" {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	empty := previous.PrevSibling
	for empty != nil && empty.Type == dom.NodeText && strings.TrimSpace(empty.DataContent) == "" {
		empty = empty.PrevSibling
	}
	if empty == nil || empty.Parent != block.Parent {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	return r.emptyEditingBlockTargetRange(host, rangeBoundary{container: empty, offset: 0})
}

func trimHorizontalRuleBoundaryWhitespace(r *Runtime, parent *dom.Node, offset int) {
	if r == nil || parent == nil || !parent.Element() {
		return
	}
	if previous := childAt(parent, offset-1); previous != nil && previous.Type == dom.NodeText {
		units := characterDataCodeUnits(previous)
		cut := len(units)
		for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
			cut--
		}
		if cut < len(units) {
			r.replaceCharacterData(previous, cut, len(units)-cut, "")
		}
	}
	if next := childAt(parent, offset); next != nil && next.Type == dom.NodeText {
		units := characterDataCodeUnits(next)
		cut := 0
		for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
			cut++
		}
		if cut > 0 {
			r.replaceCharacterData(next, 0, cut, "")
		}
	}
}

func selectedHorizontalRulePosition(root *dom.Node, start, end rangeBoundary) (*dom.Node, int) {
	if root == nil || compareBoundaryPoints(start, end) >= 0 {
		return nil, 0
	}
	var parent *dom.Node
	offset := 0
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil || parent != nil {
			return
		}
		if node.Element() && strings.EqualFold(node.Data, "hr") && node.Parent != nil && fullyContainedByRange(node, start, end) {
			parent = node.Parent
			offset = childIndex(node.Parent, node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
			if parent != nil {
				return
			}
		}
	}
	visit(root)
	return parent, offset
}

func (r *Runtime) forwardBlockWhitespaceTargetRange(host, container *dom.Node, point rangeBoundary) (rangeBoundary, rangeBoundary, bool) {
	if r == nil || host == nil || container == nil || container.Type != dom.NodeText || point.container != container {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	block := editingBlockForBoundary(host, container)
	units := characterDataCodeUnits(container)
	if point.offset < 0 || point.offset > len(units) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	for i := point.offset; i < len(units); i++ {
		if !isHTMLSpaceCodeUnit(units[i]) {
			return rangeBoundary{}, rangeBoundary{}, false
		}
	}
	for current := container; current != nil && current != host; current = current.Parent {
		parent := current.Parent
		if parent == nil {
			break
		}
		first := current.NextSibling
		for first != nil && first.Type == dom.NodeText && strings.TrimSpace(first.DataContent) == "" {
			first = first.NextSibling
		}
		if first == nil {
			continue
		}
		last := first
		if first.Element() && strings.EqualFold(first.Data, "br") {
			for last.NextSibling != nil && last.NextSibling.Element() && strings.EqualFold(last.NextSibling.Data, "br") {
				last = last.NextSibling
			}
		}
		if last.NextSibling != nil && last.NextSibling.Element() && strings.EqualFold(last.NextSibling.Data, "hr") {
			last = last.NextSibling
		} else if first.Element() && strings.EqualFold(first.Data, "hr") {
			last = first
		} else {
			break
		}
		return rangeBoundary{container: parent, offset: childIndex(parent, first)},
			rangeBoundary{container: parent, offset: childIndex(parent, last) + 1}, true
	}
	nextBoundary, ok := nextEditableTextBoundary(host, container, len(units))
	if !ok || nextBoundary.container == nil || nextBoundary.container.Type != dom.NodeText {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	nextText := nextBoundary.container
	nextBlock := editingBlockForBoundary(host, nextText)
	if nextBlock == block || editingHostFor(nextText) != host {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	startOffset := point.offset
	currentWhiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(block)["white-space"]))
	preservesSpaces := func(value string) bool {
		return value == "pre" || value == "pre-wrap" || value == "break-spaces"
	}
	if !preservesSpaces(currentWhiteSpace) && !strings.EqualFold(block.Data, "pre") {
		for startOffset > 0 && isHTMLSpaceCodeUnit(units[startOffset-1]) {
			startOffset--
		}
	}
	endOffset := 0
	nextWhiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(nextBlock)["white-space"]))
	if !preservesSpaces(nextWhiteSpace) && !strings.EqualFold(nextBlock.Data, "pre") {
		endOffset = leadingHTMLSpaceEnd(characterDataCodeUnits(nextText))
	}
	startBoundary := rangeBoundary{container: container, offset: startOffset}
	if startOffset == len(units) && !strings.EqualFold(block.Data, "li") &&
		!(strings.EqualFold(block.Data, "p") && strings.EqualFold(nextBlock.Data, "p")) {
		if next := container.NextSibling; next != nil && next.Element() && strings.EqualFold(next.Data, "br") {
			startBoundary = rangeBoundary{container: container.Parent, offset: childIndex(container.Parent, next)}
		}
	}
	return startBoundary, rangeBoundary{container: nextText, offset: endOffset}, true
}

func (r *Runtime) forwardElementTargetRange(host *dom.Node, point rangeBoundary) (rangeBoundary, rangeBoundary, bool) {
	container := point.container
	if host == nil || container == nil || !container.Element() || !isAncestorNode(host, container) ||
		point.offset < 0 || point.offset > nodeChildCount(container) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	child := childAt(container, point.offset)
	if child == nil {
		if isInsideTableCellOrCaption(container, host) {
			return point, point, true
		}
		block := nearestEditingBlock(host, container)
		if block == nil {
			block = host
		}
		if next := nextSiblingEditingBlock(host, block); next != nil {
			if strings.EqualFold(next.Data, "table") || hasNonEditableLeadingChild(next) {
				return point, point, true
			}
			if text := firstEditableTextInSubtree(next, host); text != nil {
				start := point
				if container != block && container.TextContent() == "" {
					empty := container
					for empty.Parent != nil && empty.Parent != block && empty.Parent.Element() && empty.Parent.TextContent() == "" {
						empty = empty.Parent
					}
					if empty.Parent == block {
						start = rangeBoundary{container: block, offset: childIndex(block, empty)}
					}
				}
				return start, rangeBoundary{container: text, offset: leadingTargetSpaceEnd(r, next, text)}, true
			}
		}
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if !child.Element() || editingHostFor(child) != host {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if strings.EqualFold(child.Data, "br") && child.NextSibling == nil && isInsideTableCellOrCaption(container, host) {
		return point, point, true
	}
	if !strings.EqualFold(child.Data, "br") {
		if isAtomicEditingElement(child) {
			return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: container, offset: point.offset + 1}, true
		}
		return rangeBoundary{}, rangeBoundary{}, false
	}
	endIndex := point.offset + 1
	for next := child.NextSibling; next != nil && next.Element() && strings.EqualFold(next.Data, "br"); next = next.NextSibling {
		endIndex++
	}
	if next := child.NextSibling; next != nil && next.Element() && strings.EqualFold(next.Data, "hr") {
		endIndex++
	}
	if child.NextSibling != nil {
		if nextText := firstEditableTextInSubtree(child.NextSibling, host); nextText != nil {
			leading := leadingTargetSpaceEnd(r, nearestEditingBlock(host, nextText), nextText)
			if leading > 0 {
				return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: nextText, offset: leading}, true
			}
			if editingBlockForBoundary(host, nextText) != editingBlockForBoundary(host, container) {
				return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: nextText, offset: 0}, true
			}
			return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: container, offset: endIndex}, true
		}
	}
	if child.NextSibling == nil {
		for current := container; current != nil && current != host; current = current.Parent {
			next := current.NextSibling
			for next != nil && next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
				next = next.NextSibling
			}
			if next == nil {
				continue
			}
			if next.Element() && (strings.EqualFold(next.Data, "table") || dom.ContentEditableState(next) == "false" || hasNonEditableLeadingChild(next)) {
				if hasNonEditableLeadingChild(next) && dom.ContentEditableState(next) != "false" {
					return point, rangeBoundary{container: next, offset: 0}, true
				}
				return point, point, true
			}
			if text := firstEditableTextInSubtree(next, host); text != nil {
				return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: text, offset: leadingTargetSpaceEnd(r, nearestEditingBlock(host, text), text)}, true
			}
			return point, point, true
		}
	}
	block := nearestEditingBlock(host, container)
	if next := nextSiblingEditingBlock(host, block); next != nil {
		if hasNonEditableLeadingChild(next) {
			return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: next, offset: 0}, true
		}
		if strings.EqualFold(next.Data, "table") {
			return point, point, true
		}
		if text := firstEditableTextInSubtree(next, host); text != nil {
			return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: text, offset: leadingTargetSpaceEnd(r, next, text)}, true
		}
	}
	if endIndex == nodeChildCount(container) {
		return point, point, true
	}
	return rangeBoundary{container: container, offset: point.offset}, rangeBoundary{container: container, offset: endIndex}, true
}

func isInsideTableCellOrCaption(node, host *dom.Node) bool {
	for current := node; current != nil && current != host; current = current.Parent {
		if current.Element() {
			switch strings.ToLower(current.Data) {
			case "td", "th", "caption":
				return true
			}
		}
	}
	return false
}

func leadingTargetSpaceEnd(r *Runtime, block, text *dom.Node) int {
	if r == nil || text == nil || text.Type != dom.NodeText {
		return 0
	}
	whiteSpace := ""
	if block != nil {
		whiteSpace = strings.ToLower(strings.TrimSpace(r.computedValues(block)["white-space"]))
	}
	if whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" || (block != nil && strings.EqualFold(block.Data, "pre")) {
		return 0
	}
	return leadingHTMLSpaceEnd(characterDataCodeUnits(text))
}

func editingBlockForBoundary(host, node *dom.Node) *dom.Node {
	if block := nearestEditingBlock(host, node); block != nil {
		return block
	}
	return host
}

func hasNonEditableLeadingChild(node *dom.Node) bool {
	if node == nil {
		return false
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == dom.NodeText && strings.TrimSpace(child.DataContent) == "" {
			continue
		}
		return child.Element() && dom.ContentEditableState(child) == "false"
	}
	return false
}

func emptyListItemTargetRange(host *dom.Node, point rangeBoundary) (rangeBoundary, rangeBoundary, bool) {
	if host == nil || point.container == nil {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	item := point.container
	for item != nil && item != host && (!item.Element() || !strings.EqualFold(item.Data, "li")) {
		item = item.Parent
	}
	if item == nil || item == host || editingHostFor(item) != host {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if point.container != item && !isAncestorNode(item, point.container) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if point.container == item && point.offset != 0 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	hasPlaceholder := false
	for child := item.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == dom.NodeText && strings.TrimSpace(child.DataContent) == "" {
			continue
		}
		if child.Element() && strings.EqualFold(child.Data, "br") {
			hasPlaceholder = true
			continue
		}
		return rangeBoundary{}, rangeBoundary{}, false
	}
	if !hasPlaceholder || item.Parent == nil || !item.Parent.Element() ||
		(!strings.EqualFold(item.Parent.Data, "ul") && !strings.EqualFold(item.Parent.Data, "ol")) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	list := item.Parent
	owner := list.Parent
	if owner == nil || !isAncestorNode(host, owner) {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	index := childIndex(owner, list)
	return rangeBoundary{container: owner, offset: index}, rangeBoundary{container: owner, offset: index + 1}, true
}

func (r *Runtime) emptyEditingBlockTargetRange(host *dom.Node, point rangeBoundary) (rangeBoundary, rangeBoundary, bool) {
	block := point.container
	if host == nil || block == nil || point.offset != 0 {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	for block != nil && block != host && (!block.Element() || !isEditingBlockElement(block)) {
		block = block.Parent
	}
	if block == nil || block == host || editingHostFor(block) != host || block.Parent == nil {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	hasPlaceholder := false
	var inspect func(*dom.Node) bool
	inspect = func(node *dom.Node) bool {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == dom.NodeText {
				if strings.TrimSpace(child.DataContent) != "" {
					return false
				}
				continue
			}
			if !child.Element() {
				continue
			}
			if strings.EqualFold(child.Data, "br") {
				hasPlaceholder = true
				continue
			}
			if dom.ContentEditableState(child) == "false" {
				return false
			}
			if !inspect(child) {
				return false
			}
		}
		return true
	}
	if !inspect(block) || !hasPlaceholder {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	previous := block.PrevSibling
	for previous != nil && previous.Type == dom.NodeText && strings.TrimSpace(previous.DataContent) == "" {
		previous = previous.PrevSibling
	}
	if previous == nil {
		next := block.NextSibling
		for next != nil && next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			next = next.NextSibling
		}
		if next != nil && next.Element() && dom.ContentEditableState(next) == "false" {
			after := next.NextSibling
			for after != nil && after.Type == dom.NodeText && strings.TrimSpace(after.DataContent) == "" {
				after = after.NextSibling
			}
			if after != nil && isEditingBlockElement(after) && dom.ContentEditableState(after) != "false" {
				return point, rangeBoundary{container: block.Parent, offset: childIndex(block.Parent, block) + 1}, true
			}
		}
	}
	if previous == nil || !isEditingBlockElement(previous) ||
		strings.EqualFold(previous.Data, "table") || strings.EqualFold(previous.Data, "ul") ||
		strings.EqualFold(previous.Data, "ol") || strings.EqualFold(previous.Data, "dl") {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	start := rangeBoundary{}
	if dom.ContentEditableState(previous) == "false" {
		start = rangeBoundary{container: previous.Parent, offset: childIndex(previous.Parent, previous) + 1}
	} else if editingHostFor(previous) != host {
		return rangeBoundary{}, rangeBoundary{}, false
	} else if previous.LastChild != nil && previous.LastChild.Element() && dom.ContentEditableState(previous.LastChild) == "false" {
		start = rangeBoundary{container: previous, offset: childIndex(previous, previous.LastChild) + 1}
	} else if previous.LastChild != nil && previous.LastChild.Element() && strings.EqualFold(previous.LastChild.Data, "br") {
		start = rangeBoundary{container: previous, offset: childIndex(previous, previous.LastChild)}
	} else if text := lastEditableTextInSubtree(previous, host); text != nil {
		offset := len(characterDataCodeUnits(text))
		start = rangeBoundary{container: text, offset: offset}
	} else {
		return rangeBoundary{}, rangeBoundary{}, false
	}
	return start, rangeBoundary{container: block, offset: nodeChildCount(block)}, true
}

func firstEditableTextInSubtree(node, host *dom.Node) *dom.Node {
	if node == nil {
		return nil
	}
	if node.Type == dom.NodeText {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && child != host && child.HasAttribute("contenteditable") && dom.IsContentEditable(child) {
			continue
		}
		if text := firstEditableTextInSubtree(child, host); text != nil {
			return text
		}
	}
	return nil
}

func nextEditableTextBoundary(host, container *dom.Node, offset int) (rangeBoundary, bool) {
	if host == nil || container == nil || !isAncestorNode(host, container) {
		return rangeBoundary{}, false
	}
	if container.Type == dom.NodeText {
		units := characterDataCodeUnits(container)
		if offset < 0 {
			offset = 0
		}
		if offset < len(units) {
			end := offset + 1
			if units[offset] >= 0xD800 && units[offset] <= 0xDBFF && end < len(units) && units[end] >= 0xDC00 && units[end] <= 0xDFFF {
				end++
			}
			return rangeBoundary{container: container, offset: end}, true
		}
	}
	if container.Element() {
		child := childAt(container, offset)
		if child != nil {
			if text := firstEditableTextInSubtree(child, host); text != nil {
				return rangeBoundary{container: text, offset: nextCharacterBoundary(text, 0)}, true
			}
		}
	}
	for node := container; node != nil && node != host; {
		if sibling := node.NextSibling; sibling != nil {
			node = sibling
			if text := firstEditableTextInSubtree(node, host); text != nil {
				return rangeBoundary{container: text, offset: nextCharacterBoundary(text, 0)}, true
			}
			continue
		}
		node = node.Parent
	}
	return rangeBoundary{}, false
}

func nearestEditingBlock(host, node *dom.Node) *dom.Node {
	for current := node; current != nil && current != host; current = current.Parent {
		if isEditingBlockElement(current) {
			return current
		}
	}
	return nil
}

func boundaryAtEditingBlockStart(host, block *dom.Node, boundary rangeBoundary) bool {
	if host == nil || block == nil || editingHostFor(block) != host {
		return false
	}
	if boundary.container == block {
		return boundary.offset == 0
	}
	firstText := firstEditableTextInSubtree(block, host)
	return firstText != nil && boundary.container == firstText && boundary.offset == 0
}

func boundaryAtEditingBlockEnd(host, block *dom.Node, boundary rangeBoundary) bool {
	if host == nil || block == nil || editingHostFor(block) != host {
		return false
	}
	if boundary.container == block {
		return boundary.offset >= nodeChildCount(block)
	}
	lastText := lastEditableTextInSubtree(block, host)
	return lastText != nil && boundary.container == lastText && boundary.offset >= len(characterDataCodeUnits(lastText))
}

func previousSiblingEditingBlock(host, block *dom.Node) *dom.Node {
	for current := block; current != nil && current != host; current = current.Parent {
		previous := current.PrevSibling
		for previous != nil && previous.Type == dom.NodeText && strings.TrimSpace(previous.Data) == "" {
			previous = previous.PrevSibling
		}
		if previous != nil && isEditingBlockElement(previous) {
			return previous
		}
	}
	return nil
}

func nextSiblingEditingBlock(host, block *dom.Node) *dom.Node {
	for current := block; current != nil && current != host; current = current.Parent {
		next := current.NextSibling
		for next != nil && next.Type == dom.NodeText && strings.TrimSpace(next.Data) == "" {
			next = next.NextSibling
		}
		if next != nil && isEditingBlockElement(next) {
			return next
		}
	}
	return nil
}

func isEditingBlockElement(node *dom.Node) bool {
	if node == nil || !node.Element() {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "address", "article", "aside", "blockquote", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "li", "main", "nav", "ol", "p", "pre", "section", "table", "ul":
		return true
	default:
		return false
	}
}

// mergeAdjacentEditingBlocks applies the structural part of a contenteditable
// deletion that spans sibling blocks. Keeping the operation here, beside the
// Range deletion algorithm, preserves the normal character-data and subtree
// removal notifications used by live ranges and custom elements.
func mergeAdjacentEditingBlocks(r *Runtime, start, end, host *dom.Node) bool {
	return mergeAdjacentEditingBlocksWithTrailingBreak(r, start, end, host, false)
}

func mergeAdjacentEditingBlocksWithTrailingBreak(r *Runtime, start, end, host *dom.Node, preserveTrailingBreak bool) bool {
	sameKind := start != nil && end != nil && strings.EqualFold(start.Data, end.Data)
	preParagraphJoin := start != nil && end != nil &&
		((strings.EqualFold(start.Data, "pre") && strings.EqualFold(end.Data, "p")) ||
			(strings.EqualFold(start.Data, "p") && strings.EqualFold(end.Data, "pre")))
	definitionListJoin := start != nil && end != nil && start != end && !sameKind && start.Parent != nil && end.Parent != nil &&
		start.Parent != end.Parent && strings.EqualFold(start.Parent.Data, "dl") && strings.EqualFold(end.Parent.Data, "dl") &&
		(strings.EqualFold(start.Data, "dt") || strings.EqualFold(start.Data, "dd")) &&
		(strings.EqualFold(end.Data, "dt") || strings.EqualFold(end.Data, "dd"))
	if r == nil || start == nil || end == nil || start == end || start.Parent == nil || end.Parent == nil ||
		!start.Element() || !end.Element() || (!sameKind && !preParagraphJoin && !definitionListJoin) ||
		!start.IsConnected() || !end.IsConnected() {
		return false
	}
	startParent, endParent := start.Parent, end.Parent
	crossListJoin := false
	var between []*dom.Node
	var containerBetween []*dom.Node
	if startParent == endParent {
		foundEnd := false
		for sibling := start.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling == end {
				foundEnd = true
				break
			}
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.Data) != "" {
				return false
			}
			between = append(between, sibling)
		}
		if !foundEnd {
			return false
		}
	} else if isListContainer(startParent) && isListContainer(endParent) && startParent.Parent != nil && startParent.Parent == endParent.Parent {
		for sibling := start.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return false
			}
		}
		for sibling := endParent.FirstChild; sibling != nil && sibling != end; sibling = sibling.NextSibling {
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return false
			}
		}
		foundEndList := false
		for sibling := startParent.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling == endParent {
				foundEndList = true
				break
			}
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return false
			}
			containerBetween = append(containerBetween, sibling)
		}
		if !foundEndList {
			return false
		}
		crossListJoin = true
	} else if nestedBetween, ok := adjacentNestedListRootsForJoin(start, end, host); ok {
		containerBetween = nestedBetween
		crossListJoin = true
	} else {
		if !strings.EqualFold(startParent.Data, "dl") || !strings.EqualFold(endParent.Data, "dl") ||
			startParent.Parent == nil || startParent.Parent != endParent.Parent {
			return false
		}
		foundEnd := false
		for sibling := startParent.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling == endParent {
				foundEnd = true
				break
			}
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return false
			}
			containerBetween = append(containerBetween, sibling)
		}
		if !foundEnd {
			return false
		}
	}
	lastText := lastEditableTextInSubtree(start, host)
	firstText := firstEditableTextInSubtree(end, host)
	preserveWhitespace := func(block *dom.Node) bool {
		if block == nil {
			return false
		}
		whiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(block)["white-space"]))
		if whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" {
			return true
		}
		return strings.EqualFold(block.Data, "pre")
	}
	if !preserveWhitespace(start) && lastText != nil {
		units := characterDataCodeUnits(lastText)
		cut := len(units)
		for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
			cut--
		}
		if cut < len(units) {
			r.replaceCharacterData(lastText, cut, len(units)-cut, "")
		}
	}
	if !preserveWhitespace(end) && firstText != nil {
		units := characterDataCodeUnits(firstText)
		cut := 0
		for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
			cut++
		}
		if cut > 0 {
			r.replaceCharacterData(firstText, 0, cut, "")
		}
	} else if preParagraphJoin && strings.EqualFold(end.Data, "pre") && firstText != nil {
		// The leading spaces are visible in the preformatted block, but after
		// joining into a paragraph a run of ordinary spaces would collapse.
		// Alternate non-breaking spaces into the run so it keeps its rendered
		// width in the resulting normal-flow paragraph.
		units := characterDataCodeUnits(firstText)
		spaceCount := 0
		for spaceCount < len(units) && units[spaceCount] == ' ' {
			spaceCount++
		}
		if spaceCount > 1 {
			preserved := make([]uint16, spaceCount)
			for i := range preserved {
				if i%2 == 1 {
					preserved[i] = '\u00a0'
				} else {
					preserved[i] = ' '
				}
			}
			r.replaceCharacterDataUnits(firstText, 0, spaceCount, preserved)
		}
	}
	if !preserveTrailingBreak && start.LastChild != nil && start.LastChild.Element() && strings.EqualFold(start.LastChild.Data, "br") {
		r.rangeRemoveNode(start.LastChild)
	}
	var splitBreak *dom.Node
	if crossListJoin {
		for child := end.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && strings.EqualFold(child.Data, "br") && child.NextSibling != nil && child != end.FirstChild {
				splitBreak = child
				break
			}
		}
	}
	for end.FirstChild != nil && end.FirstChild != splitBreak {
		child := end.FirstChild
		wasConnected := child.IsConnected()
		end.RemoveChild(child)
		start.AppendChild(child)
		r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
		r.notifyMutation()
		if wasConnected {
			r.invokeDisconnectedCallbacks(child)
			r.activateDOMInsertedSubtree(child)
		}
	}
	if splitBreak != nil {
		r.rangeRemoveNode(splitBreak)
	}
	for _, whitespace := range between {
		r.rangeRemoveNode(whitespace)
	}
	if splitBreak == nil {
		r.rangeRemoveNode(end)
	}
	if splitBreak == nil && endParent != startParent && crossListJoin {
		prunedRightRoot := false
		for current := endParent; current != nil && current != host && current != startParent; {
			if current.FirstChild != nil || (!isListContainer(current) && !(current.Element() && strings.EqualFold(current.Data, "li"))) {
				break
			}
			parent := current.Parent
			if isListContainer(current) && parent == startParent.Parent {
				prunedRightRoot = true
			}
			r.rangeRemoveNode(current)
			current = parent
		}
		if prunedRightRoot {
			for _, whitespace := range containerBetween {
				r.rangeRemoveNode(whitespace)
			}
		}
	} else if splitBreak == nil && endParent != startParent && endParent.FirstChild == nil {
		r.rangeRemoveNode(endParent)
		for _, whitespace := range containerBetween {
			r.rangeRemoveNode(whitespace)
		}
	}
	if startParent != start.Parent || start.Parent == nil {
		return false
	}
	return true
}

// mergeInlineBeforeBlock joins preceding inline content with a block's first
// line when a deletion crosses into that block. A line break in the block
// keeps the remaining lines grouped in the original block.
func mergeInlineBeforeBlock(r *Runtime, start rangeBoundary, block, host *dom.Node) bool {
	if r == nil || start.container == nil || block == nil || host == nil ||
		!block.Element() || !block.IsConnected() || block.Parent == nil || editingHostFor(start.container) != host || editingHostFor(block) != host {
		return false
	}
	var startRoot *dom.Node
	if start.container.Element() && start.offset > 0 {
		if previous := childAt(start.container, start.offset-1); previous != nil {
			if text := lastEditableTextInSubtree(previous, host); text != nil {
				start = rangeBoundary{container: text, offset: len(characterDataCodeUnits(text))}
			} else if !isEditingBlockElement(previous) && editingHostFor(previous) == host {
				startRoot = previous
			}
		}
	}
	if startRoot == nil && start.container.Type != dom.NodeText {
		return false
	}
	if startRoot == nil {
		startRoot = start.container
		for startRoot != nil && startRoot.Parent != block.Parent {
			startRoot = startRoot.Parent
		}
	}
	if startRoot == nil || startRoot.Parent != block.Parent || startRoot == block || isEditingBlockElement(startRoot) {
		return false
	}
	var separators []*dom.Node
	lineBreaks := 0
	next := startRoot.NextSibling
	for next != nil && next != block {
		if next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			separators = append(separators, next)
			next = next.NextSibling
			continue
		}
		if next.Element() && strings.EqualFold(next.Data, "br") && lineBreaks == 0 {
			lineBreaks++
			separators = append(separators, next)
			next = next.NextSibling
			continue
		}
		return false
	}
	if next != block {
		return false
	}
	if start.container.Type == dom.NodeText {
		if lastEditableTextInSubtree(startRoot, host) != start.container || start.offset != len(characterDataCodeUnits(start.container)) {
			return false
		}
	} else if start.container.Element() && startRoot.Parent != start.container && startRoot != start.container {
		return false
	}
	parent := block.Parent
	reference := block
	preservesWhitespace := func(node *dom.Node) bool {
		if node == nil {
			return false
		}
		whiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(node)["white-space"]))
		return whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" || strings.EqualFold(node.Data, "pre")
	}
	if !preservesWhitespace(startRoot) {
		if text := lastEditableTextInSubtree(startRoot, host); text != nil {
			units := characterDataCodeUnits(text)
			cut := len(units)
			for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
				cut--
			}
			if cut < len(units) {
				r.replaceCharacterData(text, cut, len(units)-cut, "")
			}
		}
	}
	if !preservesWhitespace(block) {
		if text := firstEditableTextInSubtree(block, host); text != nil {
			units := characterDataCodeUnits(text)
			cut := 0
			for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
				cut++
			}
			if cut > 0 {
				r.replaceCharacterData(text, 0, cut, "")
			}
		}
	}
	for _, separator := range separators {
		if separator.Parent != nil {
			r.rangeRemoveNode(separator)
		}
	}
	for child := block.FirstChild; child != nil; {
		if child.Element() && strings.EqualFold(child.Data, "br") {
			r.rangeRemoveNode(child)
			break
		}
		nextChild := child.NextSibling
		wasConnected := child.IsConnected()
		block.RemoveChild(child)
		parent.InsertNode(child, reference)
		r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
		r.notifyMutation()
		if wasConnected {
			r.invokeDisconnectedCallbacks(child)
			r.activateDOMInsertedSubtree(child)
		}
		child = nextChild
	}
	if block.FirstChild == nil {
		r.rangeRemoveNode(block)
	}
	return true
}

func mergeInlineBeforeListItem(r *Runtime, start rangeBoundary, item *dom.Node, end rangeBoundary, host *dom.Node) bool {
	if r == nil || item == nil || !item.Element() || !strings.EqualFold(item.Data, "li") || item.Parent == nil ||
		!item.IsConnected() || editingHostFor(item) != host {
		return false
	}
	list := item.Parent
	if !isListContainer(list) || list.Parent == nil || firstElementChildNode(list) != item {
		return false
	}
	firstText := firstEditableTextInSubtree(item, host)
	if firstText == nil || end.container != firstText || end.offset != 0 {
		return false
	}
	parent := list.Parent
	var startRoot *dom.Node
	if start.container != nil && start.container.Type == dom.NodeText {
		startRoot = start.container
		for startRoot != nil && startRoot.Parent != parent {
			startRoot = startRoot.Parent
		}
		if startRoot == nil || startRoot.Parent != parent || lastEditableTextInSubtree(startRoot, host) != start.container ||
			start.offset != len(characterDataCodeUnits(start.container)) || isEditingBlockElement(startRoot) {
			return false
		}
	} else {
		return false
	}
	var separators []*dom.Node
	for next := startRoot.NextSibling; next != nil && next != list; next = next.NextSibling {
		if next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			separators = append(separators, next)
			continue
		}
		if next.Element() && strings.EqualFold(next.Data, "br") && len(separators) == 0 {
			separators = append(separators, next)
			continue
		}
		return false
	}
	foundList := false
	for next := startRoot.NextSibling; next != nil; next = next.NextSibling {
		if next == list {
			foundList = true
			break
		}
	}
	if !foundList {
		return false
	}
	lineChildren := make([]*dom.Node, 0, nodeChildCount(item))
	lineLastText := lastEditableTextInSubtree(item, host)
	for child := item.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && strings.EqualFold(child.Data, "br") {
			break
		}
		if isEditingBlockElement(child) || child.Type == dom.NodeComment || child.Type == dom.NodeDoctype {
			return false
		}
		lineChildren = append(lineChildren, child)
	}
	preservesWhitespace := func(node *dom.Node) bool {
		whiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(node)["white-space"]))
		return whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" || strings.EqualFold(node.Data, "pre")
	}
	if !preservesWhitespace(startRoot) {
		if text := lastEditableTextInSubtree(startRoot, host); text != nil {
			units := characterDataCodeUnits(text)
			cut := len(units)
			for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
				cut--
			}
			if cut < len(units) {
				r.replaceCharacterData(text, cut, len(units)-cut, "")
			}
		}
	}
	if !preservesWhitespace(item) {
		units := characterDataCodeUnits(firstText)
		cut := 0
		for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
			cut++
		}
		if lineLastText != nil {
			units := characterDataCodeUnits(lineLastText)
			cut := len(units)
			for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
				cut--
			}
			if cut < len(units) {
				r.replaceCharacterData(lineLastText, cut, len(units)-cut, "")
			}
		}
		if cut > 0 {
			r.replaceCharacterData(firstText, 0, cut, "")
		}
	}
	for _, separator := range separators {
		r.rangeRemoveNode(separator)
	}
	for _, child := range lineChildren {
		if child.Parent != item {
			continue
		}
		wasConnected := child.IsConnected()
		item.RemoveChild(child)
		parent.InsertNode(child, list)
		r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
		r.notifyMutation()
		if wasConnected {
			r.invokeDisconnectedCallbacks(child)
			r.activateDOMInsertedSubtree(child)
		}
	}
	if item.FirstChild != nil && item.FirstChild.Element() && strings.EqualFold(item.FirstChild.Data, "br") {
		r.rangeRemoveNode(item.FirstChild)
	}
	if item.FirstChild == nil {
		r.rangeRemoveNode(item)
	}
	if firstElementChildNode(list) == nil {
		next := list.NextSibling
		r.rangeRemoveNode(list)
		for next != nil && next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			following := next.NextSibling
			r.rangeRemoveNode(next)
			next = following
		}
		if next != nil && next.Type == dom.NodeText && !preservesWhitespace(next) {
			units := characterDataCodeUnits(next)
			cut := 0
			for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
				cut++
			}
			if cut > 0 {
				r.replaceCharacterData(next, 0, cut, "")
			}
		}
	}
	return true
}

func firstElementChildNode(parent *dom.Node) *dom.Node {
	if parent == nil {
		return nil
	}
	for child := parent.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() {
			return child
		}
	}
	return nil
}

// mergeInlineAfterBlock joins following inline content into the final line of
// a block when a deletion crosses from the block's end into that content.
func mergeInlineAfterBlock(r *Runtime, block *dom.Node, start, end rangeBoundary, host *dom.Node) bool {
	if r == nil || block == nil || block.Parent == nil || host == nil || !block.Element() || !block.IsConnected() ||
		editingHostFor(block) != host || !boundaryAtEditingBlockEnd(host, block, start) {
		return false
	}
	if end.container == nil || editingHostFor(end.container) != host {
		return false
	}
	parent := block.Parent
	endRoot := end.container
	for endRoot != nil && endRoot.Parent != parent {
		endRoot = endRoot.Parent
	}
	if endRoot == nil || endRoot.Parent != parent || isEditingBlockElement(endRoot) {
		return false
	}
	firstText := firstEditableTextInSubtree(endRoot, host)
	if end.container.Type == dom.NodeText {
		if end.container != firstText || end.offset != 0 {
			return false
		}
	} else if end.container != endRoot || end.offset != 0 {
		return false
	}
	var separators []*dom.Node
	for next := block.NextSibling; next != nil && next != endRoot; next = next.NextSibling {
		if next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			separators = append(separators, next)
			continue
		}
		if next.Element() && strings.EqualFold(next.Data, "br") && len(separators) == 0 {
			separators = append(separators, next)
			continue
		}
		return false
	}
	found := false
	for next := block.NextSibling; next != nil; next = next.NextSibling {
		if next == endRoot {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	preservesWhitespace := func(node *dom.Node) bool {
		if node == nil {
			return false
		}
		whiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(node)["white-space"]))
		return whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" || strings.EqualFold(node.Data, "pre")
	}
	if !preservesWhitespace(block) {
		if text := lastEditableTextInSubtree(block, host); text != nil {
			units := characterDataCodeUnits(text)
			cut := len(units)
			for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
				cut--
			}
			if cut < len(units) {
				r.replaceCharacterData(text, cut, len(units)-cut, "")
			}
		}
	}
	if !preservesWhitespace(endRoot) && firstText != nil {
		units := characterDataCodeUnits(firstText)
		cut := 0
		for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
			cut++
		}
		if cut > 0 {
			r.replaceCharacterData(firstText, 0, cut, "")
		}
	}
	for _, separator := range separators {
		r.rangeRemoveNode(separator)
	}
	if endRoot.Type == dom.NodeText {
		parent.RemoveChild(endRoot)
		block.AppendChild(endRoot)
		r.notifyMutation()
	} else {
		for endRoot.FirstChild != nil {
			child := endRoot.FirstChild
			wasConnected := child.IsConnected()
			endRoot.RemoveChild(child)
			block.AppendChild(child)
			r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
			r.notifyMutation()
			if wasConnected {
				r.invokeDisconnectedCallbacks(child)
				r.activateDOMInsertedSubtree(child)
			}
		}
		r.rangeRemoveNode(endRoot)
	}
	return true
}

func mergeInlineAfterListItem(r *Runtime, item *dom.Node, start, end rangeBoundary, host *dom.Node) bool {
	if r == nil || item == nil || item.Parent == nil || !item.Element() || !strings.EqualFold(item.Data, "li") ||
		!item.IsConnected() || editingHostFor(item) != host || !boundaryAtEditingBlockEnd(host, item, start) {
		return false
	}
	list := item.Parent
	if !isListContainer(list) || list.Parent == nil || lastElementChildNode(list) != item || end.container == nil || editingHostFor(end.container) != host {
		return false
	}
	parent := list.Parent
	endRoot := end.container
	for endRoot != nil && endRoot.Parent != parent {
		endRoot = endRoot.Parent
	}
	if endRoot == nil || endRoot.Parent != parent || isEditingBlockElement(endRoot) {
		return false
	}
	firstText := firstEditableTextInSubtree(endRoot, host)
	if end.container.Type == dom.NodeText {
		if end.container != firstText || end.offset != 0 {
			return false
		}
	} else if end.container != endRoot || end.offset != 0 {
		return false
	}
	var separators []*dom.Node
	for next := list.NextSibling; next != nil && next != endRoot; next = next.NextSibling {
		if next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			separators = append(separators, next)
			continue
		}
		return false
	}
	found := false
	for next := list.NextSibling; next != nil; next = next.NextSibling {
		if next == endRoot {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	preservesWhitespace := func(node *dom.Node) bool {
		if node == nil {
			return false
		}
		whiteSpace := strings.ToLower(strings.TrimSpace(r.computedValues(node)["white-space"]))
		return whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces" || strings.EqualFold(node.Data, "pre")
	}
	if !preservesWhitespace(item) {
		if text := lastEditableTextInSubtree(item, host); text != nil {
			units := characterDataCodeUnits(text)
			cut := len(units)
			for cut > 0 && isHTMLSpaceCodeUnit(units[cut-1]) {
				cut--
			}
			if cut < len(units) {
				r.replaceCharacterData(text, cut, len(units)-cut, "")
			}
		}
	}
	if !preservesWhitespace(endRoot) && firstText != nil {
		units := characterDataCodeUnits(firstText)
		cut := 0
		for cut < len(units) && isHTMLSpaceCodeUnit(units[cut]) {
			cut++
		}
		if cut > 0 {
			r.replaceCharacterData(firstText, 0, cut, "")
		}
	}
	for _, separator := range separators {
		r.rangeRemoveNode(separator)
	}
	if endRoot.Type == dom.NodeText {
		parent.RemoveChild(endRoot)
		item.AppendChild(endRoot)
		r.notifyMutation()
	} else {
		for endRoot.FirstChild != nil {
			child := endRoot.FirstChild
			wasConnected := child.IsConnected()
			endRoot.RemoveChild(child)
			item.AppendChild(child)
			r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
			r.notifyMutation()
			if wasConnected {
				r.invokeDisconnectedCallbacks(child)
				r.activateDOMInsertedSubtree(child)
			}
		}
		r.rangeRemoveNode(endRoot)
	}
	return true
}

func lastElementChildNode(parent *dom.Node) *dom.Node {
	if parent == nil {
		return nil
	}
	for child := parent.LastChild; child != nil; child = child.PrevSibling {
		if child.Element() {
			return child
		}
	}
	return nil
}

func mergeNestedListIntoPreviousItem(r *Runtime, start, end, host *dom.Node) bool {
	if r == nil || start == nil || end == nil || host == nil || start == end ||
		!start.Element() || !end.Element() || !strings.EqualFold(start.Data, "li") || !strings.EqualFold(end.Data, "li") ||
		start.Parent == nil || !isListContainer(start.Parent) || !start.IsConnected() || !end.IsConnected() {
		return false
	}
	for owner := end.Parent; owner != nil && owner != host; owner = owner.Parent {
		if !owner.Element() || !strings.EqualFold(owner.Data, "li") || owner.Parent != start.Parent {
			continue
		}
		next := start.NextSibling
		for next != nil && next.Type == dom.NodeText && strings.TrimSpace(next.DataContent) == "" {
			next = next.NextSibling
		}
		if next != owner {
			return false
		}
		var nestedList *dom.Node
		for child := owner.FirstChild; child != nil; child = child.NextSibling {
			if isListContainer(child) && isAncestorNode(child, end) {
				nestedList = child
				continue
			}
			if child.Type == dom.NodeText && strings.TrimSpace(child.DataContent) == "" {
				continue
			}
			return false
		}
		if nestedList == nil {
			return false
		}
		wasConnected := nestedList.IsConnected()
		owner.RemoveChild(nestedList)
		start.AppendChild(nestedList)
		r.markRadioGroupsDirtyForNodes([]*dom.Node{nestedList})
		r.notifyMutation()
		if wasConnected {
			r.invokeDisconnectedCallbacks(nestedList)
			r.activateDOMInsertedSubtree(nestedList)
		}
		r.rangeRemoveNode(owner)
		ensureNestedListPlaceholder(r, nestedList)
		return true
	}
	return false
}

func removeEmptyLeadingListRoot(r *Runtime, start, end, host *dom.Node) bool {
	if r == nil || start == nil || end == nil || host == nil || start == end ||
		!start.Element() || !end.Element() || !strings.EqualFold(start.Data, "li") || !strings.EqualFold(end.Data, "li") ||
		start.TextContent() != "" || end.TextContent() == "" {
		return false
	}
	rootFor := func(node *dom.Node) *dom.Node {
		var root *dom.Node
		for current := node.Parent; current != nil && current != host; current = current.Parent {
			if isListContainer(current) {
				root = current
			}
		}
		return root
	}
	leftRoot, rightRoot := rootFor(start), rootFor(end)
	if leftRoot == nil || rightRoot == nil || leftRoot == rightRoot || leftRoot.Parent == nil || leftRoot.Parent != rightRoot.Parent || end.Parent == rightRoot {
		return false
	}
	for current := start; current != leftRoot; current = current.Parent {
		if current == nil || current.Parent == nil {
			return false
		}
		for sibling := current.Parent.FirstChild; sibling != nil; sibling = sibling.NextSibling {
			if sibling == current {
				continue
			}
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return false
			}
		}
	}
	for sibling := leftRoot.NextSibling; sibling != nil && sibling != rightRoot; sibling = sibling.NextSibling {
		if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
			return false
		}
	}
	rootParent := leftRoot.Parent
	r.rangeRemoveNode(leftRoot)
	for current := rootParent; current != nil && current != host; {
		if current.FirstChild != nil || (!isListContainer(current) && !(current.Element() && strings.EqualFold(current.Data, "li"))) {
			break
		}
		parent := current.Parent
		r.rangeRemoveNode(current)
		current = parent
	}
	return true
}

func ensureNestedListPlaceholder(r *Runtime, list *dom.Node) {
	if r == nil || !isListContainer(list) || list.TextContent() != "" {
		return
	}
	var item *dom.Node
	for child := list.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && strings.EqualFold(child.Data, "li") {
			item = child
			break
		}
	}
	if item == nil {
		item = list.Doc.NewElement("li")
		list.AppendChild(item)
		r.notifyMutation()
	}
	hasContent, hasBreak := false, false
	for child := item.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == dom.NodeText && strings.TrimSpace(child.DataContent) == "" {
			continue
		}
		if child.Element() && strings.EqualFold(child.Data, "br") {
			hasBreak = true
			continue
		}
		hasContent = true
	}
	if !hasContent && !hasBreak {
		item.AppendChild(list.Doc.NewElement("br"))
		r.notifyMutation()
	}
}

func adjacentNestedListRootsForJoin(start, end, host *dom.Node) ([]*dom.Node, bool) {
	if start == nil || end == nil || host == nil || !strings.EqualFold(start.Data, "li") || !strings.EqualFold(end.Data, "li") ||
		!isListContainer(start.Parent) || !isListContainer(end.Parent) {
		return nil, false
	}
	rootFor := func(node *dom.Node) *dom.Node {
		var root *dom.Node
		for current := node.Parent; current != nil && current != host; current = current.Parent {
			if isListContainer(current) {
				root = current
			}
		}
		return root
	}
	leftRoot, rightRoot := rootFor(start), rootFor(end)
	if leftRoot == nil || rightRoot == nil {
		return nil, false
	}
	// Within one list root, the join anchor may be either the nested list that
	// contains start or its containing outer list item. In both cases, require
	// that anchor to be the last meaningful content before end.
	if leftRoot == rightRoot && end.Parent == rightRoot {
		for sibling := start.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return nil, false
			}
		}
		for anchor := start.Parent; anchor != nil && anchor != host; anchor = anchor.Parent {
			isContainingItem := anchor.Element() && strings.EqualFold(anchor.Data, "li") && anchor.Parent == end.Parent
			isNestedList := isListContainer(anchor) && anchor != leftRoot && anchor.Parent == end.Parent
			if !isContainingItem && !isNestedList {
				continue
			}
			foundEnd := false
			for sibling := anchor.NextSibling; sibling != nil; sibling = sibling.NextSibling {
				if sibling == end {
					foundEnd = true
					break
				}
				if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
					return nil, false
				}
			}
			if !foundEnd {
				return nil, false
			}
			for current := start; current != anchor; current = current.Parent {
				if current == nil || current.Parent == nil {
					return nil, false
				}
				for sibling := current.NextSibling; sibling != nil; sibling = sibling.NextSibling {
					if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
						return nil, false
					}
				}
			}
			return nil, true
		}
		return nil, false
	}
	if leftRoot == rightRoot || leftRoot.Parent == nil || leftRoot.Parent != rightRoot.Parent {
		return nil, false
	}
	for sibling := start.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
			return nil, false
		}
	}
	for current := end; current != rightRoot; current = current.Parent {
		if current == nil || current.Parent == nil {
			return nil, false
		}
		for sibling := current.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return nil, false
			}
		}
	}
	for current := start; current != leftRoot; current = current.Parent {
		if current == nil || current.Parent == nil {
			return nil, false
		}
		for sibling := current.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
				return nil, false
			}
		}
	}
	var between []*dom.Node
	for sibling := leftRoot.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling == rightRoot {
			return between, true
		}
		if sibling.Type != dom.NodeText || strings.TrimSpace(sibling.DataContent) != "" {
			return nil, false
		}
		between = append(between, sibling)
	}
	return nil, false
}

func isListContainer(node *dom.Node) bool {
	return node != nil && node.Element() && (strings.EqualFold(node.Data, "ul") || strings.EqualFold(node.Data, "ol"))
}

func isAtomicEditingElement(node *dom.Node) bool {
	if node == nil || !node.Element() {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "br", "hr", "img", "input", "video", "audio", "iframe", "object", "embed":
		return true
	default:
		return false
	}
}

func isHTMLSpaceCodeUnit(unit uint16) bool {
	switch unit {
	case 0x0009, 0x000A, 0x000C, 0x000D, 0x0020:
		return true
	default:
		return false
	}
}

func isLeadingHTMLSpaceRun(units []uint16, offset int) bool {
	if offset < 0 || offset > len(units) {
		return false
	}
	for i := 0; i < offset; i++ {
		if !isHTMLSpaceCodeUnit(units[i]) {
			return false
		}
	}
	return offset > 0
}

func leadingHTMLSpaceEnd(units []uint16) int {
	end := 0
	for end < len(units) && isHTMLSpaceCodeUnit(units[end]) {
		end++
	}
	return end
}

func previousCharacterStart(text *dom.Node) (int, bool) {
	if text == nil || text.Type != dom.NodeText {
		return 0, false
	}
	units := characterDataCodeUnits(text)
	if len(units) == 0 {
		return 0, false
	}
	start := len(units) - 1
	if units[start] >= 0xDC00 && units[start] <= 0xDFFF && start > 0 && units[start-1] >= 0xD800 && units[start-1] <= 0xDBFF {
		start--
	}
	return start, true
}

func lastEditableTextInSubtree(node, host *dom.Node) *dom.Node {
	if node == nil || editingHostFor(node) != host {
		return nil
	}
	for child := node.LastChild; child != nil; child = child.PrevSibling {
		if text := lastEditableTextInSubtree(child, host); text != nil {
			return text
		}
	}
	if node.Type == dom.NodeText {
		return node
	}
	return nil
}

func (r *Runtime) execCommand(doc *dom.Document, args []goja.Value) goja.Value {
	if doc == nil || len(args) == 0 {
		return r.vm.ToValue(false)
	}
	command := strings.ToLower(strings.TrimSpace(args[0].String()))
	sel := r.selectionForDocument(doc)
	if r.opts.ActiveElement != nil {
		if active := r.opts.ActiveElement(); active != nil && dom.IsContentEditable(active) {
			r.EnsureContentEditableSelection(active)
		}
	}
	switch command {
	case "delete":
		inputType := r.userInputType
		if !strings.HasPrefix(inputType, "delete") {
			inputType = "deleteContentBackward"
		}
		return r.execCommandDeleteSelectionWithInputType(sel, inputType)
	case "copy":
		return r.execCommandClipboard(sel, "copy")
	case "cut":
		return r.execCommandClipboard(sel, "cut")
	case "paste":
		return r.execCommandPaste(sel)
	case "inserttext":
		if len(args) < 3 {
			return r.vm.ToValue(false)
		}
		return r.execCommandInsertText(sel, args[2].String())
	case "inserthtml":
		if len(args) < 3 {
			return r.vm.ToValue(false)
		}
		return r.execCommandInsertHTML(sel, args[2].String())
	case "bold":
		return r.execCommandFormat(sel, "b")
	case "italic":
		return r.execCommandFormat(sel, "i")
	case "underline":
		return r.execCommandFormat(sel, "u")
	case "strikethrough":
		return r.execCommandFormat(sel, "strike")
	case "superscript":
		return r.execCommandFormat(sel, "sup")
	case "subscript":
		return r.execCommandFormat(sel, "sub")
	case "insertorderedlist":
		return r.execCommandList(sel, "ol")
	case "insertunorderedlist":
		return r.execCommandList(sel, "ul")
	case "insertlinebreak":
		return r.execCommandInsertMarkup(sel, "<br>", "insertLineBreak")
	case "insertparagraph":
		return r.execCommandInsertMarkup(sel, "<div><br></div>", "insertParagraph")
	case "inserthorizontalrule":
		return r.execCommandInsertMarkup(sel, "<hr>", "insertHorizontalRule")
	case "backcolor", "hilitecolor":
		if len(args) < 3 {
			return r.vm.ToValue(false)
		}
		value := args[2].String()
		data := normalizeExecCommandColor(value)
		return r.execCommandStyle(sel, "background-color", value, "formatBackColor", data)
	case "forecolor":
		if len(args) < 3 {
			return r.vm.ToValue(false)
		}
		value := args[2].String()
		data := normalizeExecCommandColor(value)
		return r.execCommandStyle(sel, "color", value, "formatFontColor", data)
	case "fontname":
		if len(args) < 3 {
			return r.vm.ToValue(false)
		}
		value := args[2].String()
		return r.execCommandAttribute(sel, "font", "face", value, "formatFontName", value)
	case "createlink":
		if len(args) < 3 {
			return r.vm.ToValue(false)
		}
		value := args[2].String()
		return r.execCommandAttribute(sel, "a", "href", value, "insertLink", value)
	case "unlink":
		return r.execCommandUnlink(sel)
	case "justifycenter", "justifyfull", "justifyright", "justifyleft":
		align := map[string]string{"justifycenter": "center", "justifyfull": "justify", "justifyright": "right", "justifyleft": "left"}[command]
		inputType := map[string]string{"justifycenter": "formatJustifyCenter", "justifyfull": "formatJustifyFull", "justifyright": "formatJustifyRight", "justifyleft": "formatJustifyLeft"}[command]
		return r.execCommandJustify(sel, align, inputType)
	case "removeformat":
		return r.execCommandBlockStyle(sel, "", "formatRemove")
	case "indent":
		return r.execCommandBlockStyle(sel, "margin-left: 40px;", "formatIndent")
	case "outdent":
		return r.execCommandBlockStyle(sel, "", "formatOutdent")
	default:
		return r.vm.ToValue(false)
	}
}

func supportedEditCommand(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "delete", "copy", "cut", "paste", "inserttext", "inserthtml", "bold", "italic", "underline", "strikethrough", "superscript", "subscript", "insertorderedlist", "insertunorderedlist", "insertlinebreak", "insertparagraph", "inserthorizontalrule", "backcolor", "hilitecolor", "forecolor", "fontname", "createlink", "unlink", "justifycenter", "justifyfull", "justifyright", "justifyleft", "removeformat", "indent", "outdent":
		return true
	default:
		return false
	}
}

func normalizeExecCommandColor(value string) string {
	parsed, ok := css.ParseColor(value)
	if !ok {
		lower := strings.ToLower(strings.TrimSpace(value))
		switch lower {
		case "inherit", "initial", "unset", "reset", "currentcolor":
			return lower
		default:
			return value
		}
	}
	if parsed.A == 255 {
		return fmt.Sprintf("rgb(%d, %d, %d)", parsed.R, parsed.G, parsed.B)
	}
	alpha := strconv.FormatFloat(float64(parsed.A)/255, 'f', 3, 64)
	alpha = strings.TrimRight(strings.TrimRight(alpha, "0"), ".")
	if alpha == "" {
		alpha = "0"
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", parsed.R, parsed.G, parsed.B, alpha)
}

func (r *Runtime) execCommandStyle(sel *jsSelection, property, value, inputType, eventData string) goja.Value {
	return r.execCommandAttribute(sel, "span", "style", property+": "+value+";", inputType, eventData)
}

func (r *Runtime) execCommandAttribute(sel *jsSelection, tag, attribute, value, inputType, eventData string) goja.Value {
	selection, ok := selectedTextRange(sel)
	if !ok || selection.text.Parent == nil || selection.text.Doc == nil || selection.start == selection.end {
		return r.vm.ToValue(false)
	}
	units := characterDataCodeUnits(selection.text)
	prefix := string(utf16.Decode(units[:selection.start]))
	suffix := string(utf16.Decode(units[selection.end:]))
	parent := selection.text.Parent
	index := childIndex(parent, selection.text)
	parent.RemoveChild(selection.text)
	if prefix != "" {
		parent.InsertNode(selection.text.Doc.NewText(prefix), childAt(parent, index))
		index++
	}
	setCharacterDataCodeUnits(selection.text, units[selection.start:selection.end])
	wrapper := selection.text.Doc.NewElement(tag)
	wrapper.SetAttribute(attribute, value)
	wrapper.AppendChild(selection.text)
	parent.InsertNode(wrapper, childAt(parent, index))
	index++
	if suffix != "" {
		parent.InsertNode(selection.text.Doc.NewText(suffix), childAt(parent, index))
	}
	selection.rangeObj.start = rangeBoundary{container: selection.text, offset: 0}
	selection.rangeObj.end = rangeBoundary{container: selection.text, offset: selection.end - selection.start}
	r.notifyMutation()
	r.dispatchInputEvent(selection.host, &eventData, inputType)
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandUnlink(sel *jsSelection) goja.Value {
	selection, ok := selectedTextRange(sel)
	if !ok {
		return r.vm.ToValue(false)
	}
	var anchor *dom.Node
	for current := selection.text.Parent; current != nil && current != selection.host; current = current.Parent {
		if current.Element() && current.Data == "a" {
			anchor = current
			break
		}
	}
	if anchor == nil || anchor.Parent == nil {
		return r.vm.ToValue(false)
	}
	parent := anchor.Parent
	for anchor.FirstChild != nil {
		parent.InsertNode(anchor.FirstChild, anchor)
	}
	parent.RemoveChild(anchor)
	r.notifyMutation()
	r.dispatchInputEvent(selection.host, nil, "")
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandClipboard(sel *jsSelection, typ string) goja.Value {
	if sel == nil || len(sel.ranges) == 0 || sel.ranges[0] == nil {
		return r.vm.ToValue(false)
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	host := editingHostFor(ran.start.container)
	if host == nil || editingHostFor(ran.end.container) != host {
		return r.vm.ToValue(false)
	}
	plain := rangeToString(ran)
	if plain == "" {
		return r.vm.ToValue(false)
	}
	transfer := dom.NewDataTransfer()
	transfer.SetData("text/plain", plain)
	transfer.SetMode(dom.DataTransferReadWrite)
	accepted := r.DispatchTrustedClipboardEvent(host, typ, transfer)
	plain = transfer.GetData("text/plain")
	transfer.Disassociate()
	if !accepted {
		return r.vm.ToValue(false)
	}
	r.mu.Lock()
	clipboard := r.clipboardProvider
	r.mu.Unlock()
	if clipboard != nil {
		clipboard.Write(plain)
	}
	if typ == "cut" {
		return r.execCommandDeleteSelectionWithInputType(sel, "deleteByCut")
	}
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandPaste(sel *jsSelection) goja.Value {
	if sel == nil || len(sel.ranges) == 0 || sel.ranges[0] == nil {
		return r.vm.ToValue(false)
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	host := editingHostFor(ran.start.container)
	if host == nil || editingHostFor(ran.end.container) != host {
		return r.vm.ToValue(false)
	}
	r.mu.Lock()
	clipboard := r.clipboardProvider
	r.mu.Unlock()
	if clipboard == nil {
		return r.vm.ToValue(false)
	}
	plain := clipboard.Read()
	transfer := dom.NewDataTransfer()
	if plain != "" {
		transfer.SetData("text/plain", plain)
	}
	transfer.SetMode(dom.DataTransferReadOnly)
	if !r.DispatchTrustedClipboardEvent(host, "paste", transfer) {
		transfer.Disassociate()
		return r.vm.ToValue(false)
	}
	return r.execCommandInsertTextFromPaste(sel, plain, transfer)
}

func (r *Runtime) execCommandJustify(sel *jsSelection, align, inputType string) goja.Value {
	block, host, ok := r.execCommandBlock(sel)
	if !ok {
		return r.vm.ToValue(false)
	}
	if block == nil {
		block = host.Doc.NewElement("div")
		for host.FirstChild != nil {
			block.AppendChild(host.FirstChild)
		}
		host.AppendChild(block)
		rangeInBlock(sel.ranges[0], block, host)
	}
	block.SetAttribute("style", "text-align: "+align+";")
	r.notifyMutation()
	r.dispatchInputEvent(host, nil, inputType)
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandBlockStyle(sel *jsSelection, style, inputType string) goja.Value {
	block, host, ok := r.execCommandBlock(sel)
	if !ok {
		return r.vm.ToValue(false)
	}
	if block == nil {
		block = host.Doc.NewElement("div")
		for host.FirstChild != nil {
			block.AppendChild(host.FirstChild)
		}
		host.AppendChild(block)
		rangeInBlock(sel.ranges[0], block, host)
	}
	block.SetAttribute("style", style)
	r.notifyMutation()
	r.dispatchInputEvent(host, nil, inputType)
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandBlock(sel *jsSelection) (*dom.Node, *dom.Node, bool) {
	if sel == nil || len(sel.ranges) == 0 || sel.ranges[0] == nil {
		return nil, nil, false
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return nil, nil, false
	}
	host := editingHostFor(ran.start.container)
	if host == nil || editingHostFor(ran.end.container) != host {
		return nil, nil, false
	}
	block := nearestEditingBlock(host, ran.start.container)
	if block == nil {
		if ran.start.container == host {
			block = childAt(host, ran.start.offset)
		}
		if !isEditingBlockElement(block) {
			block = nil
		}
	}
	return block, host, true
}

func rangeInBlock(ran *jsRange, block, host *dom.Node) {
	if ran == nil || block == nil || host == nil {
		return
	}
	text := firstEditableTextInSubtree(block, host)
	if text == nil {
		return
	}
	start := rangeBoundary{container: text, offset: 0}
	end := rangeBoundary{container: text, offset: len(characterDataCodeUnits(text))}
	ran.start, ran.end = start, end
}

func (r *Runtime) execCommandList(sel *jsSelection, tag string) goja.Value {
	if sel == nil || len(sel.ranges) == 0 || sel.ranges[0] == nil {
		return r.vm.ToValue(false)
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	host := editingHostFor(ran.start.container)
	if host == nil || editingHostFor(ran.end.container) != host {
		return r.vm.ToValue(false)
	}
	var list *dom.Node
	for child := host.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && (child.Data == "ol" || child.Data == "ul") {
			list = child
			break
		}
	}
	if list != nil {
		if list.Data == tag {
			return r.vm.ToValue(false)
		}
		replacement := host.Doc.NewElement(tag)
		for child := list.FirstChild; child != nil; {
			next := child.NextSibling
			list.RemoveChild(child)
			replacement.AppendChild(child)
			child = next
		}
		if parent := list.Parent; parent != nil {
			parent.InsertNode(replacement, list)
			parent.RemoveChild(list)
		}
	} else {
		list = host.Doc.NewElement(tag)
		item := host.Doc.NewElement("li")
		for child := host.FirstChild; child != nil; {
			next := child.NextSibling
			host.RemoveChild(child)
			item.AppendChild(child)
			child = next
		}
		list.AppendChild(item)
		host.AppendChild(list)
	}
	r.notifyMutation()
	inputType := "insertUnorderedList"
	if tag == "ol" {
		inputType = "insertOrderedList"
	}
	r.dispatchInputEvent(host, nil, inputType)
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandInsertHTML(sel *jsSelection, markup string) goja.Value {
	return r.execCommandInsertMarkup(sel, markup, "insertFromPaste")
}

func (r *Runtime) execCommandInsertMarkup(sel *jsSelection, markup, inputType string) goja.Value {
	if sel == nil || len(sel.ranges) == 0 || len(markup) > 1<<20 {
		return r.vm.ToValue(false)
	}
	ran := sel.ranges[0]
	if ran == nil || ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	host := editingHostFor(start.container)
	if host == nil || editingHostFor(end.container) != host || !isAncestorNode(host, start.container) || !isAncestorNode(host, end.container) {
		return r.vm.ToValue(false)
	}
	parsed, err := dom.ParseFragmentBounded(host, markup, dom.ParseLimits{
		Nodes: 1024, Depth: 64, Attributes: 256, AttributeBytes: 4096,
	})
	if err != nil || parsed == nil {
		return r.vm.ToValue(false)
	}
	hasParsedNodes := len(parsed.ChildNodes()) > 0
	deleted := compareBoundaryPoints(start, end) != 0
	if deleted {
		r.jsRangeDeleteContents(ran)
	}
	point := ran.start
	parent, reference, ok := r.insertionPointForBoundary(point)
	if !ok {
		return r.vm.ToValue(false)
	}
	if hasParsedNodes {
		args := []goja.Value{r.wrapNode(parsed)}
		if reference == nil {
			args = append(args, goja.Null())
		} else {
			args = append(args, r.wrapNode(reference))
		}
		r.jsInsertBefore(goja.FunctionCall{This: r.wrapNode(parent), Arguments: args})
		if reference != nil {
			offset := childIndex(parent, reference)
			ran.start = rangeBoundary{container: parent, offset: offset}
		} else {
			ran.start = rangeBoundary{container: parent, offset: nodeChildCount(parent)}
		}
		ran.end = ran.start
	}
	changed := deleted || hasParsedNodes
	if changed {
		r.dispatchInputEvent(host, nil, inputType)
	}
	return r.vm.ToValue(changed)
}

// insertionPointForBoundary normalizes a Range boundary into a parent and
// reference node, splitting a Text node when the insertion falls inside it.
func (r *Runtime) insertionPointForBoundary(point rangeBoundary) (*dom.Node, *dom.Node, bool) {
	if point.container == nil {
		return nil, nil, false
	}
	if point.container.Type != dom.NodeText {
		offset := point.offset
		if offset < 0 {
			offset = 0
		}
		if count := nodeChildCount(point.container); offset > count {
			offset = count
		}
		return point.container, childAt(point.container, offset), true
	}
	text := point.container
	parent := text.Parent
	if parent == nil {
		return nil, nil, false
	}
	units := characterDataCodeUnits(text)
	offset := point.offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(units) {
		offset = len(units)
	}
	if offset == 0 {
		return parent, text, true
	}
	if offset == len(units) {
		return parent, text.NextSibling, true
	}
	left := string(utf16.Decode(units[:offset]))
	right := string(utf16.Decode(units[offset:]))
	r.replaceCharacterData(text, 0, len(units), left)
	rightNode := text.Doc.NewText(right)
	args := []goja.Value{r.wrapNode(rightNode)}
	if text.NextSibling == nil {
		args = append(args, goja.Null())
	} else {
		args = append(args, r.wrapNode(text.NextSibling))
	}
	r.jsInsertBefore(goja.FunctionCall{This: r.wrapNode(parent), Arguments: args})
	return parent, rightNode, true
}

func (r *Runtime) execCommandInsertText(sel *jsSelection, value string) goja.Value {
	return r.execCommandInsertTextWithInputType(sel, value, "insertText", &value, nil)
}

func (r *Runtime) execCommandInsertTextFromPaste(sel *jsSelection, value string, transfer *dom.DataTransfer) goja.Value {
	return r.execCommandInsertTextWithInputType(sel, value, "insertFromPaste", nil, transfer)
}

func (r *Runtime) execCommandInsertTextWithInputType(sel *jsSelection, value, inputType string, eventData *string, transfer *dom.DataTransfer) goja.Value {
	if inputType == "insertText" {
		if result, handled := r.execCommandInsertTextWithTypingStyles(sel, value); handled {
			return result
		}
	}
	selection, ok := selectedTextRange(sel)
	if !ok {
		return r.execCommandInsertTextAcrossNodesWithInputType(sel, value, inputType, eventData, transfer)
	}
	units := characterDataCodeUnits(selection.text)
	start, end := selection.start, selection.end
	if start > len(units) {
		start = len(units)
	}
	if end > len(units) {
		end = len(units)
	}
	inserted := utf16.Encode([]rune(value))
	updated := make([]uint16, 0, len(units)-(end-start)+len(inserted))
	updated = append(updated, units[:start]...)
	updated = append(updated, inserted...)
	updated = append(updated, units[end:]...)
	if unitsEqual(units, updated) {
		return r.vm.ToValue(false)
	}
	setCharacterDataCodeUnits(selection.text, updated)
	cursor := start + len(inserted)
	selection.rangeObj.start = rangeBoundary{container: selection.text, offset: cursor}
	selection.rangeObj.end = selection.rangeObj.start
	r.notifyMutation()
	r.dispatchInputEventWithDataTransfer(selection.host, eventData, inputType, transfer)
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandInsertTextAcrossNodes(sel *jsSelection, value string) goja.Value {
	data := value
	return r.execCommandInsertTextAcrossNodesWithInputType(sel, value, "", &data, nil)
}

func (r *Runtime) execCommandInsertTextAcrossNodesWithInputType(sel *jsSelection, value, requestedInputType string, requestedData *string, transfer *dom.DataTransfer) goja.Value {
	if requestedInputType == "insertText" {
		if result, handled := r.execCommandInsertTextWithTypingStyles(sel, value); handled {
			return result
		}
	}
	if sel == nil || len(sel.ranges) == 0 {
		return r.vm.ToValue(false)
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	host := editingHostFor(start.container)
	if host == nil || editingHostFor(end.container) != host ||
		!isAncestorNode(host, start.container) || !isAncestorNode(host, end.container) {
		return r.vm.ToValue(false)
	}
	deleted := compareBoundaryPoints(start, end) != 0
	startBlock := nearestEditingBlock(host, start.container)
	endBlock := nearestEditingBlock(host, end.container)
	horizontalRuleParent, horizontalRuleOffset := selectedHorizontalRulePosition(host, start, end)
	tableCellsHandled := deleted && requestedInputType == "insertText" && deleteTableCellSelection(r, ran, start, end, host, requestedInputType)
	var selectedCells []selectedCellPlaceholder
	if !tableCellsHandled {
		selectedCells = selectedTableCellPlaceholders(host, start, end)
	}
	if deleted {
		if !tableCellsHandled {
			r.jsRangeDeleteContents(ran)
			restoreSelectedTableCellPlaceholders(r, selectedCells)
		}
		if horizontalRuleParent != nil {
			trimHorizontalRuleBoundaryWhitespace(r, horizontalRuleParent, horizontalRuleOffset)
		}
		if !tableCellsHandled {
			mergedInlineBeforeBlock := endBlock != nil && boundaryAtEditingBlockStart(host, endBlock, end) &&
				(mergeInlineBeforeListItem(r, start, endBlock, end, host) || mergeInlineBeforeBlock(r, start, endBlock, host))
			mergedInlineAfterBlock := !mergedInlineBeforeBlock && startBlock != nil &&
				boundaryAtEditingBlockEnd(host, startBlock, start) && mergeInlineAfterBlock(r, startBlock, start, end, host)
			mergedInlineAfterListItem := !mergedInlineBeforeBlock && !mergedInlineAfterBlock && startBlock != nil &&
				startBlock.Element() && strings.EqualFold(startBlock.Data, "li") && boundaryAtEditingBlockEnd(host, startBlock, start) &&
				mergeInlineAfterListItem(r, startBlock, start, end, host)
			if !mergedInlineBeforeBlock && !mergedInlineAfterBlock && !mergedInlineAfterListItem && startBlock != nil && endBlock != nil && startBlock != endBlock {
				mergeAdjacentEditingBlocks(r, startBlock, endBlock, host)
			}
		}
	}
	boundary := ran.start
	if start.container.Type == dom.NodeText {
		boundary = rangeBoundary{container: start.container, offset: start.offset}
		if boundary.offset > len(characterDataCodeUnits(start.container)) {
			boundary.offset = len(characterDataCodeUnits(start.container))
		}
	}
	inserted := utf16.Encode([]rune(value))
	if len(inserted) == 0 && !deleted {
		return r.vm.ToValue(false)
	}
	if boundary.container == nil {
		return r.vm.ToValue(false)
	}
	if boundary.container.Type == dom.NodeText {
		units := characterDataCodeUnits(boundary.container)
		offset := boundary.offset
		if offset < 0 {
			offset = 0
		}
		if offset > len(units) {
			offset = len(units)
		}
		updated := make([]uint16, 0, len(units)+len(inserted))
		updated = append(updated, units[:offset]...)
		updated = append(updated, inserted...)
		updated = append(updated, units[offset:]...)
		setCharacterDataCodeUnits(boundary.container, updated)
		boundary = rangeBoundary{container: boundary.container, offset: offset + len(inserted)}
	} else {
		if boundary.container.Doc == nil {
			return r.vm.ToValue(false)
		}
		text := boundary.container.Doc.NewText(string(utf16.Decode(inserted)))
		index := boundary.offset
		if index < 0 {
			index = 0
		}
		if index > nodeChildCount(boundary.container) {
			index = nodeChildCount(boundary.container)
		}
		boundary.container.InsertNode(text, childAt(boundary.container, index))
		boundary = rangeBoundary{container: text, offset: len(inserted)}
	}
	ran.start, ran.end = boundary, boundary
	if len(inserted) > 0 {
		r.notifyMutation()
	}
	if deleted || len(inserted) > 0 {
		inputType := requestedInputType
		inputData := requestedData
		if inputType == "" {
			inputType = "deleteContentBackward"
			inputData = nil
			if len(inserted) > 0 {
				inputType = "insertText"
				if deleted {
					inputType = "insertReplacementText"
				}
				inputData = &value
			}
		}
		r.dispatchInputEventWithDataTransfer(host, inputData, inputType, transfer)
		return r.vm.ToValue(true)
	}
	return r.vm.ToValue(false)
}

func (r *Runtime) execCommandFormat(sel *jsSelection, tag string) goja.Value {
	selection, ok := selectedTextRange(sel)
	if !ok {
		return r.execCommandFormatAcrossNodes(sel, tag)
	}
	if selection.start == selection.end {
		return r.execCommandToggleTypingStyle(sel.ranges[0], tag)
	}
	if selection.text.Parent == nil || selection.text.Doc == nil {
		return r.vm.ToValue(false)
	}
	units := characterDataCodeUnits(selection.text)
	prefix := string(utf16.Decode(units[:selection.start]))
	suffix := string(utf16.Decode(units[selection.end:]))
	parent := selection.text.Parent
	opposite := ""
	if tag == "sub" {
		opposite = "sup"
	} else if tag == "sup" {
		opposite = "sub"
	}
	if opposite != "" && parent != nil && parent.Element() && parent.Data == opposite && parent.Parent != nil && prefix == "" && suffix == "" && parent.FirstChild == selection.text && parent.LastChild == selection.text {
		outer := parent.Parent
		replacement := selection.text.Doc.NewElement(tag)
		replacement.AppendChild(selection.text)
		outer.InsertNode(replacement, parent)
		outer.RemoveChild(parent)
		selection.rangeObj.start = rangeBoundary{container: selection.text, offset: 0}
		selection.rangeObj.end = rangeBoundary{container: selection.text, offset: len(units)}
		r.notifyMutation()
		r.dispatchInputEvent(selection.host, nil, editingFormatInputType(tag))
		return r.vm.ToValue(true)
	}
	index := childIndex(parent, selection.text)
	parent.RemoveChild(selection.text)
	if prefix != "" {
		parent.InsertNode(selection.text.Doc.NewText(prefix), childAt(parent, index))
		index++
	}
	setCharacterDataCodeUnits(selection.text, units[selection.start:selection.end])
	wrapper := selection.text.Doc.NewElement(tag)
	wrapper.AppendChild(selection.text)
	parent.InsertNode(wrapper, childAt(parent, index))
	index++
	if suffix != "" {
		parent.InsertNode(selection.text.Doc.NewText(suffix), childAt(parent, index))
	}
	selection.rangeObj.start = rangeBoundary{container: selection.text, offset: 0}
	selection.rangeObj.end = rangeBoundary{container: selection.text, offset: selection.end - selection.start}
	r.notifyMutation()
	r.dispatchInputEvent(selection.host, nil, editingFormatInputType(tag))
	return r.vm.ToValue(true)
}

func (r *Runtime) execCommandFormatAcrossNodes(sel *jsSelection, tag string) goja.Value {
	if sel == nil || len(sel.ranges) == 0 {
		return r.vm.ToValue(false)
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return r.vm.ToValue(false)
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	if compareBoundaryPoints(start, end) == 0 {
		return r.execCommandToggleTypingStyle(ran, tag)
	}
	host := editingHostFor(start.container)
	if host == nil || editingHostFor(end.container) != host ||
		!isAncestorNode(host, start.container) || !isAncestorNode(host, end.container) {
		return r.vm.ToValue(false)
	}
	textNodes := make([]*dom.Node, 0)
	var collect func(*dom.Node)
	collect = func(node *dom.Node) {
		if node == nil || (node != host && editingHostFor(node) != host) {
			return
		}
		if node.Type == dom.NodeText {
			textNodes = append(textNodes, node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(host)
	var firstSelected, lastSelected *dom.Node
	for _, text := range textNodes {
		units := characterDataCodeUnits(text)
		textStart := rangeBoundary{container: text, offset: 0}
		textEnd := rangeBoundary{container: text, offset: len(units)}
		if compareBoundaryPoints(textEnd, start) <= 0 || compareBoundaryPoints(textStart, end) >= 0 {
			continue
		}
		from, to := 0, len(units)
		if text == start.container {
			from = start.offset
		}
		if text == end.container {
			to = end.offset
		}
		if from < 0 {
			from = 0
		}
		if from > len(units) {
			from = len(units)
		}
		if to < from {
			to = from
		}
		if to > len(units) {
			to = len(units)
		}
		if from == to || text.Parent == nil || text.Doc == nil {
			continue
		}
		parent := text.Parent
		index := childIndex(parent, text)
		prefix := string(utf16.Decode(units[:from]))
		suffix := string(utf16.Decode(units[to:]))
		parent.RemoveChild(text)
		if prefix != "" {
			parent.InsertNode(text.Doc.NewText(prefix), childAt(parent, index))
			index++
		}
		setCharacterDataCodeUnits(text, units[from:to])
		wrapper := text.Doc.NewElement(tag)
		wrapper.AppendChild(text)
		parent.InsertNode(wrapper, childAt(parent, index))
		index++
		if suffix != "" {
			parent.InsertNode(text.Doc.NewText(suffix), childAt(parent, index))
		}
		if firstSelected == nil {
			firstSelected = text
		}
		lastSelected = text
	}
	if firstSelected == nil || lastSelected == nil {
		return r.vm.ToValue(false)
	}
	ran.start = rangeBoundary{container: firstSelected, offset: 0}
	ran.end = rangeBoundary{container: lastSelected, offset: len(characterDataCodeUnits(lastSelected))}
	r.notifyMutation()
	r.dispatchInputEvent(host, nil, editingFormatInputType(tag))
	return r.vm.ToValue(true)
}

type execCommandTypingStyle struct {
	enabled bool
	node    *dom.Node
	offset  int
}

func editingFormatInputType(tag string) string {
	switch tag {
	case "b":
		return "formatBold"
	case "i":
		return "formatItalic"
	case "u":
		return "formatUnderline"
	case "strike":
		return "formatStrikeThrough"
	case "sup":
		return "formatSuperscript"
	case "sub":
		return "formatSubscript"
	default:
		return "formatBlock"
	}
}

func (r *Runtime) execCommandToggleTypingStyle(ran *jsRange, tag string) goja.Value {
	if ran == nil || ran.start.container == nil || ran.end.container == nil ||
		compareBoundaryPoints(ran.start, ran.end) != 0 {
		return r.vm.ToValue(false)
	}
	host := editingHostFor(ran.start.container)
	if host == nil || !isAncestorNode(host, ran.start.container) || ran.start.container.Doc == nil {
		return r.vm.ToValue(false)
	}
	doc := ran.start.container.Doc
	if r.execCommandTypingStyles == nil {
		r.execCommandTypingStyles = make(map[*dom.Document]map[string]execCommandTypingStyle)
	}
	states := r.execCommandTypingStyles[doc]
	if states == nil {
		states = make(map[string]execCommandTypingStyle)
		r.execCommandTypingStyles[doc] = states
	}
	state, known := states[tag]
	enabled := !hasEditingFormatAncestor(ran.start.container, host, tag)
	if known && state.node == ran.start.container && state.offset == ran.start.offset {
		enabled = !state.enabled
	}
	states[tag] = execCommandTypingStyle{enabled: enabled, node: ran.start.container, offset: ran.start.offset}
	return r.vm.ToValue(true)
}

func hasEditingFormatAncestor(node, host *dom.Node, tag string) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Element() && editingFormatTagMatches(current.Data, tag) {
			return true
		}
		if current == host {
			break
		}
	}
	return false
}

func editingFormatTagMatches(name, command string) bool {
	switch command {
	case "b":
		return name == "b" || name == "strong"
	case "i":
		return name == "i" || name == "em"
	case "u":
		return name == "u"
	default:
		return name == command
	}
}

func editingFormatCommandTag(command string) string {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "bold":
		return "b"
	case "italic":
		return "i"
	case "underline":
		return "u"
	default:
		return ""
	}
}

// queryEditingFormatState implements the state and indeterminate queries for
// the formatting commands supported by execCommand. A collapsed selection
// reports its active ancestor or pending typing style; a non-collapsed
// selection is indeterminate when only some selected text has the format.
func (r *Runtime) queryEditingFormatState(doc *dom.Document, command string) (bool, bool) {
	tag := editingFormatCommandTag(command)
	if doc == nil || tag == "" {
		return false, false
	}
	sel := r.selectionForDocument(doc)
	if sel == nil || len(sel.ranges) == 0 {
		return false, false
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil {
		return false, false
	}
	start, end := ran.start, ran.end
	if compareBoundaryPoints(start, end) > 0 {
		start, end = end, start
	}
	if compareBoundaryPoints(start, end) == 0 {
		if states := r.execCommandTypingStyles[doc]; states != nil {
			if state, ok := states[tag]; ok && state.node == start.container && state.offset == start.offset {
				return state.enabled, false
			}
		}
		return hasEditingFormatAncestor(start.container, editingHostFor(start.container), tag), false
	}
	host := editingHostFor(start.container)
	if host == nil || editingHostFor(end.container) != host {
		return false, false
	}
	selected, formatted := 0, 0
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil || (node != host && editingHostFor(node) != host) {
			return
		}
		if node.Type == dom.NodeText {
			units := characterDataCodeUnits(node)
			textStart := rangeBoundary{container: node, offset: 0}
			textEnd := rangeBoundary{container: node, offset: len(units)}
			if compareBoundaryPoints(textEnd, start) <= 0 || compareBoundaryPoints(textStart, end) >= 0 {
				return
			}
			from, to := 0, len(units)
			if node == start.container {
				from = start.offset
			}
			if node == end.container {
				to = end.offset
			}
			if from < 0 {
				from = 0
			}
			if from > len(units) {
				from = len(units)
			}
			if to < from {
				to = from
			}
			if to > len(units) {
				to = len(units)
			}
			if from == to {
				return
			}
			selected++
			if hasEditingFormatAncestor(node, host, tag) {
				formatted++
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(host)
	if selected == 0 || formatted == 0 {
		return false, false
	}
	if formatted != selected {
		return false, true
	}
	return true, false
}

func (r *Runtime) execCommandInsertTextWithTypingStyles(sel *jsSelection, value string) (goja.Value, bool) {
	if sel == nil || len(sel.ranges) == 0 {
		return nil, false
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.end.container == nil ||
		compareBoundaryPoints(ran.start, ran.end) != 0 {
		return nil, false
	}
	text := ran.start.container
	host := editingHostFor(text)
	if host == nil || !isAncestorNode(host, text) || text.Parent == nil || text.Doc == nil {
		return nil, false
	}
	states := r.execCommandTypingStyles[text.Doc]
	if len(states) == 0 {
		return nil, false
	}
	var active, disabled []string
	for _, tag := range []string{"b", "i", "u"} {
		state, known := states[tag]
		if !known {
			if hasEditingFormatAncestor(text, host, tag) {
				active = append(active, tag)
			}
			continue
		}
		if state.enabled {
			if !hasEditingFormatAncestor(text, host, tag) {
				active = append(active, tag)
			}
		} else if hasEditingFormatAncestor(text, host, tag) {
			disabled = append(disabled, tag)
		}
	}
	if len(active) == 0 && len(disabled) == 0 {
		return nil, false
	}
	if text.Type != dom.NodeText {
		if len(active) == 0 || len(disabled) != 0 || text.Doc == nil || !text.Element() {
			return nil, false
		}
		insertedUnits := utf16.Encode([]rune(value))
		if len(insertedUnits) == 0 {
			return r.vm.ToValue(false), true
		}
		inserted := text.Doc.NewText(string(utf16.Decode(insertedUnits)))
		var wrapper *dom.Node = inserted
		for _, tag := range active {
			outer := newEditingFormatWrapper(text.Doc, text, host, tag)
			outer.AppendChild(wrapper)
			wrapper = outer
		}
		index := ran.start.offset
		if index < 0 {
			index = 0
		}
		if index > nodeChildCount(text) {
			index = nodeChildCount(text)
		}
		text.InsertNode(wrapper, childAt(text, index))
		ran.start = rangeBoundary{container: inserted, offset: len(insertedUnits)}
		ran.end = ran.start
		r.notifyMutation()
		inputData := value
		r.dispatchInputEvent(host, &inputData, "insertText")
		return r.vm.ToValue(true), true
	}
	units := characterDataCodeUnits(text)
	offset := ran.start.offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(units) {
		offset = len(units)
	}
	insertedUnits := utf16.Encode([]rune(value))
	if len(insertedUnits) == 0 {
		return r.vm.ToValue(false), true
	}

	if len(disabled) > 0 {
		target := editingFormatAncestor(text, host, disabled[0])
		if target == nil || target == host || target.Parent == nil {
			return nil, false
		}
		inserted := text.Doc.NewText(string(utf16.Decode(insertedUnits)))
		var wrapper *dom.Node = inserted
		for _, tag := range active {
			existing := editingFormatAncestor(text, host, tag)
			if existing != nil && !isAncestorNode(target, existing) {
				// This formatting ancestor stays around the insertion when target
				// is split, so cloning it here would apply the style twice.
				continue
			}
			outer := newEditingFormatWrapper(text.Doc, text, host, tag)
			outer.AppendChild(wrapper)
			wrapper = outer
		}
		if !splitTextOutOfFormattingAncestor(text, target, offset) {
			return nil, false
		}
		formatParent := target.Parent
		formatParent.InsertNode(wrapper, target.NextSibling)
		ran.start = rangeBoundary{container: inserted, offset: len(insertedUnits)}
		ran.end = ran.start
		r.notifyMutation()
		inputData := value
		r.dispatchInputEvent(host, &inputData, "insertText")
		return r.vm.ToValue(true), true
	}

	parent := text.Parent
	index := childIndex(parent, text)
	prefix := units[:offset]
	suffix := units[offset:]
	parent.RemoveChild(text)
	if len(prefix) > 0 {
		parent.InsertNode(text.Doc.NewText(string(utf16.Decode(prefix))), childAt(parent, index))
		index++
	}
	inserted := text.Doc.NewText(string(utf16.Decode(insertedUnits)))
	var wrapper *dom.Node = inserted
	for _, tag := range active {
		outer := newEditingFormatWrapper(text.Doc, text, host, tag)
		outer.AppendChild(wrapper)
		wrapper = outer
	}
	parent.InsertNode(wrapper, childAt(parent, index))
	if len(suffix) > 0 {
		parent.InsertNode(text.Doc.NewText(string(utf16.Decode(suffix))), childAt(parent, index+1))
	}
	ran.start = rangeBoundary{container: inserted, offset: len(insertedUnits)}
	ran.end = ran.start
	r.notifyMutation()
	inputData := value
	r.dispatchInputEvent(host, &inputData, "insertText")
	return r.vm.ToValue(true), true
}

func newEditingFormatWrapper(doc *dom.Document, context, host *dom.Node, tag string) *dom.Node {
	if existing := editingFormatAncestor(context, host, tag); existing != nil {
		clone := existing.CloneNode(false)
		clone.RemoveAttribute("id")
		return clone
	}
	return doc.NewElement(tag)
}

func editingFormatAncestor(node, host *dom.Node, tag string) *dom.Node {
	for current := node; current != nil; current = current.Parent {
		if current.Element() && editingFormatTagMatches(current.Data, tag) {
			return current
		}
		if current == host {
			break
		}
	}
	return nil
}

// splitTextOutOfFormattingAncestor preserves the content on both sides of a
// caret while moving newly inserted text outside target. Every inline ancestor
// between the Text node and target is cloned for its trailing subtree.
func splitTextOutOfFormattingAncestor(text, target *dom.Node, offset int) bool {
	if text == nil || target == nil || text.Type != dom.NodeText || text.Parent == nil ||
		target.Parent == nil || text.Doc == nil {
		return false
	}
	type splitLevel struct {
		node      *dom.Node
		following []*dom.Node
	}
	var levels []splitLevel
	child := text
	for current := text.Parent; current != nil; current = current.Parent {
		if current.Type != dom.NodeElement || child.Parent != current {
			return false
		}
		level := splitLevel{node: current}
		for sibling := child.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			level.following = append(level.following, sibling)
		}
		levels = append(levels, level)
		if current == target {
			break
		}
		child = current
	}
	if len(levels) == 0 || levels[len(levels)-1].node != target {
		return false
	}
	units := characterDataCodeUnits(text)
	if offset < 0 {
		offset = 0
	}
	if offset > len(units) {
		offset = len(units)
	}
	prefix, trailingText := units[:offset], units[offset:]
	if len(prefix) > 0 {
		setCharacterDataCodeUnits(text, prefix)
	} else {
		text.Parent.RemoveChild(text)
	}
	var suffix *dom.Node
	if len(trailingText) > 0 {
		suffix = text.Doc.NewText(string(utf16.Decode(trailingText)))
	}
	for _, level := range levels {
		clone := level.node.CloneNode(false)
		clone.RemoveAttribute("id")
		if suffix != nil {
			clone.AppendChild(suffix)
		}
		for _, sibling := range level.following {
			if sibling.Parent == level.node {
				level.node.RemoveChild(sibling)
				clone.AppendChild(sibling)
			}
		}
		suffix = nil
		if clone.FirstChild != nil {
			parent := level.node.Parent
			if parent == nil {
				return false
			}
			parent.InsertNode(clone, level.node.NextSibling)
			suffix = clone
		}
	}
	return target.Parent != nil
}

type selectedTextSelection struct {
	rangeObj *jsRange
	text     *dom.Node
	host     *dom.Node
	start    int
	end      int
}

func selectedTextRange(sel *jsSelection) (selectedTextSelection, bool) {
	if sel == nil || len(sel.ranges) == 0 {
		return selectedTextSelection{}, false
	}
	ran := sel.ranges[0]
	if ran.start.container == nil || ran.start.container != ran.end.container {
		return selectedTextSelection{}, false
	}
	textNode := ran.start.container
	start, end := ran.start.offset, ran.end.offset
	if textNode.Type != dom.NodeText {
		lo, hi := start, end
		if lo > hi {
			lo, hi = hi, lo
		}
		if !textNode.Element() || hi-lo != 1 || lo < 0 {
			return selectedTextSelection{}, false
		}
		textNode = childAt(textNode, lo)
		if textNode == nil || textNode.Type != dom.NodeText {
			return selectedTextSelection{}, false
		}
		start, end = 0, len(characterDataCodeUnits(textNode))
	}
	host := editingHostFor(textNode)
	if host == nil {
		return selectedTextSelection{}, false
	}
	if start > end {
		start, end = end, start
	}
	units := characterDataCodeUnits(textNode)
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > len(units) {
		start = len(units)
	}
	if end > len(units) {
		end = len(units)
	}
	return selectedTextSelection{rangeObj: ran, text: textNode, host: host, start: start, end: end}, true
}

func editingHostFor(node *dom.Node) *dom.Node {
	for cur := node; cur != nil; cur = cur.Parent {
		if !cur.Element() {
			continue
		}
		if cur.Doc != nil && cur.Doc.DesignMode && cur == cur.Doc.Body {
			return cur
		}
		state := dom.ContentEditableState(cur)
		switch state {
		case "false":
			return nil
		case "true", "plaintext-only":
			if cur.Parent == nil || !dom.IsContentEditable(cur.Parent) {
				return cur
			}
		}
	}
	return nil
}

// wrapSelection creates a JS Selection wrapper from a jsSelection.
func (r *Runtime) wrapSelection(sel *jsSelection) *goja.Object {
	if sel == nil {
		return nil
	}
	if r.selectionWrappers == nil {
		r.selectionWrappers = make(map[*jsSelection]*goja.Object)
	}
	if wrapper := r.selectionWrappers[sel]; wrapper != nil {
		return wrapper
	}
	obj := r.vm.NewObject()
	_ = obj.Set("__sel__", sel)

	// anchorNode
	_ = obj.Set("__getAnchorNode__", func() goja.Value {
		if len(sel.ranges) == 0 {
			return goja.Null()
		}
		return r.wrapNode(selectionAnchor(sel).container)
	})
	// anchorOffset
	_ = obj.Set("__getAnchorOffset__", func() int {
		if len(sel.ranges) == 0 {
			return 0
		}
		return selectionAnchor(sel).offset
	})
	// focusNode
	_ = obj.Set("__getFocusNode__", func() goja.Value {
		if len(sel.ranges) == 0 {
			return goja.Null()
		}
		return r.wrapNode(selectionFocus(sel).container)
	})
	// focusOffset
	_ = obj.Set("__getFocusOffset__", func() int {
		if len(sel.ranges) == 0 {
			return 0
		}
		return selectionFocus(sel).offset
	})
	// isCollapsed
	_ = obj.Set("__getIsCollapsed__", func() bool {
		if len(sel.ranges) == 0 {
			return true
		}
		ran := sel.ranges[0]
		return ran.start.container == ran.end.container && ran.start.offset == ran.end.offset
	})
	// rangeCount
	_ = obj.Set("__getRangeCount__", func() int {
		return len(sel.ranges)
	})
	// type
	_ = obj.Set("__getType__", func() string {
		if len(sel.ranges) == 0 {
			return "None"
		}
		ran := sel.ranges[0]
		if ran.start.container == ran.end.container && ran.start.offset == ran.end.offset {
			return "Caret"
		}
		return "Range"
	})

	// Methods
	_ = obj.Set("getRangeAt", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionGetRangeAt(call, sel)
	})
	_ = obj.Set("addRange", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionAddRange(call, sel)
	})
	_ = obj.Set("removeRange", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionRemoveRange(call, sel)
	})
	_ = obj.Set("removeAllRanges", func(call goja.FunctionCall) goja.Value {
		sel.ranges = nil
		sel.selectedControl = nil
		sel.backward = false
		return goja.Undefined()
	})
	_ = obj.Set("deleteFromDocument", func(call goja.FunctionCall) goja.Value {
		if len(sel.ranges) == 0 {
			return goja.Undefined()
		}
		// Selection.deleteFromDocument() deletes the selected range's contents.
		// Range.deleteContents() applies the normal DOM removal steps, including
		// custom-element disconnection reactions, and collapses the live range.
		r.jsRangeDeleteContents(sel.ranges[0])
		sel.selectedControl = nil
		return goja.Undefined()
	})
	_ = obj.Set("collapse", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionCollapse(call, sel)
	})
	_ = obj.Set("extend", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionExtend(call, sel)
	})
	_ = obj.Set("setBaseAndExtent", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionSetBaseAndExtent(call, sel)
	})
	_ = obj.Set("selectAllChildren", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionSelectAllChildren(call, sel)
	})
	_ = obj.Set("collapseToStart", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionCollapseToStart(sel)
	})
	_ = obj.Set("collapseToEnd", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionCollapseToEnd(sel)
	})
	_ = obj.Set("containsNode", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionContainsNode(call, sel)
	})
	_ = obj.Set("toString", func(call goja.FunctionCall) goja.Value {
		return r.jsSelectionToString(sel)
	})

	// Define JS getters.
	_ = r.vm.Set("__selTarget__", obj)
	_, _ = r.vm.RunString(`
		(function() {
			var o = __selTarget__;
			Object.defineProperty(o, 'anchorNode', {
				get: function() { return o.__getAnchorNode__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'anchorOffset', {
				get: function() { return o.__getAnchorOffset__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'focusNode', {
				get: function() { return o.__getFocusNode__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'focusOffset', {
				get: function() { return o.__getFocusOffset__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'isCollapsed', {
				get: function() { return o.__getIsCollapsed__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'rangeCount', {
				get: function() { return o.__getRangeCount__(); },
				configurable: true, enumerable: true
			});
			Object.defineProperty(o, 'type', {
				get: function() { return o.__getType__(); },
				configurable: true, enumerable: true
			});
		})();
	`)
	_ = r.vm.Set("__selTarget__", nil)

	r.selectionWrappers[sel] = obj
	return obj
}

func (r *Runtime) jsSelectionGetRangeAt(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'getRangeAt': 1 argument required"))
	}
	idx := int(call.Arguments[0].ToInteger())
	if idx < 0 || idx >= len(sel.ranges) {
		panic(r.vm.NewTypeError("Failed to execute 'getRangeAt': index " + strings.Repeat("0", 0) + " is out of bounds"))
	}
	ran := sel.ranges[idx]
	obj := r.vm.NewObject()
	_ = obj.Set("__range__", ran)
	r.installRangeMethods(obj, ran)
	return obj
}

func (r *Runtime) jsSelectionAddRange(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'addRange': 1 argument required"))
	}
	rangeObj, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Failed to execute 'addRange': parameter 1 is not of type 'Range'"))
	}
	ran := getRange(rangeObj)
	if ran == nil {
		panic(r.vm.NewTypeError("Failed to execute 'addRange': parameter 1 is not of type 'Range'"))
	}
	// Check if this range is already in the selection.
	for _, existing := range sel.ranges {
		if existing == ran {
			return goja.Undefined()
		}
	}
	if len(sel.ranges) == 0 {
		sel.backward = false
		sel.selectedControl = nil
	}
	sel.ranges = append(sel.ranges, ran)
	return goja.Undefined()
}

func (r *Runtime) jsSelectionRemoveRange(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'removeRange': 1 argument required"))
	}
	rangeObj, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Failed to execute 'removeRange': parameter 1 is not of type 'Range'"))
	}
	ran := getRange(rangeObj)
	if ran == nil {
		panic(r.vm.NewTypeError("Failed to execute 'removeRange': parameter 1 is not of type 'Range'"))
	}
	for i, existing := range sel.ranges {
		if existing == ran {
			sel.ranges = append(sel.ranges[:i], sel.ranges[i+1:]...)
			if len(sel.ranges) == 0 {
				sel.backward = false
				sel.selectedControl = nil
			}
			return goja.Undefined()
		}
	}
	panic(r.vm.NewTypeError("Failed to execute 'removeRange': the range is not in the selection"))
}

func (r *Runtime) jsSelectionCollapse(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 1 {
		// collapse() with no args clears the selection.
		sel.ranges = nil
		sel.selectedControl = nil
		sel.backward = false
		return goja.Undefined()
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		sel.ranges = nil
		sel.selectedControl = nil
		sel.backward = false
		return goja.Undefined()
	}
	offset := 0
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
		offset = int(call.Arguments[1].ToInteger())
	}
	ran := &jsRange{
		start: rangeBoundary{container: node, offset: offset},
		end:   rangeBoundary{container: node, offset: offset},
	}
	r.registerLiveRange(ran)
	sel.ranges = []*jsRange{ran}
	sel.selectedControl = nil
	sel.backward = false
	return goja.Undefined()
}

func (r *Runtime) jsSelectionExtend(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(sel.ranges) == 0 {
		return goja.Undefined()
	}
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'extend': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'extend': parameter 1 is not of type 'Node'"))
	}
	offset := 0
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		offset = webIDLUnsignedLong(call.Argument(1))
	}
	ran := sel.ranges[0]
	anchor := selectionAnchor(sel)
	if nodeRoot(node) != nodeRoot(anchor.container) {
		panic(r.domError("WrongDocumentError", "The nodes have different roots"))
	}
	r.validateRangePoint(node, offset, "extend")
	focus := rangeBoundary{container: node, offset: offset}
	if compareBoundaryPoints(anchor, focus) <= 0 {
		ran.start, ran.end = anchor, focus
		sel.backward = false
	} else {
		ran.start, ran.end = focus, anchor
		sel.backward = true
	}
	return goja.Undefined()
}

func (r *Runtime) jsSelectionSetBaseAndExtent(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 4 {
		panic(r.vm.NewTypeError("Failed to execute 'setBaseAndExtent': 4 arguments required"))
	}
	baseNode := r.getNode(call.Argument(0))
	if baseNode == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setBaseAndExtent': parameter 1 is not of type 'Node'"))
	}
	baseOffset := webIDLUnsignedLong(call.Argument(1))
	extentNode := r.getNode(call.Argument(2))
	if extentNode == nil {
		panic(r.vm.NewTypeError("Failed to execute 'setBaseAndExtent': parameter 3 is not of type 'Node'"))
	}
	extentOffset := webIDLUnsignedLong(call.Argument(3))
	if nodeRoot(baseNode) != nodeRoot(extentNode) {
		panic(r.domError("WrongDocumentError", "The boundary points have different roots"))
	}
	r.validateRangePoint(baseNode, baseOffset, "setBaseAndExtent")
	r.validateRangePoint(extentNode, extentOffset, "setBaseAndExtent")
	base := rangeBoundary{container: baseNode, offset: baseOffset}
	extent := rangeBoundary{container: extentNode, offset: extentOffset}
	backward := compareBoundaryPoints(base, extent) > 0
	start, end := base, extent
	if backward {
		start, end = extent, base
	}
	ran := &jsRange{start: start, end: end}
	r.registerLiveRange(ran)
	sel.ranges = []*jsRange{ran}
	sel.selectedControl = nil
	sel.backward = backward
	return goja.Undefined()
}

func selectionAnchor(sel *jsSelection) rangeBoundary {
	ran := sel.ranges[0]
	if sel.backward {
		return ran.end
	}
	return ran.start
}

func selectionFocus(sel *jsSelection) rangeBoundary {
	ran := sel.ranges[0]
	if sel.backward {
		return ran.start
	}
	return ran.end
}

func (r *Runtime) jsSelectionSelectAllChildren(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'selectAllChildren': 1 argument required"))
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'selectAllChildren': parameter 1 is not of type 'Node'"))
	}
	length := nodeChildCount(node)
	ran := &jsRange{
		start: rangeBoundary{container: node, offset: 0},
		end:   rangeBoundary{container: node, offset: length},
	}
	r.registerLiveRange(ran)
	sel.ranges = []*jsRange{ran}
	sel.selectedControl = nil
	sel.backward = false
	return goja.Undefined()
}

func (r *Runtime) jsSelectionCollapseToStart(sel *jsSelection) goja.Value {
	if len(sel.ranges) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'collapseToStart': the selection has no range"))
	}
	ran := sel.ranges[0]
	ran.end = ran.start
	sel.selectedControl = nil
	sel.backward = false
	return goja.Undefined()
}

func (r *Runtime) jsSelectionCollapseToEnd(sel *jsSelection) goja.Value {
	if len(sel.ranges) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'collapseToEnd': the selection has no range"))
	}
	ran := sel.ranges[0]
	ran.start = ran.end
	sel.selectedControl = nil
	sel.backward = false
	return goja.Undefined()
}

func (r *Runtime) jsSelectionContainsNode(call goja.FunctionCall, sel *jsSelection) goja.Value {
	if len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		return r.vm.ToValue(false)
	}
	allowPartial := false
	if len(call.Arguments) > 1 {
		allowPartial = call.Arguments[1].ToBoolean()
	}
	_ = allowPartial
	for _, ran := range sel.ranges {
		// Check if node is within the range.
		// Simplified: check if node is between start and end containers.
		startPos := treePosition(ran.start.container)
		endPos := treePosition(ran.end.container)
		nodePos := treePosition(node)
		if nodePos >= startPos && nodePos <= endPos {
			return r.vm.ToValue(true)
		}
	}
	return r.vm.ToValue(false)
}

func (r *Runtime) jsSelectionToString(sel *jsSelection) goja.Value {
	if sel.selectedControl != nil && (r.opts.ActiveElement == nil || r.opts.ActiveElement() == sel.selectedControl) {
		value := sel.selectedControl.InputControlValue()
		if sel.selectedControl.InputUserEditValueActive {
			value = sel.selectedControl.InputUserEditValue
		}
		startOffset, endOffset, _, hasSelection := r.TextControlSelection(sel.selectedControl)
		if !hasSelection {
			return r.vm.ToValue(value)
		}
		units := utf16.Encode([]rune(value))
		start, end := startOffset, endOffset
		if start < 0 {
			start = 0
		}
		if end > len(units) {
			end = len(units)
		}
		if start > end {
			start = end
		}
		return r.vm.ToValue(string(utf16.Decode(units[start:end])))
	}
	if len(sel.ranges) == 0 {
		return r.vm.ToValue("")
	}
	var sb strings.Builder
	for _, ran := range sel.ranges {
		sb.WriteString(rangeToString(ran))
	}
	return r.vm.ToValue(sb.String())
}
