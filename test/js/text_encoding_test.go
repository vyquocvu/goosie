package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestTextEncoder(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var enc = new TextEncoder();
		var encoded = enc.encode('Hello');
		var len = encoded.length;
		var first = encoded[0];
		var last = encoded[4];
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("len")
	if val.Num != 5 {
		t.Errorf("length = %v, want 5", val.Num)
	}
	val, _ = r.Global("first")
	if val.Num != 72 { // 'H'
		t.Errorf("first byte = %v, want 72", val.Num)
	}
	val, _ = r.Global("last")
	if val.Num != 111 { // 'o'
		t.Errorf("last byte = %v, want 111", val.Num)
	}
}

func TestTextEncoderEncoding(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var enc = new TextEncoder();
		var encoding = enc.encoding;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("encoding")
	if val.Str != "utf-8" {
		t.Errorf("encoding = %q, want %q", val.Str, "utf-8")
	}
}

func TestTextEncoderUnicode(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var enc = new TextEncoder();
		var encoded = enc.encode('Hello 世界');
		var len = encoded.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("len")
	// "Hello " = 6 bytes, "世界" = 6 bytes (3 each in UTF-8)
	if val.Num != 12 {
		t.Errorf("length = %v, want 12", val.Num)
	}
}

func TestTextDecoder(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var dec = new TextDecoder();
		var bytes = [72, 101, 108, 108, 111];
		var decoded = dec.decode(bytes);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("decoded")
	if val.Str != "Hello" {
		t.Errorf("decoded = %q, want %q", val.Str, "Hello")
	}
}

func TestTextDecoderEncoding(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var dec = new TextDecoder('utf-16le');
		var encoding = dec.encoding;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("encoding")
	if val.Str != "utf-16le" {
		t.Errorf("encoding = %q, want %q", val.Str, "utf-16le")
	}
}

func TestTextDecoderUnicode(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var dec = new TextDecoder();
		// UTF-8 bytes for "世界"
		var bytes = [228, 184, 150, 231, 149, 140];
		var decoded = dec.decode(bytes);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("decoded")
	if val.Str != "世界" {
		t.Errorf("decoded = %q, want %q", val.Str, "世界")
	}
}

func TestTextEncoderDecoderRoundTrip(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var enc = new TextEncoder();
		var dec = new TextDecoder();
		var original = 'Hello, 世界! 🌍';
		var encoded = enc.encode(original);
		var decoded = dec.decode(encoded);
		var match = original === decoded;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("match")
	if !val.Bool {
		t.Error("round-trip failed: encoded then decoded does not match original")
	}
}

func TestTextEncoderOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var enc = new window.TextEncoder();
		var encoded = enc.encode('test');
		var len = encoded.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("len")
	if val.Num != 4 {
		t.Errorf("length = %v, want 4", val.Num)
	}
}
