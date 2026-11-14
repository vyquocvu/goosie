package archtest_test

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

func TestEngineHasNoArenaPointerFields(t *testing.T) {
	root := moduleRoot(t)
	found, err := archtest.ArenaPointerFields(filepath.Join(root, "internal", "engine"))
	if err != nil {
		t.Fatalf("ArenaPointerFields: %v", err)
	}
	for _, f := range found {
		t.Errorf("held arena pointer field: %s", f)
	}
	if len(found) != 0 {
		t.Fatalf("engine state holds %d *layout.Object field(s); use BoxKey", len(found))
	}
}

func TestArenaPointerFieldsFlagsHeldPointersOnly(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, filepath.Join(root, "p"), map[string]string{
		"dirty.go": "package p\n\nimport \"github.com/vyquocvu/goosie/internal/layout\"\n\n" +
			"type held struct { obj *layout.Object }\n",
		"alias.go": "package p\n\nimport lay \"github.com/vyquocvu/goosie/internal/layout\"\n\n" +
			"type heldAlias struct { obj *lay.Object }\n",
		"clean.go": "package p\n\nimport \"github.com/vyquocvu/goosie/internal/layout\"\n\n" +
			"type keyed struct { key layout.BoxKey }\n" +
			"type fn struct { scroll func(obj *layout.Object) float64 }\n",
	})
	found, err := archtest.ArenaPointerFields(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(found) != 2 {
		t.Fatalf("flagged %d fields, want 2 (held, heldAlias): %v", len(found), found)
	}
}
