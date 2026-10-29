package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestHTMLDirectionalityFirstStrongAndInheritance(t *testing.T) {
	doc := domtest.Parse(`<html dir="rtl"><body><div id="auto" dir="auto">123 <span dir="rtl">اختبر</span> abc</div><div id="latin" dir="auto">123 abc اختبر</div><bdi id="bdi">اختبر SomeText</bdi></body></html>`)
	auto := doc.ElementByID("auto")
	latin := doc.ElementByID("latin")
	bdi := doc.ElementByID("bdi")
	if got := auto.Directionality(); got != "ltr" {
		t.Errorf("dir=auto with only nested explicitly-directed Arabic text = %q, want ltr", got)
	}
	if got := latin.Directionality(); got != "ltr" {
		t.Errorf("dir=auto with first strong Latin text = %q, want ltr", got)
	}
	if got := bdi.Directionality(); got != "rtl" {
		t.Errorf("default bdi directionality = %q, want rtl", got)
	}
	if got := doc.Body.Directionality(); got != "rtl" {
		t.Errorf("inherited directionality = %q, want rtl", got)
	}
}

func TestAutoDirectionalityForAssignedSlotNodes(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host">اختبر</div></body></html>`)
	host := doc.ElementByID("host")
	sr := &dom.ShadowRoot{Host: host, Mode: "open", SlotAssignment: "named"}
	host.ShadowRoot = sr
	slot := doc.NewElement("slot")
	slot.SetAttribute("dir", "auto")
	sr.AppendChild(slot)
	if got := slot.Directionality(); got != "rtl" {
		t.Errorf("slot with Arabic assigned text directionality = %q, want rtl", got)
	}
	host.RemoveChild(host.FirstChild)
	if got := slot.Directionality(); got != "ltr" {
		t.Errorf("empty dir=auto slot directionality = %q, want ltr", got)
	}
}

func TestManualSlotAssignment(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host"><span id="item">اختبر</span></div></body></html>`)
	host := doc.ElementByID("host")
	item := doc.ElementByID("item")
	sr := &dom.ShadowRoot{Host: host, Mode: "open", SlotAssignment: "manual"}
	host.ShadowRoot = sr
	slot := doc.NewElement("slot")
	sr.AppendChild(slot)
	if got := len(slot.AssignedNodes()); got != 0 {
		t.Fatalf("manual slot initially has %d assigned nodes, want 0", got)
	}
	sr.ManualSlots = map[*dom.Node][]*dom.Node{slot: {item}}
	if assigned := slot.AssignedNodes(); len(assigned) != 1 || assigned[0] != item {
		t.Fatalf("assigned nodes = %v, want item", assigned)
	}
}
