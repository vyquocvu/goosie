package style_test

import (
	"testing"
)

func TestScrollAndTimelineProperties(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="scroll-timeline: --my-scroll-tl inline; view-timeline: --my-view-tl y; animation-timeline: --my-scroll-tl;">Content</div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.ScrollTimelineName != "--my-scroll-tl" {
		t.Errorf("ScrollTimelineName = %q, want --my-scroll-tl", st.ScrollTimelineName)
	}
	if st.ScrollTimelineAxis != "inline" {
		t.Errorf("ScrollTimelineAxis = %q, want inline", st.ScrollTimelineAxis)
	}
	if st.ViewTimelineName != "--my-view-tl" {
		t.Errorf("ViewTimelineName = %q, want --my-view-tl", st.ViewTimelineName)
	}
	if st.ViewTimelineAxis != "y" {
		t.Errorf("ViewTimelineAxis = %q, want y", st.ViewTimelineAxis)
	}
	if st.AnimationTimeline != "--my-scroll-tl" {
		t.Errorf("AnimationTimeline = %q, want --my-scroll-tl", st.AnimationTimeline)
	}
}
