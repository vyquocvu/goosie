package download

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type recordingFileLauncher struct {
	opened   []string
	revealed []string
}

func (l *recordingFileLauncher) OpenFile(path string) error {
	l.opened = append(l.opened, path)
	return nil
}

func (l *recordingFileLauncher) ShowInFolder(path string) error {
	l.revealed = append(l.revealed, path)
	return nil
}

func TestManagerFileActionsRejectUnmanagedPaths(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "downloads")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(root, "downloads-other")
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	siblingFile := filepath.Join(sibling, "outside.txt")
	if err := os.WriteFile(siblingFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	launcher := &recordingFileLauncher{}
	mgr := NewManager(dir)
	mgr.SetLauncher(launcher)
	for _, path := range []string{
		outside,
		filepath.Join("..", "outside.txt"),
		siblingFile,
		dir,
		filepath.Join(dir, "missing.txt"),
	} {
		if err := mgr.OpenFile(path); err == nil {
			t.Errorf("OpenFile(%q) succeeded", path)
		}
		if err := mgr.ShowInFolder(path); err == nil {
			t.Errorf("ShowInFolder(%q) succeeded", path)
		}
	}
	if len(launcher.opened) != 0 || len(launcher.revealed) != 0 {
		t.Fatalf("rejected paths reached launcher: %+v", launcher)
	}
}

func TestManagerFileActionsResolveSymlinksBeforeValidation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "downloads")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(dir, "inside.txt")
	if err := os.WriteFile(inside, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	insideLink := filepath.Join(dir, "inside-link.txt")
	outsideLink := filepath.Join(dir, "outside-link.txt")
	if err := os.Symlink(inside, insideLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outside, outsideLink); err != nil {
		t.Fatal(err)
	}

	launcher := &recordingFileLauncher{}
	mgr := NewManager(dir)
	mgr.SetLauncher(launcher)
	if err := mgr.OpenFile(insideLink); err != nil {
		t.Fatalf("OpenFile(inside symlink): %v", err)
	}
	if err := mgr.ShowInFolder(filepath.Join("..", "downloads", "inside-link.txt")); err != nil {
		t.Fatalf("ShowInFolder(relative inside symlink): %v", err)
	}
	if err := mgr.OpenFile(outsideLink); err == nil {
		t.Fatal("OpenFile(outside symlink) succeeded")
	}
	if err := mgr.ShowInFolder(outsideLink); err == nil {
		t.Fatal("ShowInFolder(outside symlink) succeeded")
	}
	insideResolved, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(launcher.opened, []string{insideResolved}) {
		t.Fatalf("opened = %v, want [%s]", launcher.opened, insideResolved)
	}
	if !reflect.DeepEqual(launcher.revealed, []string{insideResolved}) {
		t.Fatalf("revealed = %v, want [%s]", launcher.revealed, insideResolved)
	}
}

func TestManagerFileActionsRejectMissingDirectory(t *testing.T) {
	launcher := &recordingFileLauncher{}
	mgr := NewManager(filepath.Join(t.TempDir(), "missing"))
	mgr.SetLauncher(launcher)
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mgr.OpenFile(path); err == nil {
		t.Fatal("OpenFile succeeded without a download directory")
	}
	if err := mgr.ShowInFolder(path); err == nil {
		t.Fatal("ShowInFolder succeeded without a download directory")
	}
	if len(launcher.opened) != 0 || len(launcher.revealed) != 0 {
		t.Fatalf("rejected paths reached launcher: %+v", launcher)
	}
}
