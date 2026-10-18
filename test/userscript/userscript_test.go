package userscript_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/userscript"
)

func TestParse_FullMetadata(t *testing.T) {
	code := `// ==UserScript==
// @name         GitHub Dark Theme
// @namespace    https://github.com/goosie
// @version      1.2.3
// @description  Restyles github.com
// @match        https://github.com/*
// @match        https://gist.github.com/*
// @include      https://*.github.io/*
// @exclude      https://github.com/settings/*
// @run-at       document-start
// ==/UserScript==

document.body.style.backgroundColor = "black";
`
	s, err := userscript.Parse(code, "/profiles/user/github_dark.user.js")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if s.Name != "GitHub Dark Theme" {
		t.Errorf("Name = %q, want GitHub Dark Theme", s.Name)
	}
	if s.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", s.Version)
	}
	if s.Description != "Restyles github.com" {
		t.Errorf("Description = %q, want Restyles github.com", s.Description)
	}
	if s.RunAt != userscript.RunAtDocumentStart {
		t.Errorf("RunAt = %q, want %q", s.RunAt, userscript.RunAtDocumentStart)
	}
	if len(s.Matches) != 2 {
		t.Errorf("Matches = %d, want 2", len(s.Matches))
	}
	if len(s.Includes) != 1 {
		t.Errorf("Includes = %d, want 1", len(s.Includes))
	}
	if len(s.Excludes) != 1 {
		t.Errorf("Excludes = %d, want 1", len(s.Excludes))
	}
	if !s.Enabled {
		t.Error("expected script to be enabled by default")
	}
}

func TestParse_MinimalDefaults(t *testing.T) {
	code := `console.log("No metadata header");`
	s, err := userscript.Parse(code, "my_custom_script.js")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if s.Name != "my_custom_script" {
		t.Errorf("Name = %q, want my_custom_script", s.Name)
	}
	if s.RunAt != userscript.RunAtDocumentEnd {
		t.Errorf("RunAt default = %q, want document-end", s.RunAt)
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		rawURL  string
		want    bool
	}{
		{"<all_urls>", "https://example.com/page", true},
		{"<all_urls>", "http://localhost:8080/", true},
		{"<all_urls>", "file:///tmp/doc.html", true},
		{"<all_urls>", "data:text/html,test", false},

		{"*://*/*", "https://example.com/", true},
		{"*://*/*", "http://example.com/sub/dir", true},
		{"*://*/*", "ftp://example.com/", false},

		{"https://*.example.com/*", "https://sub.example.com/index.html", true},
		{"https://*.example.com/*", "https://example.com/", true},
		{"https://*.example.com/*", "https://other.com/", false},
		{"https://*.example.com/*", "http://sub.example.com/", false}, // wrong scheme

		{"https://example.com/foo/*", "https://example.com/foo/bar", true},
		{"https://example.com/foo/*", "https://example.com/bar", false},
	}

	for _, tc := range tests {
		got := userscript.MatchPattern(tc.pattern, tc.rawURL)
		if got != tc.want {
			t.Errorf("MatchPattern(%q, %q) = %v, want %v", tc.pattern, tc.rawURL, got, tc.want)
		}
	}
}

func TestScript_MatchesURL_IncludesExcludes(t *testing.T) {
	s := &userscript.Script{
		Enabled:  true,
		Matches:  []string{"https://*.news.ycombinator.com/*", "https://news.ycombinator.com/*"},
		Excludes: []string{"https://news.ycombinator.com/login*"},
	}

	if !s.MatchesURL("https://news.ycombinator.com/") {
		t.Error("expected match on homepage")
	}
	if !s.MatchesURL("https://news.ycombinator.com/item?id=12345") {
		t.Error("expected match on story item")
	}
	if s.MatchesURL("https://news.ycombinator.com/login?goto=news") {
		t.Error("expected exclude rule to block login page")
	}

	// When disabled, no URLs match
	s.Enabled = false
	if s.MatchesURL("https://news.ycombinator.com/") {
		t.Error("disabled script must not match any URL")
	}
}

func TestManager_LoadAndMatching(t *testing.T) {
	dir := t.TempDir()

	script1 := `// ==UserScript==
// @name   Start Modifier
// @match  https://target.test/*
// @run-at document-start
// ==/UserScript==
console.log("start");
`
	script2 := `// ==UserScript==
// @name   End Modifier
// @match  https://target.test/*
// @run-at document-end
// ==/UserScript==
console.log("end");
`
	script3 := `// ==UserScript==
// @name   Other Site
// @match  https://other.test/*
// ==/UserScript==
`
	_ = os.WriteFile(filepath.Join(dir, "script1.user.js"), []byte(script1), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "script2.user.js"), []byte(script2), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "script3.js"), []byte(script3), 0o644)

	mgr := userscript.NewManager(dir)
	if err := mgr.Load(); err != nil {
		t.Fatalf("mgr.Load: %v", err)
	}

	if len(mgr.Scripts()) != 3 {
		t.Fatalf("loaded %d scripts, want 3", len(mgr.Scripts()))
	}

	startMatches := mgr.MatchingScripts("https://target.test/home", userscript.RunAtDocumentStart)
	if len(startMatches) != 1 || startMatches[0].Name != "Start Modifier" {
		t.Fatalf("startMatches = %v, want [Start Modifier]", startMatches)
	}

	endMatches := mgr.MatchingScripts("https://target.test/home", userscript.RunAtDocumentEnd)
	if len(endMatches) != 1 || endMatches[0].Name != "End Modifier" {
		t.Fatalf("endMatches = %v, want [End Modifier]", endMatches)
	}

	// Toggle disable
	s := mgr.ScriptByID("script1")
	if s == nil {
		t.Fatal("could not find script1 by ID")
	}
	mgr.SetEnabled("script1", false)

	startMatchesAfter := mgr.MatchingScripts("https://target.test/home", userscript.RunAtDocumentStart)
	if len(startMatchesAfter) != 0 {
		t.Errorf("disabled script returned in MatchingScripts: %v", startMatchesAfter)
	}
}
