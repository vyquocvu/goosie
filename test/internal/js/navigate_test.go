package js_test

import (
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestRuntimeNavigate_hrefAssignment(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetHTMLContent("<html><body></body></html>")

	navigated := make(chan string, 1)
	rt.OnNavigate = func(url string) {
		select {
		case navigated <- url:
		default:
		}
	}

	_, err := rt.RunScript(`window.location.href = "https://example.com/assigned"`)
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	select {
	case got := <-navigated:
		if got != "https://example.com/assigned" {
			t.Fatalf("navigated to %q, want %q", got, "https://example.com/assigned")
		}
	case <-time.After(time.Second):
		t.Fatal("OnNavigate not called within 1s")
	}

	href, err := rt.RunScript(`window.location.href`)
	if err != nil {
		t.Fatalf("read href: %v", err)
	}
	if href.String() != "https://example.com/assigned" {
		t.Fatalf("href = %q, want %q", href.String(), "https://example.com/assigned")
	}
}

func TestRuntimeNavigate_locationAssign(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetHTMLContent("<html><body></body></html>")

	navigated := make(chan string, 1)
	rt.OnNavigate = func(url string) {
		select {
		case navigated <- url:
		default:
		}
	}

	_, err := rt.RunScript(`window.location.assign("https://example.com/assign")`)
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	select {
	case got := <-navigated:
		if got != "https://example.com/assign" {
			t.Fatalf("navigated to %q, want %q", got, "https://example.com/assign")
		}
	case <-time.After(time.Second):
		t.Fatal("OnNavigate not called within 1s")
	}
}

func TestRuntimeNavigate_locationReplace(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetHTMLContent("<html><body></body></html>")

	navigated := make(chan string, 1)
	rt.OnNavigate = func(url string) {
		select {
		case navigated <- url:
		default:
		}
	}

	_, err := rt.RunScript(`window.location.replace("https://example.com/replace")`)
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}

	select {
	case got := <-navigated:
		if got != "https://example.com/replace" {
			t.Fatalf("navigated to %q, want %q", got, "https://example.com/replace")
		}
	case <-time.After(time.Second):
		t.Fatal("OnNavigate not called within 1s")
	}
}

func TestRuntimeNavigate_noCallbackNoCrash(t *testing.T) {
	rt := js.NewRuntime()
	rt.SetHTMLContent("<html><body></body></html>")
	_, err := rt.RunScript(`window.location.href = "https://example.com"`)
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
}