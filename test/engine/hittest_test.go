package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
)

func TestHitTestLink(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;">
<a href="https://example.com">Click me</a>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	objs := sess.Arena.Objects
	var aIdx int = -1
	for i := range objs {
		if objs[i].Node != nil && objs[i].Node.Data == "a" {
			aIdx = i
			break
		}
	}
	if aIdx < 0 {
		t.Fatal("<a> element not found in arena")
	}

	var linkX, linkY float32
	found := false
	for i := range objs {
		x0, y0, x1, y1 := objs[i].BorderRect()
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		for p := &objs[i]; p != nil; p = parentLink(objs, p) {
			if p.Node != nil && p.Node.Data == "a" {
				linkX = (x0 + x1) / 2
				linkY = (y0 + y1) / 2
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("no descendant of <a> with geometry found")
	}

	href := sess.HitTestLink(linkX, linkY)
	if href != "https://example.com" {
		t.Fatalf("HitTestLink(%v, %v) = %q, want %q", linkX, linkY, href, "https://example.com")
	}

	miss := sess.HitTestLink(0, 5000)
	if miss != "" {
		t.Fatalf("HitTestLink(0, 5000) = %q, want empty", miss)
	}
}

func parentLink(objs []layout.Object, o *layout.Object) *layout.Object {
	if o.Parent == 0 || int(o.Parent) >= len(objs) {
		return nil
	}
	return &objs[o.Parent]
}

func TestHitTestLinkNested(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;">
<p><a href="/page"><span>Link text</span></a></p>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	objs := sess.Arena.Objects
	var hitX, hitY float32
	found := false
	for i := range objs {
		x0, y0, x1, y1 := objs[i].BorderRect()
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		hasSpan := false
		hasAnchor := false
		for p := &objs[i]; p != nil; p = parentLink(objs, p) {
			if p.Node != nil {
				if p.Node.Data == "span" {
					hasSpan = true
				}
				if p.Node.Data == "a" {
					hasAnchor = true
				}
			}
		}
		if hasSpan && hasAnchor {
			hitX = (x0 + x1) / 2
			hitY = (y0 + y1) / 2
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no descendant of <a><span> with geometry found")
	}

	href := sess.HitTestLink(hitX, hitY)
	if href != "/page" {
		t.Fatalf("HitTestLink on nested span = %q, want /page", href)
	}
}

func TestHitTestLinkNoArena(t *testing.T) {
	sess, err := engine.NewSession(`<html><body>hello</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	sess.Arena = &layout.Arena{}
	if href := sess.HitTestLink(10, 10); href != "" {
		t.Fatalf("HitTestLink with empty arena = %q, want empty", href)
	}
}
