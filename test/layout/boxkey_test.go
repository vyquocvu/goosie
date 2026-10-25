package layout_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// A BoxKey packs the originating node, the box kind and an ordinal into
// disjoint bit ranges, so every distinct input triple must produce a distinct
// key. This is the collision-freedom contract the arena's future byKey index
// depends on.
func TestBoxKeyIsCollisionFree(t *testing.T) {
	nodes := []uint32{0, 1, 2, 0xFFFFFFFF}
	kinds := []layout.BoxKind{
		layout.BoxKindBlock,
		layout.BoxKindInline,
		layout.BoxKindAnonymous,
		layout.BoxKindPseudoBefore,
		layout.BoxKindPseudoAfter,
		layout.BoxKindTable,
		layout.BoxKindTableRow,
		layout.BoxKindTableCell,
		layout.BoxKindTableRowGroup,
		layout.BoxKindTableColumn,
		layout.BoxKindTableColumnGroup,
		layout.BoxKindTableCaption,
		layout.BoxKindListItem,
	}
	ordinals := []uint16{0, 1, 0xFFFF}

	seen := make(map[layout.BoxKey]string, len(nodes)*len(kinds)*len(ordinals))
	for _, node := range nodes {
		for _, kind := range kinds {
			for _, ordinal := range ordinals {
				key := layout.NewBoxKey(node, kind, ordinal)
				if key == 0 {
					t.Fatalf("NewBoxKey(%d, %d, %d) returned the unassigned zero key", node, kind, ordinal)
				}
				label := fmt.Sprintf("node=%d kind=%d ordinal=%d", node, kind, ordinal)
				if prev, ok := seen[key]; ok {
					t.Fatalf("BoxKey collision: %s and %s both map to %#x", prev, label, uint64(key))
				}
				seen[key] = label

				if got := key.NodeID(); got != node {
					t.Fatalf("%s: NodeID() = %d, want %d", label, got, node)
				}
				if got := key.Kind(); got != kind {
					t.Fatalf("%s: Kind() = %d, want %d", label, got, kind)
				}
				if got := key.Ordinal(); got != ordinal {
					t.Fatalf("%s: Ordinal() = %d, want %d", label, got, ordinal)
				}
			}
		}
	}
	if want := len(nodes) * len(kinds) * len(ordinals); len(seen) != want {
		t.Fatalf("distinct keys = %d, want %d", len(seen), want)
	}
}

// The same inputs must always produce the same key: callers rebuild the arena
// on every reflow and rely on the key surviving that rebuild.
func TestBoxKeyIsDeterministic(t *testing.T) {
	first := layout.NewBoxKey(42, layout.BoxKindInline, 9)
	second := layout.NewBoxKey(42, layout.BoxKindInline, 9)
	if first != second {
		t.Fatalf("NewBoxKey is not deterministic: %#x != %#x", uint64(first), uint64(second))
	}
}

// Build is the one place that can name a box from stable inputs, so every
// object it creates for a DOM node must carry a key, and two objects must not
// share one.
func TestBuildAssignsUniqueBoxKeys(t *testing.T) {
	doc := domtest.Parse(`<html><body><div><span>hi</span></div><p>text</p></body></html>`)
	arena := layout.Build(doc, style.Resolve(doc, nil, nil), nil)

	byKey := make(map[layout.BoxKey]layout.ObjectID)
	for i := 1; i < len(arena.Objects); i++ {
		obj := &arena.Objects[i]
		if obj.Node == nil {
			continue
		}
		if obj.Key == 0 {
			t.Fatalf("object %d (node %d) has no BoxKey", i, obj.Node.ID)
		}
		if prev, ok := byKey[obj.Key]; ok {
			t.Fatalf("objects %d and %d share BoxKey %#x", prev, i, uint64(obj.Key))
		}
		byKey[obj.Key] = layout.ObjectID(i)
	}
	if len(byKey) == 0 {
		t.Fatal("Build produced no keyed objects")
	}
}
