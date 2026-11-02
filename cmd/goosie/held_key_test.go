package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func keyTestButton(t *testing.T, html string) (*framePath, *engine.Session, *dom.Node) {
	t.Helper()
	f := newNavTestPath(t)
	tab := f.tabMgr.Active()
	sess, err := engine.NewSession(html, nil, 256)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	tab.SetSession(sess)
	button := documentNodeByID(&sess.Doc.Node, "go")
	if button == nil || !sess.FocusNode(button) {
		t.Fatal("could not find and focus the test button")
	}
	return f, sess, button
}

func documentNodeByID(n *dom.Node, id string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.GetAttribute("id") == id {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := documentNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// qc:BR-182
func TestContentKeyHoldsButtonActiveUntilSpaceRelease(t *testing.T) {
	f, _, button := keyTestButton(t, `<!DOCTYPE html><html><body><button id="go">Go</button></body></html>`)
	clicks := 0
	dom.AddEventListener(button, "click", func(*dom.Event) { clicks++ }, false)

	if !f.contentKey(' ', 0) {
		t.Fatal("Space press was not handled")
	}
	if !css.IsActive(button) {
		t.Fatal("Space press did not paint the button :active")
	}
	if clicks != 0 {
		t.Fatal("Space press activated the button before release")
	}

	if !f.releaseContentKey(' ', 0) {
		t.Fatal("Space release was not handled")
	}
	if clicks != 1 {
		t.Fatalf("click events = %d, want 1 on release", clicks)
	}
	if css.IsActive(button) {
		t.Fatal(":active remained after Space release")
	}
}

// qc:BR-182
func TestContentKeyHoldsButtonActiveUntilEnterRelease(t *testing.T) {
	f, _, button := keyTestButton(t, `<!DOCTYPE html><html><body><button id="go">Go</button></body></html>`)
	clicks := 0
	dom.AddEventListener(button, "click", func(*dom.Event) { clicks++ }, false)

	if !f.contentKey('\r', 0) {
		t.Fatal("Enter press was not handled")
	}
	if !css.IsActive(button) {
		t.Fatal("Enter press did not paint the button :active")
	}
	if clicks != 0 {
		t.Fatal("Enter press activated the button before release")
	}
	if !f.releaseContentKey('\r', 0) {
		t.Fatal("Enter release was not handled")
	}
	if clicks != 1 || css.IsActive(button) {
		t.Fatalf("after release: clicks=%d active=%v, want 1/false", clicks, css.IsActive(button))
	}
}

// qc:BR-182
func TestContentKeyReleaseForTypingIsNotConsumed(t *testing.T) {
	f, _, _ := keyTestButton(t, `<!DOCTYPE html><html><body><button id="go">Go</button></body></html>`)
	if f.releaseContentKey('x', 0) {
		t.Fatal("release of a key with no held activation was consumed")
	}
}

func TestInterceptKeyReleaseReachesContent(t *testing.T) {
	cw := &chromeWindow{
		toolbar: &toolbar.State{},
		events:  make(chan surface.Event, 8),
	}
	var gotKey rune
	var gotMods surface.KeyMod
	var gotAction surface.KeyAction
	called := false
	cw.onContentKey = func(k rune, m surface.KeyMod, a surface.KeyAction) bool {
		gotKey, gotMods, gotAction, called = k, m, a, true
		return true
	}
	if !cw.intercept(surface.Event{Kind: surface.EvKey, Key: ' ', KeyAction: surface.KeyRelease}) {
		t.Fatal("key release was not routed to content")
	}
	if !called || gotKey != ' ' || gotMods != 0 || gotAction != surface.KeyRelease {
		t.Fatalf("content release = (%q, %v, %v, called=%v)", gotKey, gotMods, gotAction, called)
	}
}
