package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) installTableInterface(p *goja.Object) {
	for _, property := range []string{"caption", "tHead", "tFoot"} {
		property := property
		getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Null()
			}
			tag := map[string]string{"caption": "caption", "tHead": "thead", "tFoot": "tfoot"}[property]
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() && child.Data == tag {
					return r.wrapNode(child)
				}
			}
			return goja.Null()
		})
		setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			table := r.getNode(call.This)
			if table == nil || len(call.Arguments) == 0 {
				return goja.Undefined()
			}
			tag := map[string]string{"caption": "caption", "tHead": "thead", "tFoot": "tfoot"}[property]
			old := firstTableChild(table, tag)
			var replacement *dom.Node
			if !goja.IsNull(call.Argument(0)) && !goja.IsUndefined(call.Argument(0)) {
				replacement = r.getNode(call.Argument(0))
				if replacement == nil || !replacement.Element() || replacement.Data != tag {
					panic(r.vm.NewTypeError("The assigned value is not the matching table element"))
				}
			}
			if old == replacement {
				return goja.Undefined()
			}
			if old != nil {
				r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(table), Arguments: []goja.Value{r.wrapNode(old)}})
			}
			if replacement == nil {
				return goja.Undefined()
			}
			var before *dom.Node
			switch property {
			case "caption":
				before = table.FirstChild
			case "tHead":
				for child := table.FirstChild; child != nil; child = child.NextSibling {
					if child.Element() && (child.Data == "tbody" || child.Data == "tfoot") {
						before = child
						break
					}
				}
			}
			if before == nil {
				r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(table), Arguments: []goja.Value{r.wrapNode(replacement)}})
			} else {
				r.jsInsertBefore(goja.FunctionCall{This: r.wrapNode(table), Arguments: []goja.Value{r.wrapNode(replacement), r.wrapNode(before)}})
			}
			return goja.Undefined()
		})
		_ = p.DefineAccessorProperty(property, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = p.DefineAccessorProperty("tBodies", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
			return directTableChildren(node, "tbody")
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = p.DefineAccessorProperty("rows", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
			return tableRows(node)
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = p.Set("createCaption", func(call goja.FunctionCall) goja.Value {
		table := r.getNode(call.This)
		if table == nil {
			return goja.Null()
		}
		for child := table.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && child.Data == "caption" {
				return r.wrapNode(child)
			}
		}
		return r.insertTableChild(table, "caption", table.FirstChild)
	})
	_ = p.Set("deleteCaption", func(call goja.FunctionCall) goja.Value {
		table := r.getNode(call.This)
		if table != nil {
			if caption := firstTableChild(table, "caption"); caption != nil {
				r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(table), Arguments: []goja.Value{r.wrapNode(caption)}})
			}
		}
		return goja.Undefined()
	})
	_ = p.Set("createTHead", func(call goja.FunctionCall) goja.Value {
		table := r.getNode(call.This)
		if table == nil {
			return goja.Null()
		}
		if head := firstTableChild(table, "thead"); head != nil {
			return r.wrapNode(head)
		}
		var before *dom.Node
		for child := table.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && (child.Data == "tbody" || child.Data == "tfoot") {
				before = child
				break
			}
		}
		return r.insertTableChild(table, "thead", before)
	})
	_ = p.Set("deleteTHead", func(call goja.FunctionCall) goja.Value {
		return r.deleteFirstTableChild(call, "thead")
	})
	_ = p.Set("createTFoot", func(call goja.FunctionCall) goja.Value {
		table := r.getNode(call.This)
		if table == nil {
			return goja.Null()
		}
		if foot := firstTableChild(table, "tfoot"); foot != nil {
			return r.wrapNode(foot)
		}
		return r.insertTableChild(table, "tfoot", nil)
	})
	_ = p.Set("deleteTFoot", func(call goja.FunctionCall) goja.Value {
		return r.deleteFirstTableChild(call, "tfoot")
	})
	_ = p.Set("deleteRow", func(call goja.FunctionCall) goja.Value {
		table := r.getNode(call.This)
		if table == nil {
			return goja.Undefined()
		}
		rows := tableRows(table)
		index := tableRowIndex(call, len(rows))
		if index == -1 {
			return goja.Undefined()
		}
		if index < 0 || index >= len(rows) {
			panic(r.domError("IndexSizeError", "The row index is out of range"))
		}
		r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(rows[index].Parent), Arguments: []goja.Value{r.wrapNode(rows[index])}})
		return goja.Undefined()
	})
}

func (r *Runtime) installTableSectionInterface(p *goja.Object) {
	_ = p.DefineAccessorProperty("rows", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
			return directTableChildren(node, "tr")
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = p.Set("insertRow", func(call goja.FunctionCall) goja.Value {
		section := r.getNode(call.This)
		if section == nil {
			return goja.Null()
		}
		rows := directTableChildren(section, "tr")
		index := tableRowInsertIndex(call, len(rows))
		if index < 0 || index > len(rows) {
			panic(r.domError("IndexSizeError", "The row index is out of range"))
		}
		var before *dom.Node
		if index < len(rows) {
			before = rows[index]
		}
		return r.insertTableChild(section, "tr", before)
	})
	_ = p.Set("deleteRow", func(call goja.FunctionCall) goja.Value {
		section := r.getNode(call.This)
		if section == nil {
			return goja.Undefined()
		}
		rows := directTableChildren(section, "tr")
		index := tableRowIndex(call, len(rows))
		if index == -1 {
			return goja.Undefined()
		}
		if index < 0 || index >= len(rows) {
			panic(r.domError("IndexSizeError", "The row index is out of range"))
		}
		r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(section), Arguments: []goja.Value{r.wrapNode(rows[index])}})
		return goja.Undefined()
	})
}

func (r *Runtime) installTableRowInterface(p *goja.Object) {
	_ = p.DefineAccessorProperty("cells", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
			return directTableChildren(node, "td", "th")
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = p.Set("insertCell", func(call goja.FunctionCall) goja.Value {
		row := r.getNode(call.This)
		if row == nil {
			return goja.Null()
		}
		cells := directTableChildren(row, "td", "th")
		index := tableRowInsertIndex(call, len(cells))
		if index < 0 || index > len(cells) {
			panic(r.domError("IndexSizeError", "The cell index is out of range"))
		}
		var before *dom.Node
		if index < len(cells) {
			before = cells[index]
		}
		return r.insertTableChild(row, "td", before)
	})
	_ = p.Set("deleteCell", func(call goja.FunctionCall) goja.Value {
		row := r.getNode(call.This)
		if row == nil {
			return goja.Undefined()
		}
		cells := directTableChildren(row, "td", "th")
		index := tableRowIndex(call, len(cells))
		if index == -1 {
			return goja.Undefined()
		}
		if index < 0 || index >= len(cells) {
			panic(r.domError("IndexSizeError", "The cell index is out of range"))
		}
		r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(row), Arguments: []goja.Value{r.wrapNode(cells[index])}})
		return goja.Undefined()
	})
}

func firstTableChild(parent *dom.Node, tag string) *dom.Node {
	for child := parent.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && child.Data == tag {
			return child
		}
	}
	return nil
}

func (r *Runtime) insertTableChild(parent *dom.Node, tag string, before *dom.Node) goja.Value {
	value := r.jsCreateElementInDocument(goja.FunctionCall{Arguments: []goja.Value{r.vm.ToValue(tag)}}, parent.Doc, r.customElementRegistryForDocument(parent.Doc))
	if before == nil {
		return r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(parent), Arguments: []goja.Value{value}})
	}
	return r.jsInsertBefore(goja.FunctionCall{This: r.wrapNode(parent), Arguments: []goja.Value{value, r.wrapNode(before)}})
}

func (r *Runtime) deleteFirstTableChild(call goja.FunctionCall, tag string) goja.Value {
	parent := r.getNode(call.This)
	if parent != nil {
		if child := firstTableChild(parent, tag); child != nil {
			r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(parent), Arguments: []goja.Value{r.wrapNode(child)}})
		}
	}
	return goja.Undefined()
}

func tableRowIndex(call goja.FunctionCall, length int) int {
	index := -1
	if len(call.Arguments) != 0 {
		index = int(call.Argument(0).ToInteger())
	}
	if index == -1 {
		return length - 1
	}
	return index
}

func tableRowInsertIndex(call goja.FunctionCall, length int) int {
	index := -1
	if len(call.Arguments) != 0 {
		index = int(call.Argument(0).ToInteger())
	}
	if index == -1 {
		return length
	}
	return index
}

func directTableChildren(node *dom.Node, tags ...string) []*dom.Node {
	if node == nil {
		return nil
	}
	var result []*dom.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if !child.Element() {
			continue
		}
		for _, tag := range tags {
			if child.Data == tag {
				result = append(result, child)
				break
			}
		}
	}
	return result
}

func tableRows(table *dom.Node) []*dom.Node {
	if table == nil {
		return nil
	}
	var result []*dom.Node
	for child := table.FirstChild; child != nil; child = child.NextSibling {
		if !child.Element() {
			continue
		}
		if child.Data == "tr" {
			result = append(result, child)
			continue
		}
		if strings.EqualFold(child.Data, "thead") || strings.EqualFold(child.Data, "tbody") || strings.EqualFold(child.Data, "tfoot") {
			result = append(result, directTableChildren(child, "tr")...)
		}
	}
	return result
}
