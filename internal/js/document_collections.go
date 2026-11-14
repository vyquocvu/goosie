package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) installDocumentCollections() {
	r.documentNamedCollections = make(map[string]map[*dom.Node]*goja.Object)

	r.defineDocumentCollection("scripts", func(node *dom.Node) []*dom.Node {
		return collectByTagInSubtree(node, "script")
	})
	r.defineDocumentCollection("images", func(node *dom.Node) []*dom.Node {
		return collectByTagInSubtree(node, "img")
	})
	r.defineDocumentCollection("links", collectLinkedAreas)
}

func (r *Runtime) defineDocumentCollection(name string, matches func(*dom.Node) []*dom.Node) {
	_ = r.documentProto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument {
			return r.newLiveHTMLCollectionMatching(func() []*dom.Node { return nil })
		}
		cache := r.documentNamedCollections[name]
		if cache == nil {
			cache = make(map[*dom.Node]*goja.Object)
			r.documentNamedCollections[name] = cache
		}
		if collection := cache[node]; collection != nil {
			return collection
		}
		collection := r.newLiveHTMLCollectionMatching(func() []*dom.Node { return matches(node) }).(*goja.Object)
		cache[node] = collection
		return collection
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func collectLinkedAreas(root *dom.Node) []*dom.Node {
	var result []*dom.Node
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		collectLinkedAreasRecur(c, &result)
	}
	return result
}

func collectLinkedAreasRecur(n *dom.Node, result *[]*dom.Node) {
	if n.Element() && (strings.EqualFold(n.Data, "a") || strings.EqualFold(n.Data, "area")) && n.HasAttribute("href") {
		*result = append(*result, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectLinkedAreasRecur(c, result)
	}
}
