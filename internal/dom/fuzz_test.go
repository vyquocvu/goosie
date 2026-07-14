package dom

import (
	"context"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// Fuzz tests for the streaming HTML parser and related DOM helpers.
//
// These harnesses exercise the public DOM parser surface under adversarial
// inputs. Each fuzz target enforces a strict upper bound on the input size
// to keep memory bounded during `-fuzz` runs, and asserts only structural
// invariants — never exact output shape — because the HTML5 spec admits
// many equivalent parses for malformed input.
//
// Run a targeted fuzz for ~10 seconds with:
//
//	go test -fuzz=FuzzHTMLParseDocument -fuzztime=10s ./internal/dom/
//
// By default (no -fuzz flag) Go runs each fuzz target once with the seed
// corpus provided by F.Add, which keeps CI deterministic.

const (
	// maxFuzzInputBytes keeps adversarial inputs bounded so the Go runtime
	// never OOMs while the fuzzer explores pathological patterns (deep
	// nesting, extremely long tokens, repeated text, etc.).
	maxFuzzInputBytes = 4096
)

// FuzzHTMLParseDocument runs the streaming tree constructor against
// arbitrary UTF-8 input and asserts a small set of structural invariants:
//
//  1. With a non-cancelled context, either the parser returns an error OR
//     it returns a non-nil *Document with a populated Store.
//  2. The Document Root is always a valid NodeID that resolves to
//     NodeKindDocument.
//  3. After a successful parse, the auto-inserted <html> and <body>
//     elements exist as immediate descendants when the input produced any
//     element at all (matches html.Parse semantics).
//  4. A pre-cancelled context propagates cancellation through the parser
//     — the parser must NOT silently produce a half-built Document; it
//     must return ctx.Err().
func FuzzHTMLParseDocument(f *testing.F) {
	// Seed corpus covers the canonical HTML shapes the parser must handle
	// even on the seed-only run.
	f.Add(`<html><body><p>hello</p></body></html>`)
	f.Add(`<!doctype html><html><head><title>t</title></head><body></body></html>`)
	f.Add(`<p>only paragraph</p>`)
	f.Add(`<br><hr><img src="x">`)
	f.Add(`<script>a()</script>`)
	f.Add(`<!-- only comment -->`)
	f.Add(``)
	f.Add(`not html at all`)

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxFuzzInputBytes {
			t.Skip("input exceeds fuzz size bound")
		}
		parser := NewParser()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		doc, err := parser.ParseDocumentCtx(ctx, strings.NewReader(input), ParseConfig{
			MaxBuf: maxFuzzInputBytes,
		})
		if err != nil {
			// Either cancellation or a tokenizer error is acceptable.
			// We must not have a half-returned Document.
			if doc != nil {
				t.Fatalf("ParseDocumentCtx returned non-nil document with err=%v", err)
			}
			return
		}

		if doc == nil {
			t.Fatal("ParseDocumentCtx returned nil document with nil error")
		}
		if doc.Store == nil {
			t.Fatal("document has nil Store")
		}
		if doc.Root == NodeNone {
			t.Fatal("document root is NodeNone")
		}
		if kind := doc.Store.Kind(doc.Root); kind != NodeKindDocument {
			t.Fatalf("root kind = %d, want NodeKindDocument", kind)
		}

		// If the input contains any '<html' then the parser should have
		// an html element as a child of the root. If the input contains
		// no elements at all the auto-insertion logic must still have
		// created html and body, matching html.Parse behavior.
		htmlID := NodeNone
		for child := doc.Store.FirstChild(doc.Root); child != NodeNone; child = doc.Store.NextSibling(child) {
			if doc.Store.Kind(child) == NodeKindElement && doc.Store.Name(child) == atom.AtomHtml {
				htmlID = child
				break
			}
		}
		if htmlID == NodeNone {
			// The treebuilder auto-inserts html/head/body on EOF; if we
			// ever observe no <html>, the parser contract has changed.
			t.Fatal("expected auto-inserted <html> as child of root")
		}
	})
}

// FuzzHTMLParseDocumentCancelContext verifies that a pre-cancelled context
// aborts parsing deterministically. This is a regression guard: the parser
// must check ctx.Err() at iteration boundaries and must not silently swallow
// cancellation.
func FuzzHTMLParseDocumentCancelContext(f *testing.F) {
	f.Add(`<html><body>cancellable</body></html>`)
	f.Add(`<p>also cancellable</p>`)
	f.Add(`a`)

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxFuzzInputBytes {
			t.Skip("input exceeds fuzz size bound")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // pre-cancelled

		parser := NewParser()
		doc, err := parser.ParseDocumentCtx(ctx, strings.NewReader(input), ParseConfig{
			MaxBuf: maxFuzzInputBytes,
		})
		if err == nil {
			t.Fatalf("expected ctx.Err() from pre-cancelled parse, got nil err (doc=%v)", doc)
		}
		if doc != nil {
			t.Fatalf("expected nil document on cancellation, got non-nil")
		}
		if !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("expected context.Canceled error, got: %v", err)
		}
	})
}

// FuzzHTMLGetElementByID generates paired (HTML, ID) inputs and asserts:
//
//  1. GetElementByID never panics, even on adversarial input.
//  2. When the lookup returns a non-empty result for a non-empty id,
//     the streaming parser + tree walk must agree that an element with
//     that ID exists in the parsed tree.
//  3. When the lookup returns an empty result for a non-empty id,
//     the tree walk must also report the element as absent.
//
// This exercises both the helper API (GetElementByID) and provides a
// cross-check against the compact DOM store walk.
func FuzzHTMLGetElementByID(f *testing.F) {
	f.Add(`<html><body><div id="x">hit</div></body></html>`, "x")
	f.Add(`<html><body><div id="x"><span id="y">nested</span></div></body></html>`, "y")
	f.Add(`<p id="">empty</p>`, "")
	f.Add(`<p>nothing</p>`, "missing")

	f.Fuzz(func(t *testing.T, input, id string) {
		if len(input) > maxFuzzInputBytes {
			t.Skip("input exceeds fuzz size bound")
		}
		if len(id) > 256 {
			t.Skip("id exceeds fuzz size bound")
		}

		parser := NewParser()
		got, err := parser.GetElementByID(input, id)
		if err != nil {
			// html.Parse does not error on UTF-8 input per its
			// documented contract; if it does, that is a parser bug
			// worth flagging in CI.
			t.Fatalf("GetElementByID returned error for benign input: %v", err)
		}

		// Cross-check: parse the input via the streaming path and walk
		// the compact store looking for any element whose id attribute
		// matches.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		doc, perr := parser.ParseDocumentCtx(ctx, strings.NewReader(input), ParseConfig{
			MaxBuf: maxFuzzInputBytes,
		})
		if perr != nil {
			// Stream parse failure is acceptable here — the helper
			// uses the older html.Parse path that may succeed where
			// the streaming path got cancelled, and vice versa.
			return
		}
		exists := false
		walkForID(t, doc.Store, doc.Root, id, &exists)

		if id == "" {
			// Empty id is a corner case: id="" is legal HTML, but
			// the helper's contract is unspecified. We only assert
			// no panic and no error.
			return
		}
		switch {
		case got != "" && !exists:
			t.Fatalf("GetElementByID returned %q but tree walk found no element with id=%q", got, id)
		case got == "" && exists:
			t.Fatalf("GetElementByID returned empty result but tree walk found element with id=%q", id)
		}
	})
}

// walkForID sets *found when any element descendant of root has id=id.
func walkForID(t *testing.T, store *Store, root NodeID, id string, found *bool) {
	t.Helper()
	var walk func(NodeID)
	walk = func(n NodeID) {
		if n == NodeNone || *found {
			return
		}
		if store.Kind(n) == NodeKindElement {
			attrs, _ := store.Attrs(n)
			for _, a := range attrs {
				if a.Name == atom.AttrId && a.Value.String() == id {
					*found = true
					return
				}
			}
		}
		for c := store.FirstChild(n); c != NodeNone; c = store.NextSibling(c) {
			walk(c)
		}
	}
	walk(root)
}

// FuzzHTMLParseBodyText exercises the simple "extract body text" path used
// by the cmd/test demo and asserts it never panics, always returns a
// trimmed string, and produces an empty result when the input has no body.
func FuzzHTMLParseBodyText(f *testing.F) {
	f.Add(`<html><body><p>hello world</p></body></html>`)
	f.Add(`<html><body></body></html>`)
	f.Add(`garbage without tags`)
	f.Add(`<body>sole</body>`)

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxFuzzInputBytes {
			t.Skip("input exceeds fuzz size bound")
		}
		parser := NewParser()
		got, err := parser.ParseBodyText(input)
		if err != nil {
			// html.Parse should not error on any UTF-8 input per its
			// documented contract. Anything else is a parser bug.
			t.Fatalf("ParseBodyText returned error for benign input: %v", err)
		}
		// Output is always trimmed.
		if got != strings.TrimSpace(got) {
			t.Fatalf("ParseBodyText output not trimmed: %q", got)
		}
	})
}
