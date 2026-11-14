package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/session"
)

func TestSessionSnapshotterJournalsOnInterval(t *testing.T) {
	b := newBrowserE2E(t)
	tab := b.f.tabMgr.Active()
	tab.SetURL("https://periodic.example/")
	tab.SetTitle("Periodic")

	path := b.f.config.sessionActiveFile
	if path == "" {
		t.Fatal("test config has no crash-journal path")
	}
	b.f.sessionSnapshotEvery = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.f.runSessionSnapshots(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		st, err := session.Load(path)
		if err == nil && len(st.Tabs) == 1 && st.Tabs[0].URL == "https://periodic.example/" {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("journal was not written on the snapshot cadence: state=%+v err=%v", st, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("snapshotter did not stop when its context was cancelled")
	}
}

func TestSessionSnapshotterSkipsPrivateRuns(t *testing.T) {
	b := newBrowserE2E(t)
	b.f.config.private = true
	b.f.sessionSnapshotEvery = time.Millisecond
	path := b.f.config.sessionActiveFile
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.f.runSessionSnapshots(ctx)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		st, loadErr := session.Load(path)
		t.Fatalf("private run wrote the journal: %+v, %v", st, loadErr)
	}
}
