package net

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadManagerWritesFileAndRecordsCompleteStatus(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, "download body"), nil
	})}
	manager := NewDownloadManager(client)
	target := filepath.Join(t.TempDir(), "download.txt")

	record := manager.DownloadWithContext(context.Background(), "https://example.test/file", target)

	if record.Status != DownloadStatusComplete {
		t.Fatalf("Status = %q, want complete: %s", record.Status, record.Error)
	}
	if record.URL != "https://example.test/file" {
		t.Errorf("URL = %q", record.URL)
	}
	if record.TargetPath != target {
		t.Errorf("TargetPath = %q", record.TargetPath)
	}
	if record.BytesWritten != int64(len("download body")) {
		t.Errorf("BytesWritten = %d", record.BytesWritten)
	}
	if record.StartedAt.IsZero() || record.FinishedAt.IsZero() {
		t.Error("download timestamps were not recorded")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "download body" {
		t.Fatalf("target data = %q, want download body", data)
	}
}
