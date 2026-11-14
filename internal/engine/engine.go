package engine

import (
	"encoding/base64"
	"errors"
	"fmt"
	stdimage "image"
	"math"
	"mime"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	imgdec "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/internal/userscript"
)

type Session struct {
	Doc          *dom.Document
	Styles       map[dom.NodeID]*style.ComputedStyle
	PseudoStyles map[style.PseudoKey]*style.ComputedStyle
	Arena        *layout.Arena

	metrics     layout.Metrics
	viewportW   float32
	viewportH   float32
	dpr         float32
	linkBase    string
	linker      CSSLinker
	documentURL string

	scriptBase       string
	scriptFetch      ScriptFetcher
	contentLanguage  string
	documentEncoding string
	iframeLoader     IFrameDocumentLoader
	iframeFormSubmit IFrameFormSubmitter

	sheets        []*css.Stylesheet
	styleResolver *style.Resolver

	imgBase                 string
	imgFetch                ImageFetcher
	natural                 map[dom.NodeID]layout.NaturalSize
	images                  map[dom.NodeID]stdimage.Image
	scriptImgs              map[dom.NodeID]scriptImagePixels
	pendingScriptImgs       map[dom.NodeID]scriptImagePixels
	scriptImgsLayoutPending bool
	bgImages                map[dom.NodeID]stdimage.Image
	pseudoBgImages          map[*style.ComputedStyle]stdimage.Image
	pseudoBgVectorRef       map[*style.ComputedStyle]string
	borderImages            map[dom.NodeID]stdimage.Image
	masks                   map[dom.NodeID]stdimage.Image
	bgVectorSrc             map[string][]byte
	bgVectorRef             map[dom.NodeID]string
	bgNoRatio               map[string]bool
	imgReservation          *imageReservation
	bgVectorUsed            int64
	scriptImgAttempts       int
	scriptImgDeadline       time.Time
	sub                     *subBudget
	cssText                 *cssTextCache
	styleInstalled          styleInstall

	imgMu sync.Mutex

	mixedMu      sync.Mutex
	mixedReports []MixedContentReport
	mixedSeen    map[string]struct{}

	deferImages bool
	pendingImgs []imgRef

	fontBase  string
	fontFetch FontFetcher
	fontReg   FontRegistry

	deferFonts   bool
	pendingFonts []fontJob

	customFonts *style.CustomFonts

	focus                  *dom.Node
	focusState             atomic.Pointer[dom.Node]
	hovered                *dom.Node
	active                 *dom.Node
	keyboardActive         *dom.Node
	keyboardActiveMarked   []*dom.Node
	pendingKey             *heldKey
	caret                  int
	controlSelectionAnchor int
	controlSelectionActive bool
	marked                 string
	markedNode             dom.NodeID
	markedHasNode          bool
	markedOffset           int
	imeComposing           bool
	focusValue             string
	focusUserChanged       bool
	focusStylePending      bool
	submittedForm          *FormSubmission
	rangePointerNode       *dom.Node
	rangePointerMoved      bool

	pendingFilePicker *dom.Node

	selActive bool
	selAnchor selPos
	selHead   selPos

	findMarks  []findMark
	findActive int

	validationAnchor  *dom.Node
	validationMessage string

	selectPopup *dom.Node

	inspectNode *dom.Node

	jsRT                  *js.Runtime
	providedDoc           *dom.Document
	parserExecutedScripts map[*dom.Node]bool
	parserExternalCount   int
	runtimeBound          bool
	onViewportScroll      func(x, y float64)
	documentStartRan      bool

	drag           *dragState
	testDriverDrag *testDriverDragCandidate

	touches  map[string]*dom.Touch
	touchSeq int

	scriptErrMu        sync.Mutex
	scriptErrs         []ScriptError
	scriptErrTotal     int
	scriptTimeoutTotal int

	jsNodeBudget *docNodeBudget

	media *mediaRegistry

	Transitions *TransitionTracker

	Animations *AnimationController

	workCounts  pipelineWorkCounts
	styleDirty  atomic.Bool
	layoutDirty atomic.Bool

	inv *Invalidation

	darkMode     bool
	highContrast bool

	userScripts []*userscript.Script

	lifecycleMu    sync.RWMutex
	opMu           sync.Mutex
	deferredActive int32
}

type WorkCounts struct {
	StyleResolves     uint64
	StyleNodeVisits   uint64
	LayoutBuilds      uint64
	DisplayListBuilds uint64
	CSSCollections    uint64
}

type pipelineWorkCounts struct {
	styleResolves     atomic.Uint64
	styleNodeVisits   atomic.Uint64
	layoutBuilds      atomic.Uint64
	displayListBuilds atomic.Uint64
	cssCollections    atomic.Uint64
}

func (s *Session) WorkCounts() WorkCounts {
	if s == nil {
		return WorkCounts{}
	}
	return WorkCounts{
		StyleResolves:     s.workCounts.styleResolves.Load(),
		StyleNodeVisits:   s.workCounts.styleNodeVisits.Load(),
		LayoutBuilds:      s.workCounts.layoutBuilds.Load(),
		DisplayListBuilds: s.workCounts.displayListBuilds.Load(),
		CSSCollections:    s.workCounts.cssCollections.Load(),
	}
}

func (s *Session) collectSheets(doc *dom.Document, authorCSS []string) ([]*css.Stylesheet, error) {
	s.workCounts.cssCollections.Add(1)
	return checkedSheets(doc, authorCSS, s.linkBase, s.linker, s.mediaOptions(), s.sub, s.cssText, s.reportMixed)
}

func (s *Session) LayoutPassCounts() [5]uint64 {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Arena == nil {
		return [5]uint64{}
	}
	return s.Arena.PassCounts
}

type CSSLinker func(base, href string) (string, error)

type IFrameDocumentLoader func(base, src string) (*dom.Document, error)

type IFrameFormSubmitter func(base, target, contentType string, body []byte) (*dom.Document, error)

type ImageFetcher func(base, url string) ([]byte, error)

type FontResult struct {
	Data    []byte
	Headers map[string]string
}

type FontFetcher func(base, url string) (FontResult, error)

type FontRegistry interface {
	Register(data []byte) (uint16, error)
}

type Option func(*Session)

func WithMetrics(m layout.Metrics) Option {
	return func(s *Session) { s.metrics = m }
}

func WithViewportH(h float32) Option {
	return func(s *Session) {
		if finite(float64(h)) && h > 0 {
			s.viewportH = h
		}
	}
}

func WithDevicePixelRatio(dpr float32) Option {
	return func(s *Session) {
		if validateScale(float64(dpr)) == nil {
			s.dpr = dpr
		}
	}
}

func validateDocumentTreeLimits(doc *dom.Document, limits dom.ParseLimits) error {
	if doc == nil {
		return fmt.Errorf("pre-parsed document is nil")
	}
	nodes, attributes, attributeBytes := 0, 0, 0
	var visit func(*dom.Node, int) error
	visit = func(node *dom.Node, depth int) error {
		if node == nil {
			return nil
		}
		nodes++
		if nodes > limits.Nodes {
			return fmt.Errorf("document node limit exceeded (%d)", limits.Nodes)
		}
		if depth > limits.Depth {
			return fmt.Errorf("document tree depth limit exceeded (%d)", limits.Depth)
		}
		attributes += len(node.Attr)
		for _, attr := range node.Attr {
			attributeBytes += len(attr.Name) + len(attr.Value)
		}
		if attributes > limits.Attributes {
			return fmt.Errorf("document attribute limit exceeded (%d)", limits.Attributes)
		}
		if attributeBytes > limits.AttributeBytes {
			return fmt.Errorf("document attribute byte limit exceeded (%d)", limits.AttributeBytes)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(&doc.Node, 0)
}

func WithLinkedCSS(base string, fetch CSSLinker) Option {
	return func(s *Session) {
		s.linkBase = base
		s.linker = fetch
		if base != "" {
			s.documentURL = base
		}
	}
}

func WithImages(base string, fetch ImageFetcher) Option {
	return func(s *Session) {
		s.imgBase = base
		s.imgFetch = fetch
		if base != "" {
			s.documentURL = base
		}
	}
}

func WithDeferredImages(base string, fetch ImageFetcher) Option {
	return func(s *Session) {
		s.imgBase = base
		s.imgFetch = fetch
		s.deferImages = true
		if base != "" {
			s.documentURL = base
		}
	}
}

func WithCustomFontLoading(base string, fetch FontFetcher, reg FontRegistry) Option {
	return func(s *Session) {
		s.fontBase = base
		s.fontFetch = fetch
		s.fontReg = reg
		if base != "" {
			s.documentURL = base
		}
	}
}

func WithDeferredCustomFontLoading(base string, fetch FontFetcher, reg FontRegistry) Option {
	return func(s *Session) {
		s.fontBase = base
		s.fontFetch = fetch
		s.fontReg = reg
		s.deferFonts = true
		if base != "" {
			s.documentURL = base
		}
	}
}

func WithScripts(base string, fetch ScriptFetcher) Option {
	return func(s *Session) {
		s.scriptBase = base
		s.scriptFetch = fetch
		if base != "" {
			s.documentURL = base
		}
	}
}

func WithIFrameDocumentLoader(loader IFrameDocumentLoader) Option {
	return func(s *Session) {
		if loader == nil {
			s.iframeLoader = nil
			return
		}
		s.iframeLoader = func(base, src string) (*dom.Document, error) {
			if abs, mixed := mixedContentURL(base, src); mixed {
				s.reportMixed(MixedContentIFrame, abs)
				return nil, errMixedContentBlocked
			}
			return loader(base, src)
		}
	}
}

func WithIFrameFormSubmitter(submit IFrameFormSubmitter) Option {
	return func(s *Session) { s.iframeFormSubmit = submit }
}

func WithContentLanguage(value string) Option {
	return func(s *Session) { s.contentLanguage = value }
}

func WithDocumentEncoding(label string) Option {
	return func(s *Session) { s.documentEncoding = documentEncodingLabel(label) }
}

func documentEncodingLabel(label string) string {
	if strings.EqualFold(label, "utf-8") {
		return "UTF-8"
	}
	if strings.HasPrefix(strings.ToLower(label), "iso-8859-") {
		return strings.ToUpper(label)
	}
	return label
}

func WithJS(rt *js.Runtime) Option {
	return func(s *Session) { s.jsRT = rt }
}

func WithDocumentTree(doc *dom.Document) Option {
	return func(s *Session) { s.providedDoc = doc }
}

func WithInvalidation(inv *Invalidation) Option {
	return func(s *Session) { s.inv = inv }
}

func WithDarkMode(enabled bool) Option {
	return func(s *Session) {
		s.darkMode = enabled
	}
}

func WithHighContrast(enabled bool) Option {
	return func(s *Session) {
		s.highContrast = enabled
	}
}

func WithUserScripts(scripts []*userscript.Script) Option {
	return func(s *Session) {
		s.userScripts = scripts
	}
}

type fontJob struct {
	family, srcURL, weightStr, styleStr string
	base                                string
}

func NewSession(html string, authorCSS []string, viewportW float32, opts ...Option) (*Session, error) {
	s := &Session{sub: &subBudget{}, cssText: &cssTextCache{}}
	for _, opt := range opts {
		opt(s)
	}
	if err := validateWidth(viewportW); err != nil {
		return nil, err
	}
	s.viewportW = viewportW
	if s.dpr <= 0 {
		s.dpr = 1.0
	}
	if len(html) > MaxDocumentBytes {
		return nil, fmt.Errorf("HTML byte limit exceeded (%d)", MaxDocumentBytes)
	}
	parseLimits := dom.ParseLimits{
		Nodes: MaxDocumentNodes, Depth: MaxDocumentDepth,
		Attributes: MaxAttributes, AttributeBytes: MaxAttributeBytes,
	}
	var doc *dom.Document
	var err error
	if s.providedDoc != nil {
		doc = s.providedDoc
		if err = validateDocumentTreeLimits(doc, parseLimits); err != nil {
			return nil, err
		}
	} else if s.jsRT != nil {
		doc, err = dom.ParseBoundedWithParserCallbacks(html, parseLimits,
			func(node *dom.Node, attrs []dom.Attribute) *dom.Node {
				if s.runtimeBound && s.jsRT != nil {
					return s.jsRT.UpgradeParserElementWithAttributes(node, attrs)
				}
				return node
			},
			func(node *dom.Node, attr dom.Attribute) {
				if s.runtimeBound && s.jsRT != nil {
					s.jsRT.ParserElementAttributeChanged(node, attr)
				}
			},
			func(node *dom.Node) {
				if s.runtimeBound && s.jsRT != nil {
					s.jsRT.ParserNodeInserted(node)
				}
			},
			func(doc *dom.Document, node *dom.Node) {
				if s.documentEncoding != "" {
					doc.CharacterSet = s.documentEncoding
				}
				doc.ContentLanguage = s.contentLanguage
				s.runParserScript(doc, node, authorCSS)
			},
		)
	} else {
		doc, err = dom.ParseBounded(html, parseLimits)
	}
	if err != nil {
		return nil, err
	}
	if s.documentEncoding != "" {
		doc.CharacterSet = s.documentEncoding
	}
	s.Doc = doc
	doc.ContentLanguage = s.contentLanguage
	s.bindJSRuntime(doc)
	s.initImageMaps()
	preScriptSheets, err := s.collectSheets(doc, authorCSS)
	if err != nil {
		return nil, err
	}
	s.styleInstalled = s.installSignature(doc)
	s.sheets = preScriptSheets
	s.styleResolver = style.NewResolver(preScriptSheets)
	s.styleDirty.Store(true)
	s.layoutDirty.Store(true)
	s.runScripts()
	s.clearDetachedFocus()
	sheets := preScriptSheets
	if sig := s.installSignature(doc); sig != s.styleInstalled {
		sheets, err = s.collectSheets(doc, authorCSS)
		if err != nil {
			return nil, err
		}
		s.styleInstalled = sig
	}
	s.loadFonts(sheets)
	s.sheets = sheets
	s.styleResolver = style.NewResolver(sheets)
	s.workCounts.styleResolves.Add(1)
	s.Styles = s.styleResolver.ResolveViewport(doc, s.styleViewport(), s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
	s.PseudoStyles = s.styleResolver.ResolvePseudoElements(doc, s.Styles, s.customFonts)
	s.styleDirty.Store(false)
	s.startAnimations()
	s.loadImages()
	if err := s.Reflow(viewportW); err != nil {
		return nil, err
	}
	startupTaskRan := s.jsRT != nil && s.jsRT.TickDeferredTimer()
	lifecycleMutated := false
	if s.jsRT != nil {
		s.jsRT.AutofocusDocument()
		lifecycleMutated = s.jsRT.FireDocumentDOMContentLoaded()
		s.jsRT.FlushIFrameLoads()
		lifecycleMutated = s.jsRT.FireDocumentLoad() || lifecycleMutated
	}
	if startupTaskRan || lifecycleMutated {
		if err := s.RestyleAndReflow(viewportW); err != nil {
			return nil, err
		}
	}
	s.noteScriptTimeout()
	return s, nil
}

func stylesheetDOMSignature(doc *dom.Document) string {
	if doc == nil {
		return ""
	}
	var signature strings.Builder
	appendField := func(value string) {
		fmt.Fprintf(&signature, "%d:%s", len(value), value)
	}
	doc.Node.Walk(func(node *dom.Node) bool {
		if !node.Element() {
			return true
		}
		switch node.Data {
		case "style":
			appendField("style")
			appendField(node.GetAttribute("type"))
			appendField(node.GetAttribute("media"))
			appendField(node.TextContent())
		case "link":
			appendField("link")
			appendField(node.GetAttribute("rel"))
			appendField(node.GetAttribute("href"))
			appendField(node.GetAttribute("media"))
		}
		return true
	})
	return signature.String()
}

type styleInstall struct {
	dom   string
	media css.MediaOptions
}

func (s *Session) installSignature(doc *dom.Document) styleInstall {
	return styleInstall{dom: stylesheetDOMSignature(doc), media: s.mediaOptions()}
}

func (s *Session) bindJSRuntime(doc *dom.Document) {
	s.Doc = doc
	s.initImageMaps()
	if s.jsRT == nil || doc == nil {
		return
	}
	firstBind := !s.runtimeBound
	if firstBind {
		if s.iframeLoader != nil {
			s.jsRT.SetIFrameLoaderWithBase(s.iframeLoader)
		}
		if s.iframeFormSubmit != nil {
			s.jsRT.SetIFrameFormSubmitter(s.iframeFormSubmit)
		}
		if s.scriptFetch != nil {
			s.jsRT.SetIFrameScriptFetcher(s.scriptFetch)
			s.jsRT.SetDynamicScriptLimit(maxExternalScripts)
			s.jsRT.SetDynamicScriptFetcher(func(base, src string) ([]byte, error) {
				abs, ok := s.resolveScriptURL(base, src)
				if !ok || !s.jsRT.AllowsScriptURL(abs, base) {
					return nil, errors.New("external script blocked")
				}
				return s.scriptFetch(base, abs)
			})
		}
		s.jsRT.SetIFrameModuleRunner(func(runtime *js.Runtime, childDoc *dom.Document, documentURL string) {
			if childDoc == nil {
				return
			}
			root := childDoc.HTML
			if root == nil {
				root = &childDoc.Node
			}
			s.runModuleScripts(runtime, root, documentURL, maxExternalScripts, time.Now().Add(ScriptFetchBudget))
		})
		s.jsRT.SetComputedStyleProvider(s.computedStyleValues)
		s.jsRT.SetPseudoStyleProvider(s.pseudoStyleValues)
		s.jsRT.SetLinkedCSSProvider(s.StyleSheetText)
		s.installLayoutProviders()
		s.jsRT.SetSubresourceBudget(s.sub)
		s.jsRT.SetImageLoader(s)
		s.jsRT.SetImagePixelSink(s.retainScriptImage)
		s.jsRT.SetViewportRect(0, 0, s.viewportW, s.viewportH)
		s.jsRT.SetDevicePixelRatio(float64(s.dpr))
		s.jsRT.ChainActiveElementProvider(s.ActiveElement)
		s.jsRT.ChainOnFocusChange(func(node *dom.Node, focused bool) {
			if focused {
				s.FocusNode(node)
			} else {
				s.BlurNode(node)
			}
		})
		s.jsRT.ChainOnFocusRestore(func(node *dom.Node) {
			s.RestoreFocusNode(node)
		})
		var testDriverClipboardText, testDriverClipboardHTML string
		s.jsRT.ChainOnTestDriverKeyDefault(func(key string, shift, ctrl, alt, meta bool) {
			if meta && !ctrl && !alt && key == "Delete" && s.FocusedEditable() {
				s.deleteContentEditableSoftLineForward()
				return
			}
			if alt && (key == "Backspace" || key == "Delete") {
				s.deleteContentEditableByWord(key == "Delete")
				return
			}
			if (ctrl || meta) && !alt {
				switch strings.ToLower(key) {
				case "backspace":
					s.deleteContentEditableByWord(false)
				case "delete":
					s.deleteContentEditableByWord(true)
				case "a":
					s.SelectAllInFocus()
				case "c":
					testDriverClipboardText, testDriverClipboardHTML = s.CopySelectionData()
				case "x":
					testDriverClipboardText, testDriverClipboardHTML, _ = s.CutSelectionData()
				case "v":
					s.PasteData(testDriverClipboardText, testDriverClipboardHTML)
				case "z":
					if host := s.Focused(); s.FocusedEditable() && host != nil && s.jsRT != nil {
						inputType := "historyUndo"
						if shift {
							inputType = "historyRedo"
						}
						if s.jsRT.DispatchTrustedInputEvent(host, nil, inputType) {
							s.jsRT.DispatchTrustedInput(host, nil, inputType, false)
						}
					}
				case "b":
					if host := s.Focused(); s.FocusedEditable() && host != nil && s.jsRT != nil {
						if s.jsRT.DispatchTrustedInputEvent(host, nil, "formatBold") {
							_, _ = s.jsRT.EvaluateUserInput(`document.execCommand("bold")`, "formatBold")
						}
					}
				}
				return
			}
			if shift && !ctrl && !alt && !meta && key == "Enter" {
				s.EditWithShift(EditEnter, 0, true)
				return
			}
			if ctrl || alt || meta {
				return
			}
			switch key {
			case "Backspace":
				s.Edit(EditBackspace, 0)
			case "Delete":
				s.Edit(EditDelete, 0)
			case "ArrowLeft":
				s.Edit(EditLeft, 0)
			case "ArrowRight":
				s.Edit(EditRight, 0)
			case "Enter":
				s.Edit(EditEnter, '\n')
			default:
				runes := []rune(key)
				if len(runes) == 1 && runes[0] >= 0x20 {
					s.Edit(EditRune, runes[0])
				}
			}
		})
		s.jsRT.ChainOnTestDriverPointer(s.TestDriverPointerAction)
		s.jsRT.ChainOnTestDriverWheel(s.TestDriverWheelAction)
		s.jsRT.ChainOnTestDriverTouch(s.TestDriverTouchAction)
		s.jsRT.ChainOnEscapeKey(func() bool {
			return s.CancelTestDriverDrag() || s.CloseTopmostAutoPopover()
		})
		s.jsRT.ChainOnUserInput(s.MarkUserInput)
		if s.media != nil {
			s.jsRT.SetMediaProvider(s.MediaController)
		}
		s.jsRT.ChainOnMutation(s.JSMutationSink())
		s.jsRT.ChainOnMutation(func() {
			s.styleDirty.Store(true)
			s.layoutDirty.Store(true)
		})
		s.jsRT.ChainOnViewportScroll(func(x, y float64) {
			if s.onViewportScroll != nil {
				s.onViewportScroll(x, y)
			}
		})
	}
	if firstBind {
		s.jsRT.SetDOM(doc)
		s.runtimeBound = true
	} else {
		s.jsRT.QueueExistingIFrames()
	}
	s.jsRT.RefreshDocumentTreeReferences()
	s.jsNodeBudget = newDocNodeBudget(doc)
	s.jsRT.SetMutationBudget(s.jsNodeBudget)
	if !s.documentStartRan {
		s.runUserScripts(userscript.RunAtDocumentStart)
		s.documentStartRan = true
	}
}

func (s *Session) runParserScript(doc *dom.Document, node *dom.Node, authorCSS []string) {
	if s.jsRT == nil || doc == nil || node == nil || !node.Element() || node.Data != "script" || !isClassicScript(node) {
		return
	}
	if node.Namespace != dom.NSHTML && (node.Namespace != dom.NSSVG || !node.SVGScriptReady) {
		return
	}
	if s.parserExecutedScripts == nil {
		s.parserExecutedScripts = make(map[*dom.Node]bool)
	}
	if s.parserExecutedScripts[node] {
		return
	}
	s.bindJSRuntime(doc)
	src := scriptSrc(node)
	if src != "" {
		if s.parserExternalCount >= maxExternalScripts {
			s.parserExecutedScripts[node] = true
			return
		}
		s.parserExternalCount++
	}
	if src != "" && (node.HasAttribute("async") || node.HasAttribute("defer")) {
		return
	}
	source, name := node.TextContent(), "inline"
	if src != "" {
		name = src
		if strings.HasPrefix(strings.ToLower(src), "data:") {
			var ok bool
			source, ok = decodeDataScriptURL(src)
			if !ok {
				s.parserExecutedScripts[node] = true
				return
			}
		} else {
			if s.scriptFetch == nil {
				s.parserExecutedScripts[node] = true
				return
			}
			abs, ok := s.resolveScriptURL(s.scriptBase, src)
			if !ok {
				s.parserExecutedScripts[node] = true
				return
			}
			name = abs
			data, err := s.scriptFetch(s.scriptBase, abs)
			if err != nil {
				s.parserExecutedScripts[node] = true
				return
			}
			source = string(data)
		}
	}
	s.parserExecutedScripts[node] = true
	if source == "" {
		return
	}
	if sig := s.installSignature(doc); sig != s.styleInstalled {
		sheets, err := s.collectSheets(doc, authorCSS)
		if err != nil {
			s.recordScriptError(err)
		} else {
			s.sheets = sheets
			s.styleResolver = style.NewResolver(sheets)
			s.styleDirty.Store(true)
			s.layoutDirty.Store(true)
			s.styleInstalled = sig
		}
	}
	if err := s.jsRT.RunParserScript(source, name); err != nil {
		s.recordScriptError(err)
	}
}

func (s *Session) loadFonts(sheets []*css.Stylesheet) {
	if s.fontFetch == nil || s.fontReg == nil {
		return
	}
	jobs := collectFontJobs(sheets, s.fontBase)
	if len(jobs) == 0 {
		return
	}
	if s.deferFonts {
		s.pendingFonts = jobs
		return
	}
	s.applyFontJobs(jobs)
}

func (s *Session) DeferredFontsPending() int {
	return len(s.pendingFonts)
}

func (s *Session) LoadDeferredFonts(onReady func(loaded int)) {
	jobs := s.pendingFonts
	s.pendingFonts = nil
	if len(jobs) == 0 {
		if onReady != nil {
			onReady(0)
		}
		return
	}
	atomic.AddInt32(&s.deferredActive, 1)
	defer atomic.AddInt32(&s.deferredActive, -1)
	s.applyFontJobs(jobs)
	if onReady != nil {
		onReady(1)
	}
}

func (s *Session) RefreshAfterFonts() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if len(s.sheets) == 0 {
		return
	}
	s.workCounts.styleResolves.Add(1)
	if s.styleResolver == nil {
		s.styleResolver = style.NewResolver(s.sheets)
	}
	s.Styles = s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))
	s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, s.Styles, s.customFonts)
}

func (s *Session) applyFontJobs(jobs []fontJob) {
	data := make([]FontResult, len(jobs))
	deadline := time.Now().Add(FontFetchBudget)
	const fontWorkers = 8
	sem := make(chan struct{}, fontWorkers)
	var wg sync.WaitGroup
	for i := range jobs {
		if time.Now().After(deadline) {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fetched, err := s.fontFetch(s.fontBase, jobs[i].srcURL)
			if err != nil {
				return
			}
			if !s.sub.take(len(fetched.Data)) {
				return
			}
			data[i] = fetched
		}(i)
	}
	wg.Wait()

	fonts := &style.CustomFonts{}
	var fetchedBytes int64
	for i, j := range jobs {
		if len(data[i].Data) == 0 {
			continue
		}
		fetchedBytes += int64(len(data[i].Data))
		if fetchedBytes > maxFontBytes {
			return
		}
		if !fontCORSAllowed(jobs[i].base, jobs[i].srcURL, data[i].Headers) {
			continue
		}
		idx, err := s.fontReg.Register(data[i].Data)
		if err != nil {
			continue
		}
		bold := j.weightStr == "bold" || j.weightStr == "700" || j.weightStr == "800" || j.weightStr == "900"
		light := j.weightStr == "300" || j.weightStr == "200" || j.weightStr == "100"
		italic := j.styleStr == "italic" || j.styleStr == "oblique"
		fonts.Register(j.family, idx, bold, italic, light)
	}
	s.opMu.Lock()
	s.customFonts = fonts
	s.opMu.Unlock()
}

func collectFontJobs(sheets []*css.Stylesheet, fontBase string) []fontJob {
	var jobs []fontJob
	for _, sheet := range sheets {
		for _, ff := range sheet.FontFaces {
			if len(jobs) >= maxFontRequests {
				break
			}
			var family, srcURL, weightStr, styleStr string
			for _, d := range ff.Declarations {
				switch d.Property {
				case "font-family":
					family = strings.Trim(d.Value, "\"'")
				case "src":
					srcURL = extractFontURL(d.Value)
				case "font-weight":
					weightStr = strings.TrimSpace(d.Value)
				case "font-style":
					styleStr = strings.TrimSpace(d.Value)
				}
			}
			if family == "" || srcURL == "" {
				continue
			}
			abs, ok := resolveSubresourceURL(fontBase, srcURL)
			if !ok {
				continue
			}
			jobs = append(jobs, fontJob{family: family, srcURL: abs, weightStr: weightStr, styleStr: styleStr, base: fontBase})
		}
	}
	return jobs
}

func extractFontURL(v string) string {
	t, ok := css.ScanURL(v)
	if !ok {
		return ""
	}
	return css.URLPayload(t)
}

const maxFontRequests = 16

const maxFontBytes = 10 << 20

const FontFetchBudget = 6 * time.Second

const maxLoadedImages = 256

type ScriptFetcher func(base, url string) ([]byte, error)

const maxExternalScripts = 32

const ScriptFetchBudget = 6 * time.Second

const ImageFetchBudget = 6 * time.Second

const MaxDocumentDecodedPixels = 33554432

type imageReservation struct {
	limit    int64
	reserved int64
	mu       sync.Mutex
}

func newImageReservation(limit int64) *imageReservation {
	return &imageReservation{limit: limit}
}

func (r *imageReservation) reserve(pixels int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pixels > r.limit-r.reserved {
		return false
	}
	r.reserved += pixels
	return true
}

func (r *imageReservation) release(pixels int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reserved -= pixels
}

type imgRef struct {
	id     dom.NodeID
	abs    string
	bg     bool
	pseudo *style.ComputedStyle
	border bool
	mask   bool
}

func (s *Session) loadImages() {
	if s.Doc == nil {
		return
	}
	s.initImageMaps()
	if s.deferImages {
		s.pendingImgs = s.collectImageRefs()
		return
	}
	refs := s.collectImageRefs()
	s.applyImages(refs, s.fetchImages(refs))
}

func (s *Session) initImageMaps() {
	if s.imgReservation != nil {
		return
	}
	s.natural = make(map[dom.NodeID]layout.NaturalSize)
	s.images = make(map[dom.NodeID]stdimage.Image)
	s.scriptImgs = make(map[dom.NodeID]scriptImagePixels)
	s.pendingScriptImgs = make(map[dom.NodeID]scriptImagePixels)
	s.bgImages = make(map[dom.NodeID]stdimage.Image)
	s.pseudoBgImages = make(map[*style.ComputedStyle]stdimage.Image)
	s.pseudoBgVectorRef = make(map[*style.ComputedStyle]string)
	s.borderImages = make(map[dom.NodeID]stdimage.Image)
	s.masks = make(map[dom.NodeID]stdimage.Image)
	s.bgVectorSrc = make(map[string][]byte)
	s.bgVectorRef = make(map[dom.NodeID]string)
	s.bgNoRatio = make(map[string]bool)
	s.imgReservation = newImageReservation(MaxDocumentDecodedPixels)
	s.bgVectorUsed = 0
}

func (s *Session) DeferredImagesPending() int {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	return len(s.pendingImgs)
}

func (s *Session) LoadDeferredImages(onReady func(loaded int)) {
	s.imgMu.Lock()
	refs := s.pendingImgs
	s.pendingImgs = nil
	s.imgMu.Unlock()
	loaded := 0
	if len(refs) > 0 && s.imgFetch != nil {
		atomic.AddInt32(&s.deferredActive, 1)
		defer atomic.AddInt32(&s.deferredActive, -1)
		byURL := s.fetchImages(refs)
		s.applyImages(refs, byURL)
		loaded = len(byURL)
	}
	if onReady != nil {
		onReady(loaded)
	}
}

func (s *Session) collectImageRefs() []imgRef {
	var refs []imgRef
	var uniq []string
	seen := make(map[string]bool)
	_, _ = walkDocument(s.Doc, func(n *dom.Node) error {
		if !n.Element() {
			return nil
		}
		if n.Data == "img" {
			if src := strings.TrimSpace(resolveImgSource(n, s.viewportW)); src != "" {
				if strings.HasPrefix(src, "data:") {
					refs = append(refs, imgRef{id: n.ID, abs: src})
					if !seen[src] {
						seen[src] = true
						uniq = append(uniq, src)
					}
					return nil
				}
				if abs, ok := s.resolveImageURL(s.imgBase, src); ok {
					refs = append(refs, imgRef{id: n.ID, abs: abs})
					if !seen[abs] {
						seen[abs] = true
						uniq = append(uniq, abs)
					}
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.BackgroundImage != "" {
			bg := st.BackgroundImage
			if strings.HasPrefix(bg, "data:") {
				refs = append(refs, imgRef{id: n.ID, abs: bg, bg: true})
				if !seen[bg] {
					seen[bg] = true
					uniq = append(uniq, bg)
				}
			} else if abs, ok := s.resolveImageURL(s.imgBase, bg); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, bg: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		for _, pseudo := range []string{"before", "after", "first-letter"} {
			ps, ok := s.PseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: pseudo}]
			if !ok || ps.BackgroundImage == "" {
				continue
			}
			bg := ps.BackgroundImage
			if strings.HasPrefix(bg, "data:") {
				refs = append(refs, imgRef{abs: bg, bg: true, pseudo: ps})
				if !seen[bg] {
					seen[bg] = true
					uniq = append(uniq, bg)
				}
			} else if abs, ok := s.resolveImageURL(s.imgBase, bg); ok {
				refs = append(refs, imgRef{abs: abs, bg: true, pseudo: ps})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.MaskImage != "" {
			m := st.MaskImage
			if strings.HasPrefix(m, "data:") {
				refs = append(refs, imgRef{id: n.ID, abs: m, mask: true})
				if !seen[m] {
					seen[m] = true
					uniq = append(uniq, m)
				}
			} else if abs, ok := s.resolveImageURL(s.imgBase, m); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, mask: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		if st, ok := s.Styles[n.ID]; ok && st.BorderImage.Source != "" {
			src := st.BorderImage.Source
			if strings.HasPrefix(src, "data:") {
				refs = append(refs, imgRef{id: n.ID, abs: src, bg: true, border: true})
				if !seen[src] {
					seen[src] = true
					uniq = append(uniq, src)
				}
			} else if abs, ok := s.resolveImageURL(s.imgBase, src); ok {
				refs = append(refs, imgRef{id: n.ID, abs: abs, bg: true, border: true})
				if !seen[abs] {
					seen[abs] = true
					uniq = append(uniq, abs)
				}
			}
		}
		return nil
	})
	return refs
}

func (s *Session) fetchImageBytes(u string) ([]byte, error) {
	if strings.HasPrefix(u, "data:") {
		return decodeDataImageURL(u)
	}
	if s.imgFetch == nil {
		return nil, fmt.Errorf("engine: no image fetcher for %q", u)
	}
	if bad, mixed := mixedContentURL(s.documentBase(), u); mixed {
		s.reportMixed(MixedContentImage, bad)
		return nil, errMixedContentBlocked
	}
	return s.imgFetch(s.imgBase, u)
}

func (s *Session) fetchImages(refs []imgRef) map[string]stdimage.Image {
	uniq := make([]string, 0, len(refs))
	seen := make(map[string]bool)
	bgOnly := make(map[string]bool)
	for _, r := range refs {
		if !seen[r.abs] {
			seen[r.abs] = true
			uniq = append(uniq, r.abs)
			bgOnly[r.abs] = r.bg
		} else if !r.bg {
			bgOnly[r.abs] = false
		}
	}
	byURL := make(map[string]stdimage.Image, len(uniq))
	var mu sync.Mutex
	loaded := 0
	if len(uniq) > maxLoadedImages {
		uniq = uniq[:maxLoadedImages]
	}
	deadline := time.Now().Add(ImageFetchBudget)
	const imageWorkers = 12
	sem := make(chan struct{}, imageWorkers)
	var wg sync.WaitGroup
	s.imgMu.Lock()
	if s.imgReservation == nil {
		s.imgReservation = newImageReservation(MaxDocumentDecodedPixels)
	}
	reservation := s.imgReservation
	s.imgMu.Unlock()
	for _, u := range uniq {
		u := u
		if time.Now().After(deadline) {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			mu.Lock()
			full := loaded >= maxLoadedImages
			mu.Unlock()
			if full {
				return
			}
			data, err := s.fetchImageBytes(u)
			if err != nil || len(data) == 0 {
				return
			}
			if !s.sub.take(len(data)) {
				return
			}
			if bgOnly[u] && imgdec.IsSVG(data) {
				hasDims, hasRatio := imgdec.SVGIntrinsicKind(data)
				if (!hasDims && !hasRatio) || imgdec.SVGPreserveNone(data) {
					mu.Lock()
					if s.bgVectorSrc == nil {
						s.bgVectorSrc = make(map[string][]byte)
					}
					s.bgVectorSrc[u] = data
					if !hasDims && !hasRatio {
						if s.bgNoRatio == nil {
							s.bgNoRatio = make(map[string]bool)
						}
						s.bgNoRatio[u] = true
					}
					mu.Unlock()
					return
				}
			}
			cfg, err := imgdec.Probe(data)
			if err != nil {
				return
			}
			pixels := int64(cfg.Width) * int64(cfg.Height)
			if !reservation.reserve(pixels) {
				return
			}
			img, err := imgdec.Decode(data)
			if err != nil {
				reservation.release(pixels)
				return
			}
			mu.Lock()
			loaded++
			byURL[u] = img
			mu.Unlock()
		}()
	}
	wg.Wait()
	return byURL
}

func (s *Session) applyImages(refs []imgRef, byURL map[string]stdimage.Image) {
	s.imgMu.Lock()
	defer s.imgMu.Unlock()
	for _, r := range refs {
		if r.mask {
			if img, ok := byURL[r.abs]; ok {
				s.masks[r.id] = img
			}
			continue
		}
		if r.bg {
			if _, ok := s.bgVectorSrc[r.abs]; ok {
				if r.border {
					continue
				}
				if r.pseudo != nil {
					s.pseudoBgVectorRef[r.pseudo] = r.abs
				} else {
					s.bgVectorRef[r.id] = r.abs
				}
				continue
			}
		}
		img, ok := byURL[r.abs]
		if !ok {
			continue
		}
		if r.border {
			s.borderImages[r.id] = img
			continue
		}
		if r.bg {
			if r.pseudo != nil {
				s.pseudoBgImages[r.pseudo] = img
			} else {
				s.bgImages[r.id] = img
			}
			continue
		}
		if b := img.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
			s.natural[r.id] = layout.NaturalSize{W: float32(b.Dx()), H: float32(b.Dy())}
			s.images[r.id] = img
		}
	}
}

func (s *Session) attachVectorBackground(candidate *layout.Arena, i int, abs string) {
	data, ok := s.bgVectorSrc[abs]
	if !ok || len(data) == 0 {
		return
	}
	obj := &candidate.Objects[i]
	if obj.Style == nil {
		return
	}
	x0, y0, x1, y1 := obj.BgAreaRect(obj.Style.BackgroundOrigin)
	areaW, areaH := x1-x0, y1-y0
	if areaW <= 0 || areaH <= 0 {
		return
	}
	var natW, natH float32
	if cfg, err := imgdec.Probe(data); err == nil {
		natW, natH = float32(cfg.Width), float32(cfg.Height)
	}
	if obj.Style.BackgroundSize == style.BgSizeAuto && imgdec.SVGDefersBothSides(data) {
		natW, natH = areaW, areaH
	}
	if bs := obj.Style.BackgroundSize; bs == style.BgSizeCover || bs == style.BgSizeContain {
		if hasDims, _ := imgdec.SVGIntrinsicKind(data); !hasDims {
			if rw, rh, ok := imgdec.SVGRatio(data); ok {
				natW, natH = float32(rw), float32(rh)
			}
		}
	}
	tileW, tileH := paint.BGTileSize(obj.Style, areaW, areaH, natW, natH, s.bgNoRatio[abs])
	tw, th := int(tileW+0.5), int(tileH+0.5)
	if tw <= 0 || th <= 0 {
		return
	}
	if tw > imgdec.MaxImageDimension || th > imgdec.MaxImageDimension {
		if obj.Style.BackgroundSize != style.BgSizeCover {
			return
		}
		scale := float64(imgdec.MaxImageDimension) / float64(tw)
		if s := float64(imgdec.MaxImageDimension) / float64(th); s < scale {
			scale = s
		}
		tw = int(float64(tw)*scale + 0.5)
		th = int(float64(th)*scale + 0.5)
		if tw <= 0 || th <= 0 {
			tw = int(areaW + 0.5)
			th = int(areaH + 0.5)
		}
		if tw <= 0 || th <= 0 || tw > imgdec.MaxImageDimension || th > imgdec.MaxImageDimension {
			return
		}
	}
	pixels := int64(tw) * int64(th)
	if s.imgReservation == nil || !s.imgReservation.reserve(pixels) {
		return
	}
	img, err := imgdec.DecodeSVGAt(data, tw, th)
	if err != nil {
		s.imgReservation.release(pixels)
		return
	}
	s.bgVectorUsed += pixels
	obj.BgImage = img
	obj.BgVector = true
	obj.BgNoRatio = s.bgNoRatio[abs]
}

func (s *Session) styleViewport() style.Viewport {
	return style.Viewport{W: s.viewportW, H: s.viewportH, DarkMode: s.darkMode}
}

func (s *Session) recordViewportWidth(w float32) {
	if finite(float64(w)) && w > 0 && w <= MaxGeometry {
		s.viewportW = w
	}
}

func (s *Session) Viewport() (w, h float32) {
	return s.viewportW, s.viewportH
}

func (s *Session) SetDevicePixelRatio(dpr float32) {
	if s == nil || validateScale(float64(dpr)) != nil {
		return
	}
	s.opMu.Lock()
	s.dpr = dpr
	s.opMu.Unlock()
	if s.jsRT != nil {
		s.jsRT.SetDevicePixelRatio(float64(dpr))
	}
}

func (s *Session) Reflow(viewportW float32) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.imgMu.Lock()
	imgLocked := true
	defer func() {
		if imgLocked {
			s.imgMu.Unlock()
		}
	}()
	if err := validateWidth(viewportW); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("missing session")
	}
	s.viewportW = viewportW
	if err := s.validateLayoutInput(); err != nil {
		return err
	}
	s.workCounts.layoutBuilds.Add(1)
	candidate := layout.Build(s.Doc, s.Styles, s.PseudoStyles)
	candidate.Metrics = s.metrics
	candidate.NaturalSizes = s.natural
	layout.Block(candidate, layout.ObjectID(1), viewportW, s.viewportH)
	if err := validateArena(candidate); err != nil {
		return err
	}
	layout.Inline(candidate, layout.ObjectID(1))
	s.applyImplicitPopoverAnchors(candidate)
	layout.Positioning(candidate, layout.ObjectID(1), viewportW, s.viewportH)
	if err := validateArena(candidate); err != nil {
		return err
	}
	if s.imgReservation != nil {
		s.imgReservation.release(s.bgVectorUsed)
		s.bgVectorUsed = 0
	}
	if s.images != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil {
				if img, ok := s.images[n.ID]; ok {
					candidate.Objects[i].Image = img
				}
				if img, ok := s.bgImages[n.ID]; ok {
					candidate.Objects[i].BgImage = img
				}
				if img, ok := s.pseudoBgImages[candidate.Objects[i].Style]; ok {
					candidate.Objects[i].BgImage = img
				}
				if img, ok := s.masks[n.ID]; ok {
					candidate.Objects[i].MaskImage = img
				}
				if img, ok := s.borderImages[n.ID]; ok {
					candidate.Objects[i].BorderImage = img
				}
				if abs, ok := s.bgVectorRef[n.ID]; ok {
					s.attachVectorBackground(candidate, i, abs)
				}
				if abs, ok := s.pseudoBgVectorRef[candidate.Objects[i].Style]; ok {
					s.attachVectorBackground(candidate, i, abs)
				}
			}
		}
	}
	if s.media != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil && n.Data == "video" {
				if c := s.media.controllerByID(n.ID); c != nil {
					candidate.Objects[i].Media = c.View()
				}
			}
		}
	} else {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil && n.Data == "video" {
				candidate.Objects[i].Media = paint.VideoView{
					State:       paint.VideoFailed,
					Message:     "playback unsupported",
					Unsupported: true,
				}
			}
		}
	}
	if s.jsRT != nil {
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil && n.Data == "canvas" {
				ops := s.jsRT.CanvasOps(int(n.ID))
				if len(ops) > 0 {
					w, h := s.jsRT.CanvasDimensions(int(n.ID))
					if canvasBitmapFits(w, h) {
						b := frame.NewBitmap(w, h)
						frameOps := make([]frame.CanvasOp, len(ops))
						for j, op := range ops {
							frameOps[j] = frame.CanvasOp{Method: op.Method, Args: op.Args}
						}
						frame.ReplayCanvasOps(b, frameOps, w, h)
						candidate.Objects[i].Image = b.AsRGBA()
						s.jsRT.SetCanvasBitmap(int(n.ID), b.RGBA, w, h)
					}
				}
			}
		}
	}
	if s.jsRT != nil {
		s.imgMu.Unlock()
		imgLocked = false
		s.jsRT.SetViewportRect(0, 0, s.viewportW, s.viewportH)
		s.jsRT.SetDevicePixelRatio(float64(s.dpr))
		for i := range candidate.Objects {
			if n := candidate.Objects[i].Node; n != nil {
				s.jsRT.SetElementRect(int(n.ID), candidate.Objects[i].X, candidate.Objects[i].Y, candidate.Objects[i].W, candidate.Objects[i].H)
				s.jsRT.SetElementSize(n, float64(candidate.Objects[i].W), float64(candidate.Objects[i].H))
			}
		}
		s.jsRT.CheckIntersections()
		s.jsRT.DrainResizeObservers()
	}
	s.Arena = candidate
	s.layoutDirty.Store(false)
	s.selActive = false
	s.findMarks = nil
	s.findActive = 0
	return nil
}

func hitBoxContains(obj *layout.Object, tx, ty float32) bool {
	x0, y0, x1, y1 := obj.BorderRect()
	if obj.Style == nil {
		return tx >= x0 && tx < x1 && ty >= y0 && ty < y1
	}
	return obj.Style.UsedBorderRadius(x1-x0, y1-y0, 1).Contains(x0, y0, x1, y1, tx, ty)
}

func (s *Session) HitTestLink(x, y float32) string {
	if s.Arena == nil {
		return ""
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		adjX, adjY := s.ancestorScrollOffset(obj)
		tx, ty := x+adjX, y+adjY
		if !hitBoxContains(obj, tx, ty) {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if cur.Style != nil && cur.Style.PointerEvents == "none" {
				break
			}
			if cur.Node != nil && cur.Node.DataAtom == dom.AtomA {
				if href := cur.Node.GetAttribute("href"); href != "" {
					return href
				}
			}
		}
	}
	return ""
}

func (s *Session) HitTestImage(x, y float32) string {
	if s.Arena == nil {
		return ""
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		adjX, adjY := s.ancestorScrollOffset(obj)
		tx, ty := x+adjX, y+adjY
		if !hitBoxContains(obj, tx, ty) {
			continue
		}
		if obj.Node != nil && obj.Node.DataAtom == dom.AtomImg {
			if src := resolveImgSource(obj.Node, s.viewportW); src != "" {
				return src
			}
		}
	}
	return ""
}

func resolveImgSource(n *dom.Node, viewportW float32) string {
	if n.Parent != nil && n.Parent.Data == "picture" {
		for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
			if c == n {
				break
			}
			if c.Element() && c.Data == "source" {
				if typ := c.GetAttribute("type"); typ != "" && !isSupportedImageType(typ) {
					continue
				}
				if media := c.GetAttribute("media"); media != "" {
					if !css.MatchMedia(media, viewportW) {
						continue
					}
				}
				if srcset := strings.TrimSpace(c.GetAttribute("srcset")); srcset != "" {
					fields := strings.Fields(srcset)
					if len(fields) > 0 {
						return fields[0]
					}
				}
			}
		}
	}
	return n.GetAttribute("src")
}

func isSupportedImageType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp", "image/svg+xml", "image/bmp":
		return true
	}
	return false
}

func (s *Session) ClickAt(x, y float32) bool {
	s.dismissValidationBubble()
	var dismissedPopup *dom.Node
	if open := s.selectPopupNode(); open != nil {
		if s.clickSelectPopup(x, y) {
			return true
		}
		dismissedPopup = open
	}
	hitNode := s.nodeAt(x, y)
	dismissed := s.dismissAutoPopoversExcept(hitNode)
	if hitNode != nil && !hitNode.Element() {
		for hitNode != nil && !hitNode.Element() {
			hitNode = hitNode.Parent
		}
	}
	if disabled := disabledFormControlAt(hitNode); disabled != nil {
		if disabled != hitNode {
			path := dom.EventPath(hitNode)
			for i, node := range path {
				if node == disabled {
					path = path[:i]
					break
				}
			}
			dom.DispatchEventWithPath(hitNode, dom.NewTrustedEvent("click", true, true), path)
		}
		return dismissed || dismissedPopup != nil
	}
	if isRangeInput(hitNode) && !dom.IsFormControlDisabled(hitNode) {
		if s.jsRT != nil {
			s.jsRT.SetTransientUserActivation(true)
		}
		click := dom.NewTrustedEvent("click", true, true)
		if !dom.DispatchEvent(hitNode, click) {
			return true
		}
		s.opMu.Lock()
		changed := s.setRangeValueAtLocked(hitNode, x)
		rt := s.jsRT
		s.opMu.Unlock()
		if changed {
			if rt != nil {
				rt.DispatchTrustedInput(hitNode, nil, "", false)
			} else {
				event := dom.NewTrustedInputEvent("input", true, false, nil, "", false)
				dom.DispatchEvent(hitNode, &event.Event)
			}
			dom.DispatchEvent(hitNode, dom.NewTrustedEvent("change", true, false))
		}
		return true
	}

	n := s.activatableAt(x, y)
	if n == nil {
		if hitNode != nil {
			if s.jsRT != nil {
				s.jsRT.SetTransientUserActivation(true)
			}
			dom.DispatchEvent(hitNode, dom.NewTrustedEvent("click", true, true))
			if sel := s.openableSelect(hitNode); sel != nil && sel != dismissedPopup {
				s.openSelectPopup(sel, x, y)
				return true
			}
		}
		return dismissed || dismissedPopup != nil
	}
	if s.jsRT != nil {
		s.jsRT.SetTransientUserActivation(true)
	}
	ev := dom.NewTrustedEvent("click", true, true)
	if !dom.DispatchEvent(n, ev) {
		return true
	}
	s.runClickDefault(n)
	return true
}

func (s *Session) toggleDetailsWithoutRuntime(n *dom.Node) {
	wasOpen := n.HasAttribute("open")
	if wasOpen {
		n.RemoveAttribute("open")
	} else {
		n.SetAttribute("open", "")
	}
	oldState, newState := "closed", "open"
	if wasOpen {
		oldState, newState = "open", "closed"
	}
	event := dom.NewTrustedToggleEvent("toggle", false, false, oldState, newState)
	dom.DispatchEvent(n, &event.Event)
	if wasOpen {
		return
	}
	for _, peer := range dom.CloseOtherNamedDetails(n) {
		peerEvent := dom.NewTrustedToggleEvent("toggle", false, false, "open", "closed")
		dom.DispatchEvent(peer, &peerEvent.Event)
	}
}

func (s *Session) runClickDefault(n *dom.Node) {
	typ := strings.ToLower(n.GetAttribute("type"))
	switch n.Data {
	case "input":
		if targetID := n.GetAttribute("popovertarget"); targetID != "" {
			action := strings.ToLower(n.GetAttribute("popovertargetaction"))
			s.handlePopoverTarget(targetID, action)
			return
		}
		switch typ {
		case "checkbox", "radio":
			s.ToggleControl(n)
		case "file":
			if !dom.IsFormControlDisabled(n) {
				s.queueFilePicker(n)
			}
		case "submit":
			if form := FindForm(n); form != nil {
				s.SubmitForm(form, n)
			}
		case "reset":
			if form := FindForm(n); form != nil {
				s.ResetForm(form)
			}
		}
	case "summary":
		for p := n.Parent; p != nil; p = p.Parent {
			if p.Element() && p.Data == "details" {
				if s.jsRT != nil {
					s.jsRT.SetDetailsOpen(p, !p.HasAttribute("open"), nil)
				} else {
					s.toggleDetailsWithoutRuntime(p)
				}
				break
			}
		}
	case "details":
		hasSummary := false
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && child.Data == "summary" {
				hasSummary = true
				break
			}
		}
		if !hasSummary {
			if s.jsRT != nil {
				s.jsRT.SetDetailsOpen(n, !n.HasAttribute("open"), nil)
			} else {
				s.toggleDetailsWithoutRuntime(n)
			}
		}
	case "button":
		if targetID := n.GetAttribute("commandfor"); targetID != "" {
			action := strings.ToLower(strings.TrimSpace(n.GetAttribute("command")))
			if action == "toggle" && s.Doc != nil {
				if target := s.Doc.ElementByID(targetID); target != nil && target.Data == "details" {
					if s.jsRT != nil {
						s.jsRT.SetDetailsOpen(target, !target.HasAttribute("open"), n)
					} else {
						wasOpen := target.HasAttribute("open")
						if wasOpen {
							target.RemoveAttribute("open")
						} else {
							target.SetAttribute("open", "")
						}
						oldState, newState := "closed", "open"
						if wasOpen {
							oldState, newState = "open", "closed"
						}
						event := dom.NewTrustedToggleEvent("toggle", false, false, oldState, newState)
						event.Source = n
						dom.DispatchEvent(target, &event.Event)
					}
					return
				}
			}
			switch action {
			case "show-popover":
				s.handlePopoverTarget(targetID, "show")
				return
			case "hide-popover":
				s.handlePopoverTarget(targetID, "hide")
				return
			case "toggle-popover":
				s.handlePopoverTarget(targetID, "toggle")
				return
			}
		}
		if targetID := n.GetAttribute("popovertarget"); targetID != "" {
			action := strings.ToLower(n.GetAttribute("popovertargetaction"))
			s.handlePopoverTarget(targetID, action)
			return
		}
		btnType := strings.ToLower(n.GetAttribute("type"))
		switch btnType {
		case "submit", "":
			if form := FindForm(n); form != nil {
				s.SubmitForm(form, n)
			}
		case "reset":
			if form := FindForm(n); form != nil {
				s.ResetForm(form)
			}
		}
	}
}

func (s *Session) activatableAt(x, y float32) *dom.Node {
	if s.Arena == nil {
		return nil
	}
	objs := s.Arena.Objects
	for i := len(objs) - 1; i >= 1; i-- {
		obj := &objs[i]
		if obj.Style != nil && obj.Style.PointerEvents == "none" {
			continue
		}
		if !hitBoxContains(obj, x, y) {
			continue
		}
		for cur := &objs[i]; cur != nil; cur = parentObj(objs, cur) {
			if cur.Style != nil && cur.Style.PointerEvents == "none" {
				break
			}
			if IsActivatable(cur.Node) {
				return cur.Node
			}
		}
	}
	return nil
}

type FormSubmission struct {
	Action  string
	Method  string
	Enctype string
	Target  string
	Data    []FormData
	Encoded string
}

func (s *Session) SubmitForm(form, submitter *dom.Node) {
	s.submitForm(form, submitter)
}

func (s *Session) queueDialogClose(node *dom.Node) {
	if node == nil {
		return
	}
	s.opMu.Lock()
	rt := s.jsRT
	s.opMu.Unlock()
	if rt == nil {
		dom.DispatchEvent(node, dom.NewTrustedEvent("close", false, false))
		return
	}
	rt.QueueTask(func() {
		dom.DispatchEvent(node, dom.NewTrustedEvent("close", false, false))
	})
}

func (s *Session) submitForm(form, submitter *dom.Node) {
	if form == nil {
		return
	}
	s.opMu.Lock()
	s.clearValidationBubbleLocked()
	var invalid []*dom.Node
	if !SubmitterNoValidate(submitter) {
		invalid = s.validateForm(form)
	}
	if len(invalid) > 0 {
		form.UserInteracted = true
		for _, control := range invalid {
			control.UserInteracted = true
		}
	}
	s.opMu.Unlock()
	if len(invalid) > 0 {
		for _, control := range invalid {
			dom.DispatchEvent(control, dom.NewTrustedEvent("invalid", false, false))
		}
		s.setFocusedNodeUnlocked(invalid[0])
		s.opMu.Lock()
		s.validationAnchor = invalid[0]
		s.validationMessage = constraintValidationMessage(invalid[0])
		s.opMu.Unlock()
		return
	}
	ev := dom.NewTrustedEvent("submit", true, true)
	if !dom.DispatchEvent(form, ev) {
		return
	}
	s.opMu.Lock()
	data := s.collectFormData(form, submitter)
	method := FormMethodFor(form, submitter)
	if strings.EqualFold(method, "dialog") {
		for cur := form.Parent; cur != nil; cur = cur.Parent {
			if cur.Element() && cur.Data == "dialog" {
				cur.RemoveAttribute("open")
				cur.IsModal = false
				previous := s.dialogFocusReturnLocked(cur)
				s.opMu.Unlock()
				if previous != nil {
					s.setFocusedNodeUnlocked(previous)
				}
				s.queueDialogClose(cur)
				return
			}
		}
		s.opMu.Unlock()
		return
	}
	rt := s.jsRT
	s.opMu.Unlock()
	if rt != nil && (strings.EqualFold(method, "GET") || strings.EqualFold(method, "POST")) &&
		rt.NavigateFormTargetToFrame(form, submitter) {
		return
	}
	s.opMu.Lock()
	action := FormActionFor(form, submitter)
	declaredEnctype := FormEnctypeFor(form, submitter)
	enctype := "application/x-www-form-urlencoded"
	var encoded string
	switch {
	case strings.EqualFold(method, "GET"):
		encoded = EncodeFormURLEncoded(data)
	case declaredEnctype == "text/plain":
		enctype = "text/plain"
		encoded = EncodeFormPlainText(data)
	case declaredEnctype == "multipart/form-data":
		boundary, body, err := EncodeFormMultipart(data)
		if err == nil {
			enctype = "multipart/form-data; boundary=" + boundary
			encoded = string(body)
		} else {
			encoded = EncodeFormURLEncoded(data)
		}
	default:
		encoded = EncodeFormURLEncoded(data)
	}

	s.submittedForm = &FormSubmission{
		Action:  action,
		Method:  method,
		Enctype: enctype,
		Target:  FormTargetFor(form, submitter),
		Data:    data,
		Encoded: encoded,
	}
	s.opMu.Unlock()
}

func (s *Session) PopSubmittedForm() (*FormSubmission, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.submittedForm == nil {
		return nil, false
	}
	sub := s.submittedForm
	s.submittedForm = nil
	return sub, true
}

func (s *Session) ResetForm(form *dom.Node) {
	if form == nil {
		return
	}
	ev := dom.NewTrustedEvent("reset", false, true)
	if !dom.DispatchEvent(form, ev) {
		return
	}

	var resetNode func(n *dom.Node)
	resetNode = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() {
			switch n.Data {
			case "input":
				typ := strings.ToLower(n.GetAttribute("type"))
				switch typ {
				case "checkbox", "radio":
					n.ResetInputChecked()
					if typ == "radio" && n.InputChecked() {
						reconcileFormRadioGroups(form)
					}
				case "submit", "reset", "button", "image":
				default:
					n.ResetInputControlValue()
					if s.jsRT != nil {
						s.jsRT.RefreshInputPatternMismatch(n)
					}
				}
			case "textarea":
				n.ResetInputControlValue()
			case "select":
				n.SelectNoSelection = false
				for _, option := range dom.SelectOptions(n) {
					option.ResetOptionSelectedness()
				}
				dom.NormalizeSelectSelectedness(n)
			case "output":
				if s.jsRT != nil {
					s.jsRT.ResetOutputElement(n)
				} else {
					value := n.ResetOutputControlValue()
					for child := n.FirstChild; child != nil; {
						next := child.NextSibling
						n.RemoveChild(child)
						child = next
					}
					if value != "" && n.Doc != nil {
						n.AppendChild(n.Doc.NewText(value))
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			resetNode(c)
		}
	}
	resetNode(form)
}

func reconcileFormRadioGroups(form *dom.Node) {
	if form == nil {
		return
	}
	checked := make(map[string]*dom.Node)
	form.Walk(func(node *dom.Node) bool {
		if !node.Element() || node.Data != "input" || !strings.EqualFold(node.GetAttribute("type"), "radio") || !node.InputChecked() {
			return true
		}
		name := node.GetAttribute("name")
		if name == "" {
			return true
		}
		if earlier := checked[name]; earlier != nil {
			earlier.SetInputChecked(false, true)
		}
		checked[name] = node
		return true
	})
}

type Match struct {
	X0, Y0, X1, Y1 float32
}

const maxFindMatches = 1000

type findMark struct {
	key   layout.BoxKey
	rect  Match
	match int
}

func (s *Session) Find(query string) []Match {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.findMarks = nil
	s.findActive = 0
	if s.Arena == nil || strings.TrimSpace(query) == "" {
		return nil
	}
	q := strings.ToLower(query)

	objs := s.Arena.Objects
	var matches []Match
	var marks []findMark
	var run []*layout.Object

	flushRun := func() {
		if len(matches) >= maxFindMatches || len(run) == 0 {
			run = run[:0]
			return
		}
		var words []*layout.Object
		for _, o := range run {
			if strings.TrimSpace(o.Node.DataContent) != "" {
				words = append(words, o)
			}
		}
		run = run[:0]
		if len(words) == 0 {
			return
		}
		texts := make([]string, len(words))
		off := make([]int, len(words))
		pos := 0
		for i, o := range words {
			texts[i] = strings.ToLower(o.Node.DataContent)
			off[i] = pos
			pos += len(texts[i]) + 1
		}
		joined := strings.Join(texts, " ")
		for at := 0; len(matches) < maxFindMatches; {
			idx := strings.Index(joined[at:], q)
			if idx < 0 {
				break
			}
			start := at + idx
			rect, first := matchRect(words, off, start, start+len(q))
			marks = append(marks, findMark{key: words[first].Key, rect: rect, match: len(matches)})
			matches = append(matches, rect)
			at = start + 1
		}
	}

	var walk func(layout.ObjectID)
	walk = func(id layout.ObjectID) {
		if id == 0 || int(id) >= len(objs) {
			return
		}
		obj := &objs[id]
		if obj.Node != nil && obj.Node.Type == dom.NodeText {
			if x0, y0, x1, y1 := obj.BorderRect(); x0 < x1 && y0 < y1 {
				if len(run) > 0 && run[len(run)-1].Parent != obj.Parent {
					flushRun()
				}
				run = append(run, obj)
			}
		}
		for k := obj.FirstKid; k != 0; k = objs[k].NextSibling {
			walk(k)
		}
	}
	walk(1)
	flushRun()
	s.findMarks = marks
	return matches
}

func (s *Session) SetFindActive(index int) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if index < 0 || index >= len(s.findMarks) {
		s.findActive = -1
		return
	}
	s.findActive = index
}

func (s *Session) ClearFind() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.findMarks = nil
	s.findActive = 0
}

func (s *Session) clearValidationBubbleLocked() {
	s.validationAnchor = nil
	s.validationMessage = ""
}

func (s *Session) dismissValidationBubble() {
	s.opMu.Lock()
	s.clearValidationBubbleLocked()
	s.opMu.Unlock()
}

func (s *Session) SelectionBounds() (frame.Rect, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	words, from, to, ok := s.selCoverage()
	if !ok || len(words) == 0 {
		return frame.Rect{}, false
	}
	if from.word < 0 {
		from.word = 0
	}
	if to.word >= len(words) {
		to.word = len(words) - 1
	}
	if from.word > to.word {
		return frame.Rect{}, false
	}
	first := true
	var x0, y0, x1, y1 float32
	for i := from.word; i <= to.word; i++ {
		cx0, cy0, cx1, cy1 := words[i].ContentRect()
		if first || cx0 < x0 {
			x0 = cx0
		}
		if first || cy0 < y0 {
			y0 = cy0
		}
		if first || cx1 > x1 {
			x1 = cx1
		}
		if first || cy1 > y1 {
			y1 = cy1
		}
		first = false
	}
	if x1 <= x0 || y1 <= y0 {
		return frame.Rect{}, false
	}
	return frame.Rect4(int32(math.Floor(float64(x0))), int32(math.Floor(float64(y0))),
		int32(math.Ceil(float64(x1))), int32(math.Ceil(float64(y1)))), true
}

func (s *Session) ScrollToID(targetID string) float32 {
	if s.Arena == nil || targetID == "" {
		return 0
	}

	objs := s.Arena.Objects
	var walk func(layout.ObjectID)
	foundY := float32(0)

	walk = func(objID layout.ObjectID) {
		if objID == 0 || int(objID) >= len(objs) {
			return
		}
		obj := &objs[objID]

		if obj.Node != nil {
			for _, attr := range obj.Node.Attr {
				if strings.EqualFold(attr.Name, "id") && attr.Value == targetID {
					_, y, _, _ := obj.BorderRect()
					if y > 0 {
						foundY = y
					}
					return
				}
			}
		}

		if foundY == 0 {
			for k := obj.FirstKid; k != 0 && foundY == 0; k = objs[k].NextSibling {
				walk(k)
			}
		}
	}

	walk(1)
	return foundY
}

func matchRect(words []*layout.Object, off []int, start, end int) (Match, int) {
	first, last := 0, len(words)-1
	for i := range words {
		if off[i] <= start {
			first = i
		}
		if off[i] < end {
			last = i
		}
	}
	x0, y0, x1, y1 := words[first].BorderRect()
	if first == last {
		if w := len(words[first].Node.DataContent); w > 0 {
			sx := x0 + (x1-x0)*float32(start-off[first])/float32(w)
			ex := x0 + (x1-x0)*float32(end-off[first])/float32(w)
			return Match{X0: sx, Y0: y0, X1: ex, Y1: y1}, first
		}
		return Match{X0: x0, Y0: y0, X1: x1, Y1: y1}, first
	}
	lx0, ly0, lx1, ly1 := words[last].BorderRect()
	if w := len(words[first].Node.DataContent); w > 0 {
		x0 = x0 + (x1-x0)*float32(start-off[first])/float32(w)
	}
	if w := len(words[last].Node.DataContent); w > 0 {
		x1 = lx0 + (lx1-lx0)*float32(end-off[last])/float32(w)
	} else {
		x1 = lx1
	}
	top, bot := y0, y1
	if ly0 < top {
		top = ly0
	}
	if ly1 > bot {
		bot = ly1
	}
	return Match{x0, top, x1, bot}, first
}

func parentObj(objs []layout.Object, o *layout.Object) *layout.Object {
	if o.Parent == 0 || int(o.Parent) >= len(objs) {
		return nil
	}
	return &objs[o.Parent]
}

func (s *Session) runScripts() {
	if s.jsRT == nil || s.Doc == nil {
		return
	}
	root := s.Doc.HTML
	if root == nil {
		root = &s.Doc.Node
	}
	type scriptItem struct {
		text        string
		ext         string
		name        string
		deferScript bool
		node        *dom.Node
	}
	var items []scriptItem
	extLeft := maxExternalScripts - s.parserExternalCount
	var collect func(n *dom.Node)
	collect = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.Data == "script" && isClassicScript(n) && !s.parserExecutedScripts[n] &&
			(n.Namespace != dom.NSSVG || n.SVGScriptReady) {
			if src := scriptSrc(n); src != "" {
				if strings.HasPrefix(strings.ToLower(src), "data:") {
					if text, ok := decodeDataScriptURL(src); ok {
						items = append(items, scriptItem{text: text, name: src, deferScript: n.HasAttribute("defer") && !n.HasAttribute("async"), node: n})
					}
				} else if s.scriptFetch != nil && extLeft > 0 {
					if abs, ok := s.resolveScriptURL(s.scriptBase, src); ok {
						extLeft--
						items = append(items, scriptItem{ext: abs, name: abs, deferScript: n.HasAttribute("defer") && !n.HasAttribute("async"), node: n})
					}
				}
			} else if text := n.TextContent(); text != "" {
				items = append(items, scriptItem{text: text, name: "inline", node: n})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(root)

	deadline := time.Now().Add(ScriptFetchBudget)
	const scriptWorkers = 8
	sem := make(chan struct{}, scriptWorkers)
	var wg sync.WaitGroup
	for i, it := range items {
		if it.ext == "" || time.Now().After(deadline) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, abs string) {
			defer wg.Done()
			defer func() { <-sem }()
			if data, err := s.scriptFetch(s.scriptBase, abs); err == nil && len(data) > 0 {
				items[i].text = string(data)
			}
		}(i, it.ext)
	}
	wg.Wait()

	for _, deferred := range []bool{false, true} {
		for _, it := range items {
			if it.text == "" || it.deferScript != deferred {
				continue
			}
			var err error
			if it.deferScript {
				err = s.jsRT.RunParserDeferScript(it.text, it.name)
			} else {
				err = s.jsRT.RunParserScript(it.text, it.name)
			}
			if err != nil {
				s.recordScriptError(err)
			}
		}
	}
	s.runModuleScripts(s.jsRT, root, s.scriptBase, extLeft, deadline)
	s.runUserScripts(userscript.RunAtDocumentEnd)
}

func decodeDataScriptURL(raw string) (string, bool) {
	if len(raw) > js.MaxScriptBytes*3+8192 {
		return "", false
	}
	scheme, _, ok := strings.Cut(raw, ":")
	if !ok || !strings.EqualFold(scheme, "data") {
		return "", false
	}
	metadata, payload, ok := strings.Cut(raw[len(scheme)+1:], ",")
	if !ok || len(metadata) > 1024 {
		return "", false
	}
	base64Encoded := strings.HasSuffix(strings.ToLower(metadata), ";base64")
	if base64Encoded {
		metadata = metadata[:len(metadata)-len(";base64")]
	}
	mediaType := "text/plain"
	if metadata != "" {
		parsedType, _, err := mime.ParseMediaType(metadata)
		if err != nil {
			return "", false
		}
		mediaType = strings.ToLower(parsedType)
	}
	switch mediaType {
	case "application/ecmascript", "application/javascript", "application/x-ecmascript", "application/x-javascript",
		"text/ecmascript", "text/javascript", "text/javascript1.0", "text/javascript1.1", "text/javascript1.2", "text/javascript1.3", "text/javascript1.4", "text/javascript1.5", "text/jscript", "text/livescript":
	default:
		return "", false
	}
	decodedPayload, err := url.PathUnescape(payload)
	if err != nil {
		return "", false
	}
	source := []byte(decodedPayload)
	if base64Encoded {
		source, err = base64.StdEncoding.DecodeString(decodedPayload)
		if err != nil {
			return "", false
		}
	}
	if len(source) > js.MaxScriptBytes {
		return "", false
	}
	return string(source), true
}

func (s *Session) runUserScripts(stage userscript.RunAt) {
	if s.jsRT == nil || len(s.userScripts) == 0 {
		return
	}
	for _, us := range s.userScripts {
		if !us.Enabled {
			continue
		}
		if us.RunAt != stage {
			if !(stage == userscript.RunAtDocumentEnd && us.RunAt == userscript.RunAtDocumentIdle) {
				continue
			}
		}
		if us.Code != "" {
			name := "userscript:" + us.Name
			if err := s.jsRT.Run(us.Code, name); err != nil {
				s.recordScriptError(err)
			}
		}
	}
}

func scriptSrc(n *dom.Node) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Name, "src") {
			return strings.TrimSpace(a.Value)
		}
	}
	if n.Namespace == dom.NSSVG {
		if href := strings.TrimSpace(n.GetAttribute("href")); href != "" {
			return href
		}
		return strings.TrimSpace(n.GetAttribute("xlink:href"))
	}
	return ""
}

func isClassicScript(n *dom.Node) bool {
	for _, a := range n.Attr {
		if !strings.EqualFold(a.Name, "type") {
			continue
		}
		t := strings.ToLower(strings.TrimSpace(a.Value))
		switch t {
		case "", "text/javascript", "application/javascript",
			"application/ecmascript", "application/x-ecmascript",
			"application/x-javascript", "text/ecmascript",
			"text/jscript", "text/livescript":
			return true
		default:
			return false
		}
	}
	return true
}

func (s *Session) Close() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	for n := s.active; n != nil && n.Element(); n = n.Parent {
		css.SetActive(n, false)
	}
	for n := s.hovered; n != nil && n.Element(); n = n.Parent {
		css.SetHover(n, false)
	}
	if s.focus != nil {
		css.SetFocus(s.focus, false)
		css.SetFocusVisible(s.focus, false)
	}
	s.active = nil
	s.hovered = nil
	s.focus = nil
	s.focusState.Store(nil)
	if s.media != nil {
		s.media.closeAll()
	}
	if s.jsRT != nil {
		_ = s.jsRT.Close()
	}
}

func (s *Session) PumpMedia() []frame.RectF {
	if s.media == nil {
		return nil
	}
	s.opMu.Lock()
	if s.Doc == nil {
		s.opMu.Unlock()
		return nil
	}
	dmg := s.media.pump(&s.Doc.Node, s.Arena)
	var dispatch []mediaDispatch
	if s.jsRT != nil {
		dispatch = s.media.drainEvents()
	}
	rt := s.jsRT
	s.opMu.Unlock()

	for _, d := range dispatch {
		rt.DispatchMediaEvents(d.node, d.events)
	}
	return dmg
}

func (s *Session) JSTitle() string {
	if s.jsRT == nil {
		return ""
	}
	return s.jsRT.Title()
}

func (s *Session) PendingLoadTimerDelay() (time.Duration, bool) {
	if s.jsRT == nil {
		return 0, false
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.jsRT.PendingLoadTimerDelay()
}

func (s *Session) PumpJS(viewportW float32) bool {
	if s.jsRT == nil {
		return false
	}
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	canvasBefore := s.jsRT.CanvasVisualStamp()
	s.jsRT.FlushScrollEvents()
	s.jsRT.Tick()
	if s.jsRT.HasPendingRAF() {
		s.jsRT.TickRAF(time.Now())
	}
	s.noteScriptTimeout()
	s.jsRT.FlushIFrameLoads()
	s.fixupDisabledFocus()
	canvasChanged := canvasBefore != s.jsRT.CanvasVisualStamp()
	layoutPending := s.drainScriptImages()
	mutated := s.focusStylePending
	if s.inv != nil {
		mutated = mutated || !s.inv.Empty()
		s.inv.Reset()
	}
	if !mutated && !layoutPending {
		return canvasChanged
	}
	if atomic.LoadInt32(&s.deferredActive) > 0 {
		s.focusStylePending = true
		return false
	}
	s.focusStylePending = false
	layouts := s.workCounts.layoutBuilds.Load()
	if err := s.RestyleAndReflow(viewportW); err != nil {
		return false
	}
	if layoutPending && s.workCounts.layoutBuilds.Load() == layouts {
		if err := s.Reflow(viewportW); err != nil {
			return false
		}
	}
	if layoutPending {
		s.clearScriptImgsLayoutPending()
	}
	return true
}

func (s *Session) RestyleAndReflow(viewportW float32) error {
	if s == nil {
		return nil
	}
	s.clearDetachedFocus()
	s.opMu.Lock()
	s.workCounts.styleResolves.Add(1)
	if s.styleResolver == nil {
		s.styleResolver = style.NewResolver(s.sheets)
	}
	if incremental, ok := s.styleResolver.ResolveInteractiveStates(s.Doc, s.styleViewport(), s.customFonts, s.Styles, s.PseudoStyles); ok {
		s.workCounts.styleNodeVisits.Add(uint64(incremental.Visited))
		layoutChanged := interactiveLayoutChanged(incremental, s.Styles, s.PseudoStyles)
		if !layoutChanged && s.Arena.RefreshComputedStylesForNodes(s.Styles, incremental.Nodes) {
			s.opMu.Unlock()
			return nil
		}
		s.opMu.Unlock()
		return s.Reflow(viewportW)
	}
	styles := s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
	pseudoStyles := s.styleResolver.ResolvePseudoElements(s.Doc, styles, s.customFonts)
	s.workCounts.styleNodeVisits.Add(uint64(len(styles)))
	if sameLayoutStyles(s.Styles, styles) && sameLayoutPseudoStyles(s.PseudoStyles, pseudoStyles) && s.Arena.RefreshComputedStyles(styles) {
		s.Styles = styles
		s.PseudoStyles = pseudoStyles
		s.opMu.Unlock()
		return nil
	}
	s.Styles = styles
	s.PseudoStyles = pseudoStyles
	s.opMu.Unlock()
	return s.Reflow(viewportW)
}

func (s *Session) HasPseudoClassRule(pseudo string) bool {
	if s == nil || pseudo == "" {
		return false
	}
	needle := ":" + strings.ToLower(strings.TrimSpace(pseudo))
	s.opMu.Lock()
	defer s.opMu.Unlock()
	for _, sheet := range s.sheets {
		if sheet == nil {
			continue
		}
		for _, rule := range sheet.Rules {
			for _, selector := range rule.SelectorStrs {
				if strings.Contains(strings.ToLower(selector), needle) {
					return true
				}
			}
		}
	}
	return false
}

func (s *Session) HasValidityClassRules() bool {
	return s.HasPseudoClassRule("valid") || s.HasPseudoClassRule("invalid") ||
		s.HasPseudoClassRule("user-valid") || s.HasPseudoClassRule("user-invalid")
}

func (s *Session) JSMutationSink() func() {
	return func() {
		if s.inv != nil {
			s.inv.Add(DOMMutation, frame.RectF{}, 0)
		}
	}
}

func (s *Session) JSLiveNodeCount() int {
	if s == nil || s.jsNodeBudget == nil {
		return 0
	}
	return s.jsNodeBudget.liveCount()
}

func (s *Session) SubresourceBytesUsed() int64 {
	if s == nil || s.sub == nil {
		return 0
	}
	s.sub.mu.Lock()
	defer s.sub.mu.Unlock()
	return s.sub.used
}

func (s *Session) HasPendingDOMMutation() bool {
	return s != nil && s.inv != nil && s.inv.domMutationPending
}

func (s *Session) CanDeferControlEditReflow() bool {
	return s.HasPendingDOMMutation() && atomic.LoadInt32(&s.deferredActive) == 0
}

func (s *Session) FocusedInputValueDoesNotAffectLayout() bool {
	if s == nil {
		return false
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.focus == nil || s.focus.DataAtom != dom.AtomInput {
		return false
	}
	switch strings.ToLower(s.focus.GetAttribute("type")) {
	case "", "text", "password", "email", "tel", "url", "search", "number":
		return true
	default:
		return false
	}
}

func (s *Session) ActiveElement() *dom.Node {
	if s == nil {
		return nil
	}
	if n := s.focusState.Load(); n != nil {
		return n
	}
	if s.Doc == nil {
		return nil
	}
	if s.Doc.Body != nil {
		return s.Doc.Body
	}
	return s.Doc.HTML
}

func (s *Session) TickAnimations(now time.Time) bool {
	if s.Animations == nil {
		return false
	}
	active := s.Animations.Update(now)
	if !active {
		s.Animations.RemoveCompleted()
	}
	return active
}

func (s *Session) AnimationsActive() bool {
	if s.Animations == nil {
		return false
	}
	return s.Animations.HasActive()
}

func (s *Session) startAnimations() {
	if s.Styles == nil {
		return
	}
	kfMap := make(map[string][]css.KeyframeStop)
	for _, sheet := range s.sheets {
		for _, kf := range sheet.Keyframes {
			kfMap[kf.Name] = kf.Stops
		}
	}
	if len(kfMap) == 0 {
		return
	}
	hasAnimations := false
	for _, cs := range s.Styles {
		if cs.AnimationName != "" {
			if _, ok := kfMap[cs.AnimationName]; ok {
				hasAnimations = true
				break
			}
		}
	}
	if !hasAnimations {
		return
	}
	if s.Animations == nil {
		s.Animations = &AnimationController{}
	}
	now := time.Now()
	for id, cs := range s.Styles {
		if cs.AnimationName == "" {
			continue
		}
		stops, ok := kfMap[cs.AnimationName]
		if !ok {
			continue
		}
		s.Animations.Add(
			id,
			cs.AnimationName,
			stops,
			cs.AnimationDuration,
			cs.AnimationDelay,
			cs.AnimationTiming,
			cs.AnimationIterCount,
			css.AnimationDirection(cs.AnimationDirection),
			css.AnimationFillMode(cs.AnimationFillMode),
			now,
		)
	}
}

func (s *Session) StyleSheetText(url string) (string, bool) {
	return s.cssText.lookup(url)
}

func (s *Session) Stylesheets() []*css.Stylesheet {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	out := make([]*css.Stylesheet, len(s.sheets))
	copy(out, s.sheets)
	return out
}

func (s *Session) DOMStats() (nodeCount int, maxDepth int) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Doc == nil {
		return 0, 0
	}
	var walk func(n *dom.Node, depth int)
	walk = func(n *dom.Node, depth int) {
		if n == nil {
			return
		}
		nodeCount++
		if depth > maxDepth {
			maxDepth = depth
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, depth+1)
		}
	}
	walk(&s.Doc.Node, 1)
	return nodeCount, maxDepth
}

func (s *Session) EvaluateJS(expr string) (string, error) {
	s.opMu.Lock()
	rt := s.jsRT
	s.opMu.Unlock()
	if rt == nil {
		return "", errors.New("engine: JS runtime not active")
	}
	return rt.Evaluate(expr)
}

func (s *Session) DarkMode() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.darkMode
}

func (s *Session) HighContrast() bool {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.highContrast
}

func (s *Session) SetDarkMode(enabled bool) {
	s.opMu.Lock()
	s.darkMode = enabled
	s.opMu.Unlock()
}

func (s *Session) SetHighContrast(enabled bool) {
	s.opMu.Lock()
	s.highContrast = enabled
	s.opMu.Unlock()
}

func (s *Session) SetViewportScrollCallback(fn func(x, y float64)) {
	s.opMu.Lock()
	s.onViewportScroll = fn
	s.opMu.Unlock()
}

func (s *Session) SyncViewportScroll(x, y float64) {
	if s.jsRT == nil {
		return
	}
	s.jsRT.SetWindowScrollX(x)
	s.jsRT.SetWindowScrollY(y)
}

func (s *Session) mediaOptions() css.MediaOptions {
	return css.MediaOptions{
		ViewportWidth: s.viewportW,
		DarkMode:      s.darkMode,
		HighContrast:  s.highContrast,
	}
}
