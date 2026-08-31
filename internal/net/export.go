package net

import (
	"context"
	"io"
	"net/http"
)

// Directives exports directives field for use by external test packages.
func (p *CSPPolicy) Directives() map[string][]string { return p.directives }

// SetDirectives sets directives field for use by external test packages.
func (p *CSPPolicy) SetDirectives(d map[string][]string) { p.directives = d }

// MatchesHostWildcard exports matchesHostWildcard for use by external test packages.
var MatchesHostWildcard = matchesHostWildcard

// Client exports client field for use by external test packages.
func (f *Fetcher) Client() *http.Client { return f.client }

// ServiceClient exports client field for use by external test packages.
func (s *Service) Client() *http.Client { return s.client }

// ParseMediaType exports parseMediaType for use by external test packages.
var ParseMediaType = parseMediaType

// LimitedContextReader is the exported type alias for limitedContextReader
// for use by external test packages.
type LimitedContextReader = limitedContextReader

// NewLimitedContextReader creates a limitedContextReader for use by external test packages.
func NewLimitedContextReader(ctx context.Context, r io.Reader, limit int64) *LimitedContextReader {
	return &limitedContextReader{ctx: ctx, reader: r, limit: limit}
}

// NewLimitedContextReaderWithCompressedSize creates a limitedContextReader with compressedSize for use by external test packages.
func NewLimitedContextReaderWithCompressedSize(ctx context.Context, r io.Reader, limit int64, compressedSize int64) *LimitedContextReader {
	return &limitedContextReader{ctx: ctx, reader: r, limit: limit, compressedSize: compressedSize}
}

// MaxRedirects exports maxRedirects constant for use by external test packages.
const MaxRedirects = maxRedirects

// MaxBodySize exports maxBodySize field for use by external test packages.
func (s *Service) MaxBodySize() int64 { return s.maxBodySize }


