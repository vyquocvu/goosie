package net_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"github.com/vyquocvu/goosie/internal/net"
)

type failingDownloadReader struct {
	reader io.Reader
	err    error
}

func (r *failingDownloadReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == io.EOF {
		return n, r.err
	}
	return n, err
}

func (r *failingDownloadReader) Close() error {
	return nil
}

type cancelingDownloadReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (r *cancelingDownloadReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

func (r *cancelingDownloadReader) Close() error {
	return nil
}

var _ func(*net.DownloadManager, string, string) (net.DownloadRecord, error) = (*net.DownloadManager).Download
var _ func(*net.DownloadManager, context.Context, string, string) (net.DownloadRecord, error) = (*net.DownloadManager).DownloadWithContext

func TestDownloadManagerStatusConstantsMatchContract(t *testing.T) {
	if net.DownloadRunning != net.DownloadStatusRunning || net.DownloadComplete != net.DownloadStatusComplete || net.DownloadFailed != net.DownloadStatusFailed {
		t.Fatalf("download status aliases = %q/%q/%q", net.DownloadRunning, net.DownloadComplete, net.DownloadFailed)
	}
}

func TestDownloadManagerWritesFileAndRecordsCompleteStatus(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusOK, "download body"), nil
	})}
	manager := net.NewDownloadManager(client)
	target := filepath.Join(t.TempDir(), "download.txt")

	record, err := manager.DownloadWithContext(context.Background(), "https://example.test/file", target)

	if err != nil {
		t.Fatalf("DownloadWithContext returned error: %v", err)
	}
	if record.Status != net.DownloadStatusComplete {
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

func TestDownloadManagerFailsHTTPErrorWithoutOverwritingTarget(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusNotFound, "not found"), nil
	})}
	manager := net.NewDownloadManager(client)
	target := filepath.Join(t.TempDir(), "download.txt")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	record, downloadErr := manager.DownloadWithContext(context.Background(), "https://example.test/missing", target)

	if downloadErr == nil {
		t.Fatal("DownloadWithContext error was nil")
	}
	if record.Status != net.DownloadStatusFailed {
		t.Fatalf("Status = %q, want failed", record.Status)
	}
	if record.Error == "" {
		t.Fatal("Error was empty")
	}
	if record.BytesWritten != 0 {
		t.Fatalf("BytesWritten = %d, want 0", record.BytesWritten)
	}
	if record.FinishedAt.IsZero() {
		t.Fatal("FinishedAt was not recorded")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("target data = %q, want existing", data)
	}
}

func TestDownloadManagerReadFailurePreservesTargetAndRemovesTemporaryFile(t *testing.T) {
	errRead := errors.New("stream interrupted")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "")
		resp.Body = &failingDownloadReader{reader: strings.NewReader("partial"), err: errRead}
		return resp, nil
	})}
	manager := net.NewDownloadManager(client)
	dir := t.TempDir()
	target := filepath.Join(dir, "download.txt")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	record, downloadErr := manager.DownloadWithContext(context.Background(), "https://example.test/file", target)

	if !errors.Is(downloadErr, errRead) {
		t.Fatalf("DownloadWithContext error = %v, want %v", downloadErr, errRead)
	}
	if record.Status != net.DownloadStatusFailed {
		t.Fatalf("Status = %q, want failed", record.Status)
	}
	if !strings.Contains(record.Error, errRead.Error()) {
		t.Fatalf("Error = %q, want %q", record.Error, errRead)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("target data = %q, want existing", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read target directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		t.Fatalf("target directory entries = %v, want only %q", entries, filepath.Base(target))
	}
}

func TestDownloadManagerCancellationBeforeCommitPreservesTarget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := newTestResponse(req, http.StatusOK, "")
		resp.Body = &cancelingDownloadReader{reader: strings.NewReader("replacement"), cancel: cancel}
		return resp, nil
	})}
	manager := net.NewDownloadManager(client)
	dir := t.TempDir()
	target := filepath.Join(dir, "download.txt")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	record, downloadErr := manager.DownloadWithContext(ctx, "https://example.test/file", target)

	if !errors.Is(downloadErr, context.Canceled) {
		t.Fatalf("DownloadWithContext error = %v, want cancellation", downloadErr)
	}
	if record.Status != net.DownloadStatusFailed {
		t.Fatalf("Status = %q, want failed", record.Status)
	}
	if !errors.Is(ctx.Err(), context.Canceled) || !strings.Contains(record.Error, context.Canceled.Error()) {
		t.Fatalf("context/error = %v/%q, want cancellation", ctx.Err(), record.Error)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("target data = %q, want existing", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read target directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		t.Fatalf("target directory entries = %v, want only %q", entries, filepath.Base(target))
	}
}

func TestDownloadManagerFailsRedirectResponseWithoutOverwritingTarget(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(req, http.StatusFound, "redirect"), nil
	})}
	manager := net.NewDownloadManager(client)
	target := filepath.Join(t.TempDir(), "download.txt")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	record, downloadErr := manager.DownloadWithContext(context.Background(), "https://example.test/file", target)

	if downloadErr == nil {
		t.Fatal("DownloadWithContext error was nil")
	}
	if record.Status != net.DownloadStatusFailed {
		t.Fatalf("Status = %q, want failed", record.Status)
	}
	if record.Error == "" {
		t.Fatal("Error was empty")
	}
	if record.BytesWritten != 0 {
		t.Fatalf("BytesWritten = %d, want 0", record.BytesWritten)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("target data = %q, want existing", data)
	}
}
