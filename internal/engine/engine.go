// Package engine wires the front-end (DOM, CSS, style, layout, paint) into the
// v2 frame path.
//
// The engine is the top of the dependency stack: it imports every front-end
// package and produces a paint.LayerDL that the surface can display. A Session
// owns the pipeline for one document; the pipeline is Parse → Style → Layout →
// Paint, and each stage is a pure function of its input so a change to one
// stage does not force the others to be rebuilt.
package engine

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
)

// Session is one document's pipeline state.
//
// A session holds the parsed DOM, the resolved styles, and the layout arena;
// each field is the output of the previous stage. The pipeline is re-run from
// the stage that changed: a style sheet edit re-runs style and below, a DOM
// edit re-runs everything. The session does not own a network client; the
// caller passes in fetched bytes so the engine stays testable without a network.
type Session struct {
	Doc    *dom.Document
	Styles map[dom.NodeID]*style.ComputedStyle
	Arena  *layout.Arena

	metrics layout.Metrics
}

// Option adjusts how a Session is built.
type Option func(*Session)

// WithMetrics supplies real font measurements so the inline pass measures word
// widths and paint emits per-glyph advances from the same source. Without it,
// layout falls back to a half-em-per-glyph estimate and paint stretches text.
func WithMetrics(m layout.Metrics) Option {
	return func(s *Session) { s.metrics = m }
}

// NewSession parses HTML and builds the pipeline state. The caller provides the
// raw HTML and any author style sheets; <style> blocks inside the document are
// collected automatically, and the UA stylesheet is added internally.
// The returned session has a fully laid-out arena ready for paint.
func NewSession(html string, authorCSS []string, viewportW float32, opts ...Option) (*Session, error) {
	s := &Session{}
	for _, opt := range opts {
		opt(s)
	}
	doc := dom.Parse(html)
	var sheets []*css.Stylesheet
	for _, src := range authorCSS {
		sheets = append(sheets, css.Parse(src))
	}
	// Document-embedded style sheets come after the caller's sheets so they
	// win ties in the cascade, matching how linked sheets precede <style>.
	for _, el := range doc.ElementsByTagName("style") {
		if src := el.TextContent(); src != "" {
			sheets = append(sheets, css.Parse(src))
		}
	}
	styles := style.Resolve(doc, sheets)
	arena := layout.Build(doc, styles)
	arena.Metrics = s.metrics
	layout.Block(arena, layout.ObjectID(1), viewportW)
	layout.Inline(arena, layout.ObjectID(1))
	s.Doc = doc
	s.Styles = styles
	s.Arena = arena
	return s, nil
}

// Paint builds a display list from the session's layout arena.
//
// The list is mutable; the caller freezes it with Publish when the frame is
// ready. scale is the device-pixel ratio: a 2x Retina display passes 2.0, and
// the builder multiplies every CSS-pixel coordinate by it to produce device
// pixels.
func (s *Session) Paint(scale float32) *paint.List {
	list := &paint.List{}
	b := paint.NewBuilder(list, s.Arena, scale, s.metrics)
	b.Build(layout.ObjectID(1))
	return list
}
