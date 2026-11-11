package archtest_test

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

// TestProductionHasNoArenaPointerMaps is the repo-wide half of the SP-4 L1 box
// identity rule: paint and engine name boxes by BoxKey/ObjectID, never hold arena
// pointers in a map, because a *layout.Object is invalidated by the next arena
// growth. internal/ and cmd/ are the shipped tree; test/ and third_party/ are free.
func TestProductionHasNoArenaPointerMaps(t *testing.T) {
	root := moduleRoot(t)
	for _, sub := range []string{"internal", "cmd"} {
		found, err := archtest.ArenaPointerMaps(filepath.Join(root, sub))
		if err != nil {
			t.Fatalf("%s: %v", sub, err)
		}
		for _, m := range found {
			t.Errorf("%s: name boxes by BoxKey/ObjectID, not *layout.Object", m)
		}
	}
}

// The non-vacuity cases: a map keyed by *layout.Object is flagged, including under
// a renamed import, while a map keyed by layout.BoxKey or layout.ObjectID is not.
// A rule that reported nothing for any input would pass the repo-wide check above
// and be worthless.
func TestArenaPointerMapsFlagsPointerKeysOnly(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, filepath.Join(root, "p"), map[string]string{
		"dirty.go": "package p\n\nimport \"github.com/vyquocvu/goosie/internal/layout\"\n\n" +
			"var byPtr = map[*layout.Object]int{}\n",
		"alias.go": "package p\n\nimport lay \"github.com/vyquocvu/goosie/internal/layout\"\n\n" +
			"var byAlias = map[*lay.Object]string{}\n",
		"clean.go": "package p\n\nimport \"github.com/vyquocvu/goosie/internal/layout\"\n\n" +
			"var byKey = map[layout.BoxKey]int{}\nvar byID = map[layout.ObjectID]int{}\n",
	})
	found, err := archtest.ArenaPointerMaps(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(found) != 2 {
		t.Fatalf("flagged %d maps, want 2 (byPtr, byAlias): %v", len(found), found)
	}
}
