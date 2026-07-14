package layoutgolden

import (
	"fmt"
	"strings"

	"github.com/vyquocvu/goosie/internal/renderer"
)

// SerializeLayoutBox produces a deterministic text representation of a
// layout box tree. The output is stable across Go versions and platforms
// because:
//
//   - All floating point values are rounded to two decimals.
//   - Only structural fields (geometry, padding, margin, display type,
//     flex/grid container parameters) are emitted. Volatile fields such
//     as NodeID, font cache keys, and color values are intentionally
//     omitted.
//   - Children are emitted in source order — the layout engine
//     preserves the document order of children, so no sorting is
//     required.
//
// The rendered-tree input is used solely to enrich each box with its
// HTML tag name (for human-readable diff output). If a box has no
// matching render node (e.g. synthetic generated content), the box is
// emitted with a `<anon>` tag label.
func SerializeLayoutBox(root *renderer.LayoutBox, tagOf func(int64) string, header string) string {
	if header == "" {
		header = "goosie layout golden snapshot"
	}
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(header)
	sb.WriteString("\n")
	if root == nil {
		sb.WriteString("(no layout boxes)\n")
		return sb.String()
	}
	walk(&sb, root, 0, tagOf)
	return sb.String()
}

// walk emits a single box plus its descendants in a deterministic,
// indented format. Each box's geometry fields are emitted as
// "key=value" pairs separated by a single space, then a newline, then
// each child indented by two additional spaces.
func walk(sb *strings.Builder, box *renderer.LayoutBox, depth int, tagOf func(int64) string) {
	indent := strings.Repeat("  ", depth)

	tag := "<unknown>"
	if tagOf != nil {
		if t := tagOf(box.NodeID); t != "" {
			tag = t
		} else {
			tag = "<anon>"
		}
	}

	fmt.Fprintf(sb, "%s%s display=%s\n", indent, tag, box.Display)
	fmt.Fprintf(sb, "%s  box=(x=%.2f y=%.2f w=%.2f h=%.2f)\n",
		indent, round(box.Box.X), round(box.Box.Y), round(box.Box.Width), round(box.Box.Height))

	if box.Display == renderer.DisplayFlex {
		fmt.Fprintf(sb, "%s  flex=(direction=%s wrap=%s justify=%s align=%s gap=%.2f)\n",
			indent,
			flexVal(box.FlexDirection),
			flexVal(box.FlexWrap),
			flexVal(box.JustifyContent),
			flexVal(box.AlignItems),
			round(box.Gap),
		)
	}
	if box.Display == renderer.DisplayGrid {
		fmt.Fprintf(sb, "%s  grid=(cols=%q rows=%q)\n",
			indent,
			flexVal(box.GridTemplateColumns),
			flexVal(box.GridTemplateRows),
		)
	}
	if anyNonZero(box.PaddingTop, box.PaddingRight, box.PaddingBottom, box.PaddingLeft) {
		fmt.Fprintf(sb, "%s  padding=(t=%.2f r=%.2f b=%.2f l=%.2f)\n",
			indent, round(box.PaddingTop), round(box.PaddingRight), round(box.PaddingBottom), round(box.PaddingLeft))
	}
	if anyNonZero(box.MarginTop, box.MarginRight, box.MarginBottom, box.MarginLeft) {
		fmt.Fprintf(sb, "%s  margin=(t=%.2f r=%.2f b=%.2f l=%.2f)\n",
			indent, round(box.MarginTop), round(box.MarginRight), round(box.MarginBottom), round(box.MarginLeft))
	}

	for _, child := range box.Children {
		if child == nil {
			continue
		}
		walk(sb, child, depth+1, tagOf)
	}
}

func anyNonZero(vs ...float32) bool {
	for _, v := range vs {
		if round(v) != 0 {
			return true
		}
	}
	return false
}

// round returns v rounded to two decimal places. We avoid math.Round to
// keep allocations and dependencies minimal in the hot serializer path.
func round(v float32) float32 {
	// Multiply by 100, add 0.5 for positive, truncate via int conversion.
	if v >= 0 {
		return float32(int32(v*100+0.5)) / 100
	}
	return float32(int32(v*100-0.5)) / 100
}

// flexVal returns the value when non-empty, otherwise "-" so the
// serialized form is column-aligned and stable across runs.
func flexVal(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// TagLookup returns a function that maps a LayoutID (a NodeID int64) to
// the corresponding HTML tag name by walking the rendered tree. The
// returned function is O(depth) per call — fine for small fixtures, and
// the layout tree itself is typically small enough that caching adds
// more complexity than it saves.
//
// Returns nil when the tree is nil, in which case the serializer omits
// the tag label.
func TagLookup(tree *renderer.RenderNode) func(int64) string {
	if tree == nil {
		return nil
	}
	// Build a flat map first so lookups are O(1).
	idx := make(map[int64]string)
	collectTags(tree, idx)
	return func(id int64) string {
		return idx[id]
	}
}

func collectTags(n *renderer.RenderNode, idx map[int64]string) {
	if n == nil {
		return
	}
	idx[n.ID] = n.TagName
	if n.Type == renderer.NodeTypeText {
		idx[n.ID] = "#text"
	}
	for _, child := range n.Children {
		collectTags(child, idx)
	}
}
