package engine

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestLayoutStyleKeyDistinguishesFlowRoot(t *testing.T) {
	block := &style.ComputedStyle{Display: style.DisplayBlock}
	flowRoot := &style.ComputedStyle{Display: style.DisplayBlock, FlowRoot: true}
	if reflect.DeepEqual(makeLayoutStyleKey(block), makeLayoutStyleKey(flowRoot)) {
		t.Error("layoutStyleKey ignores FlowRoot; a flow-root toggle must be layout-affecting")
	}
}
