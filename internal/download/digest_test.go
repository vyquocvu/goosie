package download

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const helloWorldSHA256 = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

func TestSaveStreamReportsSHA256(t *testing.T) {
	path, written, sum, err := SaveStream(context.Background(), t.TempDir(), "hello.txt",
		bytes.NewReader([]byte("hello world")), nil)
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len("hello world")) {
		t.Fatalf("written = %d, want %d", written, len("hello world"))
	}
	if sum.String() != helloWorldSHA256 {
		t.Fatalf("digest = %s, want %s", sum, helloWorldSHA256)
	}
	if filepath.Base(path) != "hello.txt" {
		t.Fatalf("saved path = %q", path)
	}
}

func TestSaveStreamToReportsSHA256(t *testing.T) {
	target := filepath.Join(t.TempDir(), "chosen.txt")
	written, sum, err := SaveStreamTo(context.Background(), target,
		bytes.NewReader([]byte("hello world")), nil)
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len("hello world")) {
		t.Fatalf("written = %d, want %d", written, len("hello world"))
	}
	if sum.String() != helloWorldSHA256 {
		t.Fatalf("digest = %s, want %s", sum, helloWorldSHA256)
	}
}

func TestManagerCompleteRecordsSHA256(t *testing.T) {
	m := NewManager(t.TempDir())
	item := m.CreateItem("https://example.test/hello.txt", "hello.txt", int64(len("hello world")))
	m.CompleteWithDigest(item.ID, "/tmp/hello.txt", helloWorldSHA256)

	got := m.Get(item.ID)
	if got == nil || got.SHA256 != helloWorldSHA256 {
		t.Fatalf("item digest = %+v, want %s", got, helloWorldSHA256)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"sha256":"`+helloWorldSHA256+`"`) {
		t.Fatalf("item JSON omits the digest: %s", data)
	}
}
