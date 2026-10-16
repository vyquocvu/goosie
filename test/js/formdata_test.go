package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestFormDataAppend(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('name', 'John');
		fd.append('age', '30');
		var name = fd.get('name');
		var age = fd.get('age');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("name")
	if val.Str != "John" {
		t.Errorf("name = %q, want %q", val.Str, "John")
	}
	val, _ = r.Global("age")
	if val.Str != "30" {
		t.Errorf("age = %q, want %q", val.Str, "30")
	}
}

func TestFormDataSet(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('key', 'old');
		fd.set('key', 'new');
		var val = fd.get('key');
		var all = fd.getAll('key');
		var len = all.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("val")
	if val.Str != "new" {
		t.Errorf("val = %q, want %q", val.Str, "new")
	}
	val, _ = r.Global("len")
	if val.Num != 1 {
		t.Errorf("getAll length = %v, want 1 (set replaces all)", val.Num)
	}
}

func TestFormDataHas(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('exists', 'yes');
		var hasExists = fd.has('exists');
		var hasMissing = fd.has('missing');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("hasExists")
	if !val.Bool {
		t.Error("has('exists') should be true")
	}
	val, _ = r.Global("hasMissing")
	if val.Bool {
		t.Error("has('missing') should be false")
	}
}

func TestFormDataDelete(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('a', '1');
		fd.append('b', '2');
		fd.delete('a');
		var a = fd.get('a');
		var b = fd.get('b');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("a")
	if val.Kind != js.KindNull {
		t.Errorf("a should be null after delete, got %v", val.Kind)
	}
	val, _ = r.Global("b")
	if val.Str != "2" {
		t.Errorf("b = %q, want %q", val.Str, "2")
	}
}

func TestFormDataGetAll(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('tag', 'a');
		fd.append('tag', 'b');
		fd.append('tag', 'c');
		var all = fd.getAll('tag');
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

func TestFormDataForEach(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('x', '1');
		fd.append('y', '2');
		var count = 0;
		fd.forEach(function(value, key) {
			count++;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("count")
	if val.Num != 2 {
		t.Errorf("forEach count = %v, want 2", val.Num)
	}
}

func TestFormDataOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new window.FormData();
		fd.append('test', 'value');
		var val = fd.get('test');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("val")
	if val.Str != "value" {
		t.Errorf("val = %q, want %q", val.Str, "value")
	}
}
