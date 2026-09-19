package layout

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/style"
)

// gridTrack is one column or row of a grid as declared. min is the track's
// definite floor, or -1 when the declaration said auto; max is its definite
// ceiling, or -1 for no ceiling; fr is its flex weight.
type gridTrack struct {
	min float32
	max float32
	fr  float32
	// content is true for minmax() with a definite upper bound, min-content, and
	// max-content. These tracks size to their items rather than stretching to
	// fill leftover space.
	content bool
	// autoRepeat marks repeat(auto-fill|auto-fit, ...), whose count depends on
	// the container's own width and so cannot be resolved by the cascade.
	autoRepeat bool
	// fit distinguishes auto-fit, which collapses tracks no item reached, from
	// auto-fill, which keeps them.
	fit bool
}

// gridArea is the rectangle a grid-template-areas name covers, in track indexes.
type gridArea struct {
	col, row         int
	colSpan, rowSpan int
}

// gridItem is a grid item: its placement and the extents that placement gave it.
type gridItem struct {
	id       ObjectID
	name     string
	col      int
	row      int
	colSpan  int
	rowSpan  int
	fixedPos bool

	mLeft, mRight, mTop, mBottom float32
	justify                      string
	align                        string

	borderW  float32
	borderH  float32
	measured bool
	srcX     float32
	contentW float32
}

// layoutGrid lays out a grid container.
//
// The subset is the one real declarations use: fixed and fractional tracks,
// repeat(n, ...) and repeat(auto-fill|auto-fit, minmax(px, 1fr)), a named
// grid-template-areas template, row-major auto-placement honouring
// grid-column/grid-row spans, one gap value on both axes, and
// justify-items/align-items/justify-content/align-content.
func layoutGrid(a *Arena, id ObjectID, containingW float32) float32 {
	obj := a.Get(id)
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	container := obj.Style
	gapX, gapY := container.ColumnGap, container.RowGap
	contentH := float32(0)
	if container.Height >= 0 {
		contentH = obj.H
	}

	areaRows, areaCols, areas := parseGridAreas(container.GridTemplateAreas)
	cols := parseGridTracks(container.GridTemplateColumns, container.FontSize, contentW)
	switch {
	case len(cols) == 0:
		// An untemplated grid still needs one implicit column to place into.
		cols = autoTracks(max(areaCols, 1))
	case areaCols > len(cols):
		cols = append(cols, autoTracks(areaCols-len(cols))...)
	}
	fit := false
	for _, t := range cols {
		if t.autoRepeat && t.fit {
			fit = true
		}
	}
	cols = expandAutoTracks(cols, contentW, gapX)

	items := collectGridItems(a, id, contentX, contentY, container, areas)
	if len(items) == 0 {
		return 0
	}
	maxRow := autoPlaceGridItems(items, len(cols))
	if fit {
		used := 0
		for _, p := range items {
			if end := p.col + p.colSpan; end > used {
				used = end
			}
		}
		if used > 0 && used < len(cols) {
			cols = cols[:used]
		}
	}

	// Columns settle first, because an item's width decides how its text wraps and
	// so decides how tall the row it lands in has to be.
	colW := make([]float32, len(cols))
	sizeGridTracks(cols, colW, contentW, gapX, normalizeAxis(container.JustifyContent) == "",
		measureAutoCols(a, items, cols))
	colLead, colGap := axisPacking(container.JustifyContent, contentW, gapX, len(cols), colW)
	xs := make([]float32, len(cols))
	for i := range colW {
		if i == 0 {
			xs[i] = contentX + colLead
		} else {
			xs[i] = xs[i-1] + colW[i-1] + colGap
		}
	}

	for i := range items {
		p := &items[i]
		last := p.col + p.colSpan - 1
		if last >= len(xs) {
			last = len(xs) - 1
		}
		colRect := xs[last] + colW[last] - xs[p.col]
		it := a.Get(p.id)
		// The horizontal origin is already final, so descendants block layout puts
		// under this item land where they belong; only Y moves after sizing.
		it.X = xs[p.col]
		it.Y = contentY
		if p.justify != "" && it.Style.Width < 0 {
			// A content-sized item takes its max-content width: laid out at an
			// effectively infinite width its text cannot wrap, so the span the words
			// actually took is the span the item gets.
			blockInto(a, p.id, maxFlexMeasureWidth)
			it = a.Get(p.id)
			inlineInto(a, p.id)
			it = a.Get(p.id)
			if srcX, right, ok := inlineExtent(a, p.id); ok {
				p.srcX, p.contentW = srcX, right-srcX
			}
			it.W = p.contentW
			p.measured = true
		} else {
			blockInto(a, p.id, colRect)
			it = a.Get(p.id)
			inlineInto(a, p.id)
			it = a.Get(p.id)
		}
		p.borderW = it.W + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		p.borderH = it.BorderH()
	}

	// Rows: a declared row keeps its size, and every other row takes the tallest
	// item sitting wholly inside it.
	rowSpecs := parseGridTracks(container.GridTemplateRows, container.FontSize, contentH)
	nRows := len(rowSpecs)
	if maxRow+1 > nRows {
		nRows = maxRow + 1
	}
	if areaRows > nRows {
		nRows = areaRows
	}
	if nRows < 1 {
		nRows = 1
	}
	for len(rowSpecs) < nRows {
		rowSpecs = append(rowSpecs, gridTrack{min: -1, max: -1})
	}
	rowH := make([]float32, nRows)
	for i, tr := range rowSpecs {
		if tr.min > 0 {
			rowH[i] = tr.min
		}
	}
	for i := range items {
		p := &items[i]
		if p.rowSpan != 1 || p.row >= nRows {
			continue
		}
		if h := p.borderH + p.mTop + p.mBottom; h > rowH[p.row] {
			rowH[p.row] = h
		}
	}
	// An item taller than the rows it spans has nowhere to go but down, so the
	// deficit lands on the last auto row of its span.
	for i := range items {
		p := &items[i]
		if p.rowSpan < 2 || p.row >= nRows {
			continue
		}
		span := p.rowSpan
		if p.row+span > nRows {
			span = nRows - p.row
		}
		deficit := p.borderH + p.mTop + p.mBottom - trackSize(rowH, p.row, span, gapY)
		if deficit <= 0 {
			continue
		}
		for r := p.row + span - 1; r >= p.row; r-- {
			if rowSpecs[r].min < 0 {
				rowH[r] += deficit
				break
			}
		}
	}

	// In a container of definite height, align-content: normal stretches the auto
	// rows to fill it. That is what gives align-items room to align within.
	if contentH > trackSize(rowH, 0, nRows, gapY) && normalizeAxis(container.AlignContent) == "" {
		auto := 0
		for _, tr := range rowSpecs[:nRows] {
			if tr.min < 0 {
				auto++
			}
		}
		if auto > 0 {
			share := (contentH - trackSize(rowH, 0, nRows, gapY)) / float32(auto)
			for i := range rowH {
				if rowSpecs[i].min < 0 {
					rowH[i] += share
				}
			}
		}
	}
	rowLead, rowGap := axisPacking(container.AlignContent, contentH, gapY, nRows, rowH)
	ys := make([]float32, nRows)
	for i := range rowH {
		if i == 0 {
			ys[i] = contentY + rowLead
		} else {
			ys[i] = ys[i-1] + rowH[i-1] + rowGap
		}
	}

	maxBottom := float32(0)
	for i := range items {
		p := &items[i]
		colLast := minIdx(p.col+p.colSpan-1, len(xs)-1)
		rowLast := minIdx(p.row+p.rowSpan-1, nRows-1)
		colRect := xs[colLast] + colW[colLast] - xs[p.col]
		rowRect := ys[rowLast] + rowH[rowLast] - ys[p.row]
		it := a.Get(p.id)

		finalX := xs[p.col] + p.mLeft
		switch p.justify {
		case "center":
			finalX = xs[p.col] + (colRect-p.borderW)/2
		case "flex-end":
			finalX = xs[p.col] + colRect - p.borderW - p.mRight
		}
		if p.align == "" && it.Style.Height < 0 {
			// Stretch, the default: the item grows into its rows, which is only extra
			// space when a row has a size of its own.
			inner := rowRect - p.mTop - p.mBottom -
				(it.PaddingTop + it.PaddingBottom + it.BorderTop + it.BorderBottom)
			if inner > it.H {
				it.H = inner
				p.borderH = it.BorderH()
			}
		}
		finalY := ys[p.row] + p.mTop
		switch p.align {
		case "center":
			finalY = ys[p.row] + (rowRect-p.borderH)/2
		case "flex-end":
			finalY = ys[p.row] + rowRect - p.borderH - p.mBottom
		}

		dx := finalX - it.X
		dy := finalY - it.Y
		if p.measured {
			// The words are still where the measurement pass left them, so they
			// travel with the item instead of being allocated a second time.
			extra := float32(0)
			if spare := it.W - p.contentW; spare > 0 {
				switch it.Style.TextAlign {
				case style.TextAlignCenter:
					extra = spare / 2
				case style.TextAlignRight:
					extra = spare
				}
			}
			shiftInlineContent(a, p.id, finalX+it.BorderLeft+it.PaddingLeft+extra-p.srcX, dy)
			shiftBlockKids(a, p.id, dx, dy)
			it = a.Get(p.id)
			it.X = finalX
			it.Y = finalY
		} else {
			shiftSubtree(a, p.id, dx, dy)
			it = a.Get(p.id)
		}
		if bottom := (it.Y - contentY) + it.BorderH() + p.mBottom; bottom > maxBottom {
			maxBottom = bottom
		}
	}
	if contentH > maxBottom {
		maxBottom = contentH
	}
	return maxBottom
}

// shiftBlockKids moves an item's block descendants, which shiftInlineContent
// reaches through without moving itself.
func shiftBlockKids(a *Arena, id ObjectID, dx, dy float32) {
	obj := a.Get(id)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		if isBlock(a.Get(kid)) {
			shiftSubtree(a, kid, dx, dy)
		}
	}
}

// collectGridItems takes the in-flow children of a grid container. Raw text and
// out-of-flow boxes are not items, but a skipped text node still has its content
// cleared so the paint pass cannot draw it at the box's zero position.
func collectGridItems(a *Arena, id ObjectID, contentX, contentY float32, container *style.ComputedStyle, areas map[string]gridArea) []gridItem {
	obj := a.Get(id)
	var out []gridItem
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Type != 1 {
			if k.Node.Type == 2 {
				k.Node.DataContent = ""
			}
			continue
		}
		if isOutOfFlow(k.Style) {
			k.StaticX = contentX
			k.StaticY = contentY
			k.flags |= flagOutOfFlow
			continue
		}
		justify := normalizeAxis(k.Style.JustifyItems)
		if justify == "" {
			justify = normalizeAxis(container.JustifyItems)
		}
		align := normalizeAxis(k.Style.AlignSelf)
		if align == "" {
			align = normalizeAxis(container.AlignItems)
		}
		p := gridItem{
			id:      kid,
			name:    k.Style.GridArea,
			colSpan: k.Style.GridColumnSpan,
			rowSpan: k.Style.GridRowSpan,
			mLeft:   flexMargin(k.Style.MarginLeft),
			mRight:  flexMargin(k.Style.MarginRight),
			mTop:    flexMargin(k.Style.MarginTop),
			mBottom: flexMargin(k.Style.MarginBottom),
			justify: justify,
			align:   align,
		}
		if p.colSpan < 1 {
			p.colSpan = 1
		}
		if p.rowSpan < 1 {
			p.rowSpan = 1
		}
		if ar, ok := areas[p.name]; ok {
			p.col, p.row = ar.col, ar.row
			p.colSpan, p.rowSpan = ar.colSpan, ar.rowSpan
			p.fixedPos = true
		}
		out = append(out, p)
	}
	return out
}

// autoPlaceGridItems gives every item a cell: an item named by
// grid-template-areas keeps that rectangle, and the rest take the first free slot
// scanning row-major from the placement cursor, which is CSS's sparse packing. It
// reports the deepest row used.
func autoPlaceGridItems(items []gridItem, k int) int {
	if k < 1 {
		k = 1
	}
	used := make([][]bool, 0, 8)
	free := func(row, col, rowSpan, colSpan int) bool {
		for r := row; r < row+rowSpan; r++ {
			if r >= len(used) {
				return true
			}
			for c := col; c < col+colSpan; c++ {
				if c >= k || used[r][c] {
					return false
				}
			}
		}
		return true
	}
	mark := func(row, col, rowSpan, colSpan int) {
		for len(used) < row+rowSpan {
			used = append(used, make([]bool, k))
		}
		for r := row; r < row+rowSpan; r++ {
			for c := col; c < col+colSpan && c < k; c++ {
				used[r][c] = true
			}
		}
	}
	for i := range items {
		if items[i].colSpan > k {
			items[i].colSpan = k
		}
		if items[i].fixedPos {
			mark(items[i].row, items[i].col, items[i].rowSpan, items[i].colSpan)
		}
	}
	curRow, curCol := 0, 0
	for i := range items {
		p := &items[i]
		if p.fixedPos {
			continue
		}
		for curRow < 4096 {
			if curCol+p.colSpan > k {
				curCol = 0
				curRow++
				continue
			}
			if free(curRow, curCol, p.rowSpan, p.colSpan) {
				p.row, p.col = curRow, curCol
				mark(curRow, curCol, p.rowSpan, p.colSpan)
				break
			}
			curCol++
		}
	}
	lowest := 0
	for _, p := range items {
		if end := p.row + p.rowSpan; end > lowest {
			lowest = end
		}
	}
	return lowest - 1
}

// parseGridTracks splits a track-list value into tracks. repeat(n, ...) expands
// here; repeat(auto-fill|auto-fit, ...) stays one marked track until the
// container's width is known. em and rem resolve against fontSize, and a
// percentage against inlineSize, which is 0 for a track axis with no definite
// size - a percentage row of an auto-height grid is then left to size itself.
func parseGridTracks(v string, fontSize, inlineSize float32) []gridTrack {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || v == "none" {
		return nil
	}
	var out []gridTrack
	for _, tok := range splitGridTracks(v) {
		if !strings.HasPrefix(tok, "repeat(") {
			out = append(out, parseGridTrack(tok, fontSize, inlineSize))
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(tok, "repeat("), ")")
		parts := splitTopLevel(inner)
		if len(parts) < 2 {
			continue
		}
		head, body := parts[0], strings.Join(parts[1:], " ")
		if head == "auto-fill" || head == "auto-fit" {
			for _, t := range parseGridTracks(body, fontSize, inlineSize) {
				t.autoRepeat = true
				t.fit = head == "auto-fit"
				out = append(out, t)
			}
			continue
		}
		n, err := strconv.Atoi(head)
		if err != nil || n <= 0 {
			continue
		}
		for i := 0; i < n; i++ {
			out = append(out, parseGridTracks(body, fontSize, inlineSize)...)
		}
	}
	return out
}

func parseGridTrack(tok string, fontSize, inlineSize float32) gridTrack {
	tok = strings.TrimSpace(tok)
	if strings.HasPrefix(tok, "minmax(") {
		inner := strings.TrimSuffix(strings.TrimPrefix(tok, "minmax("), ")")
		if parts := strings.Split(inner, ","); len(parts) == 2 {
			lo, hi := parseGridTrack(strings.TrimSpace(parts[0]), fontSize, inlineSize), parseGridTrack(strings.TrimSpace(parts[1]), fontSize, inlineSize)
			t := gridTrack{min: lo.min, max: -1}
			if hi.fr > 0 {
				t.fr = hi.fr
			} else if hi.min >= 0 {
				t.max = hi.min
				t.content = true
			}
			return t
		}
	}
	if tok == "min-content" || tok == "max-content" {
		return gridTrack{min: -1, max: -1, content: true}
	}
	if strings.HasSuffix(tok, "fr") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(tok, "fr"), 32); err == nil && f > 0 {
			return gridTrack{min: -1, max: -1, fr: float32(f)}
		}
		return gridTrack{min: -1, max: -1}
	}
	if f, ok := gridLen(tok, fontSize, inlineSize); ok {
		return gridTrack{min: f, max: -1}
	}
	return gridTrack{min: -1, max: -1}
}

// gridLen reads a definite length out of a track declaration. Track lists stay
// text down to layout because a percentage track means the container's width,
// which the cascade has no way to know, so the units resolve here against the
// size layout already has.
func gridLen(s string, fontSize, inlineSize float32) (float32, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		if inlineSize <= 0 {
			return 0, false
		}
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 32)
		if err != nil {
			return 0, false
		}
		return float32(f) * inlineSize / 100, true
	}
	for _, u := range []struct {
		suffix string
		scale  float32
	}{
		{"px", 1},
		{"rem", 16},
		{"em", fontSize},
		{"ex", fontSize},
	} {
		if !strings.HasSuffix(s, u.suffix) || u.scale <= 0 {
			continue
		}
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 32)
		if err != nil {
			return 0, false
		}
		return float32(f) * u.scale, true
	}
	if s == "0" {
		return 0, true
	}
	return 0, false
}

// splitGridTracks splits a track list on whitespace and top-level commas so that
// "repeat(3, minmax(50px, 1fr))" survives as one token.
func splitGridTracks(v string) []string {
	var out []string
	depth := 0
	start := 0
	flush := func(end int) {
		if s := strings.TrimSpace(v[start:end]); s != "" {
			out = append(out, s)
		}
	}
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ', '\t', '\n', ',':
			if depth == 0 {
				flush(i)
				start = i + 1
			}
		}
	}
	flush(len(v))
	return out
}

// splitTopLevel splits an argument list on the commas that are not inside a
// nested function.
func splitTopLevel(v string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(v[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(v[start:]))
}

// expandAutoTracks resolves repeat(auto-fill|auto-fit, ...) into as many copies as
// fit at their own minimum, never zero.
func expandAutoTracks(tracks []gridTrack, avail, gap float32) []gridTrack {
	idx := -1
	for i, t := range tracks {
		if t.autoRepeat {
			idx = i
			break
		}
	}
	if idx < 0 {
		return tracks
	}
	t := tracks[idx]
	min := t.min
	if min <= 0 {
		min = 1
	}
	n := int((avail + gap) / (min + gap))
	if n < 1 {
		n = 1
	}
	out := make([]gridTrack, 0, len(tracks)+n)
	out = append(out, tracks[:idx]...)
	for i := 0; i < n; i++ {
		o := t
		o.autoRepeat = false
		out = append(out, o)
	}
	return append(out, tracks[idx+1:]...)
}

func autoTracks(n int) []gridTrack {
	out := make([]gridTrack, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, gridTrack{min: -1, max: -1})
	}
	return out
}

// measureAutoCols probes each auto-sized or content-sized column's items at an
// effectively infinite width and reports the widest outer box one of them asks
// for. Content tracks always need measurement; auto tracks are measured only
// when the template also has flexible tracks (otherwise the leftover split
// handles them).
func measureAutoCols(a *Arena, items []gridItem, cols []gridTrack) []float32 {
	frs, hasContent, hasAuto := 0, false, false
	for _, t := range cols {
		if t.fr > 0 {
			frs++
		}
		if t.content {
			hasContent = true
		}
		if t.min < 0 && !t.content {
			hasAuto = true
		}
	}
	if !hasContent && (frs == 0 || !hasAuto) {
		return nil
	}
	out := make([]float32, len(cols))
	for i := range items {
		p := &items[i]
		if p.colSpan != 1 || p.col < 0 || p.col >= len(cols) {
			continue
		}
		c := &cols[p.col]
		if c.fr > 0 || (c.min > 0 && !c.content) {
			continue
		}
		if !c.content && frs == 0 {
			continue
		}
		if w := gridItemMaxContentW(a, p.id); w > out[p.col] {
			out[p.col] = w
		}
	}
	return out
}

// gridItemMaxContentW is an item's max-content contribution to its column: the
// outer width its content takes when nothing wraps, including the box and the
// margins that ride with it. A declared width wins over the measurement, because
// an item that was told how wide to be never asks for more.
func gridItemMaxContentW(a *Arena, id ObjectID) float32 {
	it := a.Get(id)
	blockInto(a, id, maxFlexMeasureWidth)
	it = a.Get(id)
	if it.W <= 0 {
		it.W = maxFlexMeasureWidth - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
		it = a.Get(id)
	}
	contentW := float32(-1)
	if resolvePctLength(it.Style.Width, maxFlexMeasureWidth) >= 0 {
		contentW = it.W
	} else if !blockifiesChildren(it.Style) {
		clearInlineLaidOut(a, id)
		inlineInto(a, id)
		it = a.Get(id)
		if srcX, right, ok := inlineExtent(a, id); ok {
			contentW = right - srcX
		} else if w := itemMaxContentW(a, id); w > 0 {
			contentW = w
		}
	}
	if contentW < 0 {
		contentW = 0
	}
	// The probe left the subtree wrapped to the wide measure, so the inline pass
	// has to run again at the width the column actually gets.
	clearInlineLaidOut(a, id)
	mLeft, mRight := it.Style.MarginLeft, it.Style.MarginRight
	if mLeft == style.MarginAuto {
		mLeft = 0
	}
	if mRight == style.MarginAuto {
		mRight = 0
	}
	return contentW + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight + mLeft + mRight
}

// sizeGridTracks resolves track sizes against the space available: a definite
// track keeps its floor, a content track takes its items' size clamped to its
// bounds, an auto track takes the width its items ask for, and fractional
// tracks share what the definite ones left in proportion to their weights.
// An auto track grows on a stretching axis only when nothing else claimed the
// leftover; content tracks never stretch.
func sizeGridTracks(tracks []gridTrack, sizes []float32, avail, gap float32, stretch bool, content []float32) {
	n := len(tracks)
	if n == 0 || len(sizes) < n {
		return
	}
	used := float32(0)
	sumFr := float32(0)
	for i, t := range tracks {
		sizes[i] = 0
		if t.fr > 0 {
			sumFr += t.fr
			continue
		}
		if t.content {
			s := float32(0)
			if i < len(content) && content[i] > 0 {
				s = content[i]
			} else if t.min > 0 {
				s = t.min
			}
			if t.max >= 0 && s > t.max {
				s = t.max
			}
			if t.min > 0 && s < t.min {
				s = t.min
			}
			sizes[i] = s
			used += s
			continue
		}
		if t.min > 0 {
			sizes[i] = t.min
			used += t.min
		}
	}
	leftover := avail - gapsOf(gap, n) - used
	if leftover < 0 {
		leftover = 0
	}
	if sumFr > 0 {
		autoSum := float32(0)
		for i, t := range tracks {
			if t.fr <= 0 && !t.content && t.min < 0 && i < len(content) && content[i] > 0 {
				sizes[i] = content[i]
				autoSum += content[i]
			}
		}
		if room := avail - gapsOf(gap, n) - used; autoSum > room {
			if room <= 0 {
				room = 0
			}
			for i, t := range tracks {
				if t.fr <= 0 && !t.content && t.min < 0 && sizes[i] > 0 {
					sizes[i] *= room / autoSum
				}
			}
			autoSum = room
		}
		leftover = avail - gapsOf(gap, n) - used - autoSum
		if leftover < 0 {
			leftover = 0
		}
		freeze := make([]bool, n)
		for {
			weight := float32(0)
			for i, t := range tracks {
				if t.fr > 0 && !freeze[i] {
					weight += t.fr
				}
			}
			if weight <= 0 {
				break
			}
			hyp := leftover / weight
			frozen := false
			for i, t := range tracks {
				if t.fr > 0 && !freeze[i] && t.min > hyp*t.fr {
					freeze[i] = true
					sizes[i] = t.min
					leftover -= t.min
					frozen = true
				}
			}
			if !frozen {
				for i, t := range tracks {
					if t.fr > 0 && !freeze[i] {
						sizes[i] = hyp * t.fr
					}
				}
				break
			}
			if leftover < 0 {
				leftover = 0
			}
		}
		return
	}
	if stretch && leftover > 0 {
		auto := 0
		for _, t := range tracks {
			if t.fr <= 0 && !t.content && t.min < 0 {
				auto++
			}
		}
		if auto > 0 {
			for i, t := range tracks {
				if t.fr <= 0 && !t.content && t.min < 0 {
					sizes[i] = leftover / float32(auto)
				}
			}
		}
	}
}

// axisPacking turns justify-content or align-content into the leading offset and
// the per-track gap that place a fixed-size track list in its container: the space
// the tracks did not use is what the alignment has to work with.
func axisPacking(mode string, avail, gap float32, n int, sizes []float32) (lead, trackGap float32) {
	trackGap = gap
	if n == 0 || n > len(sizes) {
		return
	}
	remaining := avail - trackSize(sizes, 0, n, gap)
	if remaining <= 0 {
		return
	}
	return distributeFlex(normalizeAxis(mode), false, remaining, gap, n)
}

// normalizeAxis maps the grid keywords onto the flexbox ones the packing helper
// speaks. An empty result means normal: pack at the start, stretch what can.
func normalizeAxis(v string) string {
	switch s := strings.TrimSpace(strings.ToLower(v)); s {
	case "start", "self-start", "left":
		return "flex-start"
	case "end", "self-end", "right":
		return "flex-end"
	case "center", "flex-start", "flex-end", "space-between", "space-around", "space-evenly":
		return s
	default:
		return ""
	}
}

// trackSize is the distance a run of tracks covers, the gaps between them included.
func trackSize(sizes []float32, start, span int, gap float32) float32 {
	if start >= len(sizes) || span <= 0 {
		return 0
	}
	if start+span > len(sizes) {
		span = len(sizes) - start
	}
	total := gapsOf(gap, span)
	for i := start; i < start+span; i++ {
		total += sizes[i]
	}
	return total
}

func gapsOf(gap float32, n int) float32 {
	if n < 2 {
		return 0
	}
	return gap * float32(n-1)
}

func minIdx(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseGridAreas reads grid-template-areas into named rectangles and reports the
// row and column counts the template implies.
func parseGridAreas(v string) (rows, cols int, areas map[string]gridArea) {
	areas = map[string]gridArea{}
	if strings.TrimSpace(v) == "" {
		return 0, 0, areas
	}
	var lines [][]string
	for _, tok := range quotedStrings(v) {
		fields := strings.Fields(tok)
		if len(fields) == 0 {
			continue
		}
		lines = append(lines, fields)
		if len(fields) > cols {
			cols = len(fields)
		}
	}
	rows = len(lines)
	for r, fields := range lines {
		for c, name := range fields {
			if name == "." {
				continue
			}
			ar, ok := areas[name]
			if !ok {
				areas[name] = gridArea{col: c, row: r, colSpan: 1, rowSpan: 1}
				continue
			}
			if c < ar.col {
				ar.colSpan += ar.col - c
				ar.col = c
			} else if end := c + 1; end > ar.col+ar.colSpan {
				ar.colSpan = end - ar.col
			}
			if r < ar.row {
				ar.rowSpan += ar.row - r
				ar.row = r
			} else if end := r + 1; end > ar.row+ar.rowSpan {
				ar.rowSpan = end - ar.row
			}
			areas[name] = ar
		}
	}
	return
}

func quotedStrings(v string) []string {
	var out []string
	for i := 0; i < len(v); i++ {
		if v[i] != '"' && v[i] != '\'' {
			continue
		}
		q := v[i]
		j := i + 1
		for j < len(v) && v[j] != q {
			j++
		}
		if j >= len(v) {
			break
		}
		out = append(out, v[i+1:j])
		i = j
	}
	return out
}
