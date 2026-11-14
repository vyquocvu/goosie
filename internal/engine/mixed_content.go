package engine

import (
	"errors"
	"net/url"
	"strings"
)

const maxMixedContentReports = 32

type MixedContentKind string

const (
	MixedContentScript     MixedContentKind = "script"
	MixedContentStylesheet MixedContentKind = "stylesheet"
	MixedContentIFrame     MixedContentKind = "iframe"
	MixedContentImage      MixedContentKind = "image"
)

type MixedContentReport struct {
	Kind MixedContentKind
	URL  string
}

var errMixedContentBlocked = errors.New("engine: blocked insecure mixed content")

func mixedContentURL(base, ref string) (string, bool) {
	b, err := url.Parse(base)
	if err != nil || !strings.EqualFold(b.Scheme, "https") {
		return "", false
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", false
	}
	u := b.ResolveReference(r)
	if !strings.EqualFold(u.Scheme, "http") {
		return "", false
	}
	return u.String(), true
}

func (s *Session) reportMixed(kind MixedContentKind, rawURL string) {
	if s == nil || rawURL == "" {
		return
	}
	key := string(kind) + "\x00" + rawURL
	s.mixedMu.Lock()
	defer s.mixedMu.Unlock()
	if _, ok := s.mixedSeen[key]; ok {
		return
	}
	if len(s.mixedReports) >= maxMixedContentReports {
		return
	}
	if s.mixedSeen == nil {
		s.mixedSeen = make(map[string]struct{})
	}
	s.mixedSeen[key] = struct{}{}
	s.mixedReports = append(s.mixedReports, MixedContentReport{Kind: kind, URL: rawURL})
}

func (s *Session) MixedContentReports() []MixedContentReport {
	if s == nil {
		return nil
	}
	s.mixedMu.Lock()
	defer s.mixedMu.Unlock()
	if len(s.mixedReports) == 0 {
		return nil
	}
	out := make([]MixedContentReport, len(s.mixedReports))
	copy(out, s.mixedReports)
	return out
}

func (s *Session) resolveScriptURL(base, src string) (string, bool) {
	abs, ok := resolveSubresourceURL(base, src)
	if !ok {
		if bad, mixed := mixedContentURL(base, src); mixed {
			s.reportMixed(MixedContentScript, bad)
		}
	}
	return abs, ok
}

func (s *Session) resolveImageURL(base, src string) (string, bool) {
	abs, ok := resolveSubresourceURL(base, src)
	if !ok {
		if bad, mixed := mixedContentURL(base, src); mixed {
			s.reportMixed(MixedContentImage, bad)
		}
	}
	return abs, ok
}

func (s *Session) documentBase() string {
	if s.documentURL != "" {
		return s.documentURL
	}
	return s.imgBase
}
