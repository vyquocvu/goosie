package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestExclusiveAccordionDetailsGrouping(t *testing.T) {
	doc := domtest.Parse(`<html><body>
<details id="d1" name="faq" open>
  <summary>Q1</summary>
  <p>Answer 1</p>
</details>
<details id="d2" name="faq">
  <summary>Q2</summary>
  <p>Answer 2</p>
</details>
<details id="d3" name="other">
  <summary>Q3</summary>
  <p>Answer 3</p>
</details>
</body></html>`)

	d1 := doc.ElementByID("d1")
	d2 := doc.ElementByID("d2")
	d3 := doc.ElementByID("d3")

	if !d1.HasAttribute("open") {
		t.Fatal("d1 should initially be open")
	}
	if d2.HasAttribute("open") {
		t.Fatal("d2 should initially be closed")
	}

	// Open d2: since d1 and d2 share name="faq", opening d2 must close d1!
	d2.SetAttribute("open", "")

	if !d2.HasAttribute("open") {
		t.Errorf("d2 should be open")
	}
	if d1.HasAttribute("open") {
		t.Errorf("d1 should have been automatically closed when d2 opened")
	}

	// d3 is in group "other", so opening d3 should not close d2
	d3.SetAttribute("open", "")
	if !d3.HasAttribute("open") {
		t.Errorf("d3 should be open")
	}
	if !d2.HasAttribute("open") {
		t.Errorf("d2 should still be open because d3 is in a different name group")
	}
}

func TestExclusiveAccordionToggleEvent(t *testing.T) {
	doc := domtest.Parse(`<html><body>
<details id="d1" name="accordion" open>
  <summary>Item 1</summary>
</details>
<details id="d2" name="accordion">
  <summary>Item 2</summary>
</details>
</body></html>`)

	d1 := doc.ElementByID("d1")
	d2 := doc.ElementByID("d2")

	d1ClosedDispatched := false
	dom.AddEventListener(d1, "toggle", func(e *dom.Event) {
		d1ClosedDispatched = true
	}, false)

	d2.SetAttribute("open", "")

	if !d1ClosedDispatched {
		t.Errorf("d1 should have dispatched a toggle event when closed by d2")
	}
}
