package engine

import (
	"errors"
	"net/url"
	"strings"
)

// Mixed content: an HTTPS document that names a subresource over plaintext
// HTTP. Active content (scripts, stylesheets, iframes) can run code or read
// and rewrite the secure page, so it is refused outright. Passive content
// (images) can only paint, so the spec's floor is a warning; Goosie records
// every refusal in the same report so the embedding layer can surface it.
const maxMixedContentReports = 32

// MixedContentKind names the subresource family a refusal came from.
type MixedContentKind string

const (
	MixedContentScript     MixedContentKind = "script"
	MixedContentStylesheet MixedContentKind = "stylesheet"
	MixedContentIFrame     MixedContentKind = "iframe"
	MixedContentImage      MixedContentKind = "image"
)

// MixedContentReport is one plaintext URL an HTTPS document named, and the
// family that named it. The list is bounded per document.
type MixedContentReport struct {
	Kind MixedContentKind
	URL  string
}

// errMixedContentBlocked is returned to a caller whose fetch was refused by
// the mixed-content policy. It never reaches page script as a distinct error;
// the failed load surfaces as the element's normal error event.
var errMixedContentBlocked = errors.New("engine: blocked insecure mixed content")

// mixedContentURL resolves ref against base and reports the absolute URL when
// the reference downgrades HTTPS to plaintext HTTP. A relative reference, a
// scheme-relative //host/path, and an explicit HTTPS URL all inherit the
// secure scheme and are not mixed content.
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

// reportMixed records one refusal. The buffer is bounded and deduplicated so a
// document that names the same plaintext URL thousands of times cannot turn
// its diagnostic list into an allocation. It is safe to call from the script,
// font and image goroutines.
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

// MixedContentReports returns a copy of the plaintext subresource URLs this
// document refused or downgraded. The order is the order the document named
// them; at most maxMixedContentReports entries are kept.
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

// resolveScriptURL is resolveSubresourceURL plus the mixed-content report for
// an active script. Every classic and module script fetch resolves here, so a
// plaintext bundle is refused and attributed once per document.
func (s *Session) resolveScriptURL(base, src string) (string, bool) {
	abs, ok := resolveSubresourceURL(base, src)
	if !ok {
		if bad, mixed := mixedContentURL(base, src); mixed {
			s.reportMixed(MixedContentScript, bad)
		}
	}
	return abs, ok
}

// resolveImageURL is resolveSubresourceURL plus the passive mixed-content
// report. Images resolve here during the document walk; the fetcher itself
// never sees a downgraded URL.
func (s *Session) resolveImageURL(base, src string) (string, bool) {
	abs, ok := resolveSubresourceURL(base, src)
	if !ok {
		if bad, mixed := mixedContentURL(base, src); mixed {
			s.reportMixed(MixedContentImage, bad)
		}
	}
	return abs, ok
}

// documentBase is the URL subresources resolve against when a path did not
// carry its own base, such as a script-created image.
func (s *Session) documentBase() string {
	if s.documentURL != "" {
		return s.documentURL
	}
	return s.imgBase
}
