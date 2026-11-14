package engine_test

import (
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
)

const mediaHTML = `<html><body><p id="p">hi</p></body></html>`

const narrowSheet = `@media (max-width: 600px) { p { color: rgb(255, 0, 0); } }`

var mediaRed = css.Color{R: 255, G: 0, B: 0, A: 255}

func mediaColor(t *testing.T, s *engine.Session) css.Color {
	t.Helper()
	st := s.Styles[s.Doc.ElementByID("p").ID]
	if st == nil {
		t.Fatal("the paragraph has no computed style")
	}
	return st.Color
}

func TestRefreshMatchesMediaAgainstItsOwnWidth(t *testing.T) {
	s, err := engine.NewSession(mediaHTML, []string{narrowSheet}, 1200)
	if err != nil {
		t.Fatalf("build session: %v", err)
	}
	if got := mediaColor(t, s); got == mediaRed {
		t.Fatalf("a 1200 px viewport matched `max-width: 600px` (got %+v): the test fixture "+
			"is not testing what it claims", got)
	}

	if !s.Refresh(engine.Plan{FullDoc: true}, []string{narrowSheet}, 500) {
		t.Fatal("the full-document refresh reported no work")
	}
	if got := mediaColor(t, s); got != mediaRed {
		t.Errorf("after restyling the same document at 500 px, p color = %+v, want %+v: css.Parse "+
			"tested the @media condition against the process-wide width left behind by the last "+
			"NewSession, not against the 500 px Refresh was handed", got, mediaRed)
	}
}

func TestConcurrentLoadsDoNotShareMediaWidth(t *testing.T) {
	const tabs = 24
	var wg sync.WaitGroup
	for i := 0; i < tabs; i++ {
		width := float32(1200)
		if i%2 == 0 {
			width = 400
		}
		wg.Add(1)
		go func(width float32) {
			defer wg.Done()
			s, err := engine.NewSession(mediaHTML, []string{narrowSheet}, width)
			if err != nil {
				t.Errorf("build session at %v px: %v", width, err)
				return
			}
			got := mediaColor(t, s)
			if width <= 600 && got != mediaRed {
				t.Errorf("a document that loaded at %v px does not match `max-width: 600px` "+
					"(color %+v): another tab's width was in the global when its sheet parsed", width, got)
			}
			if width > 600 && got == mediaRed {
				t.Errorf("a document that loaded at %v px matched `max-width: 600px` (color %+v): "+
					"another tab's narrower width was in the global when its sheet parsed", width, got)
			}
		}(width)
	}
	wg.Wait()
}

func TestPrintStyleAndLinkSkippedOnScreen(t *testing.T) {
	html := `<html><head>
		<style media="print">p { color: rgb(255, 0, 0); }</style>
		<style media="screen">p { color: rgb(0, 255, 0); }</style>
	</head><body><p id="p">text</p></body></html>`
	s, err := engine.NewSession(html, nil, 1000)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	green := css.Color{R: 0, G: 255, B: 0, A: 255}
	if got := mediaColor(t, s); got != green {
		t.Fatalf("expected green (%+v) on screen, got %+v", green, got)
	}
}
