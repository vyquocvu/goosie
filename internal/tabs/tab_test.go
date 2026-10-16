package tabs

import (
	"context"
	"sync"
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

// TestConcurrentTabAccessIsRaceFree hammers the pump/drain/Present sharing:
// tab switches and collection reads, field writes and reads, and history ops
// from several goroutines at once. Bare fields tripped the detector here
// before privatization.
func TestConcurrentTabAccessIsRaceFree(t *testing.T) {
	mgr := NewManager(nil)
	a := mgr.NewTab()
	b := mgr.NewTab()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				switch i {
				case 0:
					mgr.SwitchTo(a.ID())
					mgr.SwitchTo(b.ID())
					_ = mgr.Active()
					_ = mgr.Tabs()
					_ = mgr.Count()
				case 1:
					a.SetURL("https://example.com/page")
					a.SetTitle("Example")
					a.SetScrollY(int32(j))
					a.SetLoading(j%2 == 0)
					_ = a.URL()
					_ = a.Title()
					_ = a.ScrollY()
					_ = a.Loading()
				case 2:
					b.SetURL("https://example.org/other")
					b.SetError("boom")
					b.SetBGColor(1)
					_ = b.URL()
					_ = b.Error()
					_ = b.BGColor()
				case 3:
					a.History().Push("https://example.com/")
					_, _, _ = a.History().PeekBack()
					b.History().Push("https://example.org/")
					_, _ = b.History().Entries()
				}
			}
		}(i)
	}
	wg.Wait()
}
