package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestSessionLayoutPassCounts(t *testing.T) {
	s := newFocusSession(t, `<div style="display:flex"><i>a</i></div>`)
	counts := s.LayoutPassCounts()
	if counts[layout.FCFlexRow] == 0 {
		t.Fatal("flex-row pass count = 0 after laying out a flex container")
	}
}
