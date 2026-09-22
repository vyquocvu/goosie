package tabs

import (
	"context"
	"testing"
)

func TestNavControllerIndependent(t *testing.T) {
	tab1 := newTab(1)
	tab2 := newTab(2)

	if &tab1.nav == &tab2.nav {
		t.Fatal("tabs share nav controller")
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	tab1.nav.cancel = cancel1
	tab1.nav.serial = 1

	ctx2, cancel2 := context.WithCancel(context.Background())
	tab2.nav.cancel = cancel2
	tab2.nav.serial = 2

	tab1.nav.cancel()

	if ctx1.Err() == nil {
		t.Fatal("tab1 context not cancelled")
	}
	if ctx2.Err() != nil {
		t.Fatal("tab2 context cancelled when tab1 cancelled")
	}
}

func TestNavControllerSerial(t *testing.T) {
	tab := newTab(1)

	if tab.nav.serial != 0 {
		t.Fatalf("initial serial = %d, want 0", tab.nav.serial)
	}

	tab.nav.serial++
	if tab.nav.serial != 1 {
		t.Fatalf("serial = %d, want 1", tab.nav.serial)
	}
}
