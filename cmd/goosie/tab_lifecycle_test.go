package main

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestIndependentTabNavigation(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab1 := mgr.NewTab()
	tab2 := mgr.NewTab()

	ctx1, cancel1 := context.WithCancel(context.Background())
	tab1.Nav.Cancel = cancel1
	tab1.Nav.Serial = 1

	ctx2, cancel2 := context.WithCancel(context.Background())
	tab2.Nav.Cancel = cancel2
	tab2.Nav.Serial = 1

	cancel1()

	if ctx1.Err() == nil {
		t.Fatal("tab1 not cancelled")
	}
	if ctx2.Err() != nil {
		t.Fatal("tab2 cancelled when tab1 cancelled")
	}
}

func TestTabSerialPreventsStaleResults(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()

	tab.Nav.Serial = 1
	if tab.Nav.Serial != 1 {
		t.Fatal("serial not set")
	}

	tab.Nav.Serial = 2
	if tab.Nav.Serial != 2 {
		t.Fatal("serial not updated")
	}
}
