package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestHTMLTableCollectionsStayLiveAndUseDirectTableRelationships(t *testing.T) {
	doc := domtest.Parse(`<table id="table"><caption id="caption"></caption><thead id="head"><tr id="header"><th></th></tr></thead><tbody id="body"><tr id="row"><td></td><td></td></tr></tbody><tfoot id="foot"><tr></tr></tfoot><template><tr id="hidden-row"></tr></template></table>`)
	rt, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var table = document.getElementById("table");
		var section = document.getElementById("body");
		var row = document.getElementById("row");
		var template = document.querySelector("template");
		var captionMatches = table.caption.id === "caption";
		var headAndFootMatch = table.tHead.id === "head" && table.tFoot.id === "foot";
		var tableCountsMatch = table.rows.length === 3 && table.tBodies.length === 1;
		var rowsSkipTemplateContent = table.rows.item(2).firstChild === null;
		var templateParentMatches = template.parentNode === table;
		var sectionRowsMatch = section.rows.length === 1;
		var cellsMatch = row.cells.length === 2;
		row.appendChild(document.createElement("td"));
		var collectionsStayLive = row.cells.length === 3 && table.rows.length === 3;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		"captionMatches": true, "headAndFootMatch": true, "tableCountsMatch": true,
		"rowsSkipTemplateContent": true, "templateParentMatches": true,
		"sectionRowsMatch": true, "cellsMatch": true,
		"collectionsStayLive": true,
	} {
		value, ok := rt.Global(key)
		if !ok || value.Bool != want {
			t.Errorf("%s = %v (present %t), want %t", key, value, ok, want)
		}
	}
}

func TestGetElementsByTagNameDistinguishesHTMLAndSVGNames(t *testing.T) {
	doc := domtest.Parse(`<div id="root"><font id="html-font"></font><svg><font id="svg-font"/></svg></div>`)
	rt, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var root = document.getElementById("root");
		var upper = root.getElementsByTagName("FONT");
		var lower = root.getElementsByTagName("font");
		var upperHTML = upper.length === 1 && upper[0].namespaceURI === "http://www.w3.org/1999/xhtml";
		var lowerBoth = lower.length === 2 && lower[0].id === "html-font" &&
			lower[1].namespaceURI === "http://www.w3.org/2000/svg";
		var svgCase = root.getElementsByTagName("altGlyph").length === 0;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	for key := range map[string]bool{"upperHTML": true, "lowerBoth": true, "svgCase": true} {
		value, ok := rt.Global(key)
		if !ok || !value.Bool {
			t.Errorf("%s = %v (present %t), want true", key, value, ok)
		}
	}
}
