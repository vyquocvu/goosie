package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// qc:BR-124
func TestURLConstructor(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var u = new URL('https://user:pass@example.com:8080/path?q=1#hash');
		var protocol = u.protocol;
		var hostname = u.hostname;
		var port = u.port;
		var pathname = u.pathname;
		var search = u.search;
		var hash = u.hash;
		var username = u.username;
		var password = u.password;
		var origin = u.origin;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	tests := []struct {
		name string
		want string
	}{
		{"protocol", "https:"},
		{"hostname", "example.com"},
		{"port", "8080"},
		{"pathname", "/path"},
		{"search", "?q=1"},
		{"hash", "#hash"},
		{"username", "user"},
		{"password", "pass"},
		{"origin", "https://example.com:8080"},
	}

	for _, tt := range tests {
		val, ok := r.Global(tt.name)
		if !ok {
			t.Fatalf("%s not found", tt.name)
		}
		if val.Str != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, val.Str, tt.want)
		}
	}
}

// qc:BR-124
func TestURLWithBase(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var u = new URL('/path', 'https://example.com');
		var href = u.href;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("href")
	if !ok {
		t.Fatal("href not found")
	}
	if val.Str != "https://example.com/path" {
		t.Errorf("href = %q, want %q", val.Str, "https://example.com/path")
	}
}

// qc:BR-124
func TestURLSearchParams(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var sp = new URLSearchParams('?a=1&b=2&a=3');
		var a = sp.get('a');
		var b = sp.get('b');
		var c = sp.get('c');
		var hasA = sp.has('a');
		var hasC = sp.has('c');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("a")
	if val.Str != "1" {
		t.Errorf("a = %q, want %q", val.Str, "1")
	}
	val, _ = r.Global("b")
	if val.Str != "2" {
		t.Errorf("b = %q, want %q", val.Str, "2")
	}
	val, _ = r.Global("c")
	if val.Kind != js.KindNull {
		t.Errorf("c = %v, want null", val.Kind)
	}
	val, _ = r.Global("hasA")
	if !val.Bool {
		t.Error("hasA should be true")
	}
	val, _ = r.Global("hasC")
	if val.Bool {
		t.Error("hasC should be false")
	}
}

func TestURLSearchParamsMutate(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var sp = new URLSearchParams();
		sp.append('a', '1');
		sp.append('b', '2');
		sp.set('a', '10');
		sp.delete('b');
		var a = sp.get('a');
		var b = sp.get('b');
		var str = sp.toString();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("a")
	if val.Str != "10" {
		t.Errorf("a = %q, want %q", val.Str, "10")
	}
	val, _ = r.Global("b")
	if val.Kind != js.KindNull {
		t.Errorf("b should be null after delete")
	}
	val, _ = r.Global("str")
	if val.Str != "a=10" {
		t.Errorf("toString = %q, want %q", val.Str, "a=10")
	}
}

func TestURLSearchParamsGetAll(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var sp = new URLSearchParams('a=1&a=2&a=3');
		var all = sp.getAll('a');
		var len = all.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("len")
	if val.Num != 3 {
		t.Errorf("getAll length = %v, want 3", val.Num)
	}
}

func TestURLSearchParamsOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var sp = new window.URLSearchParams('x=1');
		var x = sp.get('x');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("x")
	if val.Str != "1" {
		t.Errorf("x = %q, want %q", val.Str, "1")
	}
}

func TestURLSearchParamsSort(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var sp = new URLSearchParams('c=3&a=1&b=2');
		sp.sort();
		var str = sp.toString();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("str")
	if val.Str != "a=1&b=2&c=3" {
		t.Errorf("sorted = %q, want %q", val.Str, "a=1&b=2&c=3")
	}
}

// qc:BR-124
func TestURLSearchParamsSpecialCharactersRoundTrip(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var single = new URLSearchParams();
		single.append('only', 'a b&c=d');
		var singleSerialized = single.toString();
		var singleParsed = new URLSearchParams(singleSerialized).get('only');

		var multi = new URLSearchParams();
		multi.append('q', 'a b&c=d');
		multi.append('name', 'Nguyễn Văn A');
		var multiSerialized = multi.toString();
		var multiParsed = new URLSearchParams(multiSerialized);
		var multiQ = multiParsed.get('q');
		var multiName = multiParsed.get('name');
		var paramCount = multiSerialized.split('&').length;

		var fromURL = new URL('https://example.com/s?q=a+b%26c%3Dd').searchParams.get('q');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if val, _ := r.Global("singleSerialized"); val.Str != "only=a+b%26c%3Dd" {
		t.Errorf("serialized = %q, want %q", val.Str, "only=a+b%26c%3Dd")
	}
	if val, _ := r.Global("singleParsed"); val.Str != "a b&c=d" {
		t.Errorf("parsed = %q, want %q", val.Str, "a b&c=d")
	}
	if val, _ := r.Global("multiQ"); val.Str != "a b&c=d" {
		t.Errorf("multi q = %q, want %q", val.Str, "a b&c=d")
	}
	if val, _ := r.Global("multiName"); val.Str != "Nguyễn Văn A" {
		t.Errorf("multi name = %q, want %q", val.Str, "Nguyễn Văn A")
	}
	if val, _ := r.Global("paramCount"); val.Num != 2 {
		t.Errorf("paramCount = %v, want 2 (special characters must not create parameters)", val.Num)
	}
	if val, _ := r.Global("fromURL"); val.Str != "a b&c=d" {
		t.Errorf("URL query = %q, want %q", val.Str, "a b&c=d")
	}
}
