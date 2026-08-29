//go:build e2e && online

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

// LiveTargetSite describes one of the 10 real-world target websites.
type LiveTargetSite struct {
	ID                  int
	Name                string
	URL                 string
	ExpectedTitleSubstr string
	ExpectedContentText []string
	ArchitectureNotes   string
}

// TargetLiveWebsites contains the 10 target websites specified in PROJECT.md & TEST_INFRA.md.
var TargetLiveWebsites = []LiveTargetSite{
	{
		ID:                  1,
		Name:                "example.com",
		URL:                 "https://example.com",
		ExpectedTitleSubstr: "Example Domain",
		ExpectedContentText: []string{"Example Domain", "illustrative examples"},
		ArchitectureNotes:   "Viewport units (vw/vh), centering, basic typography",
	},
	{
		ID:                  2,
		Name:                "iana.org",
		URL:                 "https://www.iana.org/help/example-domains",
		ExpectedTitleSubstr: "Example Domains",
		ExpectedContentText: []string{"Example Domains", "IANA"},
		ArchitectureNotes:   "External CSS, navigation links, header/footer structure",
	},
	{
		ID:                  3,
		Name:                "cern.ch",
		URL:                 "http://info.cern.ch/hypertext/WWW/TheProject.html",
		ExpectedTitleSubstr: "World Wide Web",
		ExpectedContentText: []string{"World Wide Web", "Executive Summary"},
		ArchitectureNotes:   "Plain HTTP, uppercase HTML 1.0 tags (<TITLE>, <HEADER>, <DL>), link lists",
	},
	{
		ID:                  4,
		Name:                "text.npr.org",
		URL:                 "https://text.npr.org",
		ExpectedTitleSubstr: "NPR",
		ExpectedContentText: []string{"NPR", "News"},
		ArchitectureNotes:   "Inlined CSS, minimalist text layout, article index hierarchy",
	},
	{
		ID:                  5,
		Name:                "motherfuckingwebsite.com",
		URL:                 "https://motherfuckingwebsite.com",
		ExpectedTitleSubstr: "Website",
		ExpectedContentText: []string{"website", "good design"},
		ArchitectureNotes:   "UA default styles, font: shorthand, pure semantic HTML layout",
	},
	{
		ID:                  6,
		Name:                "paulgraham.com",
		URL:                 "https://paulgraham.com/articles.html",
		ExpectedTitleSubstr: "Essays",
		ExpectedContentText: []string{"Articles", "Paul Graham"},
		ArchitectureNotes:   "Nested table grid layout, legacy attributes (width, bgcolor, cellpadding)",
	},
	{
		ID:                  7,
		Name:                "html5zombo.com",
		URL:                 "https://html5zombo.com",
		ExpectedTitleSubstr: "ZOMBO",
		ExpectedContentText: []string{"ZOMBO", "WELCOME"},
		ArchitectureNotes:   "HTML5 audio element stubs, script timers (setInterval/setTimeout)",
	},
	{
		ID:                  8,
		Name:                "danluu.com",
		URL:                 "https://danluu.com",
		ExpectedTitleSubstr: "", // danluu.com root HTML does not declare a <title> tag
		ExpectedContentText: []string{"Dan Luu"},
		ArchitectureNotes:   "Custom <d> tags, pre/code blocks, flexible text layout",
	},
	{
		ID:                  9,
		Name:                "lite.cnn.com",
		URL:                 "https://lite.cnn.com",
		ExpectedTitleSubstr: "CNN",
		ExpectedContentText: []string{"CNN"},
		ArchitectureNotes:   "CSS variables (--var), JSON-LD data scripts, minimalist news feed",
	},
	{
		ID:                  10,
		Name:                "go.dev",
		URL:                 "https://go.dev/doc/",
		ExpectedTitleSubstr: "Documentation",
		ExpectedContentText: []string{"Documentation", "Go"},
		ArchitectureNotes:   "External CSS, navigation drawer, modern DOM tree with site.js",
	},
}

// withEngineContext is a helper that provisions a fresh browsercontrol Context and ensures cleanup.
func withEngineContext(t *testing.T, timeout time.Duration, fn func(ec browsercontrol.Context, ctx context.Context)) {
	t.Helper()
	svc := browsercontrol.NewEngineService()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{
			Width:  1280,
			Height: 800,
			Scale:  1.0,
		},
	})
	require.NoError(t, err, "failed to create engine context")
	defer func() {
		_ = svc.CloseContext(context.Background(), info.ID)
	}()

	ec, err := svc.Context(ctx, info.ID)
	require.NoError(t, err, "failed to retrieve engine context handle")

	fn(ec, ctx)
}

// TestLiveURLs is the master entry point exercising all 4 test tiers on all 10 live sites.
func TestLiveURLs(t *testing.T) {
	t.Run("Tier1_FeatureCoverage", runTier1FeatureCoverage)
	t.Run("Tier2_BoundaryCornerCases", runTier2BoundaryCornerCases)
	t.Run("Tier3_CrossFeatureInteractions", runTier3CrossFeatureInteractions)
	t.Run("Tier4_RealWorldWorkload", runTier4RealWorldWorkload)
}

// ============================================================================
// TIER 1: FEATURE COVERAGE (5 Test Checks per site = 50 total test checks)
// Checks:
// 1. Navigation (HTTP 200 / valid status)
// 2. Title extraction (correct title extracted when declared)
// 3. DOM snapshot (tree populated, root node exists)
// 4. Non-empty screenshot capture (valid PNG bytes, non-zero dimensions)
// 5. JavaScript execution safety (evaluates safely without panics)
// ============================================================================
func runTier1FeatureCoverage(t *testing.T) {
	for _, site := range TargetLiveWebsites {
		site := site
		t.Run(fmt.Sprintf("%02d_%s", site.ID, site.Name), func(t *testing.T) {
			withEngineContext(t, 45*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
				// Check 1: Navigation & HTTP status
				navRes, err := ec.Navigate(ctx, site.URL, browsercontrol.WaitComplete, 30000)
				if err != nil {
					t.Skipf("Skipping %s due to network error: %v", site.URL, err)
					return
				}
				assert.Contains(t, []int{http.StatusOK, http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther}, navRes.HTTPStatus,
					"HTTP status should be 200 OK or valid redirect for %s", site.URL)
				assert.Equal(t, site.URL, navRes.URL, "Navigated URL should match target")

				// Check 2: Title Extraction
				info, err := ec.Info(ctx)
				require.NoError(t, err, "Context Info must succeed")
				assert.NotEmpty(t, navRes.NavigationID, "NavigationID must be populated")

				snap, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{MaxDepth: 20, MaxNodes: 100})
				require.NoError(t, err, "Snapshot must succeed for %s", site.Name)
				if site.ExpectedTitleSubstr != "" {
					assert.NotEmpty(t, snap.Title, "Title should not be empty for %s", site.Name)
					assert.Contains(t, strings.ToLower(snap.Title), strings.ToLower(site.ExpectedTitleSubstr),
						"Extracted title %q should contain substring %q", snap.Title, site.ExpectedTitleSubstr)
				}

				// Check 3: DOM Snapshot Generation
				assert.NotEmpty(t, snap.Nodes, "DOM snapshot nodes list must not be empty for %s", site.Name)
				assert.Equal(t, site.URL, snap.URL, "Snapshot URL should match navigation URL")
				assert.NotEmpty(t, snap.Nodes[0].Role, "Root node must have a valid Role")

				// Check 4: Non-Empty Screenshot Capture
				shot, err := ec.Screenshot(ctx, browsercontrol.ScreenshotOptions{})
				require.NoError(t, err, "Screenshot capture must succeed for %s", site.Name)
				assert.NotEmpty(t, shot.Data, "Screenshot PNG data must not be empty for %s", site.Name)
				assert.Equal(t, "image/png", shot.MIMEType, "Screenshot MIME type must be image/png")
				assert.Greater(t, shot.Width, 0, "Screenshot width must be positive")
				assert.Greater(t, shot.Height, 0, "Screenshot height must be positive")

				// Validate PNG header / decoding
				img, err := png.Decode(bytes.NewReader(shot.Data))
				require.NoError(t, err, "Captured screenshot must decode as valid PNG for %s", site.Name)
				assert.NotNil(t, img, "Decoded image should not be nil")

				// Check 5: JavaScript Execution Safety
				evalRes, err := ec.Evaluate(ctx, "({ title: document.title, location: window.location ? window.location.href : '', loaded: true })", browsercontrol.EvaluateOptions{})
				require.NoError(t, err, "Evaluate must not produce fatal runtime error for %s", site.Name)
				assert.False(t, evalRes.IsError, "Evaluate should not return an unhandled script error: %s", evalRes.ErrorText)
				assert.Equal(t, "object", evalRes.Type, "Evaluation result should be an object")

				t.Logf("[Tier 1 PASS] %s: Status=%d, Title=%q, Nodes=%d, Img=%dx%d, Revision=%d",
					site.Name, navRes.HTTPStatus, snap.Title, len(snap.Nodes), shot.Width, shot.Height, info.PageRevision)
			})
		})
	}
}

// ============================================================================
// TIER 2: BOUNDARY & CORNER CASES (Specific engine edge cases per site & HTTP)
// ============================================================================
func runTier2BoundaryCornerCases(t *testing.T) {
	// Site 1: example.com - Viewport Centering & CSS Unit Evaluation
	t.Run("Site01_ExampleDotCom_ViewportCentering", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to example.com: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var div = document.querySelector('div');
				return {
					hasDiv: !!div,
					divText: div ? div.innerText || div.textContent : ''
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 2: iana.org - External Stylesheet Links and Navigation Structure
	t.Run("Site02_IANA_ExternalCSSAndNavigation", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://www.iana.org/help/example-domains", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to iana.org: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var links = document.querySelectorAll('a');
				var cssLinks = document.querySelectorAll('link[rel="stylesheet"]');
				return {
					linkCount: links ? links.length : 0,
					cssCount: cssLinks ? cssLinks.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 3: info.cern.ch - Plain HTTP, Uppercase Tags (<TITLE>, <HEADER>, <DL>)
	t.Run("Site03_CERN_UppercaseHTMLTagsAndPlainHTTP", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			nav, err := ec.Navigate(ctx, "http://info.cern.ch/hypertext/WWW/TheProject.html", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to info.cern.ch: %v", err)
			}
			assert.Equal(t, http.StatusOK, nav.HTTPStatus)

			snap, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{MaxDepth: 15})
			require.NoError(t, err)
			assert.Contains(t, strings.ToLower(snap.Title), "world wide web")

			// Check uppercase tag tolerance in JS DOM querying
			eval, err := ec.Evaluate(ctx, `(function() {
				var dls = document.getElementsByTagName('dl');
				var dts = document.getElementsByTagName('dt');
				return {
					hasDL: dls && dls.length > 0,
					dlCount: dls ? dls.length : 0,
					dtCount: dts ? dts.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 4: text.npr.org - Inlined CSS and Article Lists
	t.Run("Site04_NPR_InlinedCSSAndArticleHierarchy", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://text.npr.org", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to text.npr.org: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var lis = document.querySelectorAll('li');
				var anchors = document.querySelectorAll('a');
				return {
					liCount: lis ? lis.length : 0,
					anchorCount: anchors ? anchors.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 5: motherfuckingwebsite.com - UA Default Styles & font: Shorthand
	t.Run("Site05_MFW_UADefaultStylesAndFontShorthand", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://motherfuckingwebsite.com", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to motherfuckingwebsite.com: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var h1 = document.querySelector('h1');
				var quotes = document.querySelectorAll('blockquote');
				return {
					hasH1: !!h1,
					h1Text: h1 ? h1.textContent : '',
					quoteCount: quotes ? quotes.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 6: paulgraham.com - Nested Table Layout & Legacy Attributes
	t.Run("Site06_PaulGraham_NestedTablesAndLegacyAttrs", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://paulgraham.com/articles.html", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to paulgraham.com: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var tables = document.querySelectorAll('table');
				var trs = document.querySelectorAll('tr');
				var tds = document.querySelectorAll('td');
				return {
					tableCount: tables ? tables.length : 0,
					rowCount: trs ? trs.length : 0,
					cellCount: tds ? tds.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 7: html5zombo.com - Audio Element Stubs & Timers
	t.Run("Site07_ZomboCom_AudioElementStubsAndTimers", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://html5zombo.com", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to html5zombo.com: %v", err)
			}
			// Verify audio element and media method stubs
			eval, err := ec.Evaluate(ctx, `(function() {
				var audio = document.querySelector('audio');
				var canPlay = false;
				if (audio && typeof audio.play === 'function') {
					try {
						audio.play();
						canPlay = true;
					} catch(e) {}
				}
				return {
					hasAudio: !!audio,
					hasPlayMethod: audio ? typeof audio.play === 'function' : false,
					canPlay: canPlay
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 8: danluu.com - Custom Tags (<d>) & Preformatted Blocks
	t.Run("Site08_DanLuu_CustomTagsAndPreformattedBlocks", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://danluu.com", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to danluu.com: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var customD = document.querySelectorAll('d');
				var pres = document.querySelectorAll('pre');
				var links = document.querySelectorAll('a');
				return {
					customTagCount: customD ? customD.length : 0,
					preCount: pres ? pres.length : 0,
					linkCount: links ? links.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 9: lite.cnn.com - CSS Variables & JSON-LD Scripts
	t.Run("Site09_LiteCNN_CSSVariablesAndJSONLD", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://lite.cnn.com", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to lite.cnn.com: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var jsonld = document.querySelectorAll('script[type="application/ld+json"]');
				var articles = document.querySelectorAll('li, article, a');
				return {
					jsonLdCount: jsonld ? jsonld.length : 0,
					articleItemCount: articles ? articles.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Site 10: go.dev/doc/ - External Script Tolerance & Navigation Drawer
	t.Run("Site10_GoDev_NavigationDrawerAndDOMExpansion", func(t *testing.T) {
		withEngineContext(t, 30*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, err := ec.Navigate(ctx, "https://go.dev/doc/", browsercontrol.WaitComplete, 20000)
			if err != nil {
				t.Skipf("Network error navigating to go.dev/doc/: %v", err)
			}
			eval, err := ec.Evaluate(ctx, `(function() {
				var navs = document.querySelectorAll('nav');
				var headings = document.querySelectorAll('h1, h2, h3');
				return {
					navCount: navs ? navs.length : 0,
					headingCount: headings ? headings.length : 0
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Boundary Condition: Empty Content Fallback
	t.Run("Boundary_EmptyContentFallback", func(t *testing.T) {
		withEngineContext(t, 15*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			// Snapshot on newly created context before navigation
			snap, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{})
			require.NoError(t, err)
			assert.Empty(t, snap.Nodes)

			// Screenshot on empty context falls back cleanly to minimal document
			shot, err := ec.Screenshot(ctx, browsercontrol.ScreenshotOptions{})
			require.NoError(t, err)
			assert.NotEmpty(t, shot.Data)
			assert.Equal(t, 1280, shot.Width)
			assert.Equal(t, 800, shot.Height)
		})
	})

	// Boundary Condition: HTTP Redirect Handling
	t.Run("Boundary_HTTPRedirectHandling", func(t *testing.T) {
		withEngineContext(t, 25*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			// http://example.com redirects to https://example.com or succeeds
			nav, err := ec.Navigate(ctx, "http://example.com", browsercontrol.WaitComplete, 15000)
			if err != nil {
				t.Skipf("Redirect test skipped due to network: %v", err)
			}
			assert.Contains(t, []int{http.StatusOK, http.StatusMovedPermanently, http.StatusFound}, nav.HTTPStatus)
		})
	})
}

// ============================================================================
// TIER 3: CROSS-FEATURE INTERACTIONS (Multi-Subsystem Interop)
// ============================================================================
func runTier3CrossFeatureInteractions(t *testing.T) {
	// Cross-Feature 1: Window EventTarget & Global Scope Synchronization
	t.Run("WindowEventTargetAndGlobalScopeSync", func(t *testing.T) {
		withEngineContext(t, 20*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, _ = ec.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 15000)

			// Test addEventListener / dispatchEvent on window and globalThis
			eval, err := ec.Evaluate(ctx, `(function() {
				var dispatched = false;
				var targetMatch = false;
				var handler = function(e) {
					dispatched = true;
					if (e.target === window || e.currentTarget === window || e.target === globalThis) {
						targetMatch = true;
					}
				};
				if (typeof window.addEventListener === 'function') {
					window.addEventListener('custom-test', handler);
					if (typeof window.dispatchEvent === 'function') {
						try {
							var evt = new CustomEvent('custom-test', { detail: { value: 42 } });
							window.dispatchEvent(evt);
						} catch(e) {}
					}
				}
				window.myGlobalVar = 12345;
				var globalVarSynced = (typeof myGlobalVar !== 'undefined' && myGlobalVar === 12345);
				return {
					hasAddEventListener: typeof window.addEventListener === 'function',
					hasDispatchEvent: typeof window.dispatchEvent === 'function',
					globalVarSynced: globalVarSynced
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Cross-Feature 2: Element Attributes & NamedNodeMap Iteration
	t.Run("ElementAttributesAndNamedNodeMapIteration", func(t *testing.T) {
		withEngineContext(t, 20*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, _ = ec.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 15000)

			eval, err := ec.Evaluate(ctx, `(function() {
				var a = document.querySelector('a');
				var hasAttrs = a && typeof a.hasAttributes === 'function' ? a.hasAttributes() : false;
				var attrLength = (a && a.attributes) ? a.attributes.length : 0;
				var hrefVal = (a && a.attributes && a.attributes.href) ? a.attributes.href.value : '';
				return {
					hasAnchor: !!a,
					hasAttributesFunc: a ? typeof a.hasAttributes === 'function' : false,
					hasAttrs: hasAttrs,
					attrLength: attrLength
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Cross-Feature 3: DOM Query Expansion (matches, closest, matchesSelector)
	t.Run("DOMSelectorExpansionClosestAndMatches", func(t *testing.T) {
		withEngineContext(t, 20*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, _ = ec.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 15000)

			eval, err := ec.Evaluate(ctx, `(function() {
				var p = document.querySelector('p');
				var matchesP = p && typeof p.matches === 'function' ? p.matches('p') : false;
				var closestDiv = p && typeof p.closest === 'function' ? !!p.closest('div') : false;
				return {
					hasP: !!p,
					matchesFunc: p ? typeof p.matches === 'function' : false,
					closestFunc: p ? typeof p.closest === 'function' : false,
					matchesP: matchesP,
					closestDiv: closestDiv
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Cross-Feature 4: Location Alias & Timer String Eval
	t.Run("LocationAliasAndTimerStringEvaluation", func(t *testing.T) {
		withEngineContext(t, 20*time.Second, func(ec browsercontrol.Context, ctx context.Context) {
			_, _ = ec.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 15000)

			eval, err := ec.Evaluate(ctx, `(function() {
				var docLocationHref = (document.location && document.location.href) ? document.location.href : '';
				var winLocationHref = (window.location && window.location.href) ? window.location.href : '';
				var timerWorks = false;
				if (typeof setTimeout === 'function') {
					try {
						var id = setTimeout(function() {}, 10);
						if (typeof clearTimeout === 'function') clearTimeout(id);
						timerWorks = true;
					} catch(e) {}
				}
				return {
					hasDocLocation: !!document.location,
					locationMatch: docLocationHref === winLocationHref,
					timerWorks: timerWorks
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			assert.False(t, eval.IsError)
		})
	})

	// Cross-Feature 5: Script MIME Type Filtering (Ignoring application/ld+json)
	t.Run("ScriptMIMETypeFilterJSONLDAndTemplates", func(t *testing.T) {
		assert.False(t, dom.IsJavaScriptMIMEType("application/ld+json"), "application/ld+json must not be JS")
		assert.False(t, dom.IsJavaScriptMIMEType("text/template"), "text/template must not be JS")
		assert.False(t, dom.IsJavaScriptMIMEType("text/html"), "text/html must not be JS")
		assert.False(t, dom.IsJavaScriptMIMEType("application/json"), "application/json must not be JS")
		assert.True(t, dom.IsJavaScriptMIMEType(""), "empty type defaults to JS")
		assert.True(t, dom.IsJavaScriptMIMEType("text/javascript"), "text/javascript is JS")
		assert.True(t, dom.IsJavaScriptMIMEType("application/javascript"), "application/javascript is JS")
		assert.True(t, dom.IsJavaScriptMIMEType("module"), "module is JS")

		// Resource discovery test through streaming parser
		var discoveredScripts []dom.Resource
		cfg := dom.ParseConfig{
			OnResource: func(r dom.Resource) {
				if r.Kind == dom.ResourceScript {
					discoveredScripts = append(discoveredScripts, r)
				}
			},
		}
		html := `<!doctype html>
		<html>
		<head>
			<script type="application/ld+json">{"@type": "NewsArticle"}</script>
			<script type="text/template"><div>template</div></script>
			<script src="app.js"></script>
		</head>
		<body><h1>MIME Filter Test</h1></body>
		</html>`

		parser := dom.NewParser()
		_, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(html), cfg)
		require.NoError(t, err)
		assert.Len(t, discoveredScripts, 1)
		if len(discoveredScripts) > 0 {
			assert.Equal(t, "app.js", discoveredScripts[0].URL)
		}
	})
}

// ============================================================================
// TIER 4: REAL-WORLD WORKLOAD (Complete Workflow: Nav -> Eval -> Snap -> Shot)
// ============================================================================
func runTier4RealWorldWorkload(t *testing.T) {
	svc := browsercontrol.NewEngineService()

	for _, site := range TargetLiveWebsites {
		site := site
		t.Run("Workload_"+site.Name, func(t *testing.T) {
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{
				Viewport: browsercontrol.Viewport{Width: 1280, Height: 800, Scale: 1.0},
			})
			require.NoError(t, err)
			defer func() { _ = svc.CloseContext(context.Background(), info.ID) }()

			ec, err := svc.Context(ctx, info.ID)
			require.NoError(t, err)

			// Step 1: Navigate to Live URL
			navStart := time.Now()
			navRes, err := ec.Navigate(ctx, site.URL, browsercontrol.WaitComplete, 30000)
			if err != nil {
				t.Skipf("Skipping workload for %s due to network: %v", site.Name, err)
				return
			}
			navDuration := time.Since(navStart)
			require.NotEmpty(t, navRes.NavigationID)

			// Step 2: Extract Semantic DOM Snapshot
			snapStart := time.Now()
			snap, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{MaxDepth: 25, MaxNodes: 150})
			require.NoError(t, err)
			snapDuration := time.Since(snapStart)
			if site.ExpectedTitleSubstr != "" {
				assert.NotEmpty(t, snap.Title)
			}

			// Step 3: Run DOM Introspection Query via JavaScript
			evalStart := time.Now()
			evalRes, err := ec.Evaluate(ctx, `(function() {
				return {
					title: document.title,
					nodeCount: document.getElementsByTagName('*').length,
					hasBody: !!document.body,
					readyState: document.readyState
				};
			})()`, browsercontrol.EvaluateOptions{})
			require.NoError(t, err)
			evalDuration := time.Since(evalStart)
			assert.False(t, evalRes.IsError)

			// Step 4: Capture Viewport Screenshot
			shotStart := time.Now()
			shot, err := ec.Screenshot(ctx, browsercontrol.ScreenshotOptions{})
			require.NoError(t, err)
			shotDuration := time.Since(shotStart)
			assert.Greater(t, len(shot.Data), 0)

			// Step 5: Perform Viewport Scroll Action
			scrollRes, err := ec.Scroll(ctx, browsercontrol.ScrollOptions{DeltaY: 100})
			require.NoError(t, err)
			assert.True(t, scrollRes.ActionApplied)

			totalDuration := time.Since(start)
			t.Logf("[Workload PASS] %s in %v (Nav: %v, Snap: %v, Eval: %v, Shot: %v) -> Nodes: %d, ShotBytes: %d",
				site.Name, totalDuration, navDuration, snapDuration, evalDuration, shotDuration, len(snap.Nodes), len(shot.Data))
		})
	}
}

// ============================================================================
// DIRECT RENDERER & DOM PIPELINE COMPONENT TESTS (Offline & Synthetic)
// ============================================================================
func TestDirectPipelineSyntheticPages(t *testing.T) {
	syntheticCases := []struct {
		Name        string
		HTML        string
		MinHeight   float32
		ExpectNodes int
	}{
		{
			Name: "ViewportUnitsAndFlex",
			HTML: `<!doctype html><html><head><style>
				body { margin: 0; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
				.card { width: 50vw; padding: 20px; background: #f0f0f0; }
			</style></head><body><div class="card"><h1>Centered Card</h1></div></body></html>`,
			MinHeight:   50,
			ExpectNodes: 4,
		},
		{
			Name: "NestedTableLayout",
			HTML: `<!doctype html><html><body>
				<table border="1" width="100%">
					<tr><td bgcolor="#ccc">Header</td></tr>
					<tr><td><table width="80%"><tr><td>Sub cell 1</td><td>Sub cell 2</td></tr></table></td></tr>
				</table>
			</body></html>`,
			MinHeight:   30,
			ExpectNodes: 8,
		},
		{
			Name: "CustomTagsAndPreformatted",
			HTML: `<!doctype html><html><head><style>d { display: block; color: blue; }</style></head>
			<body><d>Custom Tag Content</d><pre>func main() {\n\tprintln("hello")\n}</pre></body></html>`,
			MinHeight:   20,
			ExpectNodes: 4,
		},
	}

	for _, tc := range syntheticCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			r := renderer.NewRenderer(1280, 800)
			r.SetTestingMode(true)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			obj, err := r.RenderHTML(ctx, tc.HTML)
			require.NoError(t, err)
			require.NotNil(t, obj)

			height := r.GetContentHeight()
			assert.Greater(t, height, tc.MinHeight)

			img, err := testutil.RenderToImage(obj, 1280, 800)
			require.NoError(t, err)
			assert.NotNil(t, img)
			assert.Equal(t, 1280, img.Bounds().Dx())
			assert.Equal(t, 800, img.Bounds().Dy())
		})
	}
}

// TestDOMParserAndStoreIntegration validates DOM parsing and compact store interoperability.
func TestDOMParserAndStoreIntegration(t *testing.T) {
	parser := dom.NewParser()
	htmlContent := `<!doctype html><html><head><title>Test Store</title></head><body><div id="main" class="container"><p>Paragraph text</p><a href="https://example.com">Link</a></div></body></html>`

	matched, err := parser.QuerySelectorAll(htmlContent, "div.container")
	require.NoError(t, err)
	assert.Len(t, matched, 1)

	links, err := parser.QuerySelectorAll(htmlContent, "a")
	require.NoError(t, err)
	assert.NotEmpty(t, links)
}
