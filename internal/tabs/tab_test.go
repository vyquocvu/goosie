package tabs

import (
	"context"
	"testing"
)

func TestNavControllerIndependent(t *testing.T) {
	tab1 := newTab(1)
	tab2 := newTab(2)

	if &tab1.Nav == &tab2.Nav {
		t.Fatal("tabs share nav controller")
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	tab1.Nav.Cancel = cancel1
	tab1.Nav.Serial = 1

	ctx2, cancel2 := context.WithCancel(context.Background())
	tab2.Nav.Cancel = cancel2
	tab2.Nav.Serial = 2

	tab1.Nav.Cancel()

	if ctx1.Err() == nil {
		t.Fatal("tab1 context not cancelled")
	}
	if ctx2.Err() != nil {
		t.Fatal("tab2 context cancelled when tab1 cancelled")
	}
}

func TestNavControllerSerial(t *testing.T) {
	tab := newTab(1)

	if tab.Nav.Serial != 0 {
		t.Fatalf("initial serial = %d, want 0", tab.Nav.Serial)
	}

	tab.Nav.Serial++
	if tab.Nav.Serial != 1 {
		t.Fatalf("serial = %d, want 1", tab.Nav.Serial)
	}
}
