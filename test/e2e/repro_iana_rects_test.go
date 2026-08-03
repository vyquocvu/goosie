//go:build e2e && online

package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReproIANAElementRects(t *testing.T) {
	url := "https://www.iana.org/help/example-domains"
	pwPage := newPage(t)
	defer pwPage.Close()
	require.NoError(t, pwPage.SetViewportSize(1280, 800))
	_, err := pwPage.Goto(url)
	require.NoError(t, err)

	result, err := pwPage.Evaluate(`() => {
		function rect(sel) {
			const el = document.querySelector(sel);
			if (!el) return null;
			const r = el.getBoundingClientRect();
			const cs = window.getComputedStyle(el);
			return {sel, x:r.x, y:r.y, w:r.width, h:r.height, display:cs.display,
				flexDirection:cs.flexDirection, flexBasis:cs.flexBasis, flexGrow:cs.flexGrow,
				width:cs.width, padding:cs.padding, margin:cs.margin, position:cs.position};
		}
		const sels = ['article', 'article.sidenav', 'main', '#sidenav', 'ol.breadcrumb',
			'h1', 'header', '#header', '#logo', '.navigation', 'footer', '#footer', 'body'];
		return sels.map(rect);
	}`)
	require.NoError(t, err)
	for _, r := range result.([]interface{}) {
		fmt.Printf("%+v\n", r)
	}
}
