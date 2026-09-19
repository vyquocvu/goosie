package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// A story byline is the shape that broke this: an inline-block holding block
// content - a list of tag pills - sitting between two runs of text on one line.
// Laid out as a row box it started at the content edge again and drew over the
// title, and the text after it wrapped onto a line of its own.
const bylineHTML = `<html><body style="margin:0">
	<article>
	<span><a>Go nineteen release notes</a></span>
	<ul style="display:inline-block;margin:0 4px;padding:0"><li style="display:inline-block;margin-right:4px"><a>a11y</a></li><li style="display:inline-block;margin-right:4px"><a>programming</a></li></ul>
	<em>go.dev</em>
	<footer>posted by someone</footer>
	</article>
</body></html>`

func TestInlineBlockSharesItsLineWithText(t *testing.T) {
	arena := session(t, bylineHTML, 800)
	layout.Inline(arena, layout.ObjectID(1))

	title := findWordObject(arena, "notes")
	first := findWordObject(arena, "Go")
	tags := findByTag(arena, "ul")
	pills := findAllByTag(arena, "li")
	domain := findWordObject(arena, "go.dev")
	byline := findByTag(arena, "footer")
	if first == nil || title == nil || tags == nil || domain == nil || byline == nil || len(pills) != 2 {
		t.Fatalf("missing boxes: title=%v tags=%v pills=%d domain=%v byline=%v",
			title != nil, tags != nil, len(pills), domain != nil, byline != nil)
	}

	// The pills are inline-level boxes in the list, so they sit side by side and
	// the list shrink-wraps around them rather than one per line.
	if pills[1].Y != pills[0].Y {
		t.Errorf("tag pills stacked: first Y=%g, second Y=%g", pills[0].Y, pills[1].Y)
	}
	if !(pills[1].X > pills[0].X+pills[0].W) {
		t.Errorf("second pill X=%g did not clear the first at %g", pills[1].X, pills[0].X+pills[0].W)
	}

	// Title, pills and domain are one line: the list joins the text flow after
	// the title instead of restarting at the content edge. An atomic inline-block
	// sits on the line's bottom edge rather than at the text's top, so the test
	// asks only that the two boxes share the line's band.
	if !(tags.Y < title.Y+title.H && tags.Y+tags.H > title.Y) {
		t.Errorf("tag list band %g..%g missed the title's line at %g..%g",
			tags.Y, tags.Y+tags.H, title.Y, title.Y+title.H)
	}
	if !(tags.X > title.X+title.W) {
		t.Errorf("tag list X=%g did not clear the title ending at %g", tags.X, title.X+title.W)
	}
	if domain.Y != title.Y {
		t.Errorf("domain Y=%g wrapped off the title's line at %g", domain.Y, title.Y)
	}
	if !(domain.X > tags.X+tags.W) {
		t.Errorf("domain X=%g did not clear the tag list ending at %g", domain.X, tags.X+tags.W)
	}

	// The block footer still takes a slot of its own below the line.
	if byline.Y < tags.Y+tags.H {
		t.Errorf("byline Y=%g overlaps the tag list ending at %g", byline.Y, tags.Y+tags.H)
	}
}

// A container of nothing but inline-blocks keeps flowing as a row: the inline
// pass owns the line only once there is text to interleave with.
func TestInlineBlockRowStillFlowsWithoutText(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
		<section><div style="display:inline-block;width:100px">one</div><div style="display:inline-block;width:100px">two</div></section>
	</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if divs[0].Y != divs[1].Y {
		t.Fatalf("row broke: first Y=%g, second Y=%g", divs[0].Y, divs[1].Y)
	}
	if !almostEqual(divs[1].X, 100) {
		t.Errorf("second box X=%g, want 100", divs[1].X)
	}
}
