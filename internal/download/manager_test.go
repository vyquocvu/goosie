package download

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDownloadManagerLifecycle(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	if mgr.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", mgr.Dir(), dir)
	}
	mgr.SetDir(filepath.Join(dir, "new"))
	if !strings.HasSuffix(mgr.Dir(), "new") {
		t.Errorf("SetDir failed: got %q", mgr.Dir())
	}

	// 1. Create item
	item := mgr.CreateItem("https://example.com/file.zip", "file.zip", 1024*1024*10) // 10 MB
	if item.ID != "dl-1" {
		t.Errorf("expected ID dl-1, got %q", item.ID)
	}
	if item.Status != StatusActive {
		t.Errorf("status = %v, want StatusActive", item.Status)
	}
	if mgr.ActiveCount() != 1 {
		t.Errorf("ActiveCount = %d, want 1", mgr.ActiveCount())
	}
	if mgr.TotalCount() != 1 {
		t.Errorf("TotalCount = %d, want 1", mgr.TotalCount())
	}

	// 2. Progress update
	mgr.UpdateProgress(item.ID, 1024*1024*5) // 5 MB (50%)
	it := mgr.Get(item.ID)
	if it == nil {
		t.Fatal("Get(item.ID) returned nil")
	}
	if it.DownloadedBytes != 1024*1024*5 {
		t.Errorf("DownloadedBytes = %d, want 5MB", it.DownloadedBytes)
	}
	if pct := it.Percent(); pct < 49.9 || pct > 50.1 {
		t.Errorf("Percent() = %f, want ~50.0", pct)
	}
	if sz := it.FormattedSize(); !strings.Contains(sz, "5.0 MB") || !strings.Contains(sz, "10.0 MB") {
		t.Errorf("FormattedSize() = %q", sz)
	}

	// Overall progress
	if op := mgr.OverallProgress(); op < 49.9 || op > 50.1 {
		t.Errorf("OverallProgress() = %f, want ~50.0", op)
	}

	// 3. Complete
	mgr.Complete(item.ID, filepath.Join(dir, "file.zip"))
	it = mgr.Get(item.ID)
	if it.Status != StatusCompleted {
		t.Errorf("Status = %v, want StatusCompleted", it.Status)
	}
	if it.Percent() != 100.0 {
		t.Errorf("Percent after completion = %f, want 100.0", it.Percent())
	}
	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount after completion = %d, want 0", mgr.ActiveCount())
	}

	// 4. Cancel
	item2 := mgr.CreateItem("https://example.com/cancel.tar", "cancel.tar", 500)
	mgr.Cancel(item2.ID)
	it2 := mgr.Get(item2.ID)
	if it2.Status != StatusCanceled {
		t.Errorf("status = %v, want StatusCanceled", it2.Status)
	}

	// 5. Fail
	item3 := mgr.CreateItem("https://example.com/broken.iso", "broken.iso", 1000)
	mgr.Fail(item3.ID, errors.New("network timeout"))
	it3 := mgr.Get(item3.ID)
	if it3.Status != StatusFailed || it3.Error != "network timeout" {
		t.Errorf("failed item: %v, error: %s", it3.Status, it3.Error)
	}

	// 6. ClearCompleted
	if mgr.TotalCount() != 3 {
		t.Errorf("TotalCount before clear = %d, want 3", mgr.TotalCount())
	}
	mgr.ClearCompleted()
	if mgr.TotalCount() != 0 {
		t.Errorf("TotalCount after clear = %d, want 0", mgr.TotalCount())
	}
}

func TestFormatBytesAndSpeed(t *testing.T) {
	if s := FormatBytes(500); s != "500 B" {
		t.Errorf("FormatBytes(500) = %q", s)
	}
	if s := FormatBytes(1024 * 1024); s != "1.0 MB" {
		t.Errorf("FormatBytes(1MB) = %q", s)
	}
	if s := FormatBytes(1024 * 1024 * 1024 * 2); s != "2.0 GB" {
		t.Errorf("FormatBytes(2GB) = %q", s)
	}
	if s := FormatSpeed(1024 * 512); s != "512.0 KB/s" {
		t.Errorf("FormatSpeed = %q", s)
	}
	if s := FormatSpeed(0); s != "--" {
		t.Errorf("FormatSpeed(0) = %q", s)
	}
}

func TestCancelStopsOperationAndCannotBeOverwrittenByCompletion(t *testing.T) {
	mgr := NewManager(t.TempDir())
	item := mgr.CreateItem("https://example.com/large.bin", "large.bin", 1024)
	ctx, cancel := context.WithCancel(context.Background())
	mgr.SetCancel(item.ID, cancel)
	mgr.Cancel(item.ID)
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("operation context error = %v, want context.Canceled", ctx.Err())
	}
	mgr.Complete(item.ID, filepath.Join(t.TempDir(), "large.bin"))
	if got := mgr.Get(item.ID).Status; got != StatusCanceled {
		t.Fatalf("status after late completion = %q, want canceled", got)
	}
}

func TestConcurrentManager(t *testing.T) {
	mgr := NewManager(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			it := mgr.CreateItem("https://example.com/file", "test.bin", 1000)
			mgr.UpdateProgress(it.ID, 500)
			_ = mgr.ActiveItems()
			_ = mgr.Items()
			_ = mgr.OverallProgress()
			if idx%2 == 0 {
				mgr.Complete(it.ID, "/tmp/test.bin")
			} else {
				mgr.Fail(it.ID, errors.New("err"))
			}
		}(i)
	}
	wg.Wait()
	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount = %d, want 0", mgr.ActiveCount())
	}
	if mgr.TotalCount() != 20 {
		t.Errorf("TotalCount = %d, want 20", mgr.TotalCount())
	}
}

func TestSubscribe(t *testing.T) {
	mgr := NewManager(t.TempDir())
	ch := make(chan *Item, 5)
	mgr.Subscribe(func(it *Item) {
		ch <- it
	})

	it := mgr.CreateItem("https://example.com/s", "s.txt", 100)
	select {
	case received := <-ch:
		if received.ID != it.ID {
			t.Errorf("received ID %q, want %q", received.ID, it.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscription event")
	}
}
