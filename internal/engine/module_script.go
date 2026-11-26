package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

const (
	maxModuleScripts    = 32
	maxModuleGraphDepth = 8
	maxModuleGraphBytes = 2 << 20
)

var errStaticModuleSubset = errors.New("module syntax is outside the supported static default-import subset")

type moduleTokenKind uint8

const (
	moduleTokenIdentifier moduleTokenKind = iota
	moduleTokenString
	moduleTokenTemplate
	moduleTokenPunct
	moduleTokenNumber
	moduleTokenRegex
)

type moduleToken struct {
	kind       moduleTokenKind
	start, end int
	line       int
	value      string
}

type moduleRequest struct {
	spec       string
	target     string
	targetID   int
	start, end int
}

type moduleImport struct {
	local     string
	imported  string
	request   int
	namespace bool
}

type moduleExport struct {
	name      string
	local     string
	request   int
	targetID  int
	imported  string
	namespace bool
	star      bool
}

type moduleRecord struct {
	id       int
	url      string
	base     string
	name     string
	source   string
	tokens   []moduleToken
	body     string
	requests []moduleRequest
	imports  []moduleImport
	exports  []moduleExport
	locals   map[string]bool
	depth    int
	state    uint8
	executed bool
	running  bool
}

type moduleEdit struct {
	start, end int
	text       string
}

type moduleExportRef struct {
	module int
	name   string
	ns     bool
}

type moduleLinker struct {
	session         *Session
	runtime         *js.Runtime
	documentURL     string
	deadline        time.Time
	records         []*moduleRecord
	byURL           map[string]*moduleRecord
	graphBytes      int
	moduleCount     int
	registryName    string
	registryUp      bool
	registryRecords int
	resolved        []map[string]moduleExportRef
	ambiguous       []map[string]bool
}

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
	linker := newModuleLinker(s, runtime, baseURL, deadline)
	if linker == nil {
		return
	}
	defer linker.closeRegistry()
	externalRemaining := externalLeft
	var deferred []*dom.Node
	var visit func(*dom.Node)
	visit = func(n *dom.Node) {
		if n == nil {
			return
		}
		if isModuleScript(n) && !s.parserExecutedScripts[n] {
			if s.parserExecutedScripts == nil {
				s.parserExecutedScripts = make(map[*dom.Node]bool)
			}
			s.parserExecutedScripts[n] = true
			if scriptSrc(n) != "" && externalRemaining <= 0 {
				s.recordScriptError(errors.New("external module script limit exhausted"))
			} else if n.HasAttribute("async") {
				if scriptSrc(n) != "" {
					externalRemaining--
				}
				s.runModuleScriptNode(linker, n, baseURL)
			} else {
				deferred = append(deferred, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	for _, n := range deferred {
		if scriptSrc(n) != "" {
			if externalRemaining <= 0 {
				s.recordScriptError(errors.New("external module script limit exhausted"))
				continue
			}
			externalRemaining--
		}
		s.runModuleScriptNode(linker, n, baseURL)
	}
}

func (s *Session) runModuleScriptNode(linker *moduleLinker, n *dom.Node, baseURL string) {
	if !linker.runtime.AllowsScriptURL(baseURL, linker.documentURL) {
		s.recordScriptError(errors.New("module script blocked by Content-Security-Policy"))
		return
	}
	src := scriptSrc(n)
	if src == "" {
		nonce := strings.TrimSpace(n.GetAttribute("nonce"))
		if !linker.runtime.AllowsInlineScriptWithNonce(nonce) {
			s.recordScriptError(errors.New("inline module script blocked by Content-Security-Policy"))
			return
		}
		record, err := linker.addRecord(n.TextContent(), baseURL, "inline module", "", 1)
		if err != nil {
			s.recordScriptError(err)
			return
		}
		if err := linker.execute(record); err != nil {
			s.recordScriptError(err)
		}
		return
	}
	abs, ok := s.resolveScriptURL(baseURL, src)
	if !ok {
		s.recordScriptError(fmt.Errorf("cannot resolve module script %q", src))
		return
	}
	record, err := linker.loadURL(baseURL, abs, 1)
	if err != nil {
		s.recordScriptError(err)
		return
	}
	if err := linker.execute(record); err != nil {
		s.recordScriptError(err)
	}
}

func newModuleLinker(s *Session, runtime *js.Runtime, documentURL string, deadline time.Time) *moduleLinker {
	if s == nil || runtime == nil {
		return nil
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil
	}
	return &moduleLinker{
		session:      s,
		runtime:      runtime,
		documentURL:  documentURL,
		deadline:     deadline,
		byURL:        make(map[string]*moduleRecord),
		registryName: "__goosie_modules_" + hex.EncodeToString(random[:]),
	}
}

func (l *moduleLinker) addRecord(source, base, name, url string, depth int) (*moduleRecord, error) {
	if depth > maxModuleGraphDepth {
		return nil, errors.New("module graph exceeds the 8-level depth limit")
	}
	if len(source) > js.MaxScriptBytes {
		return nil, js.ErrTooLarge
	}
	if l.graphBytes+len(source) > maxModuleGraphBytes {
		return nil, errors.New("module graph exceeds the 2 MiB admission limit")
	}
	if l.moduleCount >= maxModuleScripts {
		return nil, errors.New("module graph exceeds the 32-module admission limit")
	}
	tokens, err := lexModule(source)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	record := &moduleRecord{
		id:     len(l.records),
		url:    url,
		base:   base,
		name:   name,
		source: source,
		tokens: tokens,
		locals: make(map[string]bool),
		depth:  depth,
	}
	if err := parseModule(record); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	l.records = append(l.records, record)
	l.moduleCount++
	l.graphBytes += len(source)
	if url != "" {
		l.byURL[url] = record
	}
	return record, nil
}

func (l *moduleLinker) loadURL(base, target string, depth int) (*moduleRecord, error) {
	if depth > maxModuleGraphDepth {
		return nil, errors.New("module graph exceeds the 8-level depth limit")
	}
	if time.Now().After(l.deadline) {
		return nil, errors.New("module graph fetch budget exhausted")
	}
	abs, ok := l.session.resolveScriptURL(base, target)
	if !ok {
		return nil, fmt.Errorf("cannot resolve module import %q", target)
	}
	if !l.runtime.AllowsScriptURL(abs, l.documentURL) {
		return nil, fmt.Errorf("module %q blocked by Content-Security-Policy", abs)
	}
	if existing := l.byURL[abs]; existing != nil {
		return existing, nil
	}
	if l.moduleCount >= maxModuleScripts {
		return nil, errors.New("module graph exceeds the 32-module admission limit")
	}
	var body []byte
	var headers map[string]string
	finalURL := abs
	if l.session.moduleFetch != nil {
		result, err := l.session.moduleFetch(base, abs)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch module %q: %w", abs, err)
		}
		body = result.Body
		headers = result.Headers
		if result.URL != "" {
			finalURL = result.URL
		}
		if result.StatusCode >= 400 {
			return nil, fmt.Errorf("module %q returned HTTP %d", abs, result.StatusCode)
		}
	} else {
		if l.session.scriptFetch == nil {
			return nil, fmt.Errorf("failed to fetch module %q", abs)
		}
		data, err := l.session.scriptFetch(base, abs)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch module %q: %w", abs, err)
		}
		body = data
	}
	if time.Now().After(l.deadline) {
		return nil, errors.New("module graph fetch budget exhausted")
	}
	if len(body) > js.MaxScriptBytes {
		return nil, fmt.Errorf("module %q: %w", abs, js.ErrTooLarge)
	}
	if err := validateModuleResponse(l.documentURL, abs, finalURL, headers); err != nil {
		return nil, fmt.Errorf("module %q: %w", abs, err)
	}
	if l.graphBytes+len(body) > maxModuleGraphBytes {
		return nil, errors.New("module graph exceeds the 2 MiB admission limit")
	}
	record, err := l.addRecord(string(body), abs, abs, abs, depth)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func validateModuleResponse(documentURL, requestedURL, finalURL string, headers map[string]string) error {
	corsURL := finalURL
	if corsURL == "" {
		corsURL = requestedURL
	}
	if js.IsCORSRequest(documentURL, requestedURL) || js.IsCORSRequest(documentURL, corsURL) {
		origin := moduleOrigin(documentURL)
		if headers == nil || origin == "" || !js.CheckCORSResponse(origin, headers, false) {
			return errors.New("CORS response refused")
		}
	}
	if headers == nil {
		if js.IsCORSRequest(documentURL, corsURL) {
			return errors.New("CORS response metadata missing")
		}
		return nil
	}
	contentType := js.GetHeader(headers, "Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("JavaScript MIME type %q refused", contentType)
	}
	switch strings.ToLower(mediaType) {
	case "application/ecmascript", "application/javascript", "application/x-ecmascript", "application/x-javascript",
		"text/ecmascript", "text/javascript", "text/javascript1.0", "text/javascript1.1", "text/javascript1.2",
		"text/javascript1.3", "text/javascript1.4", "text/javascript1.5", "text/jscript", "text/livescript":
		return nil
	default:
		return fmt.Errorf("JavaScript MIME type %q refused", contentType)
	}
}

func moduleOrigin(rawURL string) string {
	scheme, rest, ok := strings.Cut(rawURL, "://")
	if !ok {
		return ""
	}
	end := strings.IndexAny(rest, "/?#")
	if end >= 0 {
		rest = rest[:end]
	}
	if rest == "" {
		return ""
	}
	return strings.ToLower(scheme) + "://" + strings.ToLower(rest)
}

func (l *moduleLinker) loadGraph(record *moduleRecord, depth int) error {
	if depth > maxModuleGraphDepth {
		return errors.New("module graph exceeds the 8-level depth limit")
	}
	if record.state == 2 || record.running {
		return nil
	}
	record.running = true
	defer func() { record.running = false }()
	for i := range record.requests {
		if time.Now().After(l.deadline) {
			return errors.New("module graph fetch budget exhausted")
		}
		request := &record.requests[i]
		abs, ok := l.session.resolveScriptURL(record.base, request.spec)
		if !ok {
			return fmt.Errorf("cannot resolve module import %q", request.spec)
		}
		request.target = abs
		target, err := l.loadURL(record.base, abs, depth+1)
		if err != nil {
			return err
		}
		request.targetID = target.id
		for j := range record.exports {
			if record.exports[j].request == i {
				record.exports[j].targetID = target.id
			}
		}
		if err := l.loadGraph(target, depth+1); err != nil {
			return err
		}
	}
	record.state = 2
	return nil
}

func (l *moduleLinker) prepare(record *moduleRecord) error {
	if err := l.loadGraph(record, record.depth); err != nil {
		return err
	}
	l.resolveExports()
	if err := l.validateBindings(); err != nil {
		return err
	}
	return nil
}

func (l *moduleLinker) resolveExports() {
	l.resolved = make([]map[string]moduleExportRef, len(l.records))
	l.ambiguous = make([]map[string]bool, len(l.records))
	for i := range l.resolved {
		l.resolved[i] = make(map[string]moduleExportRef)
		l.ambiguous[i] = make(map[string]bool)
	}
	for iteration := 0; iteration < len(l.records); iteration++ {
		next := make([]map[string]moduleExportRef, len(l.records))
		ambiguous := make([]map[string]bool, len(l.records))
		for i, record := range l.records {
			next[i] = make(map[string]moduleExportRef)
			ambiguous[i] = make(map[string]bool)
			for _, exp := range record.exports {
				if exp.star {
					continue
				}
				ref := moduleExportRef{module: i, name: exp.name}
				if exp.namespace {
					ref = moduleExportRef{module: exp.targetID, name: "*", ns: true}
				} else if exp.local == "" {
					ref = moduleExportRef{module: exp.targetID, name: exp.imported}
				}
				next[i][exp.name] = ref
			}
		}
		for i, record := range l.records {
			for _, exp := range record.exports {
				if !exp.star {
					continue
				}
				for name, ref := range l.resolved[exp.targetID] {
					if name == "default" || ambiguous[i][name] {
						continue
					}
					if existing, ok := next[i][name]; ok && existing != ref {
						delete(next[i], name)
						ambiguous[i][name] = true
						continue
					}
					next[i][name] = ref
				}
			}
		}
		l.resolved, l.ambiguous = next, ambiguous
	}
}

func (l *moduleLinker) validateBindings() error {
	for _, record := range l.records {
		seenImports := make(map[string]bool)
		for _, imp := range record.imports {
			if seenImports[imp.local] {
				return fmt.Errorf("%s: duplicate import binding %q", record.name, imp.local)
			}
			seenImports[imp.local] = true
			if record.locals[imp.local] {
				return fmt.Errorf("%s: import binding %q conflicts with a local declaration", record.name, imp.local)
			}
			if imp.namespace {
				continue
			}
			targetID := record.requests[imp.request].targetID
			if targetID < 0 || targetID >= len(l.resolved) || l.ambiguous[targetID][imp.imported] {
				return fmt.Errorf("%s: module does not provide export %q", record.name, imp.imported)
			}
			if _, ok := l.resolved[targetID][imp.imported]; !ok {
				return fmt.Errorf("%s: module does not provide export %q", record.name, imp.imported)
			}
		}
		for _, exp := range record.exports {
			if exp.local != "" && !moduleHasLocalBinding(record, exp.local) {
				return fmt.Errorf("%s: export %q has no local binding", record.name, exp.name)
			}
			if exp.local != "" || exp.star {
				continue
			}
			targetID := record.requests[exp.request].targetID
			if exp.namespace {
				if targetID < 0 || targetID >= len(l.records) {
					return fmt.Errorf("%s: unresolved namespace re-export", record.name)
				}
				continue
			}
			if targetID < 0 || targetID >= len(l.resolved) || l.ambiguous[targetID][exp.imported] {
				return fmt.Errorf("%s: module does not provide re-export %q", record.name, exp.imported)
			}
			if _, ok := l.resolved[targetID][exp.imported]; !ok {
				return fmt.Errorf("%s: module does not provide re-export %q", record.name, exp.imported)
			}
		}
	}
	return nil
}

func moduleHasLocalBinding(record *moduleRecord, name string) bool {
	if record.locals[name] {
		return true
	}
	for _, imp := range record.imports {
		if imp.local == name {
			return true
		}
	}
	return false
}

func (l *moduleLinker) execute(record *moduleRecord) error {
	if record.executed {
		return nil
	}
	if err := l.prepare(record); err != nil {
		return err
	}
	if err := l.ensureRegistry(); err != nil {
		return err
	}
	return l.executeRecord(record)
}

func (l *moduleLinker) executeRecord(record *moduleRecord) error {
	if record.executed {
		return nil
	}
	if record.running {
		return nil
	}
	record.running = true
	defer func() { record.running = false }()
	for _, request := range record.requests {
		target := l.records[request.targetID]
		if err := l.executeRecord(target); err != nil {
			return err
		}
	}
	wrapped, err := l.wrapModule(record)
	if err != nil {
		return err
	}
	if err := l.runtime.Run(wrapped, record.name); err != nil {
		return err
	}
	record.executed = true
	return nil
}

func (l *moduleLinker) ensureRegistry() error {
	if l.registryUp {
		if l.registryRecords == len(l.records) {
			return nil
		}
		encoded, err := l.registryEntries(l.registryRecords)
		if err != nil {
			return err
		}
		source := l.registryName + ".extend(" + string(encoded) + ");"
		if err := l.runtime.Run(source, "module graph"); err != nil {
			return err
		}
		l.registryRecords = len(l.records)
		return nil
	}
	source, err := l.registrySource()
	if err != nil {
		return err
	}
	if err := l.runtime.Run(source, "module graph"); err != nil {
		return err
	}
	l.registryUp = true
	l.registryRecords = len(l.records)
	return nil
}

func (l *moduleLinker) closeRegistry() {
	if !l.registryUp {
		return
	}
	_ = l.runtime.Run("delete globalThis["+strconv.Quote(l.registryName)+"];", "module graph")
	l.registryUp = false
}

func (l *moduleLinker) registrySource() (string, error) {
	encoded, err := l.registryEntries(0)
	if err != nil {
		return "", err
	}
	var source strings.Builder
	source.WriteString("Object.defineProperty(globalThis,")
	source.WriteString(strconv.Quote(l.registryName))
	source.WriteString(",{configurable:true,value:(function(){var records=")
	source.Write(encoded)
	source.WriteString(`;var graph={extend:function(entries){Array.prototype.push.apply(records,entries);},ns:function(id){var rec=records[id];if(!rec.n){rec.n=Object.create(null);Object.keys(rec.e).forEach(function(name){Object.defineProperty(rec.n,name,{enumerable:true,get:function(){return graph.get(id,name);}});});Object.preventExtensions(rec.n);}return rec.n;},get:function(id,name){var rec=records[id];if(!rec.r){throw new ReferenceError("module binding accessed before initialization");}var ref=rec.e[name];if(!ref){throw new SyntaxError("module does not provide export "+name);}if(ref[2]){return graph.ns(ref[0]);}var target=records[ref[0]];if(!target.r){throw new ReferenceError("module binding accessed before initialization");}if(!Object.prototype.hasOwnProperty.call(target.v,name)){throw new SyntaxError("module does not provide export "+ref[1]);}return target.v[name];},imports:function(id){var rec=records[id],env=Object.create(null);Object.keys(rec.i).forEach(function(local){var ref=rec.i[local];Object.defineProperty(env,local,{enumerable:true,get:function(){if(ref[2]){return graph.ns(ref[0]);}return graph.get(ref[0],ref[1]);},set:function(){throw new TypeError("module imports are read-only");}});});return env;},set:function(id,value){var rec=records[id];rec.v=value;rec.r=true;}};return graph;})()});`)
	return source.String(), nil
}

func (l *moduleLinker) registryEntries(start int) ([]byte, error) {
	type registryRecord struct {
		Exports map[string][3]any `json:"e"`
		Imports map[string][3]any `json:"i"`
	}
	entries := make([]registryRecord, len(l.records)-start)
	for i := start; i < len(l.records); i++ {
		record := l.records[i]
		entries[i-start].Exports = make(map[string][3]any)
		entries[i-start].Imports = make(map[string][3]any)
		for name, ref := range l.resolved[i] {
			entries[i-start].Exports[name] = [3]any{ref.module, ref.name, ref.ns}
		}
		for _, imp := range record.imports {
			targetID := record.requests[imp.request].targetID
			if imp.namespace {
				entries[i-start].Imports[imp.local] = [3]any{targetID, "*", true}
				continue
			}
			ref := l.resolved[targetID][imp.imported]
			entries[i-start].Imports[imp.local] = [3]any{ref.module, ref.name, ref.ns}
		}
	}
	return json.Marshal(entries)
}

func (l *moduleLinker) wrapModule(record *moduleRecord) (string, error) {
	graphName := l.uniqueName(record, "__goosieGraph")
	idName := l.uniqueName(record, "__goosieModuleID")
	importsName := l.uniqueName(record, "__goosieImports")
	exportsName := l.uniqueName(record, "__goosieExports")
	var source strings.Builder
	fmt.Fprintf(&source, "(function(%s,%s,%s){var %s=Object.create(null);with(%s){\n", graphName, idName, importsName, exportsName, importsName)
	source.WriteString(record.body)
	source.WriteByte('\n')
	names := make([]string, 0, len(record.exports))
	for _, exp := range record.exports {
		if exp.local != "" {
			names = append(names, exp.name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		local := ""
		for _, exp := range record.exports {
			if exp.name == name && exp.local != "" {
				local = exp.local
				break
			}
		}
		fmt.Fprintf(&source, "Object.defineProperty(%s,%s,{enumerable:true,get:function(){return %s;}});\n", exportsName, strconv.Quote(name), local)
	}
	fmt.Fprintf(&source, "}%s.set(%s,%s);}).call(undefined,%s,%d,%s.imports(%d));", graphName, idName, exportsName, l.registryName, record.id, l.registryName, record.id)
	if len(source.String()) > js.MaxScriptBytes {
		return "", js.ErrTooLarge
	}
	return source.String(), nil
}

func (l *moduleLinker) uniqueName(record *moduleRecord, base string) string {
	name := base
	for {
		used := false
		for _, token := range record.tokens {
			if token.kind == moduleTokenIdentifier && token.value == name {
				used = true
				break
			}
		}
		if !used {
			return name
		}
		name += "_"
	}
}

func (s *Session) executeDefaultImportModule(runtime *js.Runtime, source, baseURL, name string, deadline time.Time, graphBytes, moduleCount *int) error {
	linker := newModuleLinker(s, runtime, baseURL, deadline)
	if linker == nil {
		return errors.New("module runtime unavailable")
	}
	if graphBytes != nil {
		linker.graphBytes = *graphBytes
	}
	if moduleCount != nil {
		linker.moduleCount = *moduleCount
	}
	defer linker.closeRegistry()
	record, err := linker.addRecord(source, baseURL, name, "", 1)
	if err != nil {
		return err
	}
	if graphBytes != nil {
		*graphBytes = linker.graphBytes
	}
	if moduleCount != nil {
		*moduleCount = linker.moduleCount
	}
	return linker.execute(record)
}

func parseModule(record *moduleRecord) error {
	tokens := record.tokens
	var edits []moduleEdit
	for i := 0; i < len(tokens); {
		token := tokens[i]
		if token.kind != moduleTokenIdentifier || (token.value != "import" && token.value != "export") || !moduleTopLevelAt(tokens, i) {
			i++
			continue
		}
		if token.value == "import" {
			end, imports, _, err := parseModuleImport(record, tokens, i)
			if err != nil {
				return err
			}
			edits = append(edits, moduleEdit{start: token.start, end: tokens[end-1].end, text: ""})
			record.imports = append(record.imports, imports...)
			i = end
			continue
		}
		end, exports, declarationEdits, err := parseModuleExport(record, tokens, i)
		if err != nil {
			return err
		}
		edits = append(edits, declarationEdits...)
		record.exports = append(record.exports, exports...)
		i = end
	}
	if err := collectModuleLocals(record); err != nil {
		return err
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var body strings.Builder
	cursor := 0
	for _, edit := range edits {
		if edit.start < cursor || edit.end < edit.start {
			return errors.New("overlapping module declarations")
		}
		body.WriteString(record.source[cursor:edit.start])
		body.WriteString(edit.text)
		cursor = edit.end
	}
	body.WriteString(record.source[cursor:])
	record.body = body.String()
	for _, token := range tokens {
		if token.kind == moduleTokenIdentifier && token.value == "await" && moduleTopLevelAt(tokens, tokenIndex(tokens, token)) {
			return errors.New("top-level await is outside the supported static module subset")
		}
	}
	return nil
}

func tokenIndex(tokens []moduleToken, target moduleToken) int {
	for i := range tokens {
		if tokens[i].start == target.start {
			return i
		}
	}
	return -1
}

func moduleTopLevelAt(tokens []moduleToken, index int) bool {
	depth := 0
	for i := 0; i < index; i++ {
		switch tokens[i].value {
		case "{", "${", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
		}
	}
	return depth == 0
}

func parseModuleImport(record *moduleRecord, tokens []moduleToken, start int) (int, []moduleImport, []moduleRequest, error) {
	i := start + 1
	if i >= len(tokens) {
		return 0, nil, nil, errors.New("incomplete import declaration")
	}
	if tokens[i].value == "(" || tokens[i].value == "." {
		return 0, nil, nil, fmt.Errorf("%w: dynamic import and import.meta", errStaticModuleSubset)
	}
	var imports []moduleImport
	var requestIndex int
	if tokens[i].kind == moduleTokenString {
		spec, err := tokens[i].value, error(nil)
		requestIndex = len(record.requests)
		record.requests = append(record.requests, moduleRequest{spec: spec, start: tokens[i].start, end: tokens[i].end})
		i++
		return moduleDeclarationEnd(tokens, i), imports, record.requests[requestIndex:], err
	}
	if tokens[i].kind == moduleTokenIdentifier {
		imports = append(imports, moduleImport{local: tokens[i].value, imported: "default", request: len(record.requests)})
		i++
		if i < len(tokens) && tokens[i].value == "," {
			i++
		} else {
			if err := parseImportFrom(record, tokens, &i, &imports); err != nil {
				return 0, nil, nil, err
			}
			return moduleDeclarationEnd(tokens, i), imports, record.requests[len(record.requests)-1:], nil
		}
	}
	if i < len(tokens) && tokens[i].value == "*" {
		i++
		if i >= len(tokens) || tokens[i].value != "as" || i+1 >= len(tokens) || tokens[i+1].kind != moduleTokenIdentifier {
			return 0, nil, nil, errors.New("namespace import requires an identifier")
		}
		imports = append(imports, moduleImport{local: tokens[i+1].value, request: len(record.requests), namespace: true})
		i += 2
	} else if i < len(tokens) && tokens[i].value == "{" {
		named, next, err := parseImportNames(tokens, i, len(record.requests))
		if err != nil {
			return 0, nil, nil, err
		}
		imports = append(imports, named...)
		i = next
	} else {
		return 0, nil, nil, errors.New("import clause is unsupported")
	}
	if err := parseImportFrom(record, tokens, &i, &imports); err != nil {
		return 0, nil, nil, err
	}
	requestStart := len(record.requests) - 1
	return moduleDeclarationEnd(tokens, i), imports, record.requests[requestStart:], nil
}

func parseImportFrom(record *moduleRecord, tokens []moduleToken, index *int, imports *[]moduleImport) error {
	i := *index
	if i >= len(tokens) || tokens[i].value != "from" || i+1 >= len(tokens) || tokens[i+1].kind != moduleTokenString {
		return errors.New("import clause requires a module specifier")
	}
	request := len(record.requests)
	record.requests = append(record.requests, moduleRequest{spec: tokens[i+1].value, start: tokens[i+1].start, end: tokens[i+1].end})
	for j := range *imports {
		if (*imports)[j].request < 0 {
			(*imports)[j].request = request
		}
	}
	*index = moduleDeclarationEnd(tokens, i+2)
	return nil
}

func parseImportNames(tokens []moduleToken, start, request int) ([]moduleImport, int, error) {
	var imports []moduleImport
	i := start + 1
	for {
		if i >= len(tokens) {
			return nil, 0, errors.New("unterminated named import")
		}
		if tokens[i].value == "}" {
			return imports, i + 1, nil
		}
		if tokens[i].kind != moduleTokenIdentifier && tokens[i].kind != moduleTokenString {
			return nil, 0, errors.New("named import requires an identifier")
		}
		imported := tokens[i].value
		i++
		local := imported
		if i < len(tokens) && tokens[i].value == "as" {
			if i+1 >= len(tokens) || tokens[i+1].kind != moduleTokenIdentifier {
				return nil, 0, errors.New("import alias requires an identifier")
			}
			local = tokens[i+1].value
			i += 2
		} else if tokens[i-1].kind == moduleTokenString {
			return nil, 0, errors.New("string-named import requires an alias")
		}
		imports = append(imports, moduleImport{local: local, imported: imported, request: request})
		if i >= len(tokens) {
			return nil, 0, errors.New("unterminated named import")
		}
		if tokens[i].value == "," {
			i++
			continue
		}
		if tokens[i].value != "}" {
			return nil, 0, errors.New("named import requires a comma")
		}
	}
}

func moduleDeclarationEnd(tokens []moduleToken, index int) int {
	if index < len(tokens) && tokens[index].value == ";" {
		return index + 1
	}
	return index
}

func parseModuleExport(record *moduleRecord, tokens []moduleToken, start int) (int, []moduleExport, []moduleEdit, error) {
	i := start + 1
	if i >= len(tokens) {
		return 0, nil, nil, errors.New("incomplete export declaration")
	}
	if tokens[i].value == "default" {
		return parseDefaultExport(record, tokens, start, i+1)
	}
	if tokens[i].value == "{" {
		exports, next, err := parseExportNames(record, tokens, i)
		if err != nil {
			return 0, nil, nil, err
		}
		end := moduleDeclarationEnd(tokens, next)
		return end, exports, []moduleEdit{{start: tokens[start].start, end: tokens[end-1].end}}, nil
	}
	if tokens[i].value == "*" {
		i++
		namespace := false
		name := ""
		if i < len(tokens) && tokens[i].value == "as" {
			if i+1 >= len(tokens) || (tokens[i+1].kind != moduleTokenIdentifier && tokens[i+1].kind != moduleTokenString) {
				return 0, nil, nil, errors.New("namespace re-export requires a name")
			}
			namespace = true
			name = tokens[i+1].value
			i += 2
		}
		request, next, err := parseExportFrom(record, tokens, i)
		if err != nil {
			return 0, nil, nil, err
		}
		exp := moduleExport{request: request, namespace: namespace, star: !namespace}
		if namespace {
			exp.name = name
		}
		end := moduleDeclarationEnd(tokens, next)
		return end, []moduleExport{exp}, []moduleEdit{{start: tokens[start].start, end: tokens[end-1].end}}, nil
	}
	declarationStart := i
	if tokens[i].value == "async" && i+1 < len(tokens) && tokens[i+1].value == "function" {
		i++
	}
	switch tokens[i].value {
	case "const", "let", "var":
		names, next, err := parseVariableDeclaration(tokens, i)
		if err != nil {
			return 0, nil, nil, err
		}
		var exports []moduleExport
		for _, name := range names {
			record.locals[name] = true
			exports = append(exports, moduleExport{name: name, local: name})
		}
		return next, exports, []moduleEdit{{start: tokens[start].start, end: tokens[declarationStart].start}}, nil
	case "function", "class":
		name, end, err := parseNamedFunctionOrClass(tokens, i)
		if err != nil {
			return 0, nil, nil, err
		}
		record.locals[name] = true
		return end, []moduleExport{{name: name, local: name}}, []moduleEdit{{start: tokens[start].start, end: tokens[declarationStart].start}}, nil
	default:
		return 0, nil, nil, fmt.Errorf("%w: export declaration", errStaticModuleSubset)
	}
}

func parseDefaultExport(record *moduleRecord, tokens []moduleToken, start, expressionStart int) (int, []moduleExport, []moduleEdit, error) {
	synthetic := record.defaultName()
	i := expressionStart
	if i < len(tokens) && tokens[i].value == "async" && i+1 < len(tokens) && tokens[i+1].value == "function" {
		i++
	}
	if i < len(tokens) && (tokens[i].value == "function" || tokens[i].value == "class") {
		name := ""
		nameIndex := i + 1
		if nameIndex < len(tokens) && tokens[nameIndex].value == "*" {
			nameIndex++
		}
		if nameIndex < len(tokens) && tokens[nameIndex].kind == moduleTokenIdentifier {
			name = tokens[nameIndex].value
		}
		declarationEnd, err := functionOrClassEnd(tokens, i)
		if err != nil {
			return 0, nil, nil, err
		}
		record.locals[synthetic] = true
		if name != "" {
			text := record.source[tokens[i].start:tokens[declarationEnd-1].end] + "\nconst " + synthetic + " = " + name + ";"
			return declarationEnd, []moduleExport{{name: "default", local: synthetic}}, []moduleEdit{{start: tokens[start].start, end: tokens[declarationEnd-1].end, text: text}}, nil
		}
		text := "const " + synthetic + " = " + record.source[tokens[i].start:tokens[declarationEnd-1].end] + ";"
		return declarationEnd, []moduleExport{{name: "default", local: synthetic}}, []moduleEdit{{start: tokens[start].start, end: tokens[declarationEnd-1].end, text: text}}, nil
	}
	expressionEnd, err := moduleExpressionEnd(tokens, expressionStart)
	if err != nil {
		return 0, nil, nil, err
	}
	text := "const " + synthetic + " = (" + record.source[tokens[expressionStart].start:tokens[expressionEnd-1].end] + ");"
	record.locals[synthetic] = true
	return expressionEnd, []moduleExport{{name: "default", local: synthetic}}, []moduleEdit{{start: tokens[start].start, end: tokens[expressionEnd-1].end, text: text}}, nil
}

func (r *moduleRecord) defaultName() string {
	name := "__goosieDefault"
	for {
		used := false
		for _, token := range r.tokens {
			if token.kind == moduleTokenIdentifier && token.value == name {
				used = true
				break
			}
		}
		if !used {
			return name
		}
		name += "_"
	}
}

func parseExportNames(record *moduleRecord, tokens []moduleToken, start int) ([]moduleExport, int, error) {
	var exports []moduleExport
	var origins []string
	request := -1
	i := start + 1
	for {
		if i >= len(tokens) {
			return nil, 0, errors.New("unterminated export list")
		}
		if tokens[i].value == "}" {
			i++
			break
		}
		if tokens[i].kind != moduleTokenIdentifier && tokens[i].kind != moduleTokenString {
			return nil, 0, errors.New("export list requires an identifier")
		}
		local := tokens[i].value
		name := local
		i++
		if i < len(tokens) && tokens[i].value == "as" {
			if i+1 >= len(tokens) || (tokens[i+1].kind != moduleTokenIdentifier && tokens[i+1].kind != moduleTokenString) {
				return nil, 0, errors.New("export alias requires a name")
			}
			name = tokens[i+1].value
			i += 2
		} else if tokens[i-1].kind == moduleTokenString {
			return nil, 0, errors.New("string-named export requires an alias")
		}
		exports = append(exports, moduleExport{name: name, local: local})
		origins = append(origins, local)
		if i >= len(tokens) {
			return nil, 0, errors.New("unterminated export list")
		}
		if tokens[i].value == "," {
			i++
			continue
		}
		if tokens[i].value != "}" {
			return nil, 0, errors.New("export list requires a comma")
		}
	}
	if i < len(tokens) && tokens[i].value == "from" {
		var err error
		request, i, err = parseExportFrom(record, tokens, i)
		if err != nil {
			return nil, 0, err
		}
		for j := range exports {
			exports[j].local = ""
			exports[j].imported = origins[j]
			exports[j].request = request
		}
	}
	return exports, i, nil
}

func parseExportFrom(record *moduleRecord, tokens []moduleToken, index int) (int, int, error) {
	if index >= len(tokens) || tokens[index].value != "from" || index+1 >= len(tokens) || tokens[index+1].kind != moduleTokenString {
		return 0, 0, errors.New("re-export requires a module specifier")
	}
	request := len(record.requests)
	record.requests = append(record.requests, moduleRequest{spec: tokens[index+1].value, start: tokens[index+1].start, end: tokens[index+1].end})
	return request, moduleDeclarationEnd(tokens, index+2), nil
}

func parseNamedFunctionOrClass(tokens []moduleToken, start int) (string, int, error) {
	i := start + 1
	if i < len(tokens) && tokens[i].value == "*" {
		i++
	}
	if i >= len(tokens) || tokens[i].kind != moduleTokenIdentifier {
		return "", 0, errors.New("exported declaration requires a name")
	}
	name := tokens[i].value
	end, err := functionOrClassEnd(tokens, start)
	return name, end, err
}

func functionOrClassEnd(tokens []moduleToken, start int) (int, error) {
	i := start + 1
	if i < len(tokens) && tokens[i].value == "*" {
		i++
	}
	if i < len(tokens) && tokens[i].kind == moduleTokenIdentifier {
		i++
	}
	parenDepth := 0
	for ; i < len(tokens); i++ {
		switch tokens[i].value {
		case "(":
			parenDepth++
		case ")":
			parenDepth--
		case "{":
			if parenDepth == 0 {
				return matchingBraceEnd(tokens, i), nil
			}
		}
	}
	return 0, errors.New("declaration has no body")
}

func matchingBraceEnd(tokens []moduleToken, start int) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].value {
		case "{", "${":
			depth++
		case "}":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(tokens)
}

func moduleExpressionEnd(tokens []moduleToken, start int) (int, error) {
	if start >= len(tokens) {
		return 0, errors.New("default export requires an expression")
	}
	depth := 0
	last := start
	for i := start; i < len(tokens); i++ {
		value := tokens[i].value
		switch value {
		case "(", "[", "{", "${":
			depth++
		case ")", "]", "}":
			if depth == 0 {
				return i, nil
			}
			depth--
		case ";":
			if depth == 0 {
				return i, nil
			}
		}
		if depth == 0 && i > start && tokens[i].line > tokens[i-1].line && !moduleExpressionContinues(tokens[i]) {
			return i, nil
		}
		last = i + 1
	}
	return last, nil
}

func moduleExpressionContinues(token moduleToken) bool {
	switch token.value {
	case ".", "?.", "[", "(", ",", "?", ":", "=", "+", "-", "*", "/", "%", "**", "&&", "||", "??", "&", "|", "^", "==", "===", "!=", "!==", "<", ">", "<=", ">=", "<<", ">>", ">>>", "in", "instanceof":
		return true
	}
	return token.kind == moduleTokenTemplate
}

func parseVariableDeclaration(tokens []moduleToken, start int) ([]string, int, error) {
	var names []string
	i := start + 1
	for {
		bound, next, err := parseBindingPattern(tokens, i)
		if err != nil {
			return nil, 0, err
		}
		names = append(names, bound...)
		i = next
		depth := 0
		for i < len(tokens) {
			value := tokens[i].value
			if depth == 0 && (value == ";" || value == ",") {
				break
			}
			if depth == 0 && i > next && tokens[i].line > tokens[i-1].line && moduleStatementStarts(tokens[i]) {
				return names, i, nil
			}
			switch value {
			case "(", "[", "{", "${":
				depth++
			case ")", "]", "}":
				depth--
			}
			i++
		}
		if i >= len(tokens) {
			return names, i, nil
		}
		if tokens[i].value == ";" {
			return names, i + 1, nil
		}
		i++
	}
}

func parseBindingPattern(tokens []moduleToken, start int) ([]string, int, error) {
	if start >= len(tokens) {
		return nil, 0, errors.New("variable declaration requires a binding")
	}
	if tokens[start].kind == moduleTokenIdentifier {
		return []string{tokens[start].value}, start + 1, nil
	}
	open := tokens[start].value
	if open != "{" && open != "[" {
		return nil, 0, errors.New("destructured export declaration is unsupported")
	}
	close := "}"
	if open == "[" {
		close = "]"
	}
	var names []string
	i := start + 1
	for {
		if i >= len(tokens) {
			return nil, 0, errors.New("unterminated binding pattern")
		}
		if tokens[i].value == close {
			return names, i + 1, nil
		}
		if tokens[i].value == "," {
			i++
			continue
		}
		if tokens[i].value == "..." {
			i++
		}
		if open == "{" {
			if tokens[i].kind != moduleTokenIdentifier && tokens[i].kind != moduleTokenString {
				return nil, 0, errors.New("object binding requires a property name")
			}
			i++
			if i < len(tokens) && tokens[i].value == ":" {
				i++
			}
		}
		bound, next, err := parseBindingPattern(tokens, i)
		if err != nil {
			return nil, 0, err
		}
		names = append(names, bound...)
		i = next
		if i < len(tokens) && tokens[i].value == "=" {
			i = skipModuleExpression(tokens, i+1, close)
		}
		if i < len(tokens) && tokens[i].value == "," {
			i++
			continue
		}
		if i >= len(tokens) || tokens[i].value != close {
			return nil, 0, errors.New("binding pattern requires a comma")
		}
	}
}

func skipModuleExpression(tokens []moduleToken, start int, stop string) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		value := tokens[i].value
		if depth == 0 && (value == "," || value == stop) {
			return i
		}
		switch value {
		case "(", "[", "{", "${":
			depth++
		case ")", "]", "}":
			depth--
		}
	}
	return len(tokens)
}

func moduleStatementStarts(token moduleToken) bool {
	if token.kind != moduleTokenIdentifier {
		return false
	}
	switch token.value {
	case "const", "let", "var", "function", "class", "if", "for", "while", "switch", "return", "throw", "try", "import", "export":
		return true
	}
	return false
}

func collectModuleLocals(record *moduleRecord) error {
	tokens := record.tokens
	for i := 0; i < len(tokens); i++ {
		if !moduleTopLevelAt(tokens, i) || tokens[i].kind != moduleTokenIdentifier {
			continue
		}
		switch tokens[i].value {
		case "const", "let", "var":
			names, next, err := parseVariableDeclaration(tokens, i)
			if err != nil {
				return err
			}
			for _, name := range names {
				record.locals[name] = true
			}
			i = next - 1
		case "function", "class":
			name, next, err := parseNamedFunctionOrClass(tokens, i)
			if err != nil {
				return err
			}
			record.locals[name] = true
			i = next - 1
		case "async":
			if i+1 < len(tokens) && tokens[i+1].value == "function" {
				name, next, err := parseNamedFunctionOrClass(tokens, i+1)
				if err != nil {
					return err
				}
				record.locals[name] = true
				i = next - 1
			}
		}
	}
	return nil
}

func lexModule(source string) ([]moduleToken, error) {
	lexer := moduleLexer{source: source, line: 1}
	tokens, err := lexer.scanCode(false)
	if err != nil {
		return nil, err
	}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].kind == moduleTokenIdentifier && tokens[i].value == "import" && (tokens[i+1].value == "(" || tokens[i+1].value == ".") {
			return nil, fmt.Errorf("%w: dynamic import and import.meta", errStaticModuleSubset)
		}
	}
	return tokens, nil
}

type moduleLexer struct {
	source string
	index  int
	line   int
}

func (l *moduleLexer) scanCode(stopTemplateExpression bool) ([]moduleToken, error) {
	var tokens []moduleToken
	depth := 0
	for l.index < len(l.source) {
		ch := l.source[l.index]
		if stopTemplateExpression && ch == '}' && depth == 0 {
			return tokens, nil
		}
		if moduleSpace(ch) {
			l.advance(1)
			continue
		}
		if ch == '/' && l.index+1 < len(l.source) && l.source[l.index+1] == '/' {
			l.advance(2)
			for l.index < len(l.source) && l.source[l.index] != '\n' {
				l.advance(1)
			}
			continue
		}
		if ch == '/' && l.index+1 < len(l.source) && l.source[l.index+1] == '*' {
			startLine := l.line
			l.advance(2)
			closed := false
			for l.index < len(l.source) {
				if l.source[l.index] == '*' && l.index+1 < len(l.source) && l.source[l.index+1] == '/' {
					l.advance(2)
					closed = true
					break
				}
				l.advance(1)
			}
			if !closed {
				return nil, fmt.Errorf("line %d: unterminated comment", startLine)
			}
			continue
		}
		start, line := l.index, l.line
		if ch == '\'' || ch == '"' {
			value, err := l.scanString()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, moduleToken{kind: moduleTokenString, start: start, end: l.index, line: line, value: value})
			continue
		}
		if ch == '`' {
			templateTokens, err := l.scanTemplate()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, templateTokens...)
			continue
		}
		if ch == '/' && moduleRegexStarts(tokens) && !(l.index+1 < len(l.source) && (l.source[l.index+1] == '/' || l.source[l.index+1] == '*')) {
			if err := l.scanRegex(); err != nil {
				return nil, err
			}
			tokens = append(tokens, moduleToken{kind: moduleTokenRegex, start: start, end: l.index, line: line})
			continue
		}
		r, size := utf8.DecodeRuneInString(l.source[l.index:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("line %d: invalid UTF-8", line)
		}
		if unicode.IsLetter(r) || r == '_' || r == '$' {
			l.advance(size)
			for l.index < len(l.source) {
				r, size = utf8.DecodeRuneInString(l.source[l.index:])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
					break
				}
				l.advance(size)
			}
			tokens = append(tokens, moduleToken{kind: moduleTokenIdentifier, start: start, end: l.index, line: line, value: l.source[start:l.index]})
			continue
		}
		if unicode.IsDigit(r) {
			l.advance(size)
			for l.index < len(l.source) {
				ch := l.source[l.index]
				if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F' || ch == '.' || ch == '_' || ch == 'x' || ch == 'X' || ch == 'o' || ch == 'O' || ch == 'b' || ch == 'B' || ch == 'n' || ch == 'N' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-') {
					break
				}
				l.advance(1)
			}
			tokens = append(tokens, moduleToken{kind: moduleTokenNumber, start: start, end: l.index, line: line})
			continue
		}
		value := l.source[l.index : l.index+1]
		if strings.HasPrefix(l.source[l.index:], "${") {
			value = "${"
		} else {
			for _, punct := range []string{">>>=", "===", "!==", "**=", "&&=", "||=", "??=", "<<=", ">>=", ">>>", "...", "=>", "==", "!=", "<=", ">=", "++", "--", "&&", "||", "??", "?.", "**", "<<", ">>", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^="} {
				if strings.HasPrefix(l.source[l.index:], punct) {
					value = punct
					break
				}
			}
		}
		l.advance(len(value))
		switch value {
		case "(", "[", "{", "${":
			depth++
		case ")", "]", "}":
			depth--
		}
		tokens = append(tokens, moduleToken{kind: moduleTokenPunct, start: start, end: l.index, line: line, value: value})
	}
	if stopTemplateExpression {
		return nil, errors.New("unterminated template expression")
	}
	return tokens, nil
}

func (l *moduleLexer) scanTemplate() ([]moduleToken, error) {
	var tokens []moduleToken
	l.advance(1)
	for {
		start, line := l.index, l.line
		for l.index < len(l.source) {
			ch := l.source[l.index]
			if ch == '\\' {
				l.advance(1)
				if l.index < len(l.source) {
					l.advance(1)
				}
				continue
			}
			if ch == '`' {
				tokens = append(tokens, moduleToken{kind: moduleTokenTemplate, start: start, end: l.index, line: line})
				l.advance(1)
				return tokens, nil
			}
			if ch == '$' && l.index+1 < len(l.source) && l.source[l.index+1] == '{' {
				tokens = append(tokens, moduleToken{kind: moduleTokenTemplate, start: start, end: l.index, line: line})
				l.advance(2)
				tokens = append(tokens, moduleToken{kind: moduleTokenPunct, start: l.index - 2, end: l.index, line: line, value: "${"})
				expression, err := l.scanCode(true)
				if err != nil {
					return nil, err
				}
				if l.index >= len(l.source) || l.source[l.index] != '}' {
					return nil, errors.New("unterminated template expression")
				}
				tokens = append(tokens, expression...)
				closeStart := l.index
				l.advance(1)
				tokens = append(tokens, moduleToken{kind: moduleTokenPunct, start: closeStart, end: l.index, line: l.line, value: "}"})
				break
			}
			l.advance(1)
		}
		if l.index >= len(l.source) && (len(tokens) == 0 || tokens[len(tokens)-1].value != "`") {
			return nil, fmt.Errorf("line %d: unterminated template literal", line)
		}
	}
}

func (l *moduleLexer) scanString() (string, error) {
	startLine := l.line
	quote := l.source[l.index]
	l.advance(1)
	var value strings.Builder
	for l.index < len(l.source) {
		ch := l.source[l.index]
		l.advance(1)
		if ch == quote {
			return value.String(), nil
		}
		if ch == '\n' || ch == '\r' {
			return "", fmt.Errorf("line %d: unterminated string literal", startLine)
		}
		if ch != '\\' {
			value.WriteByte(ch)
			continue
		}
		if l.index >= len(l.source) {
			break
		}
		escaped := l.source[l.index]
		l.advance(1)
		switch escaped {
		case '\n':
		case '\r':
			if l.index < len(l.source) && l.source[l.index] == '\n' {
				l.advance(1)
			}
		case 'n':
			value.WriteByte('\n')
		case 'r':
			value.WriteByte('\r')
		case 't':
			value.WriteByte('\t')
		case 'b':
			value.WriteByte('\b')
		case 'f':
			value.WriteByte('\f')
		case 'v':
			value.WriteByte('\v')
		case '0':
			value.WriteByte(0)
		case 'x':
			r, err := l.scanHex(2)
			if err != nil {
				return "", err
			}
			value.WriteRune(r)
		case 'u':
			if l.index < len(l.source) && l.source[l.index] == '{' {
				l.advance(1)
				start := l.index
				for l.index < len(l.source) && l.source[l.index] != '}' {
					l.advance(1)
				}
				if l.index >= len(l.source) {
					return "", errors.New("unterminated Unicode escape")
				}
				number, err := strconv.ParseUint(l.source[start:l.index], 16, 32)
				if err != nil {
					return "", err
				}
				l.advance(1)
				value.WriteRune(rune(number))
			} else {
				r, err := l.scanHex(4)
				if err != nil {
					return "", err
				}
				value.WriteRune(r)
			}
		default:
			value.WriteByte(escaped)
		}
	}
	return "", fmt.Errorf("line %d: unterminated string literal", startLine)
}

func (l *moduleLexer) scanHex(count int) (rune, error) {
	if l.index+count > len(l.source) {
		return 0, errors.New("incomplete hexadecimal escape")
	}
	number, err := strconv.ParseUint(l.source[l.index:l.index+count], 16, 32)
	if err != nil {
		return 0, err
	}
	l.advance(count)
	return rune(number), nil
}

func (l *moduleLexer) scanRegex() error {
	l.advance(1)
	inClass := false
	for l.index < len(l.source) {
		ch := l.source[l.index]
		l.advance(1)
		if ch == '\\' {
			if l.index < len(l.source) {
				l.advance(1)
			}
			continue
		}
		if ch == '[' {
			inClass = true
			continue
		}
		if ch == ']' {
			inClass = false
			continue
		}
		if ch == '/' && !inClass {
			for l.index < len(l.source) {
				r, size := utf8.DecodeRuneInString(l.source[l.index:])
				if !unicode.IsLetter(r) {
					break
				}
				l.advance(size)
			}
			return nil
		}
		if ch == '\n' || ch == '\r' {
			return errors.New("unterminated regular expression")
		}
	}
	return errors.New("unterminated regular expression")
}

func moduleRegexStarts(tokens []moduleToken) bool {
	if len(tokens) == 0 {
		return true
	}
	previous := tokens[len(tokens)-1]
	if previous.kind == moduleTokenIdentifier {
		switch previous.value {
		case "return", "throw", "case", "delete", "void", "typeof", "new", "in", "of", "instanceof", "yield", "await", "else", "do":
			return true
		}
		return false
	}
	if previous.kind == moduleTokenString || previous.kind == moduleTokenNumber || previous.kind == moduleTokenTemplate || previous.kind == moduleTokenRegex {
		return false
	}
	switch previous.value {
	case ")", "]", "}", "++", "--":
		return false
	}
	return true
}

func moduleSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v'
}

func (l *moduleLexer) advance(count int) {
	for i := 0; i < count && l.index < len(l.source); i++ {
		if l.source[l.index] == '\n' {
			l.line++
		}
		l.index++
	}
}
