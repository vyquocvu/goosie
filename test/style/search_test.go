package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestSearchElementDefaultBlock(t *testing.T) {
	st := styleFor(t, `<html><body><search><input type="text"></search></body></html>`, "", "search")
	if st == nil {
		t.Fatal("search style not found")
	}
	if st.Display != style.DisplayBlock {
		t.Errorf("search display = %v, want DisplayBlock (%v)", st.Display, style.DisplayBlock)
	}
}
