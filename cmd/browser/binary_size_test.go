package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const maxBinarySize = 100 << 20

func TestBinarySize(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "goosie")

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", out, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	fi, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}

	size := fi.Size()
	if size > maxBinarySize {
		t.Errorf("binary size %d bytes (%.1f MB) exceeds %d bytes (%.1f MB)",
			size, float64(size)/1e6, maxBinarySize, float64(maxBinarySize)/1e6)
	}

	t.Logf("binary size: %d bytes (%.1f MB)", size, float64(size)/1e6)
}
