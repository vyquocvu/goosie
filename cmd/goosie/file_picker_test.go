package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPickedFilesReadsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "a.txt")
	binaryPath := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(textPath, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte{0x00, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := readPickedFiles([]string{textPath, binaryPath})
	if err != nil {
		t.Fatalf("readPickedFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("read %d files, want 2", len(files))
	}
	if files[0].Name != "a.txt" || files[0].ContentType != "text/plain" || !bytes.Equal(files[0].Data, []byte("abc")) {
		t.Errorf("first file = %+v, want a.txt/text/plain/abc", files[0])
	}
	if files[1].Name != "b.bin" || files[1].ContentType != "application/octet-stream" || !bytes.Equal(files[1].Data, []byte{0x00, 0x01}) {
		t.Errorf("second file = %+v, want b.bin/application/octet-stream", files[1])
	}
}

func TestReadPickedFilesRejectsDirectory(t *testing.T) {
	if _, err := readPickedFiles([]string{t.TempDir()}); err == nil {
		t.Fatal("readPickedFiles accepted a directory")
	}
}

func TestReadPickedFilesRejectsOversizeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxPickedFileBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	if _, err := readPickedFiles([]string{path}); err == nil {
		t.Fatal("readPickedFiles accepted an oversize file")
	}
}

func TestReadPickedFilesRejectsTooManyFiles(t *testing.T) {
	paths := make([]string, maxPickedFileCount+1)
	for i := range paths {
		paths[i] = filepath.Join(t.TempDir(), "missing")
	}
	if _, err := readPickedFiles(paths); err == nil {
		t.Fatal("readPickedFiles accepted too many files")
	}
}

func TestFileContentTypeUsesExtensionThenBytes(t *testing.T) {
	if got := fileContentType("note.txt", nil); got != "text/plain" {
		t.Errorf("txt type = %q, want text/plain", got)
	}
	if got := fileContentType("image.unknown", []byte("\x89PNG\r\n\x1a\n")); got != "image/png" {
		t.Errorf("detected type = %q, want image/png", got)
	}
	if got := fileContentType("empty.unknown", nil); got != "application/octet-stream" {
		t.Errorf("empty type = %q, want application/octet-stream", got)
	}
}
