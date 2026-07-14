package js

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Dynamic import() detection (M12.1 — "ES module graph required beyond
// the supported script subset")
//
// Goja treats `import` as a reserved word at parse time and rejects any
// script that uses it (dynamic import() expression or otherwise) with a
// SyntaxError. The engine still wants to know when a page attempts to
// use the dynamic import() surface, so the fallback layer can mark it
// for compatibility. We surface that signal by scanning script source
// for the `import(` token (the dynamic call form) before delegating to
// Goja. The scan respects line/block comments and string literals so
// that words that merely LOOK like `import(...)` inside a comment or a
// string literal do not trigger a false positive.
//
// Static declarations (`import x from "y"`, `import.meta`) are NOT
// detected by this scanner — the HTML parser already detects
// `<script type="module">` via dom.ParseConfig.OnUnsupportedFeature.
// The runtime scan exists only to detect the dynamic call form that
// the parser cannot see (it appears only in script bodies).
// ---------------------------------------------------------------------------

// jsIdentChar reports whether the byte may appear in a JavaScript
// identifier. Used to enforce word boundaries around `import` so that
// `imported`, `myimport`, `$import`, etc. are not mistaken for the
// keyword.
func jsIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '$'
}

// jsWhitespace reports whether the byte is a JS whitespace character
// (the four ASCII whitespace forms that may appear between `import`
// and the opening `(` of a dynamic import call).
func jsWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// ScanAndReportUnsupportedJSFeatures scans the script source for JS
// constructs the engine does not support (currently: dynamic
// import() expressions) and reports each detected kind at most once
// per Runtime via the runtime detection callback.
//
// Nil-safe: no-op when no callback is installed. Short-circuits on
// scripts that do not contain the substring "import" to avoid any
// per-byte work in the common case.
//
// Callers normally do not invoke this directly — RunScript does it
// automatically. The method is exported so it can be tested in
// isolation and called by code paths that have a script body but are
// not using RunScript (e.g. CSP pre-checks).
func (r *Runtime) ScanAndReportUnsupportedJSFeatures(source string) {
	if r.OnRuntimeUnsupportedFeature == nil {
		return
	}
	// Fast path: most scripts do not contain the word "import" at all.
	if !strings.Contains(source, "import") {
		return
	}
	r.scanForDynamicImport(source)
}

// scanForDynamicImport walks the source, tracking JS lexical context
// (line/block comments, string literals, template literals), and
// reports FeatureESModule the first time it sees `import` followed
// by optional whitespace and `(`. The Runtime-level dedup means we
// can return as soon as we report — no need to keep scanning.
func (r *Runtime) scanForDynamicImport(source string) {
	n := len(source)
	i := 0

	for i < n {
		c := source[i]

		switch c {
		case '/':
			// Comment? `//` (line) or `/*` (block).
			if i+1 < n {
				if source[i+1] == '/' {
					// Line comment — skip to next newline (don't consume newline).
					i += 2
					for i < n && source[i] != '\n' {
						i++
					}
					continue
				}
				if source[i+1] == '*' {
					// Block comment — skip to closing `*/`.
					i += 2
					for i+1 < n && !(source[i] == '*' && source[i+1] == '/') {
						i++
					}
					if i+1 < n {
						i += 2 // consume `*/`
					} else {
						i = n // unterminated block comment — bail out
					}
					continue
				}
			}
			i++

		case '"', '\'':
			// Single- or double-quoted string literal. Respect `\` escapes.
			quote := c
			i++
			for i < n && source[i] != quote {
				if source[i] == '\\' && i+1 < n {
					// Skip the escape and the escaped char. The escaped
					// char might be a quote that would otherwise end
					// the string literal — so we MUST skip both bytes.
					i += 2
					continue
				}
				// Newline inside a non-template string literal is
				// technically a syntax error in JS, but we treat it
				// as a string terminator here so that a malformed
				// source doesn't trap the scanner.
				if source[i] == '\n' {
					break
				}
				i++
			}
			if i < n && source[i] == quote {
				i++ // consume closing quote
			}

		case '`':
			// Template literal — simplified handling. We do NOT recurse
			// into ${...} interpolations because a dynamic `import(`
			// inside a ${...} expression still constitutes real page
			// behavior (and is detected by the recursive scanner at
			// runtime). For detection purposes, treat the whole
			// template literal as opaque.
			i++
			for i < n && source[i] != '`' {
				if source[i] == '\\' && i+1 < n {
					i += 2
					continue
				}
				i++
			}
			if i < n && source[i] == '`' {
				i++ // consume closing backtick
			}

		default:
			// Look for `import` keyword at this position with proper
			// identifier boundaries.
			if c == 'i' && i+6 <= n && source[i:i+6] == "import" {
				before := byte(0)
				if i > 0 {
					before = source[i-1]
				}
				after := byte(0)
				if i+6 < n {
					after = source[i+6]
				}
				if !jsIdentChar(before) && !jsIdentChar(after) {
					// Optional whitespace, then `(` for the dynamic
					// call form.
					j := i + 6
					for j < n && jsWhitespace(source[j]) {
						j++
					}
					if j < n && source[j] == '(' {
						r.reportRuntimeUnsupportedFeature(dom.FeatureESModule)
						// Dedup at the report layer means we can
						// stop scanning now. Any further matches
						// would just be redundant.
						return
					}
				}
			}
			i++
		}
	}
}
