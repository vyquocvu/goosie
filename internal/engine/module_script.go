package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// The embedded JS runtime does not implement ECMAScript Module Records. This
// adapter handles the common static default-import form for modules whose
// dependencies export a JSON-serializable default value. It intentionally
// rejects other module syntax instead of running it as classic script.
const (
	maxModuleScripts    = 32
	maxModuleGraphDepth = 8
	maxModuleGraphBytes = 2 << 20
)

var (
	defaultImportLine = regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_$][\w$]*)\s+from\s+(['"])([^'"\r\n]+)['"]\s*;?\s*$`)
	defaultExportLine = regexp.MustCompile(`(?m)^\s*export\s+default\s+`)
)

func isModuleScript(n *dom.Node) bool {
	if n == nil || !n.Element() || n.Data != "script" {
		return false
	}
	for _, a := range n.Attr {
		if strings.EqualFold(a.Name, "type") {
			return strings.EqualFold(strings.TrimSpace(a.Value), "module")
		}
	}
	return false
}

func (s *Session) runModuleScripts(runtime *js.Runtime, root *dom.Node, baseURL string, externalLeft int, deadline time.Time) {
	if runtime == nil || root == nil {
		return
	}
	count := 0
	externalRemaining := externalLeft
	graphBytes := 0
	var visit func(*dom.Node)
	visit = func(n *dom.Node) {
		if n == nil || count >= maxModuleScripts {
			return
		}
		if isModuleScript(n) && !s.parserExecutedScripts[n] {
			count++
			if s.parserExecutedScripts == nil {
				s.parserExecutedScripts = make(map[*dom.Node]bool)
			}
			s.parserExecutedScripts[n] = true
			if !n.HasAttribute("async") {
				if time.Now().After(deadline) {
					s.recordScriptError(errors.New("module script fetch budget exhausted"))
				} else {
					source, base, name, ok := s.moduleScriptSource(n, baseURL, externalRemaining)
					if ok {
						if scriptSrc(n) != "" {
							externalRemaining--
						}
						if err := s.executeDefaultImportModule(runtime, source, base, name, deadline, &graphBytes, &count); err != nil {
							s.recordScriptError(err)
						}
					} else {
						s.recordScriptError(fmt.Errorf("failed to fetch module script %q", scriptSrc(n)))
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
			if count >= maxModuleScripts {
				break
			}
		}
	}
	visit(root)
}

func (s *Session) moduleScriptSource(n *dom.Node, baseURL string, externalLeft int) (source, resolvedBase, name string, ok bool) {
	src := scriptSrc(n)
	if src == "" {
		return n.TextContent(), baseURL, "inline module", true
	}
	if externalLeft <= 0 || s.scriptFetch == nil {
		return "", "", "", false
	}
	abs, valid := resolveSubresourceURL(baseURL, src)
	if !valid {
		return "", "", "", false
	}
	data, err := s.scriptFetch(baseURL, abs)
	if err != nil || len(data) == 0 || len(data) > js.MaxScriptBytes {
		return "", "", "", false
	}
	return string(data), abs, abs, true
}

func (s *Session) executeDefaultImportModule(runtime *js.Runtime, source, baseURL, name string, deadline time.Time, graphBytes, moduleCount *int) error {
	if len(source) > js.MaxScriptBytes {
		return js.ErrTooLarge
	}
	if *graphBytes+len(source) > maxModuleGraphBytes {
		return errors.New("module graph exceeds the 2 MiB admission limit")
	}
	*graphBytes += len(source)
	seen := make(map[string]bool)
	imports := defaultImportLine.FindAllStringSubmatch(source, -1)
	for _, match := range imports {
		if time.Now().After(deadline) {
			return errors.New("module graph fetch budget exhausted")
		}
		abs, ok := resolveSubresourceURL(baseURL, match[3])
		if !ok || s.scriptFetch == nil {
			return fmt.Errorf("cannot resolve module import %q", match[3])
		}
		if seen[abs] {
			return fmt.Errorf("cyclic or repeated module import %q is unsupported", abs)
		}
		*moduleCount++
		if *moduleCount > maxModuleScripts {
			return errors.New("module graph exceeds the 32-module admission limit")
		}
		seen[abs] = true
		data, err := s.scriptFetch(baseURL, abs)
		if err != nil || len(data) == 0 || len(data) > js.MaxScriptBytes {
			return fmt.Errorf("failed to fetch module dependency %q", abs)
		}
		*graphBytes += len(data)
		if *graphBytes > maxModuleGraphBytes {
			return errors.New("module graph exceeds the 2 MiB admission limit")
		}
		value, err := s.moduleDefaultJSON(runtime, string(data), abs, 1, graphBytes, moduleCount, deadline, seen)
		if err != nil {
			return err
		}
		replacement := "const " + match[1] + " = " + value + ";"
		source = strings.Replace(source, match[0], replacement, 1)
	}
	if strings.Contains(source, "import ") || defaultExportLine.MatchString(source) || strings.Contains(source, "export ") {
		return errors.New("module syntax is outside the supported static default-import subset")
	}
	// Module declarations are scoped to the module and top-level this is
	// undefined. The strict function wrapper preserves those properties for
	// scripts that do not need the unavailable native Module Record machinery.
	wrapped := "(function(){\"use strict\";\n" + source + "\n}).call(undefined);"
	return runtime.Run(wrapped, name)
}

func (s *Session) moduleDefaultJSON(runtime *js.Runtime, source, baseURL string, depth int, graphBytes, moduleCount *int, deadline time.Time, seen map[string]bool) (string, error) {
	if depth > maxModuleGraphDepth {
		return "", errors.New("module graph exceeds the 8-level depth limit")
	}
	if len(source) > js.MaxScriptBytes || *graphBytes > maxModuleGraphBytes {
		return "", errors.New("module graph exceeds its source-size limit")
	}
	imports := defaultImportLine.FindAllStringSubmatch(source, -1)
	var bindings strings.Builder
	for _, match := range imports {
		if time.Now().After(deadline) {
			return "", errors.New("module graph fetch budget exhausted")
		}
		abs, ok := resolveSubresourceURL(baseURL, match[3])
		if !ok || seen[abs] || s.scriptFetch == nil {
			return "", fmt.Errorf("cyclic or unresolved module import %q", match[3])
		}
		*moduleCount++
		if *moduleCount > maxModuleScripts {
			return "", errors.New("module graph exceeds the 32-module admission limit")
		}
		seen[abs] = true
		data, err := s.scriptFetch(baseURL, abs)
		if err != nil || len(data) == 0 || *graphBytes+len(data) > maxModuleGraphBytes {
			return "", fmt.Errorf("failed to fetch module dependency %q", abs)
		}
		*graphBytes += len(data)
		value, err := s.moduleDefaultJSON(runtime, string(data), abs, depth+1, graphBytes, moduleCount, deadline, seen)
		if err != nil {
			return "", err
		}
		source = strings.Replace(source, match[0], "", 1)
		bindings.WriteString("const ")
		bindings.WriteString(match[1])
		bindings.WriteString(" = ")
		bindings.WriteString(value)
		bindings.WriteString(";\n")
	}
	if strings.Contains(source, "import ") || strings.Contains(source, "export ") {
		// The only accepted export is a single default expression at module
		// scope. Parse below by replacing that declaration with a return.
		loc := defaultExportLine.FindStringIndex(source)
		if loc == nil {
			return "", errors.New("module dependency must have one default export")
		}
		prefix := source[:loc[0]]
		rest := source[loc[1]:]
		if strings.TrimSpace(prefix) != "" {
			return "", errors.New("module dependency statements before export default are unsupported")
		}
		expression := strings.TrimSpace(rest)
		expression = strings.TrimSuffix(expression, ";")
		if expression == "" || strings.Contains(expression, "export ") || strings.Contains(expression, "import ") {
			return "", errors.New("module dependency must contain one default expression")
		}
		result, err := runtime.Evaluate("JSON.stringify((function(){\"use strict\";\n" + bindings.String() + "return (" + expression + ");}).call(undefined))")
		if err != nil {
			return "", err
		}
		var value any
		if err := json.Unmarshal([]byte(result), &value); err != nil {
			return "", errors.New("module default export is not JSON-serializable")
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", errors.New("module default export could not be encoded")
		}
		return string(encoded), nil
	}
	return "", errors.New("module dependency must use export default")
}
