package layout

import (
	"strconv"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

type tableCell struct {
	id               ObjectID
	row, col         int
	rowSpan, colSpan int
	minW, maxW       float32
	usedH            float32
}

type tableRow struct {
	id    ObjectID
	cells []ObjectID
}

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
		obj = a.Get(id)
		obj.W = 0
		obj.H = captionH
		return obj.H
	}

	gaps := spacingH * float32(colCount+1)
	available := contentW - gaps
	if s.BorderCollapse {
		available = contentW
	}
	colMin, colMax := columnExtents(a, cells, colCount, spacingH, s.BorderCollapse, containingW)
	colMin, colMax, colCount = floorColWidths(a, id, colMin, colMax, colCount, containingW)
	if resolvePctLength(s.Width, containingW) < 0 {
		natural := sumOf(colMax)
		if natural > available {
			natural = available
		}
		available = natural
		if min := sumOf(colMin); min > available {
			available = min
		}
	} else if min := sumOf(colMin); min > available {
		available = min
	}
	if available < 0 {
		available = 0
	}
	colW := distributeColumns(colMin, colMax, available)
	if s != nil {
		leftAuto := s.MarginLeftAuto
		rightAuto := s.MarginRightAuto
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
				cellW += cell.Style.UsedBorderWidths()[1]
			}
			resolveBoxSizes(a, cell, cellW, definiteH(a, cell.Parent))
			cell.W = cellW - (cell.BorderLeft + cell.PaddingLeft + cell.PaddingRight + cell.BorderRight)
			if cell.W < 0 {
				cell.W = 0
			}
			blockIntoSized(a, cid, cellW)
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
			row.W = available + gaps
			if !s.BorderCollapse {
				row.X += spacingH
				row.W -= 2 * spacingH
				if row.W < 0 {
					row.W = 0
				}
			}
			row.Y = rowY
			row.H = rowH
		}
		cursor = rowY + rowH + spacingV
	}

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

	obj = a.Get(id)
	obj.W = available + gaps
	if s.BorderCollapse {
		obj.BorderRight = outerBorder(a, id, cells, colCount)
		obj.BorderBottom = lastRowBorder(a, id, cells, len(rows))
		obj.W -= obj.BorderRight
		if obj.W < 0 {
			obj.W = 0
		}
	}
	obj.H = height
	return height
}

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
	for kid := a.Get(id).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isRow(k) {
			flushRow()
			continue
		}
		run = append(run, kid)
	}
	flushRow()
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

func isTableStructure(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	switch s.Display {
	case style.DisplayTable, style.DisplayInlineTable, style.DisplayTableRow, style.DisplayTableCell,
		style.DisplayTableRowGroup, style.DisplayTableHeaderGroup,
		style.DisplayTableFooterGroup, style.DisplayTableColumn,
		style.DisplayTableColumnGroup, style.DisplayTableCaption:
		return true
	}
	return false
}

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

func cellByRef(cells []tableCell, id ObjectID) *tableCell {
	for i := range cells {
		if cells[i].id == id {
			return &cells[i]
		}
	}
	return nil
}

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
	if w := cellSpecifiedWidth(a, s, containingW); w > maxW {
		maxW = w
	}
	return minW, maxW
}

func cellSpecifiedWidth(a *Arena, s *style.ComputedStyle, containingW float32) float32 {
	if s.WidthCalc.Set {
		if v, ok := s.WidthCalc.Resolve(containingW, a.ViewportW, a.ViewportH); ok && v >= 0 {
			return v
		}
		return -1
	}
	return resolvePctLength(s.Width, containingW)
}

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

func measureInlineContent(a *Arena, id ObjectID) (minW, maxW float32) {
	cur := float32(0)
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
				flush()
				if subMax+chrome > maxW {
					maxW = subMax + chrome
				}
				if subMin+chrome > minW {
					minW = subMin + chrome
				}
				continue
			}
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
