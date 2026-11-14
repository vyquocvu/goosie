package engine

import (
	"fmt"
	"sync"

	"github.com/vyquocvu/goosie/internal/dom"
)

type docNodeBudget struct {
	mu   sync.Mutex
	root *dom.Node
	live int
}

func newDocNodeBudget(doc *dom.Document) *docNodeBudget {
	b := &docNodeBudget{root: &doc.Node}
	size, _, _ := measureSubtree(b.root)
	b.live = size
	return b
}

func (b *docNodeBudget) Reserve(parent *dom.Node, children []*dom.Node) error {
	if b == nil || parent == nil {
		return nil
	}
	pd, parentLive := b.depthOf(parent)
	if !parentLive {
		return nil
	}
	add := 0
	for _, c := range children {
		if c == nil {
			continue
		}
		size, height, err := measureSubtree(c)
		if err != nil {
			return err
		}
		if pd+1+height > MaxDocumentDepth {
			return fmt.Errorf("insertion would exceed the document depth limit (%d)", MaxDocumentDepth)
		}
		if !b.isLive(c) {
			add += size
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.live+add > MaxDocumentNodes {
		return fmt.Errorf("insertion would exceed the document node limit (%d)", MaxDocumentNodes)
	}
	b.live += add
	return nil
}

func (b *docNodeBudget) Release(children []*dom.Node) {
	if b == nil {
		return
	}
	credit := 0
	for _, c := range children {
		if c == nil {
			continue
		}
		size, _, err := measureSubtree(c)
		if err != nil {
			return
		}
		credit += size
	}
	if credit == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.live -= credit
	if b.live < 0 {
		b.live = 0
	}
}

func (b *docNodeBudget) IsLive(n *dom.Node) bool {
	_, live := b.depthOf(n)
	return live
}

func (b *docNodeBudget) liveCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.live
}

func (b *docNodeBudget) isLive(n *dom.Node) bool {
	_, live := b.depthOf(n)
	return live
}

func (b *docNodeBudget) depthOf(n *dom.Node) (int, bool) {
	if b == nil || n == nil {
		return 0, false
	}
	depth := 0
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == b.root {
			return depth, true
		}
		depth++
		if depth > MaxDocumentDepth+2 {
			return 0, false
		}
	}
	return 0, false
}

func measureSubtree(n *dom.Node) (size int, height int, err error) {
	if n == nil {
		return 0, 0, nil
	}
	type entry struct {
		node  *dom.Node
		depth int
	}
	stack := []entry{{n, 0}}
	seen := make(map[*dom.Node]bool)
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[e.node] {
			return size, height, fmt.Errorf("document node cycle")
		}
		seen[e.node] = true
		size++
		if size > MaxDocumentNodes {
			return size, height, fmt.Errorf("HTML node limit exceeded (%d)", MaxDocumentNodes)
		}
		if e.depth > MaxDocumentDepth {
			return size, height, fmt.Errorf("HTML tree depth limit exceeded (%d)", MaxDocumentDepth)
		}
		if e.depth > height {
			height = e.depth
		}
		if len(e.node.Attr) > MaxAttributes {
			return size, height, fmt.Errorf("HTML attribute count limit exceeded (%d)", MaxAttributes)
		}
		for _, a := range e.node.Attr {
			if len(a.Name)+len(a.Value) > MaxAttributeBytes {
				return size, height, fmt.Errorf("HTML attribute byte limit exceeded (%d)", MaxAttributeBytes)
			}
		}
		siblings := 0
		for c := e.node.FirstChild; c != nil; c = c.NextSibling {
			siblings++
			if siblings > MaxDocumentNodes || c.Parent != e.node {
				return size, height, fmt.Errorf("invalid document node links")
			}
		}
		for c := e.node.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, entry{c, e.depth + 1})
		}
	}
	return size, height, nil
}
