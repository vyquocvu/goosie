// Package userscript manages user scripts (Greasemonkey/Tampermonkey style)
// and their injection into web documents at specified lifecycle stages.
package userscript

import (
	"bufio"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// RunAt specifies when during the document lifecycle the script should execute.
type RunAt string

const (
	// RunAtDocumentStart executes after DOM tree and window creation, but before inline/external scripts run.
	RunAtDocumentStart RunAt = "document-start"
	// RunAtDocumentEnd executes after initial DOM parsing and inline/external scripts have executed.
	RunAtDocumentEnd RunAt = "document-end"
	// RunAtDocumentIdle executes after load event / initial paint.
	RunAtDocumentIdle RunAt = "document-idle"
)

// Script represents a user script with metadata and executable code.
type Script struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace,omitempty"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Matches     []string `json:"matches"`
	Includes    []string `json:"includes,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
	RunAt       RunAt    `json:"runAt"`
	Code        string   `json:"code"`
	Path        string   `json:"path,omitempty"`
	Enabled     bool     `json:"enabled"`
}

// Parse extracts the ==UserScript== metadata header and code from raw JavaScript.
func Parse(content, filename string) (*Script, error) {
	s := &Script{
		Enabled: true,
		RunAt:   RunAtDocumentEnd,
		Code:    content,
		Path:    filename,
	}

	base := filepath.Base(filename)
	base = strings.TrimSuffix(base, ".user.js")
	base = strings.TrimSuffix(base, ".js")
	s.ID = base
	s.Name = base

	scanner := bufio.NewScanner(strings.NewReader(content))
	inMeta := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "// ==UserScript==") {
			inMeta = true
			continue
		}
		if strings.HasPrefix(line, "// ==/UserScript==") {
			inMeta = false
			break
		}
		if !inMeta {
			continue
		}

		if !strings.HasPrefix(line, "//") {
			continue
		}
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "//"))
		if !strings.HasPrefix(trimmed, "@") {
			continue
		}

		parts := strings.SplitN(trimmed[1:], " ", 2)
		if len(parts) < 2 {
			parts = strings.SplitN(trimmed[1:], "\t", 2)
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := ""
		if len(parts) > 1 {
			val = strings.TrimSpace(parts[1])
		}

		switch key {
		case "name":
			if val != "" {
				s.Name = val
			}
		case "namespace":
			s.Namespace = val
		case "version":
			s.Version = val
		case "description":
			s.Description = val
		case "match":
			if val != "" {
				s.Matches = append(s.Matches, val)
			}
		case "include":
			if val != "" {
				s.Includes = append(s.Includes, val)
			}
		case "exclude":
			if val != "" {
				s.Excludes = append(s.Excludes, val)
			}
		case "run-at":
			switch strings.ToLower(val) {
			case "document-start":
				s.RunAt = RunAtDocumentStart
			case "document-idle":
				s.RunAt = RunAtDocumentIdle
			default:
				s.RunAt = RunAtDocumentEnd
			}
		}
	}

	return s, scanner.Err()
}

// MatchesURL reports whether the script should execute on rawURL based on its
// @match, @include, and @exclude directives.
func (s *Script) MatchesURL(rawURL string) bool {
	if !s.Enabled {
		return false
	}

	// Any @exclude match vetoes execution immediately
	for _, excl := range s.Excludes {
		if MatchPattern(excl, rawURL) {
			return false
		}
	}

	// If no matches/includes specified, default is no-match
	if len(s.Matches) == 0 && len(s.Includes) == 0 {
		return false
	}

	// Check @match rules
	for _, m := range s.Matches {
		if MatchPattern(m, rawURL) {
			return true
		}
	}

	// Check @include rules (simple wildcard globs)
	for _, inc := range s.Includes {
		if MatchGlob(inc, rawURL) {
			return true
		}
	}

	return false
}

// MatchPattern tests a Chrome/Greasemonkey match pattern (<scheme>://<host>/<path> or <all_urls>).
func MatchPattern(pattern, rawURL string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "<all_urls>" {
		u, err := url.Parse(rawURL)
		if err != nil {
			return false
		}
		sch := strings.ToLower(u.Scheme)
		return sch == "http" || sch == "https" || sch == "file"
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	urlScheme := strings.ToLower(u.Scheme)
	urlHost := strings.ToLower(u.Host)
	urlHostname := strings.ToLower(u.Hostname())
	urlPath := u.EscapedPath()
	if urlPath == "" {
		urlPath = "/"
	}
	if u.RawQuery != "" {
		urlPath += "?" + u.RawQuery
	}

	// Syntax: <scheme>://<host>/<path> or file:///<path>
	parts := strings.SplitN(pattern, "://", 2)
	if len(parts) != 2 {
		return MatchGlob(pattern, rawURL)
	}

	schemePattern := strings.ToLower(parts[0])
	rest := parts[1]

	// Check scheme
	if schemePattern == "*" {
		if urlScheme != "http" && urlScheme != "https" {
			return false
		}
	} else if schemePattern != urlScheme {
		return false
	}

	// Split host and path
	slashIdx := strings.Index(rest, "/")
	if slashIdx == -1 {
		return false
	}
	hostPattern := strings.ToLower(rest[:slashIdx])
	pathPattern := rest[slashIdx:]

	// Check host
	if hostPattern == "*" {
		// Matches any host
	} else if strings.HasPrefix(hostPattern, "*.") {
		suffix := hostPattern[2:]
		if urlHostname != suffix && !strings.HasSuffix(urlHostname, "."+suffix) {
			return false
		}
	} else if hostPattern != urlHost && hostPattern != urlHostname {
		return false
	}

	// Check path with wildcards
	return MatchGlob(pathPattern, urlPath)
}

// MatchGlob matches string against a wildcard pattern with '*' matching any characters.
func MatchGlob(pattern, target string) bool {
	if pattern == "*" {
		return true
	}
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			sb.WriteString(".*")
		case '.', '+', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			sb.WriteString("\\")
			sb.WriteByte(ch)
		default:
			sb.WriteByte(ch)
		}
	}
	sb.WriteString("$")

	re, err := regexp.Compile(sb.String())
	if err != nil {
		return false
	}
	return re.MatchString(target)
}
