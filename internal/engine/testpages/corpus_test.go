package testpages

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestListIncludesLongArticleAndDocumentationPages(t *testing.T) {
	pages := List()
	if len(pages) != 5 {
		t.Fatalf("List() returned %d pages, want 5", len(pages))
	}

	want := map[string]string{
		"long_article":  "Long Article",
		"documentation": "Documentation Page",
		"table_heavy":   "Table-Heavy Data Grid",
		"form_heavy":    "Form-Heavy Settings Page",
		"image_heavy":   "Image-Heavy Page",
	}
	for _, page := range pages {
		title, ok := want[page.Name]
		if !ok {
			t.Fatalf("unexpected page %q", page.Name)
		}
		if page.Title != title {
			t.Fatalf("page %q title = %q, want %q", page.Name, page.Title, title)
		}
		if page.HTMLBytes == 0 || page.CSSBytes == 0 {
			t.Fatalf("page %q reported empty byte counts: html=%d css=%d", page.Name, page.HTMLBytes, page.CSSBytes)
		}
		delete(want, page.Name)
	}
	for name := range want {
		t.Fatalf("missing page %q", name)
	}
}

func TestGetReturnsDeterministicPageContent(t *testing.T) {
	page, ok := Get("long_article")
	if !ok {
		t.Fatal("Get(long_article) did not find page")
	}

	if !strings.Contains(page.HTML, `<article id="long-article"`) {
		t.Fatalf("long article HTML does not contain article root")
	}
	if !strings.Contains(page.HTML, "Chapter 12") {
		t.Fatalf("long article HTML does not include expected terminal section")
	}
	if !strings.Contains(page.CSS, "article.long-form") {
		t.Fatalf("long article CSS does not include expected article selector")
	}

	again, ok := Get("long_article")
	if !ok {
		t.Fatal("second Get(long_article) did not find page")
	}
	if page.HTML != again.HTML || page.CSS != again.CSS {
		t.Fatal("page content changed between accesses")
	}
}

func TestGetRejectsUnknownPage(t *testing.T) {
	page, ok := Get("missing")
	if ok {
		t.Fatalf("Get(missing) returned ok with page %q", page.Name)
	}
}

func TestGetContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetContext(ctx, "documentation")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetContext canceled error = %v, want context.Canceled", err)
	}
}

func TestGetContextRejectsUnknownPage(t *testing.T) {
	_, err := GetContext(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetContext missing error = %v, want ErrNotFound", err)
	}
}

func TestDocumentationPageExercisesReferenceLayout(t *testing.T) {
	page, ok := Get("documentation")
	if !ok {
		t.Fatal("Get(documentation) did not find page")
	}

	for _, fragment := range []string{
		`<nav class="sidebar"`,
		`<main class="docs-content"`,
		`<pre><code>`,
		`<table>`,
		`<form class="feedback-form"`,
	} {
		if !strings.Contains(page.HTML, fragment) {
			t.Fatalf("documentation HTML missing %q", fragment)
		}
	}
	if !strings.Contains(page.CSS, ".docs-shell") {
		t.Fatalf("documentation CSS missing docs shell selector")
	}
}

func TestTableHeavyPageExercisesReferenceLayout(t *testing.T) {
	page, ok := Get("table_heavy")
	if !ok {
		t.Fatal("Get(table_heavy) did not find page")
	}

	for _, fragment := range []string{
		`<table`,
		`<thead>`,
		`<tbody>`,
		`<tfoot>`,
		`colspan=`,
		`rowspan=`,
		`<th`,
		`<td`,
	} {
		if !strings.Contains(page.HTML, fragment) {
			t.Fatalf("table_heavy HTML missing %q", fragment)
		}
	}
	if !strings.Contains(page.CSS, "table") {
		t.Fatalf("table_heavy CSS missing table selector")
	}
}

func TestFormHeavyPageExercisesReferenceLayout(t *testing.T) {
	page, ok := Get("form_heavy")
	if !ok {
		t.Fatal("Get(form_heavy) did not find page")
	}

	for _, fragment := range []string{
		`<form`,
		`<fieldset`,
		`<legend`,
		`<label`,
		`<input type="text"`,
		`<input type="email"`,
		`<input type="checkbox"`,
		`<input type="radio"`,
		`<select`,
		`<option`,
		`<textarea`,
		`<button type="submit"`,
	} {
		if !strings.Contains(page.HTML, fragment) {
			t.Fatalf("form_heavy HTML missing %q", fragment)
		}
	}
	if !strings.Contains(page.CSS, "form") {
		t.Fatalf("form_heavy CSS missing form selector")
	}
}

func TestImageHeavyPageExercisesReferenceLayout(t *testing.T) {
	page, ok := Get("image_heavy")
	if !ok {
		t.Fatal("Get(image_heavy) did not find page")
	}

	for _, fragment := range []string{
		`<div class="gallery"`,
		`<div class="gallery-item"`,
		`<img src="data:image/png;base64,`,
		`alt="Blue Image"`,
		`alt="Red Image"`,
	} {
		if !strings.Contains(page.HTML, fragment) {
			t.Fatalf("image_heavy HTML missing %q", fragment)
		}
	}
	if !strings.Contains(page.CSS, ".gallery") {
		t.Fatalf("image_heavy CSS missing gallery selector")
	}
}

func TestRepeatedListCallsAreStable(t *testing.T) {
	first := List()
	second := List()

	if len(first) != len(second) {
		t.Fatalf("List lengths differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("List entry %d changed: %#v vs %#v", i, first[i], second[i])
		}
	}
}
