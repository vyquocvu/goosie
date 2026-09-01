package a11y

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/dom"
)

// TestARIA_LandmarkRoles verifies the standard WAI-ARIA landmark
// roles are retained through the DOM parser and remain queryable
// for accessibility scanning.
func TestARIA_LandmarkRoles(t *testing.T) {
	html := `
		<html><body>
			<header role="banner">Site header</header>
			<nav role="navigation">Main nav</nav>
			<main role="main">Content</main>
			<aside role="complementary">Sidebar</aside>
			<footer role="contentinfo">Site footer</footer>
		</body></html>
	`

	cases := []struct {
		role string
		tag  string
	}{
		{"banner", "header"},
		{"navigation", "nav"},
		{"main", "main"},
		{"complementary", "aside"},
		{"contentinfo", "footer"},
	}
	p := dom.NewParser()
	for _, c := range cases {
		c := c
		t.Run(c.role, func(t *testing.T) {
			el, err := p.QuerySelector(html, c.tag)
			if err != nil || el == nil {
				t.Fatalf("query for %q failed: %v", c.tag, err)
			}
			assert.Equal(t, c.role, el.Attributes["role"], "role attribute")
		})
	}
}

// TestARIA_RequiredState verifies aria-required is preserved on form
// controls per WCAG 3.3.1 (Error Identification).
func TestARIA_RequiredState(t *testing.T) {
	html := `
		<html><body>
			<input id="requiredEmail" type="email" aria-required="true" />
			<input id="optionalName" type="text" />
		</body></html>
	`
	p := dom.NewParser()
	reqEl, _ := p.QuerySelector(html, "#requiredEmail")
	optEl, _ := p.QuerySelector(html, "#optionalName")
	assert.Equal(t, "true", reqEl.Attributes["aria-required"])
	_, ok := optEl.Attributes["aria-required"]
	assert.False(t, ok, "optional input must not have aria-required")
}

// TestARIA_HiddenState verifies aria-hidden=true is preserved on
// any element per the WAI-ARIA Authoring practice.
func TestARIA_HiddenState(t *testing.T) {
	html := `
		<html><body>
			<div id="decorative" aria-hidden="true">Decorative icon</div>
			<button id="cta">Click me</button>
			<span id="falseHidden" aria-hidden="false">Visible label</span>
		</body></html>
	`
	p := dom.NewParser()
	deco, _ := p.QuerySelector(html, "#decorative")
	cta, _ := p.QuerySelector(html, "#cta")
	falseHidden, _ := p.QuerySelector(html, "#falseHidden")

	assert.Equal(t, "true", deco.Attributes["aria-hidden"])
	_, hasAriaHidden := cta.Attributes["aria-hidden"]
	assert.False(t, hasAriaHidden, "interactive controls must not have aria-hidden=true")
	assert.Equal(t, "false", falseHidden.Attributes["aria-hidden"])
}

// TestARIA_LiveRegionFlags verifies aria-live polite/assertive
// values are preserved per WCAG 4.1.3 (Status Messages).
func TestARIA_LiveRegionFlags(t *testing.T) {
	html := `
		<html><body>
			<div id="status" aria-live="polite">Status updates here</div>
			<div id="alert" aria-live="assertive">Critical alert</div>
			<div id="off" aria-live="off">Static region</div>
		</body></html>
	`
	p := dom.NewParser()
	cases := []struct {
		id    string
		value string
	}{
		{"status", "polite"},
		{"alert", "assertive"},
		{"off", "off"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, func(t *testing.T) {
			el, _ := p.QuerySelector(html, "#"+c.id)
			assert.Equal(t, c.value, el.Attributes["aria-live"])
		})
	}
}

// TestARIA_ExpandedState verifies aria-expanded preserves the
// boolean string contract (true/false per WAI-ARIA Authoring
// guidance).
func TestARIA_ExpandedState(t *testing.T) {
	html := `
		<html><body>
			<button id="closed" aria-expanded="false">Closed menu</button>
			<button id="open" aria-expanded="true">Open menu</button>
			<details id="dt"><summary>Show more</summary></details>
		</body></html>
	`
	p := dom.NewParser()
	closed, _ := p.QuerySelector(html, "#closed")
	open, _ := p.QuerySelector(html, "#open")
	assert.Equal(t, "false", closed.Attributes["aria-expanded"])
	assert.Equal(t, "true", open.Attributes["aria-expanded"])
}

// TestARIA_CurrentLocation verifies aria-current accepts the
// documented token list (page, step, location, date, time, true,
// false).
func TestARIA_CurrentLocation(t *testing.T) {
	html := `
		<html><body>
			<nav>
				<a id="home" href="/" aria-current="page">Home</a>
				<a id="step" href="/step" aria-current="step">Step 3</a>
				<a id="loc" href="/here" aria-current="location">You are here</a>
				<a id="neutral" href="/elsewhere">Elsewhere</a>
			</nav>
		</body></html>
	`
	p := dom.NewParser()
	cases := []struct {
		id    string
		value string
	}{
		{"home", "page"},
		{"step", "step"},
		{"loc", "location"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, func(t *testing.T) {
			el, _ := p.QuerySelector(html, "#"+c.id)
			assert.Equal(t, c.value, el.Attributes["aria-current"])
		})
	}
	neutral, _ := p.QuerySelector(html, "#neutral")
	_, has := neutral.Attributes["aria-current"]
	assert.False(t, has, "neutral link must not have aria-current")
}

// TestARIA_LabelAndLabelledBy verifies aria-label and aria-labelledby
// are preserved, both directly and via reference IDs.
func TestARIA_LabelAndLabelledBy(t *testing.T) {
	html := `
		<html><body>
			<button id="labelled" aria-label="Submit form">→</button>
			<span id="visiblelabel">Search</span>
			<input id="labelledby" type="text" aria-labelledby="visiblelabel" />
			<input id="describedby" type="text" aria-describedby="hint" />
			<span id="hint">Search by name or email</span>
		</body></html>
	`
	p := dom.NewParser()
	labelled, _ := p.QuerySelector(html, "#labelled")
	labelledby, _ := p.QuerySelector(html, "#labelledby")
	describedby, _ := p.QuerySelector(html, "#describedby")

	assert.Equal(t, "Submit form", labelled.Attributes["aria-label"])
	assert.Equal(t, "visiblelabel", labelledby.Attributes["aria-labelledby"])
	assert.Equal(t, "hint", describedby.Attributes["aria-describedby"])
}

// TestARIA_RoleOnTab verifies the role attribute can be queried
// via attribute selector (used by accessibility scanners).
func TestARIA_RoleOnTab(t *testing.T) {
	html := `
		<html><body>
			<div role="tab" id="t1">Tab 1</div>
			<div role="tab" id="t2">Tab 2</div>
			<div role="tabpanel" id="t1-panel">Panel 1</div>
		</body></html>
	`
	p := dom.NewParser()
	tabs, err := p.QuerySelectorAll(html, "[role=tab]")
	if err != nil {
		t.Fatalf("QuerySelectorAll failed: %v", err)
	}
	assert.Len(t, tabs, 2, "two role=tab elements should be queryable")
	for _, tab := range tabs {
		assert.Equal(t, "tab", tab.Attributes["role"])
	}
}

// TestARIA_DisambiguationAttribute verifies atomic element labeling
// patterns: aria-atomic, aria-busy, aria-relevant on a single
// live region.
func TestARIA_DisambiguationAttribute(t *testing.T) {
	html := `
		<html><body>
			<div id="combo"
				 aria-live="polite"
				 aria-atomic="true"
				 aria-busy="false"
				 aria-relevant="additions text">
				Messages
			</div>
		</body></html>
	`
	p := dom.NewParser()
	combo, _ := p.QuerySelector(html, "#combo")
	assert.Equal(t, "polite", combo.Attributes["aria-live"])
	assert.Equal(t, "true", combo.Attributes["aria-atomic"])
	assert.Equal(t, "false", combo.Attributes["aria-busy"])
	assert.Equal(t, "additions text", combo.Attributes["aria-relevant"])
}
