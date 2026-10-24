package download

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveStreamHandlesFilesLargerThanDocumentLimit(t *testing.T) {
	dir := t.TempDir()
	body := bytes.Repeat([]byte("d"), (16<<20)+4096)
	var lastProgress int64
	path, written, err := SaveStream(context.Background(), dir, "large.bin", bytes.NewReader(body), func(n int64) {
		lastProgress = n
	})
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len(body)) || lastProgress != written {
		t.Fatalf("written/progress = %d/%d, want %d", written, lastProgress, len(body))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(body)) {
		t.Fatalf("saved size = %d, want %d", info.Size(), len(body))
	}
}

func TestSaveStreamCancellationRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 256*1024))
	path, _, err := SaveStream(ctx, dir, "partial.bin", body, func(int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveStream error = %v, want context.Canceled", err)
	}
	if path != "" {
		t.Fatalf("canceled SaveStream path = %q, want empty", path)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancellation left files behind: %+v", entries)
	}
}

func TestSaveStreamToReplacesDestinationOnlyAfterCompleteStream(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chosen.bin")
	if err := os.WriteFile(target, []byte("old contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	written, err := SaveStreamTo(ctx, target, bytes.NewReader(bytes.Repeat([]byte("x"), 256*1024)), func(int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SaveStreamTo error = %v, want context.Canceled", err)
	}
	if written == 0 {
		t.Fatal("canceled stream should report bytes written before cancellation")
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "old contents" {
		t.Fatalf("canceled stream changed destination to %q, %v", contents, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "chosen.bin" {
		t.Fatalf("canceled stream left temporary files: %+v", entries)
	}

	written, err = SaveStreamTo(context.Background(), target, bytes.NewReader([]byte("new contents")), nil)
	if err != nil || written != int64(len("new contents")) {
		t.Fatalf("completed SaveStreamTo = %d, %v", written, err)
	}
	contents, err = os.ReadFile(target)
	if err != nil || string(contents) != "new contents" {
		t.Fatalf("completed stream destination = %q, %v", contents, err)
	}
}
