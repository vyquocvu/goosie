package userscript

import (
	"bufio"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type RunAt string

const (
	RunAtDocumentStart RunAt = "document-start"
	RunAtDocumentEnd   RunAt = "document-end"
	RunAtDocumentIdle  RunAt = "document-idle"
)

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

func (s *Script) MatchesURL(rawURL string) bool {
	if !s.Enabled {
		return false
	}

	for _, excl := range s.Excludes {
		if MatchPattern(excl, rawURL) {
			return false
		}
	}

	if len(s.Matches) == 0 && len(s.Includes) == 0 {
		return false
	}

	for _, m := range s.Matches {
		if MatchPattern(m, rawURL) {
			return true
		}
	}

	for _, inc := range s.Includes {
		if MatchGlob(inc, rawURL) {
			return true
		}
	}

	return false
}

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

	parts := strings.SplitN(pattern, "://", 2)
	if len(parts) != 2 {
		return MatchGlob(pattern, rawURL)
	}

	schemePattern := strings.ToLower(parts[0])
	rest := parts[1]

	if schemePattern == "*" {
		if urlScheme != "http" && urlScheme != "https" {
			return false
		}
	} else if schemePattern != urlScheme {
		return false
	}

	slashIdx := strings.Index(rest, "/")
	if slashIdx == -1 {
		return false
	}
	hostPattern := strings.ToLower(rest[:slashIdx])
	pathPattern := rest[slashIdx:]

	if hostPattern == "*" {
	} else if strings.HasPrefix(hostPattern, "*.") {
		suffix := hostPattern[2:]
		if urlHostname != suffix && !strings.HasSuffix(urlHostname, "."+suffix) {
			return false
		}
	} else if hostPattern != urlHost && hostPattern != urlHostname {
		return false
	}

	return MatchGlob(pathPattern, urlPath)
}

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
