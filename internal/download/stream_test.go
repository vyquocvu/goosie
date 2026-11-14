package download

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qc:BR-153
func TestSaveStreamHandlesFilesLargerThanDocumentLimit(t *testing.T) {
	dir := t.TempDir()
	body := bytes.Repeat([]byte("d"), (16<<20)+4096)
	var lastProgress int64
	path, written, sum, err := SaveStream(context.Background(), dir, "large.bin", bytes.NewReader(body), func(n int64) {
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
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, body) {
		t.Fatalf("saved bytes differ from the response (%d vs %d bytes)", len(saved), len(body))
	}
	if want := Digest(sha256.Sum256(body)); sum != want {
		t.Fatalf("digest = %s, want %s", sum, want)
	}
}

func TestSaveStreamCancellationRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 256*1024))
	path, _, _, err := SaveStream(ctx, dir, "partial.bin", body, func(int64) { cancel() })
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

func TestSaveStreamPausableKeepsPartialFile(t *testing.T) {
	dir := t.TempDir()
	body := bytes.Repeat([]byte("abc"), 100000)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	var pausedAt int64
	res, err := SaveStreamPausable(ctx, dir, "big.bin", bytes.NewReader(body), func(n int64) {
		if n >= 100000 {
			pausedAt = n
			cancel(ErrPaused)
		}
	})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("SaveStreamPausable error = %v, want ErrPaused", err)
	}
	if res.Path != "" {
		t.Fatalf("paused stream published final path %q", res.Path)
	}
	if res.TempPath == "" {
		t.Fatal("paused stream did not report its partial file")
	}
	if base := filepath.Base(res.TempPath); !strings.Contains(base, ".goosie-download") {
		t.Fatalf("partial file %q does not carry the .goosie-download marker", base)
	}
	if res.Bytes != pausedAt || pausedAt == 0 {
		t.Fatalf("paused at %d, result bytes = %d", pausedAt, res.Bytes)
	}
	info, err := os.Stat(res.TempPath)
	if err != nil {
		t.Fatalf("partial file missing: %v", err)
	}
	if info.Size() != pausedAt {
		t.Fatalf("partial file size = %d, want %d", info.Size(), pausedAt)
	}
	if _, err := os.Stat(filepath.Join(dir, "big.bin")); !os.IsNotExist(err) {
		t.Fatalf("paused stream linked a final file: %v", err)
	}

	total, sum, err := AppendToTemp(context.Background(), res.TempPath, bytes.NewReader(body[pausedAt:]), nil)
	if err != nil {
		t.Fatalf("AppendToTemp: %v", err)
	}
	if total != int64(len(body)) {
		t.Fatalf("resumed total = %d, want %d", total, len(body))
	}
	if want := Digest(sha256.Sum256(body)); sum != want {
		t.Fatalf("resumed digest = %s, want %s", sum, want)
	}
	contents, err := os.ReadFile(res.TempPath)
	if err != nil || !bytes.Equal(contents, body) {
		t.Fatalf("resumed file contents mismatch (err=%v, len=%d)", err, len(contents))
	}
}

func TestSaveStreamPausableCancellationRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	res, err := SaveStreamPausable(ctx, dir, "partial.bin", bytes.NewReader(bytes.Repeat([]byte("x"), 256*1024)), func(int64) {
		cancel(context.Canceled)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveStreamPausable error = %v, want context.Canceled", err)
	}
	if res.TempPath != "" {
		t.Fatalf("canceled stream reported temp path %q", res.TempPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancellation left files behind: %+v", entries)
	}
}

func TestSaveStreamToPausableKeepsPartialBesideTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chosen.bin")
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	res, err := SaveStreamToPausable(ctx, target, bytes.NewReader(bytes.Repeat([]byte("y"), 128*1024)), func(int64) {
		cancel(ErrPaused)
	})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("SaveStreamToPausable error = %v, want ErrPaused", err)
	}
	if res.TempPath == "" || filepath.Dir(res.TempPath) != dir {
		t.Fatalf("partial file = %q, want a sibling of %q", res.TempPath, target)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("paused stream created destination: %v", err)
	}
}

func TestAppendToTempPauseKeepsPartialFile(t *testing.T) {
	dir := t.TempDir()
	temp := filepath.Join(dir, ".goosie-download-part")
	if err := os.WriteFile(temp, []byte("hello "), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	var pausedAt int64
	total, _, err := AppendToTemp(ctx, temp, bytes.NewReader(bytes.Repeat([]byte("z"), 256*1024)), func(n int64) {
		if n >= 128*1024 {
			pausedAt = n
			cancel(ErrPaused)
		}
	})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("AppendToTemp error = %v, want ErrPaused", err)
	}
	if total != pausedAt {
		t.Fatalf("paused total = %d, want %d", total, pausedAt)
	}
	info, err := os.Stat(temp)
	if err != nil || info.Size() != pausedAt {
		t.Fatalf("partial file after pause = %v, size %v; want %d", err, info, pausedAt)
	}
}

func TestAppendToTempCancellationRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	temp := filepath.Join(dir, ".goosie-download-part")
	if err := os.WriteFile(temp, []byte("hello "), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	_, _, err := AppendToTemp(ctx, temp, bytes.NewReader(bytes.Repeat([]byte("z"), 256*1024)), func(int64) {
		cancel(context.Canceled)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AppendToTemp error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(temp); !os.IsNotExist(statErr) {
		t.Fatalf("canceled append left partial file behind: %v", statErr)
	}
}

func TestAppendToTempRejectsMissingFile(t *testing.T) {
	_, _, err := AppendToTemp(context.Background(), filepath.Join(t.TempDir(), "missing"), io.NopCloser(bytes.NewReader(nil)), nil)
	if err == nil {
		t.Fatal("AppendToTemp accepted a missing partial file")
	}
}

func TestSaveStreamToReplacesDestinationOnlyAfterCompleteStream(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chosen.bin")
	if err := os.WriteFile(target, []byte("old contents"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	written, _, err := SaveStreamTo(ctx, target, bytes.NewReader(bytes.Repeat([]byte("x"), 256*1024)), func(int64) { cancel() })
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

	written, _, err = SaveStreamTo(context.Background(), target, bytes.NewReader([]byte("new contents")), nil)
	if err != nil || written != int64(len("new contents")) {
		t.Fatalf("completed SaveStreamTo = %d, %v", written, err)
	}
	contents, err = os.ReadFile(target)
	if err != nil || string(contents) != "new contents" {
		t.Fatalf("completed stream destination = %q, %v", contents, err)
	}
}
