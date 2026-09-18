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
	"fmt"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
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
	if err := validateWidth(viewportW); err != nil {
		return nil, err
	}
	if len(html) > MaxDocumentBytes {
		return nil, fmt.Errorf("HTML byte limit exceeded (%d)", MaxDocumentBytes)
	}
	doc, err := dom.ParseBounded(html, dom.ParseLimits{
		Nodes: MaxDocumentNodes, Depth: MaxDocumentDepth,
		Attributes: MaxAttributes, AttributeBytes: MaxAttributeBytes,
	})
	if err != nil {
		return nil, err
	}
	sheets, err := checkedSheets(doc, authorCSS)
	if err != nil {
		return nil, err
	}
	s.Doc = doc
	s.Styles = style.Resolve(doc, sheets)
	if err := s.Reflow(viewportW); err != nil {
		return nil, err
	}
	return s, nil
}

// Reflow runs layout only, retaining the document, computed style identities,
// and font metrics. The old arena is untouched unless the candidate validates.
// Like Paint, this method must be called by the session's single owner.
func (s *Session) Reflow(viewportW float32) error {
	if err := validateWidth(viewportW); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("missing session")
	}
	if err := s.validateLayoutInput(); err != nil {
		return err
	}
	candidate := layout.Build(s.Doc, s.Styles)
	candidate.Metrics = s.metrics
	layout.Block(candidate, layout.ObjectID(1), viewportW)
	if err := validateArena(candidate); err != nil {
		return err
	}
	layout.Inline(candidate, layout.ObjectID(1))
	if err := validateArena(candidate); err != nil {
		return err
	}
	s.Arena = candidate
	return nil
}

// PaintChecked validates geometry and scale before any device-coordinate
// conversion, then checks tile metadata bounds before handing off the list.
func (s *Session) PaintChecked(scale float32) (*paint.List, error) {
	if err := validateScale(float64(scale)); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("missing session")
	}
	if err := validateArena(s.Arena); err != nil {
		return nil, err
	}
	list := s.Paint(scale)
	if err := ValidateExtent(list.Bounds()); err != nil {
		return nil, err
	}
	return list, nil
}

// Paint builds a display list from a trusted session's layout arena. Binaries
// processing untrusted input must use PaintChecked instead.
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

// BackgroundColor returns the effective canvas background color.
// If html has a non-transparent background, that is used. Otherwise if body
// has a non-transparent background, that is used. If both are transparent,
// white RGB(255, 255, 255) is returned.
func (s *Session) BackgroundColor() frame.Color {
	if s.Doc != nil {
		if s.Doc.HTML != nil {
			if st, ok := s.Styles[s.Doc.HTML.ID]; ok && st.BackgroundColor.A > 0 {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
		if s.Doc.Body != nil {
			if st, ok := s.Styles[s.Doc.Body.ID]; ok && st.BackgroundColor.A > 0 {
				return frame.RGBA(st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B, st.BackgroundColor.A)
			}
		}
	}
	return frame.RGB(255, 255, 255)
}
