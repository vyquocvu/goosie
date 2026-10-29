package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestHTMLDoctypeModeAndIdentifiers(t *testing.T) {
	cases := []struct {
		name, source, doctype, publicID, systemID string
		quirks, limited                           bool
	}{
		{name: "modern doctype", source: "<!doctype html>", doctype: "html"},
		{name: "truncated public keyword still emits doctype", source: "<!DOCTYPE html pu", doctype: "html", quirks: true},
		{name: "empty identifiers stay standards", source: `<!doctype html PUBLIC "" "">`, doctype: "html", publicID: "", systemID: ""},
		{name: "missing doctype", source: "<p>x</p>", quirks: true},
		{name: "legacy transitional without system id", source: `<!doctype html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">`, doctype: "html", publicID: "-//W3C//DTD HTML 4.01 Transitional//EN", quirks: true},
		{name: "legacy transitional with system id", source: `<!doctype html PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN" "http://www.w3.org/TR/html4/loose.dtd">`, doctype: "html", publicID: "-//W3C//DTD HTML 4.01 Transitional//EN", systemID: "http://www.w3.org/TR/html4/loose.dtd", limited: true},
	}
	limits := dom.ParseLimits{Nodes: 256, Depth: 32, Attributes: 64, AttributeBytes: 4096}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := dom.ParseBounded(tc.source, limits)
			if err != nil {
				t.Fatal(err)
			}
			if doc.QuirksMode != tc.quirks || doc.LimitedQuirksMode != tc.limited {
				t.Fatalf("modes = quirks:%v limited:%v; want quirks:%v limited:%v", doc.QuirksMode, doc.LimitedQuirksMode, tc.quirks, tc.limited)
			}
			if tc.doctype == "" {
				if doc.DocType != nil {
					t.Fatalf("unexpected doctype %q", doc.DocType.DataContent)
				}
				return
			}
			if doc.DocType == nil || doc.DocType.DoctypeName != tc.doctype || doc.DocType.PublicID != tc.publicID || doc.DocType.SystemID != tc.systemID {
				t.Fatalf("doctype = %#v; want name=%q public=%q system=%q", doc.DocType, tc.doctype, tc.publicID, tc.systemID)
			}
		})
	}
}

func TestEmptyHTMLDoctypeName(t *testing.T) {
	for _, source := range []string{"<!DOCTYPE >", "<!DOCTYPE", "<!DOCTYPE\n>"} {
		doc, err := dom.ParseBounded(source, dom.ParseLimits{
			Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 256,
		})
		if err != nil {
			t.Fatalf("parse %q: %v", source, err)
		}
		if doc.DocType == nil || doc.DocType.DoctypeName != "" {
			t.Errorf("parse %q doctype = %#v, want empty doctype name", source, doc.DocType)
		}
	}
}
