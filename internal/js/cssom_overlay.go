package js

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

const cssomStateKey = "__goosieCSSOMState"

const (
	cssOMDefaultBytes        = 4 << 20
	cssOMDefaultTokens       = 1 << 20
	cssOMDefaultRules        = 32768
	cssOMDefaultSelectors    = 65536
	cssOMDefaultDeclarations = 1 << 18
)

type CSSOMSheet struct {
	Owner       *dom.Node
	Text        string
	BaseURL     string
	Media       string
	Linked      bool
	Constructed bool
	Overlay     bool
	Disabled    bool
	Apply       bool
}

type CSSOMOverlay struct {
	Revision uint64
	Sheets   []CSSOMSheet
}

type CSSOMOverlayFunc func() CSSOMOverlay

type CSSOMAdmissionFunc func(string) error

type cssomStateObject struct {
	state *cssomRuntimeState
}

func (o *cssomStateObject) Get(string) goja.Value       { return nil }
func (o *cssomStateObject) Set(string, goja.Value) bool { return false }
func (o *cssomStateObject) Has(string) bool             { return false }
func (o *cssomStateObject) Delete(string) bool          { return false }
func (o *cssomStateObject) Keys() []string              { return nil }

type cssomRuntimeState struct {
	mu           sync.Mutex
	revision     uint64
	admission    CSSOMAdmissionFunc
	constructed  map[*goja.Object]*cachedCSSRules
	documentDeps map[*dom.Document][]goja.Value
}

func (r *Runtime) cssomState() *cssomRuntimeState {
	if r == nil || r.vm == nil || r.cssStyleSheetProto == nil {
		return nil
	}
	if value := r.cssStyleSheetProto.Get(cssomStateKey); value != nil && !goja.IsUndefined(value) {
		if object, ok := value.(*goja.Object); ok {
			if holder, ok := object.Export().(*cssomStateObject); ok {
				return holder.state
			}
		}
	}
	state := &cssomRuntimeState{
		constructed:  make(map[*goja.Object]*cachedCSSRules),
		documentDeps: make(map[*dom.Document][]goja.Value),
	}
	_ = r.cssStyleSheetProto.DefineDataProperty(cssomStateKey, r.vm.NewDynamicObject(&cssomStateObject{state: state}), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	return state
}

func (r *Runtime) CSSOMOverlayProvider() CSSOMOverlayFunc {
	return r.cssomOverlaySnapshot
}

func (r *Runtime) SetCSSOMAdmissionProvider(fn CSSOMAdmissionFunc) {
	if state := r.cssomState(); state != nil {
		state.mu.Lock()
		state.admission = fn
		state.mu.Unlock()
	}
}

func (r *Runtime) CSSOMOverlaySnapshot() CSSOMOverlay {
	return r.cssomOverlaySnapshot()
}

func (r *Runtime) cssomOverlaySnapshot() CSSOMOverlay {
	state := r.cssomState()
	if state == nil {
		return CSSOMOverlay{}
	}
	state.mu.Lock()
	revision := state.revision
	adopted := make(map[*dom.Document][]goja.Value, len(state.documentDeps))
	for doc, sheets := range state.documentDeps {
		adopted[doc] = append([]goja.Value(nil), sheets...)
	}
	state.mu.Unlock()
	out := CSSOMOverlay{Revision: revision}
	doc := r.domDoc
	for _, owner := range styleSheetOwners(doc) {
		view := r.sheetRuleView(owner)
		sheet := CSSOMSheet{
			Owner:   owner,
			Text:    view.effective,
			BaseURL: r.opts.URL,
			Media:   owner.GetAttribute("media"),
			Apply:   true,
		}
		if owner.Element() && owner.Data == "link" {
			sheet.Linked = true
			if href, ok := r.linkedSheetHref(owner); ok {
				sheet.BaseURL = href
			}
			sheet.Disabled = owner.HasAttribute("disabled")
			sheet.Apply = !sheet.Disabled && !alternateStyleSheet(owner) && r.allowsStyleSheetURL(sheet.BaseURL)
		} else {
			sheet.Apply = r.allowsInlineStyleSheet(owner)
		}
		sheet.Disabled = sheet.Disabled || r.sheetDisabled[owner] || view.disabled
		sheet.Overlay = view.effective != view.text
		out.Sheets = append(out.Sheets, sheet)
	}
	for _, object := range adopted[doc] {
		view := r.constructedSheetView(object)
		if view == nil {
			continue
		}
		out.Sheets = append(out.Sheets, CSSOMSheet{
			Text:        view.effective,
			BaseURL:     r.opts.URL,
			Constructed: true,
			Disabled:    view.disabled,
			Apply:       !view.disabled && r.allowsInlineStyleSheet(nil),
		})
	}
	state.mu.Lock()
	out.Revision = state.revision
	state.mu.Unlock()
	return out
}

func alternateStyleSheet(owner *dom.Node) bool {
	if owner == nil {
		return false
	}
	for _, token := range strings.Fields(owner.GetAttribute("rel")) {
		if strings.EqualFold(token, "alternate") {
			return true
		}
	}
	return false
}

func (r *Runtime) allowsStyleSheetURL(raw string) bool {
	return r == nil || r.opts.CSP == nil || r.opts.CSP.AllowsStyle(raw, r.opts.URL)
}

func (r *Runtime) allowsInlineStyleSheet(owner *dom.Node) bool {
	if r == nil || r.opts.CSP == nil {
		return true
	}
	nonce := ""
	if owner != nil {
		nonce = owner.GetAttribute("nonce")
	}
	return r.opts.CSP.AllowsInlineStyleWithNonce(nonce)
}

func (r *Runtime) AllowsStyleSheetURL(raw string) bool {
	return r.allowsStyleSheetURL(raw)
}

func (r *Runtime) markCSSOMDirty() {
	if state := r.cssomState(); state != nil {
		state.mu.Lock()
		state.revision++
		state.mu.Unlock()
	}
}

func (r *Runtime) notifyCSSOMChange(view *cachedCSSRules) {
	if view != nil && view.object != nil {
		_ = view.object.Set("__constructedCSS", view.effective)
	}
	r.syncAdoptedStyleSheetTexts()
	r.markCSSOMDirty()
	r.notifyMutation()
}

func (r *Runtime) syncAdoptedStyleSheetTexts() {
	for root, values := range r.adoptedStyleSheets {
		texts := make([]string, 0, len(values))
		for _, value := range values {
			if view := r.constructedSheetView(value); view != nil {
				texts = append(texts, view.effective)
			}
		}
		root.AdoptedStyleSheetTexts = texts
	}
}

func (r *Runtime) admitCSSOMText(text string) {
	if err := defaultCSSOMAdmission(text); err != nil {
		panic(r.vm.NewGoError(err))
	}
	state := r.cssomState()
	if state == nil {
		return
	}
	state.mu.Lock()
	admit := state.admission
	state.mu.Unlock()
	if admit != nil {
		if err := admit(text); err != nil {
			panic(r.vm.NewGoError(err))
		}
	}
}

func defaultCSSOMAdmission(text string) error {
	if len(text) > cssOMDefaultBytes {
		return fmt.Errorf("CSS byte limit exceeded (%d)", cssOMDefaultBytes)
	}
	rules, declarations := 0, 0
	for _, c := range text {
		switch c {
		case '{':
			rules++
		case ';':
			declarations++
		}
	}
	if rules > cssOMDefaultRules {
		return fmt.Errorf("CSS rule limit exceeded (%d)", cssOMDefaultRules)
	}
	if declarations > cssOMDefaultDeclarations {
		return fmt.Errorf("CSS declaration limit exceeded (%d)", cssOMDefaultDeclarations)
	}
	tokens := 0
	tokenizer := css.NewTokenizer(text)
	for {
		token := tokenizer.Next()
		if token.Type == css.TokenEOF {
			break
		}
		tokens++
		if tokens > cssOMDefaultTokens {
			return fmt.Errorf("CSS token limit exceeded (%d)", cssOMDefaultTokens)
		}
	}
	selectors := 0
	for _, rule := range css.Parse(text).Rules {
		selectors += len(rule.Selectors)
		if selectors > cssOMDefaultSelectors {
			return fmt.Errorf("CSS selector count limit exceeded (%d)", cssOMDefaultSelectors)
		}
	}
	return nil
}

func (r *Runtime) newConstructedStyleSheet(newTarget *goja.Object) *goja.Object {
	obj := r.vm.NewObject()
	prototype := r.cssStyleSheetProto
	if newTarget != nil {
		if value := newTarget.Get("prototype"); value != nil && goja.IsUndefined(value) == false && goja.IsNull(value) == false {
			if candidate, ok := value.(*goja.Object); ok {
				prototype = candidate
			}
		}
	}
	_ = obj.SetPrototype(prototype)
	view := &cachedCSSRules{object: obj, constructed: true}
	if state := r.cssomState(); state != nil {
		state.mu.Lock()
		state.constructed[obj] = view
		state.mu.Unlock()
	}
	if r.cssSheetOwners == nil {
		r.cssSheetOwners = make(map[*goja.Object]*dom.Node)
	}
	r.cssSheetOwners[obj] = nil
	r.markCSSOMDirty()
	return obj
}

func (r *Runtime) constructedSheetView(value goja.Value) *cachedCSSRules {
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return nil
	}
	if state := r.cssomState(); state != nil {
		state.mu.Lock()
		view := state.constructed[obj]
		state.mu.Unlock()
		return view
	}
	return nil
}

func (r *Runtime) requireSheetView(value goja.Value) *cachedCSSRules {
	obj, ok := value.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	if view := r.constructedSheetView(obj); view != nil {
		return view
	}
	owner, registered := r.cssSheetOwners[obj]
	if !registered || owner == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	return r.sheetRuleView(owner)
}

func (r *Runtime) cssRuleListForSheetValue(value goja.Value) goja.Value {
	if view := r.constructedSheetView(value); view != nil {
		if view.list == nil {
			view.list = r.newCSSRuleList(view)
		}
		return view.list
	}
	return r.cssRuleList(r.sheetOwner(value))
}

func (r *Runtime) resetSheetRuleView(view *cachedCSSRules, text string) {
	if view == nil {
		return
	}
	for object, info := range r.cssRuleWrappers {
		if info.view == view {
			delete(r.cssRuleWrappers, object)
		}
	}
	view.effective = text
	view.parsed = css.Parse(text).Rules
	view.rules = r.wrapCSSRules(view, view.parsed)
	r.markCSSOMDirty()
}

func (r *Runtime) insertSheetRule(view *cachedCSSRules, source string, index uint32) {
	r.admitCSSOMText(source)
	parsed := css.Parse(source)
	if len(parsed.Rules) != 1 {
		panic(r.domError("SyntaxError", "Failed to execute 'insertRule' on 'CSSStyleSheet': Failed to parse the rule '"+source+"'."))
	}
	if uint64(index) > uint64(len(view.parsed)) {
		max := len(view.parsed)
		panic(r.domError("IndexSizeError", "Failed to execute 'insertRule' on 'CSSStyleSheet': The index provided ("+strconv.FormatUint(uint64(index), 10)+") is larger than the maximum index ("+strconv.Itoa(max)+")."))
	}
	text, ok := insertCSSOMRuleText(view.effective, int(index), source)
	if !ok {
		panic(r.domError("SyntaxError", "Failed to execute 'insertRule' on 'CSSStyleSheet': Failed to parse the rule '"+source+"'."))
	}
	r.admitCSSOMText(text)
	next := css.Parse(text).Rules
	if len(next) != len(view.parsed)+1 {
		panic(r.domError("SyntaxError", "Failed to execute 'insertRule' on 'CSSStyleSheet': Failed to parse the rule '"+source+"'."))
	}
	at := int(index)
	for object, info := range r.cssRuleWrappers {
		if info.view == view && info.index >= at {
			info.index++
			r.cssRuleWrappers[object] = info
		}
	}
	view.effective = text
	view.parsed = next
	view.rules = append(view.rules, nil)
	copy(view.rules[at+1:], view.rules[at:])
	view.rules[at] = r.wrapCSSStyleRule(view, at)
	r.notifyCSSOMChange(view)
}

func (r *Runtime) deleteSheetRule(view *cachedCSSRules, index uint32) {
	if uint64(index) >= uint64(len(view.parsed)) {
		max := int64(len(view.parsed)) - 1
		panic(r.domError("IndexSizeError", "Failed to execute 'deleteRule' on 'CSSStyleSheet': The index provided ("+strconv.FormatUint(uint64(index), 10)+") is larger than the maximum index ("+strconv.FormatInt(max, 10)+")."))
	}
	at := int(index)
	text, ok := mutateCSSOMRuleText(view.effective, at, "", true)
	if !ok {
		panic(r.domError("SyntaxError", "Failed to execute 'deleteRule' on 'CSSStyleSheet': The rule could not be removed."))
	}
	r.admitCSSOMText(text)
	next := css.Parse(text).Rules
	if len(next) != len(view.parsed)-1 {
		panic(r.domError("SyntaxError", "Failed to execute 'deleteRule' on 'CSSStyleSheet': The rule could not be removed."))
	}
	removed := view.rules[at]
	if info, ok := r.cssRuleWrappers[removed]; ok {
		info.index = -1
		r.cssRuleWrappers[removed] = info
	}
	for object, info := range r.cssRuleWrappers {
		if info.view == view && info.index > at {
			info.index--
			r.cssRuleWrappers[object] = info
		}
	}
	view.effective = text
	view.parsed = next
	view.rules = append(view.rules[:at], view.rules[at+1:]...)
	r.notifyCSSOMChange(view)
}

type cssomTextSegment struct {
	start     int
	end       int
	ruleStart int
	ruleCount int
}

func cssomTextSegments(text string, ruleCount int) ([]cssomTextSegment, bool) {
	var segments []cssomTextSegment
	ruleStart := 0
	for i := 0; i < len(text); {
		start := i
		for i < len(text) {
			if isCSSOMSpace(text[i]) {
				i++
				continue
			}
			if i+1 < len(text) && text[i] == '/' && text[i+1] == '*' {
				if end := strings.Index(text[i+2:], "*/"); end >= 0 {
					i += end + 4
					continue
				}
				i = len(text)
				break
			}
			break
		}
		if i >= len(text) {
			segments = append(segments, cssomTextSegment{start: start, end: i, ruleStart: ruleStart})
			break
		}
		quote := byte(0)
		escaped := false
		parens, braces := 0, 0
		for i < len(text) {
			c := text[i]
			if quote != 0 {
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if c == quote {
					quote = 0
				}
				i++
				continue
			}
			if c == '\'' || c == '"' {
				quote = c
				i++
				continue
			}
			if c == '/' && i+1 < len(text) && text[i+1] == '*' {
				if end := strings.Index(text[i+2:], "*/"); end >= 0 {
					i += end + 4
					continue
				}
				i = len(text)
				break
			}
			switch c {
			case '(':
				parens++
			case ')':
				if parens > 0 {
					parens--
				}
			case '{':
				braces++
			case '}':
				if braces > 0 {
					braces--
					if braces == 0 && parens == 0 {
						i++
						goto segmentDone
					}
				}
			case ';':
				if braces == 0 && parens == 0 {
					i++
					goto segmentDone
				}
			}
			i++
		}
	segmentDone:
		count := len(css.Parse(text[start:i]).Rules)
		segments = append(segments, cssomTextSegment{start: start, end: i, ruleStart: ruleStart, ruleCount: count})
		ruleStart += count
	}
	return segments, ruleStart == ruleCount
}

func isCSSOMSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func cssomSegmentForRule(segments []cssomTextSegment, index int) (cssomTextSegment, bool) {
	for _, segment := range segments {
		if index >= segment.ruleStart && index < segment.ruleStart+segment.ruleCount {
			return segment, true
		}
	}
	return cssomTextSegment{}, false
}

func insertCSSOMRuleText(text string, index int, rule string) (string, bool) {
	parsed := css.Parse(text)
	if index < 0 || index > len(parsed.Rules) {
		return "", false
	}
	if index == len(parsed.Rules) {
		if text != "" && !isCSSOMSpace(text[len(text)-1]) {
			text += "\n"
		}
		return text + rule, true
	}
	segments, ok := cssomTextSegments(text, len(parsed.Rules))
	if !ok {
		return "", false
	}
	segment, ok := cssomSegmentForRule(segments, index)
	if !ok {
		return "", false
	}
	return text[:segment.start] + rule + "\n" + text[segment.start:], true
}

func mutateCSSOMRuleText(text string, index int, replacement string, remove bool) (string, bool) {
	parsed := css.Parse(text)
	if index < 0 || index >= len(parsed.Rules) {
		return "", false
	}
	segments, ok := cssomTextSegments(text, len(parsed.Rules))
	if !ok {
		return "", false
	}
	segment, ok := cssomSegmentForRule(segments, index)
	if !ok {
		return "", false
	}
	if segment.ruleCount == 1 {
		if remove {
			return text[:segment.start] + text[segment.end:], true
		}
		return text[:segment.start] + replacement + text[segment.end:], true
	}
	piece := text[segment.start:segment.end]
	open, close := cssomBlockBodyRange(piece)
	if open < 0 || close < 0 || !strings.HasPrefix(strings.TrimSpace(piece), "@") {
		return "", false
	}
	body := piece[open:close]
	next, ok := mutateCSSOMRuleText(body, index-segment.ruleStart, replacement, remove)
	if !ok {
		return "", false
	}
	return text[:segment.start] + piece[:open] + next + piece[close:] + text[segment.end:], true
}

func cssomBlockBodyRange(text string) (int, int) {
	quote := byte(0)
	escaped := false
	open := -1
	depth := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			if end := strings.Index(text[i+2:], "*/"); end >= 0 {
				i += end + 3
				continue
			}
		}
		switch c {
		case '{':
			if open < 0 {
				open = i + 1
			}
			depth++
		case '}':
			depth--
			if depth == 0 && open >= 0 {
				return open, i
			}
		}
	}
	return -1, -1
}
