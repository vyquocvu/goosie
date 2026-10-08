package layout

import (
	"strconv"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

// tableCell is one cell placed in the table grid, with the column extents its
// content asked for and the height that content took before the cell was
// stretched over its rows.
type tableCell struct {
	id               ObjectID
	row, col         int
	rowSpan, colSpan int
	minW, maxW       float32
	usedH            float32
}

// tableRow is a row as the grid sees it: the `<tr>` and the cells starting in it.
type tableRow struct {
	id    ObjectID
	cells []ObjectID
}

// layoutTable lays out a table with the auto column algorithm.
//
// Columns start at the max-content width of their cells and are then stretched
// or squeezed to the table's used width. Both border models are supported: a
// separated table spaces its cells apart and each keeps four edges, while a
// collapsing table has one line between neighbours, which it gets by letting
// every cell draw only its left and top edge and handing the table the right and
// bottom edges of its outermost cells.
func layoutTable(a *Arena, id ObjectID, containingW float32) float32 {
	obj := a.Get(id)
	s := obj.Style
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	spacingH, spacingV := s.BorderSpacingH, s.BorderSpacingV
	if s.BorderCollapse {
		spacingH, spacingV = 0, 0
	}

	// The caption is a block of the table's own width above the grid.
	captionH := float32(0)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display != style.DisplayTableCaption {
			continue
		}
		k.X = contentX
		k.Y = contentY + captionH
		blockInto(a, kid, contentW)
		inlineInto(a, kid)
		captionH += a.Get(kid).BorderH()
	}

	var rows []tableRow
	wrapAnonymousTableBoxes(a, id)
	collectRows(a, id, &rows)
	colCount := 0
	cells := buildTableGrid(a, rows, &colCount)
	if colCount == 0 {
		// A table without columns has no content box, but it still paints
		// its padding and borders: W/H stay content-sized (zero plus any
		// caption) so BorderRect adds the padding exactly once. Zeroing
		// here also repairs whatever containing-width placeholder the box
		// sizing pass left behind.
		obj = a.Get(id)
		obj.W = 0
		obj.H = captionH
		return obj.H
	}

	// The space the columns share: everything inside the table's content box
	// that the gaps between and around the cells leave over.
	gaps := spacingH * float32(colCount+1)
	available := contentW - gaps
	if s.BorderCollapse {
		available = contentW
	}
	colMin, colMax := columnExtents(a, cells, colCount, spacingH, s.BorderCollapse, containingW)
	colMin, colMax, colCount = floorColWidths(a, id, colMin, colMax, colCount, containingW)
	if resolvePctLength(s.Width, containingW) < 0 {
		// An auto width shrinks to the content, capped by what the container
		// allows, which is why a narrow table does not fill the line. Column
		// definitions still floor it: <col width=50> holds its column open
		// even when every cell is empty.
		natural := sumOf(colMax)
		if natural > available {
			natural = available
		}
		available = natural
		if min := sumOf(colMin); min > available {
			available = min
		}
	} else if min := sumOf(colMin); min > available {
		// A specified width is only a wish: the used width is the greater of it
		// and the table's minimum content width, so a table narrower than the
		// cells it cannot shrink still makes room for them.
		available = min
	}
	if available < 0 {
		available = 0
	}
	colW := distributeColumns(colMin, colMax, available)
	// Auto margins centre a shrink-to-fit table the way they centre any
	// fixed-width block, but the width only exists now that the columns
	// are dealt: shift the box before placing a single child, so the
	// columns below land inside the centred box instead of the
	// pre-centering one. The border box is the dealt content plus gaps,
	// padding and borders; obj.W itself is only finalized below.
	if s != nil {
		leftAuto := s.MarginLeft == style.MarginAuto
		rightAuto := s.MarginRight == style.MarginAuto
		if leftAuto || rightAuto {
			boxW := available + gaps + obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight
			remaining := containingW - boxW
			if remaining < 0 {
				remaining = 0
			}
			if leftAuto && rightAuto {
				obj.MarginLeft, obj.MarginRight = remaining/2, remaining/2
			} else if leftAuto {
				obj.MarginLeft = remaining
			} else {
				obj.MarginRight = remaining
			}
			obj = a.Get(id)
			obj.X += obj.MarginLeft
			contentX = obj.X + obj.BorderLeft + obj.PaddingLeft
		}
	}
	colX := make([]float32, colCount)
	x := contentX
	if !s.BorderCollapse {
		x += spacingH
	}
	for i := range colX {
		colX[i] = x
		x += colW[i] + spacingH
	}

	// Rows stack top to bottom. A cell spanning rows reserves an equal share of
	// its height in each row it covers, so a tall cell in the first of several
	// rows keeps the later rows from riding over it.
	type rowBox struct{ y, h float32 }
	placed := make([]rowBox, len(rows))
	reserved := make([]float32, len(rows))
	cursor := contentY + captionH
	if !s.BorderCollapse {
		cursor += spacingV
	}
	for ri := range rows {
		rowY := cursor
		rowH := reserved[ri]
		// A row's own height is a floor on the row rather than a fixed size, and
		// it holds even when the row brings no cell to prop it up: that is how a
		// list separates its items with an empty spacer row.
		if rs := a.Get(rows[ri].id).Style; rs != nil && rs.Height >= 0 && rs.Height > rowH {
			rowH = rs.Height
		}
		for _, cid := range rows[ri].cells {
			c := cellByRef(cells, cid)
			if c == nil || c.col >= colCount {
				continue
			}
			cell := a.Get(cid)
			cell.X = colX[c.col]
			cell.Y = rowY
			cellW := colW[c.col]
			for col := c.col + 1; col < c.col+c.colSpan && col < colCount; col++ {
				cellW += colW[col] + spacingH
			}
			if s.BorderCollapse {
				// The column width counts the cell's own left edge but not its
				// right one, which the cell beside it draws. Hand the extra back
				// to the content box, which is where the collapsed model puts it.
				cellW += cell.Style.UsedBorderWidths()[1]
			}
			blockInto(a, cid, cellW)
			inlineInto(a, cid)
			cell = a.Get(cid)
			c.usedH = cell.H
			if s.BorderCollapse {
				cell.BorderRight = 0
				cell.BorderBottom = 0
			}
			h := cell.MarginTop + cell.BorderH() + cell.MarginBottom
			if h > rowH {
				rowH = h
			}
			if c.rowSpan > 1 {
				share := h / float32(c.rowSpan)
				for r := c.row; r < c.row+c.rowSpan && r < len(reserved); r++ {
					if share > reserved[r] {
						reserved[r] = share
					}
				}
			}
		}
		placed[ri] = rowBox{rowY, rowH}
		if row := a.Get(rows[ri].id); row.Style != nil {
			row.X = contentX
			row.Y = rowY
			row.W = available + gaps
			row.H = rowH
		}
		cursor = rowY + rowH + spacingV
	}

	// Every cell fills the rows it covers: a single-row one reaches its row's
	// height, and a spanning one reaches the whole span. That is what makes a
	// spanning cell's borders and background run the full height.
	for i := range cells {
		c := &cells[i]
		last := c.row + c.rowSpan
		if last > len(rows) {
			last = len(rows)
		}
		if last <= c.row {
			continue
		}
		box := placed[last-1].y + placed[last-1].h - placed[c.row].y
		if !s.BorderCollapse {
			box += spacingV * float32(last-c.row-1)
		}
		cell := a.Get(c.id)
		inner := box - cell.MarginTop - cell.MarginBottom - cell.PaddingTop - cell.PaddingBottom -
			cell.BorderTop - cell.BorderBottom
		if inner <= cell.H {
			continue
		}
		grow := inner - cell.H
		cell.H = inner
		if centerCellContent(cell.Style) && c.usedH > 0 {
			shiftInlineContent(a, c.id, 0, grow/2)
		}
	}

	height := cursor - contentY
	if !s.BorderCollapse {
		height += spacingV
	}

	obj = a.Get(id)
	obj.W = available + gaps
	if s.BorderCollapse {
		obj.BorderRight = outerBorder(a, id, cells, colCount)
		obj.BorderBottom = lastRowBorder(a, id, cells, len(rows))
		// The two lines the cells do not draw sit inside the table's box, so the
		// grid and the border box end on the same pixel.
		obj.W -= obj.BorderRight
		if obj.W < 0 {
			obj.W = 0
		}
	}
	obj.H = height
	return height
}

// centerCellContent reports whether a cell's content sits in the middle of its
// box. A cell's vertical-align defaults to baseline, and the table model reads
// that as middle, which is what centres a spanning cell's text in the rows it
// covers.
func centerCellContent(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	switch s.VerticalAlign {
	case "", "middle", "baseline", "auto":
		return true
	}
	return false
}

// collectRows gathers a table's rows, descending through row groups and through
// any anonymous box block flow wrapped stray content in.
// wrapAnonymousTableBoxes generates the missing table structure CSS
// requires: runs of non-row children (stray blocks, text, a bare body)
// become anonymous rows, and runs of non-cell children inside every row
// become anonymous cells. Whitespace-only text and display:none boxes hold
// their slots but join no run. Without this a table whose content skips
// the row/cell levels collects no grid at all and collapses to zero.
func wrapAnonymousTableBoxes(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.Style == nil {
		return
	}
	isRow := func(k *Object) bool {
		if k.Style == nil {
			return false
		}
		switch k.Style.Display {
		case style.DisplayTableRow, style.DisplayTableRowGroup,
			style.DisplayTableHeaderGroup, style.DisplayTableFooterGroup,
			style.DisplayTableCaption, style.DisplayTableColumn,
			style.DisplayTableColumnGroup:
			return true
		}
		return false
	}
	var run []ObjectID
	flushRow := func() {
		if len(run) > 0 && tableRunHasContent(a, run) {
			tbl := a.Get(id)
			spliceAnonymousBlock(a, id, run, style.AnonymousTableStyle(tbl.Style, style.DisplayTableRow))
		}
		run = nil
	}
	// Splicing reallocates the arena, so every step re-fetches through the
	// stable kid IDs instead of holding object pointers across a flush.
	for kid := a.Get(id).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isRow(k) {
			flushRow()
			continue
		}
		run = append(run, kid)
	}
	flushRow()
	// Cells inside every row, real or just synthesized: a row's own style
	// parents the anonymous cells the way the cascade would.
	for kid := a.Get(id).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display != style.DisplayTableRow {
			continue
		}
		var cells []ObjectID
		flushCell := func() {
			if len(cells) > 0 && tableRunHasContent(a, cells) {
				r := a.Get(kid)
				spliceAnonymousBlock(a, kid, cells, style.AnonymousTableStyle(r.Style, style.DisplayTableCell))
			}
			cells = nil
		}
		for c := a.Get(kid).FirstKid; c != 0; c = a.Get(c).NextSibling {
			cc := a.Get(c)
			if cc.Style == nil || cc.Style.Display == style.DisplayNone || cc.Style.Display == style.DisplayTableCell {
				flushCell()
				continue
			}
			cells = append(cells, c)
		}
		flushCell()
	}
}

// isTableStructure reports whether display participates in table layout:
// rows, cells, sections, columns and captions all take their anonymous
// boxes from the table algorithm, never from the generic inline wrapper.
func isTableStructure(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	switch s.Display {
	case style.DisplayTable, style.DisplayTableRow, style.DisplayTableCell,
		style.DisplayTableRowGroup, style.DisplayTableHeaderGroup,
		style.DisplayTableFooterGroup, style.DisplayTableColumn,
		style.DisplayTableColumnGroup, style.DisplayTableCaption:
		return true
	}
	return false
}

// tableRunHasContent is runGeneratesContent plus anonymous wrappers: the
// generic inline pass may already have boxed stray table content (a block
// holding cols and text), and that wrapper counts as content even though it
// carries no DOM node of its own. Pure whitespace still joins no run.
func tableRunHasContent(a *Arena, kids []ObjectID) bool {
	for _, kid := range kids {
		k := a.Get(kid)
		if k.Node == nil {
			return true
		}
	}
	return runGeneratesContent(a, kids)
}

func collectRows(a *Arena, id ObjectID, rows *[]tableRow) {
	for kid := a.Get(id).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		switch k.Style.Display {
		case style.DisplayTableRow:
			var cells []ObjectID
			for c := k.FirstKid; c != 0; c = a.Get(c).NextSibling {
				if cs := a.Get(c).Style; cs != nil && cs.Display == style.DisplayTableCell {
					cells = append(cells, c)
				}
			}
			*rows = append(*rows, tableRow{id: kid, cells: cells})
		case style.DisplayTableRowGroup, style.DisplayTableHeaderGroup, style.DisplayTableFooterGroup:
			collectRows(a, kid, rows)
		default:
			if k.flags&flagAnonymous != 0 {
				collectRows(a, kid, rows)
			}
		}
	}
}

// buildTableGrid places every cell in the grid, honouring the spans its
// attributes declare, and reports how many columns the table ended up with.
func buildTableGrid(a *Arena, rows []tableRow, colCount *int) []tableCell {
	var cells []tableCell
	occupied := map[[2]int]bool{}
	for ri, row := range rows {
		col := 0
		for _, cid := range row.cells {
			for occupied[[2]int{ri, col}] {
				col++
			}
			c := tableCell{id: cid, row: ri, col: col, rowSpan: tableSpan(a, cid, "rowspan"), colSpan: tableSpan(a, cid, "colspan")}
			for r := ri; r < ri+c.rowSpan; r++ {
				for cc := col; cc < col+c.colSpan; cc++ {
					occupied[[2]int{r, cc}] = true
					if cc+1 > *colCount {
						*colCount = cc + 1
					}
				}
			}
			cells = append(cells, c)
			col += c.colSpan
		}
	}
	return cells
}

// tableSpan reads a colspan or rowspan attribute, which is always at least one.
func tableSpan(a *Arena, id ObjectID, name string) int {
	obj := a.Get(id)
	if obj.Node == nil {
		return 1
	}
	v, err := strconv.Atoi(obj.Node.GetAttribute(name))
	if err != nil || v < 1 {
		return 1
	}
	return v
}

// cellByRef finds the grid entry for a cell object.
func cellByRef(cells []tableCell, id ObjectID) *tableCell {
	for i := range cells {
		if cells[i].id == id {
			return &cells[i]
		}
	}
	return nil
}

// columnExtents reports the narrowest and widest each column can go. A spanning
// cell adds its own share to the columns it covers, since those together have to
// hold it.
// floorColWidths folds <col> definitions into the column extents: a column
// definition floors its column's minimum, so empty fixed-width columns hold
// the grid open, and definitions past the cells' grid extend it, so a
// colspan never outgrows the grid its COLs define. Cols nest directly under
// the table or inside colgroups; span multiplies one definition across that
// many columns.
func floorColWidths(a *Arena, id ObjectID, colMin, colMax []float32, colCount int, containingW float32) ([]float32, []float32, int) {
	var defs []float32
	var walk func(pid ObjectID)
	walk = func(pid ObjectID) {
		for kid := a.Get(pid).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil {
				continue
			}
			switch k.Style.Display {
			case style.DisplayTableColumn:
				w := cellSpecifiedWidth(a, k.Style, containingW)
				span := tableSpan(a, kid, "span")
				for i := 0; i < span; i++ {
					defs = append(defs, w)
				}
			case style.DisplayTableColumnGroup:
				walk(kid)
			}
		}
	}
	walk(id)
	for len(colMin) < len(defs) {
		colMin = append(colMin, 0)
		colMax = append(colMax, 0)
		colCount = len(colMin)
	}
	for i := range defs {
		if i >= len(colMin) || defs[i] < 0 {
			continue
		}
		// A definition floors the minimum only: wider content still
		// stretches the column past it.
		if defs[i] > colMin[i] {
			colMin[i] = defs[i]
		}
	}
	return colMin, colMax, colCount
}

func columnExtents(a *Arena, cells []tableCell, colCount int, spacingH float32, collapse bool, containingW float32) ([]float32, []float32) {
	colMin := make([]float32, colCount)
	colMax := make([]float32, colCount)
	for i := range cells {
		c := &cells[i]
		c.minW, c.maxW = cellExtents(a, c.id, spacingH, collapse, containingW)
		if c.colSpan == 1 {
			if c.col < colCount {
				if c.minW > colMin[c.col] {
					colMin[c.col] = c.minW
				}
				if c.maxW > colMax[c.col] {
					colMax[c.col] = c.maxW
				}
			}
			continue
		}
		end := minF(float32(c.col+c.colSpan), float32(colCount))
		span := int(end) - c.col
		if span < 1 {
			continue
		}
		spread := func(need float32, widths []float32) {
			sum := float32(0)
			for col := c.col; col < int(end); col++ {
				sum += widths[col]
			}
			if need <= sum {
				return
			}
			share := (need - sum) / float32(span)
			for col := c.col; col < int(end); col++ {
				widths[col] += share
			}
		}
		spread(c.minW, colMin)
		spread(c.maxW, colMax)
	}
	return colMin, colMax
}

// distributeColumns shares the width a table has to give its columns: at the
// max-content widths when there is room to spare, otherwise squeezed toward the
// min-content widths they cannot go below.
func distributeColumns(colMin, colMax []float32, available float32) []float32 {
	widths := make([]float32, len(colMin))
	totalMax, totalMin := sumOf(colMax), sumOf(colMin)
	switch {
	case len(colMin) == 0:
	case totalMax <= available:
		excess := available - totalMax
		for i := range widths {
			widths[i] = colMax[i]
			if totalMax > 0 {
				widths[i] += excess * colMax[i] / totalMax
			} else {
				widths[i] += excess / float32(len(colMin))
			}
		}
	case totalMin >= available:
		for i := range widths {
			if totalMin > 0 {
				widths[i] = available * colMin[i] / totalMin
			}
		}
	default:
		room, wanted := available-totalMin, totalMax-totalMin
		for i := range widths {
			widths[i] = colMin[i]
			if wanted > 0 {
				widths[i] += room * (colMax[i] - colMin[i]) / wanted
			}
		}
	}
	for i := range widths {
		if widths[i] < 0 {
			widths[i] = 0
		}
	}
	return widths
}

func sumOf(v []float32) float32 {
	total := float32(0)
	for _, x := range v {
		total += x
	}
	return total
}

// cellExtents measures one cell's content, including the padding and the borders
// it has to keep room for. A collapsing cell owns only its left edge, so its
// right border belongs to the next column; a separated one also reserves the gap
// after it.
func cellExtents(a *Arena, id ObjectID, spacingH float32, collapse bool, containingW float32) (float32, float32) {
	obj := a.Get(id)
	minW, maxW := measureInlineContent(a, id)
	if obj.Style == nil {
		return minW, maxW
	}
	s := obj.Style
	ubw := s.UsedBorderWidths()
	chrome := resolvePctLength(s.PaddingLeft, obj.W) + resolvePctLength(s.PaddingRight, obj.W) +
		ubw[3] + ubw[1]
	if collapse {
		chrome = resolvePctLength(s.PaddingLeft, obj.W) + resolvePctLength(s.PaddingRight, obj.W) + ubw[3]
	} else {
		chrome += spacingH
	}
	if minW > 0 {
		minW += chrome
	}
	if maxW > 0 {
		maxW += chrome
	}
	// A specified cell width floors the column's max-content width: an empty
	// fixed-width cell still holds its column open (CSS 2.1 §17.5.2.2 treats
	// the used width as a minimum over the content measure). Percentage and
	// calc widths resolve against the table's containing block; an
	// unresolvable one contributes nothing rather than collapsing the column.
	if w := cellSpecifiedWidth(a, s, containingW); w > maxW {
		maxW = w
	}
	return minW, maxW
}

// cellSpecifiedWidth reports the cell's declared width as a definite length,
// or -1 when it is auto or unresolvable here.
func cellSpecifiedWidth(a *Arena, s *style.ComputedStyle, containingW float32) float32 {
	if s.WidthCalc.Set {
		if v, ok := s.WidthCalc.Resolve(containingW, a.ViewportW, a.ViewportH); ok && v >= 0 {
			return v
		}
		return -1
	}
	return resolvePctLength(s.Width, containingW)
}

// outerBorder is the line a collapsing table draws on its right edge: the widest
// right border among the table itself and the cells that end at that edge.
func outerBorder(a *Arena, id ObjectID, cells []tableCell, colCount int) float32 {
	best := float32(0)
	if s := a.Get(id).Style; s != nil {
		best = s.UsedBorderWidths()[1]
	}
	for i := range cells {
		c := &cells[i]
		if c.col+c.colSpan != colCount || c.id == 0 {
			continue
		}
		obj := a.Get(c.id)
		if obj.Style != nil && obj.Style.UsedBorderWidths()[1] > best {
			best = obj.Style.UsedBorderWidths()[1]
		}
	}
	return best
}

// lastRowBorder is the bottom edge a collapsing table draws, taken from the cells
// that reach its last row.
func lastRowBorder(a *Arena, id ObjectID, cells []tableCell, rowCount int) float32 {
	best := float32(0)
	if s := a.Get(id).Style; s != nil {
		best = s.UsedBorderWidths()[2]
	}
	for i := range cells {
		c := &cells[i]
		if c.row+c.rowSpan != rowCount || c.id == 0 {
			continue
		}
		obj := a.Get(c.id)
		if obj.Style != nil && obj.Style.UsedBorderWidths()[2] > best {
			best = obj.Style.UsedBorderWidths()[2]
		}
	}
	return best
}

// measureInlineContent reports the narrowest and widest a box's content can get:
// its longest word, and its whole run on a single line.
//
// The column algorithm needs both before any cell has a position, so this reads
// the text's own advances rather than laying it out and measuring the result.
func measureInlineContent(a *Arena, id ObjectID) (minW, maxW float32) {
	cur := float32(0)
	// A collapsible space joins the line only when content follows it:
	// hanging end-of-line spaces are removed, so pending holds the
	// advance until the next word (or inline-block) commits it, and a
	// flush drops it.
	pending := float32(0)
	flush := func() {
		if cur > maxW {
			maxW = cur
		}
		cur = 0
		pending = 0
	}
	for kid := a.Get(id).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Type == dom.NodeText {
			text := k.Node.DataContent
			if k.Style.WhiteSpace == style.WhiteSpacePreline {
				text = collapseWhitespacePreserveNewlines(text)
			} else if shouldCollapseWhitespace(k) {
				text = collapseWhitespace(text)
			}
			for _, word := range splitWords(text) {
				if word == "\n" {
					flush()
					continue
				}
				w := measureWord(word, k.Style.FontSize, a.Metrics, k.Style.FontSlot(), k.Style.LetterSpacing)
				if word == " " {
					w += k.Style.WordSpacing
				}
				if word != " " && w > minW {
					minW = w
				}
				if word == " " {
					if cur > 0 {
						pending += w
					}
					continue
				}
				cur += pending + w
				pending = 0
			}
			continue
		}
		subMin, subMax := measureInlineContent(a, kid)
		if isBlock(k) || k.Style.Display == style.DisplayInlineBlock {
			chrome := k.Style.PaddingLeft + k.Style.PaddingRight + k.Style.BorderLeftWidth + k.Style.BorderRightWidth +
				flexMargin(k.Style.MarginLeft) + flexMargin(k.Style.MarginRight)
			// A box with a declared width brings that width to the column, whether
			// or not it holds any text: the vote arrow on Hacker News is an empty
			// 10px div with 8px of margin, and measuring only its absent words
			// collapsed the column and slid every title left. A fixed width also
			// floors the column, since a box that cannot shrink drags the table
			// wider with it.
			if own := declaredOuterW(k.Style); own >= 0 {
				if own > subMax {
					subMax = own
				}
				if own > subMin {
					subMin = own
				}
			}
			if floor := flexMargin(k.Style.MinWidth); floor > subMin {
				subMin = floor
			}
			if isBlock(k) && k.Style.Display != style.DisplayInlineBlock {
				// A true block starts its own line: the column fits the
				// widest one, not their sum.
				flush()
				if subMax+chrome > maxW {
					maxW = subMax + chrome
				}
				if subMin+chrome > minW {
					minW = subMin + chrome
				}
				continue
			}
			// An inline-block shares its line with whatever follows, so it
			// accumulates into the running line like a long word: two
			// siblings on one line measure their sum, not their max. It
			// also commits any pending space before it.
			if subMin+chrome > minW {
				minW = subMin + chrome
			}
			cur += pending + subMax + chrome
			pending = 0
			continue
		}
		if subMin > minW {
			minW = subMin
		}
		cur += subMax
	}
	flush()
	return minW, maxW
}

// declaredOuterW is the content width a box states for itself - an explicit
// width held between its own min- and max-width - or -1 when its width is auto.
// Percentages are absent on purpose: an intrinsic measure runs before the
// column has a width to resolve them against.
func declaredOuterW(s *style.ComputedStyle) float32 {
	if s == nil {
		return -1
	}
	w := s.Width
	if s.MaxWidth >= 0 && w > s.MaxWidth {
		w = s.MaxWidth
	}
	if s.MinWidth > w {
		w = s.MinWidth
	}
	if w < 0 {
		return -1
	}
	if s.BoxSizing == style.BoxSizingBorderBox {
		w -= s.PaddingLeft + s.PaddingRight + s.BorderLeftWidth + s.BorderRightWidth
		if w < 0 {
			w = 0
		}
	}
	return w
}
