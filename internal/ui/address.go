package ui

import (
	"net/url"
	"strings"
)

func ResolveAddressInput(input, searchEngine string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.String()
	}
	if looksLikeHost(trimmed) {
		scheme := "https://"
		if strings.HasPrefix(trimmed, "localhost") || strings.HasPrefix(trimmed, "127.0.0.1") {
			scheme = "http://"
		}
		return scheme + trimmed
	}
	if searchEngine == "" {
		searchEngine = "https://www.google.com/search?q="
	}
	return searchEngine + url.QueryEscape(trimmed)
}

func looksLikeHost(input string) bool {
	if strings.Contains(input, " ") {
		return false
	}
	return strings.Contains(input, ".") || strings.HasPrefix(input, "localhost") || strings.HasPrefix(input, "127.0.0.1")
}
