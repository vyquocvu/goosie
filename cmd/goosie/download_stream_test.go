package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/download"
	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestLoadURLCancelsStreamingAttachmentAndRemovesPartialFile(t *testing.T) {
	bodyStarted := make(chan struct{})
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=large.bin")
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("first chunk"))
		w.(http.Flusher).Flush()
		close(bodyStarted)
		<-r.Context().Done()
		close(disconnected)
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	result := make(chan error, 1)
	go func() {
		_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr)
		result <- err
	}()

	select {
	case <-bodyStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("download response body did not start")
	}
	deadline := time.Now().Add(3 * time.Second)
	var item *download.Item
	for time.Now().Before(deadline) {
		items := mgr.ActiveItems()
		if len(items) == 1 && items[0].DownloadedBytes > 0 {
			item = items[0]
			break
		}
		time.Sleep(time.Millisecond)
	}
	if item == nil {
		t.Fatal("streamed download did not report progress")
	}
	mgr.Cancel(item.ID)

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("loadURLCtx error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop the streaming navigation")
	}
	select {
	case <-disconnected:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not observe the canceled request")
	}
	if got := mgr.Get(item.ID).Status; got != download.StatusCanceled {
		t.Fatalf("download status = %q, want canceled", got)
	}
	entries, err := os.ReadDir(mgr.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial download remains after cancel: %+v", entries)
	}
}

func TestStreamingDownloadPausesAndResumesWithRange(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789"), 50000)
	split := 200000
	var rangeHeader atomic.Value
	firstStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="big.bin"`)
		if rh := r.Header.Get("Range"); rh != "" {
			start := 0
			if _, scanErr := fmt.Sscanf(rh, "bytes=%d-", &start); scanErr != nil {
				t.Errorf("unexpected Range header %q: %v", rh, scanErr)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rangeHeader.Store(rh)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)-start))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[start:])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload[:split])
		w.(http.Flusher).Flush()
		select {
		case firstStarted <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	result := make(chan error, 1)
	go func() {
		_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr)
		result <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("download response body did not start")
	}
	item := waitForDownload(t, mgr)
	if !mgr.Pause(item.ID) {
		t.Fatal("pause was not accepted for a streaming download")
	}

	select {
	case err := <-result:
		var paused downloadPaused
		if !errors.As(err, &paused) {
			t.Fatalf("loadURLCtx error = %v, want downloadPaused", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pause did not stop the streaming navigation")
	}

	paused := mgr.Get(item.ID)
	if paused.Status != download.StatusPaused {
		t.Fatalf("status after pause = %q, want paused", paused.Status)
	}
	if paused.TempPath == "" {
		t.Fatal("paused download has no partial file")
	}
	info, err := os.Stat(paused.TempPath)
	if err != nil {
		t.Fatalf("partial file missing: %v", err)
	}
	if info.Size() == 0 || info.Size() > int64(split) {
		t.Fatalf("partial file size = %d, want 1..%d", info.Size(), split)
	}
	if paused.DownloadedBytes != info.Size() {
		t.Fatalf("paused bytes = %d, partial file size = %d", paused.DownloadedBytes, info.Size())
	}
	pausedAt := info.Size()

	if !mgr.Resume(item.ID) {
		t.Fatal("resume was not accepted")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got := mgr.Get(item.ID)
		if got.Status == download.StatusCompleted {
			break
		}
		if got.Status == download.StatusFailed {
			t.Fatalf("resume failed: %s", got.Error)
		}
		time.Sleep(time.Millisecond)
	}
	done := mgr.Get(item.ID)
	if done.Status != download.StatusCompleted {
		t.Fatalf("status after resume = %q, want completed", done.Status)
	}
	if got, _ := rangeHeader.Load().(string); got != fmt.Sprintf("bytes=%d-", pausedAt) {
		t.Fatalf("resume Range header = %q, want bytes=%d-", got, pausedAt)
	}
	contents, err := os.ReadFile(done.LocalPath)
	if err != nil || !bytes.Equal(contents, payload) {
		t.Fatalf("downloaded file mismatch (err=%v, len=%d)", err, len(contents))
	}
	sum := sha256.Sum256(payload)
	if done.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("SHA256 = %q, want %q", done.SHA256, hex.EncodeToString(sum[:]))
	}
	if done.TempPath != "" {
		t.Fatalf("completed download still names temp path %q", done.TempPath)
	}
	entries, err := os.ReadDir(mgr.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "big.bin" {
		t.Fatalf("download directory after resume = %+v", entries)
	}
}

func TestResumeRestartsWhenServerIgnoresRange(t *testing.T) {
	payload := bytes.Repeat([]byte("0123456789abcdef"), 25000)
	split := 120000
	var rangeHeader atomic.Value
	firstStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="restart.bin"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		if rh := r.Header.Get("Range"); rh != "" {
			rangeHeader.Store(rh)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload[:split])
		w.(http.Flusher).Flush()
		select {
		case firstStarted <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	result := make(chan error, 1)
	go func() {
		_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr)
		result <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("download response body did not start")
	}
	item := waitForDownload(t, mgr)
	if !mgr.Pause(item.ID) {
		t.Fatal("pause was not accepted for a streaming download")
	}
	select {
	case err := <-result:
		var paused downloadPaused
		if !errors.As(err, &paused) {
			t.Fatalf("loadURLCtx error = %v, want downloadPaused", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pause did not stop the streaming navigation")
	}
	paused := mgr.Get(item.ID)
	if !mgr.Resume(item.ID) {
		t.Fatal("resume was not accepted")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got := mgr.Get(item.ID)
		if got.Status == download.StatusCompleted {
			break
		}
		if got.Status == download.StatusFailed {
			t.Fatalf("resume failed instead of restarting: %s", got.Error)
		}
		time.Sleep(time.Millisecond)
	}
	done := mgr.Get(item.ID)
	if done.Status != download.StatusCompleted {
		t.Fatalf("status after 200 resume = %q, want completed", done.Status)
	}
	if got, _ := rangeHeader.Load().(string); got != fmt.Sprintf("bytes=%d-", paused.DownloadedBytes) {
		t.Fatalf("resume Range header = %q, want bytes=%d-", got, paused.DownloadedBytes)
	}
	contents, err := os.ReadFile(done.LocalPath)
	if err != nil || !bytes.Equal(contents, payload) {
		t.Fatalf("restarted file mismatch (err=%v, len=%d, want %d)", err, len(contents), len(payload))
	}
	sum := sha256.Sum256(payload)
	if done.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("SHA256 = %q, want %q", done.SHA256, hex.EncodeToString(sum[:]))
	}
	entries, err := os.ReadDir(mgr.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "restart.bin" {
		t.Fatalf("download directory after restart = %+v", entries)
	}
}

func TestStreamingDownloadWithoutContentLengthReportsBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("chunk"), 40000)
	split := 80000
	firstChunk := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="unknown.bin"`)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write(payload[:split])
		w.(http.Flusher).Flush()
		select {
		case firstChunk <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write(payload[split:])
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	result := make(chan error, 1)
	go func() {
		_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr)
		result <- err
	}()

	select {
	case <-firstChunk:
	case <-time.After(3 * time.Second):
		t.Fatal("download response body did not start")
	}
	item := waitForDownload(t, mgr)
	if item.TotalBytes > 0 {
		t.Fatalf("TotalBytes = %d, want unknown without Content-Length", item.TotalBytes)
	}
	if pct := item.Percent(); pct >= 0 {
		t.Fatalf("Percent = %f, want an unknown total", pct)
	}
	if size := item.FormattedSize(); strings.Contains(size, " / ") || size != download.FormatBytes(item.DownloadedBytes) {
		t.Fatalf("FormattedSize = %q, want just the received bytes", size)
	}
	close(release)
	select {
	case err := <-result:
		var done downloadDone
		if !errors.As(err, &done) {
			t.Fatalf("loadURLCtx error = %v, want downloadDone", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download without Content-Length did not finish")
	}
	done := mgr.Get(item.ID)
	if done.Status != download.StatusCompleted || done.DownloadedBytes != int64(len(payload)) {
		t.Fatalf("completed item = %+v, want %d bytes", done, len(payload))
	}
	contents, err := os.ReadFile(done.LocalPath)
	if err != nil || !bytes.Equal(contents, payload) {
		t.Fatalf("unknown-length file mismatch (err=%v, len=%d)", err, len(contents))
	}
}

func TestPausedDownloadCancelRemovesPartialFile(t *testing.T) {
	payload := bytes.Repeat([]byte("z"), 300000)
	firstStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="cancel.bin"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload[:100000])
		w.(http.Flusher).Flush()
		select {
		case firstStarted <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	result := make(chan error, 1)
	go func() {
		_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr)
		result <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("download response body did not start")
	}
	item := waitForDownload(t, mgr)
	mgr.Pause(item.ID)
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("pause did not stop the streaming navigation")
	}
	tempPath := mgr.Get(item.ID).TempPath
	if tempPath == "" {
		t.Fatal("paused download has no partial file")
	}
	mgr.Cancel(item.ID)
	if got := mgr.Get(item.ID).Status; got != download.StatusCanceled {
		t.Fatalf("status after cancel = %q, want canceled", got)
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("cancel left partial file behind: %v", err)
	}
}

func waitForDownload(t *testing.T, mgr *download.Manager) *download.Item {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items := mgr.ActiveItems()
		if len(items) == 1 && items[0].DownloadedBytes > 0 {
			return items[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("download did not report progress")
	return nil
}

type fakeSavePathPicker struct {
	path      string
	chosen    bool
	err       error
	directory string
	filename  string
}

func (p *fakeSavePathPicker) ChooseSavePath(directory, filename string) (string, bool, error) {
	p.directory, p.filename = directory, filename
	return p.path, p.chosen, p.err
}

func TestLoadURLSavesStreamingAttachmentToChosenPath(t *testing.T) {
	const payload = "selected destination contents"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="suggested.bin"`)
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	destination := filepath.Join(t.TempDir(), "nested", "chosen.bin")
	picker := &fakeSavePathPicker{path: destination, chosen: true}
	_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr, picker)
	var done downloadDone
	if !errors.As(err, &done) {
		t.Fatalf("loadURLCtx error = %v, want downloadDone", err)
	}
	if got, want := done.path, destination; got != want {
		t.Fatalf("download path = %q, want %q", got, want)
	}
	if picker.directory != mgr.Dir() || picker.filename != "suggested.bin" {
		t.Fatalf("picker defaults = %q/%q, want %q/suggested.bin", picker.directory, picker.filename, mgr.Dir())
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != payload {
		t.Fatalf("selected file contents = %q, %v", contents, err)
	}
	items := mgr.ActiveItems()
	if len(items) != 0 {
		t.Fatalf("completed download remains active: %+v", items)
	}
	all := mgr.Items()
	if len(all) != 1 || all[0].Status != download.StatusCompleted || all[0].LocalPath != destination {
		t.Fatalf("download manager item = %+v", all)
	}
}

func TestLoadURLCanceledSaveDialogCreatesNoDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="cancel.bin"`)
		_, _ = io.WriteString(w, strings.Repeat("x", 4096))
	}))
	defer server.Close()

	client := transport.DefaultClient()
	defer client.Close()
	mgr := download.NewManager(t.TempDir())
	picker := &fakeSavePathPicker{chosen: false}
	_, _, _, _, _, err := loadURLCtx(context.Background(), client, nil, server.URL, 800, 600, 1, mgr.Dir(), false, false, mgr, picker)
	if !errors.Is(err, errDownloadSaveCanceled) {
		t.Fatalf("loadURLCtx error = %v, want save dialog canceled", err)
	}
	if len(mgr.Items()) != 0 {
		t.Fatalf("canceled save dialog created manager items: %+v", mgr.Items())
	}
	entries, err := os.ReadDir(mgr.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled save dialog created files: %+v", entries)
	}
}
