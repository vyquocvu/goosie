package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
