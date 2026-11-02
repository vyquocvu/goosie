package net

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetStreamRangeSendsRangeHeaderAndReturnsPartialResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=5-" {
			t.Errorf("Range header = %q, want bytes=5-", got)
		}
		w.Header().Set("Content-Range", "bytes 5-10/11")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, " world")
	}))
	defer server.Close()

	client := DefaultClient()
	defer client.Close()
	ranged, ok := client.(RangeStreamingHTTP)
	if !ok {
		t.Fatal("default client does not implement RangeStreamingHTTP")
	}
	res, err := ranged.GetStreamRange(context.Background(), server.URL+"/file.bin", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != " world" {
		t.Fatalf("body = %q, want \" world\"", body)
	}
}

func TestGetStreamRangeRejectsNegativeOffset(t *testing.T) {
	client := DefaultClient()
	defer client.Close()
	ranged := client.(RangeStreamingHTTP)
	if _, err := ranged.GetStreamRange(context.Background(), "http://example.com/", -1); err == nil {
		t.Fatal("negative range offset was accepted")
	}
}
