package net_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestGetStreamReadsBodiesBeyondBufferedResponseLimit(t *testing.T) {
	body := bytes.Repeat([]byte("s"), transport.MaxResponseBytes+1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "16778240")
		_, _ = w.Write(body)
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()
	streamer, ok := client.(transport.StreamingHTTP)
	if !ok {
		t.Fatal("default client does not implement StreamingHTTP")
	}
	resp, err := streamer.GetStream(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Close()
	if resp.ContentLength != int64(len(body)) {
		t.Fatalf("ContentLength = %d, want %d", resp.ContentLength, len(body))
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("stream returned %d bytes, want %d", len(got), len(body))
	}
	traces := client.NetworkLog().List()
	if len(traces) == 0 || traces[len(traces)-1].Bytes != len(body) {
		t.Fatalf("stream network trace = %+v, want %d bytes", traces, len(body))
	}
}

func TestGetStreamReturnsHeadersBeforeBodyAndHonorsCancellation(t *testing.T) {
	bodyStarted := make(chan struct{})
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=large.bin")
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("chunk"))
		w.(http.Flusher).Flush()
		close(bodyStarted)
		<-r.Context().Done()
		close(disconnected)
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()
	streamer := client.(transport.StreamingHTTP)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := streamer.GetStream(ctx, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Headers["Content-Disposition"] == "" {
		t.Fatal("stream response did not expose headers before body completion")
	}
	await(t, bodyStarted, "stream body to start")
	prefix := make([]byte, len("chunk"))
	if _, err := io.ReadFull(resp.Body, prefix); err != nil {
		t.Fatal(err)
	}
	if string(prefix) != "chunk" {
		t.Fatalf("stream prefix = %q", prefix)
	}
	cancel()
	_ = resp.Close()
	await(t, disconnected, "canceled stream to close server request")
}

func TestGetStreamCloseCanRaceWithBodyRead(t *testing.T) {
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(disconnected)
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()
	resp, err := client.(transport.StreamingHTTP).GetStream(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(readDone)
	}()
	if err := resp.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-readDone:
	case <-time.After(3 * time.Second):
		t.Fatal("closing response did not unblock its body reader")
	}
	await(t, disconnected, "concurrently closed response to cancel server")
}
