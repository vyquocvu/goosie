// Package download decides whether a response must be saved instead of
// rendered and writes it under a collision-free name.
package download

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ShouldDownload reports whether a main-document response must be saved to
// disk instead of rendered: an attachment disposition always downloads, and
// any content type the engine cannot render downloads too. An absent content
// type preserves the historical render behavior.
func ShouldDownload(contentType, disposition string) bool {
	if strings.Contains(strings.ToLower(disposition), "attachment") {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "text/html", "application/xhtml+xml", "text/plain":
		return false
	case "":
		return false
	default:
		return true
	}
}

// FileName picks a save name from the disposition's filename parameter, then
// the URL path, then a fallback, reduced to one safe path component.
func FileName(disposition, rawURL string) string {
	if name := sanitize(dispositionParam(disposition)); name != "" {
		return name
	}
	if u, err := url.Parse(rawURL); err == nil {
		base := path.Base(u.Path)
		if base != "" && base != "/" && base != "." {
			if name := sanitize(base); name != "" {
				return name
			}
		}
	}
	return "download"
}

// dispositionParam extracts filename="value" (or bare value) from a
// Content-Disposition header value.
func dispositionParam(disposition string) string {
	for _, part := range strings.Split(disposition, ";") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "filename="); ok {
			return strings.Trim(rest, `"`)
		}
	}
	return ""
}

// sanitize strips control characters and keeps only the last path segment.
func sanitize(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, name)
	fields := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// Save writes body under dir with the given name, appending (1), (2), ... to
// the base when the name is taken. It returns the written path.
func Save(dir, name string, body []byte) (string, error) {
	if name == "" {
		name = "download"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("goosie: download dir: %w", err)
	}
	base, ext := name, filepath.Ext(name)
	if ext != "" {
		base = strings.TrimSuffix(name, ext)
	}
	full := filepath.Join(dir, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(full); os.IsNotExist(err) {
			break
		}
		if i > 999 {
			return "", fmt.Errorf("goosie: download: no free name for %q in %q", name, dir)
		}
		full = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
	return full, os.WriteFile(full, body, 0o644)
}
