package main

import (
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/tabs"
)

const unresponsiveRetryBudget = 30 * time.Second

type scriptPolicy struct {
	disabled bool
	timeout  time.Duration
}

func unresponsiveScriptPolicy(rawURL string) (target string, policy scriptPolicy, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Scheme, "goosie") || !strings.EqualFold(u.Host, "unresponsive") {
		return "", scriptPolicy{}, false
	}
	action := u.Query().Get("action")
	target = u.Query().Get("url")
	if !isHTTPURL(target) {
		return "", scriptPolicy{}, false
	}
	switch action {
	case "stop":
		return target, scriptPolicy{disabled: true}, true
	case "wait":
		return target, scriptPolicy{timeout: unresponsiveRetryBudget}, true
	}
	return "", scriptPolicy{}, false
}

func formatUnresponsivePageHTML(target string, count int) string {
	shown := "this page"
	if target != "" {
		shown = "<code>" + html.EscapeString(target) + "</code>"
	}
	lead := "Goosie stopped a script on " + shown + " after " + js.DefaultTimeout.String() + " because it did not return."
	if count > 1 {
		lead = fmt.Sprintf("Goosie stopped %d scripts on %s after %s because they did not return.", count, shown, js.DefaultTimeout.String())
	}
	stopHref := "goosie://unresponsive?action=stop&url=" + url.QueryEscape(target)
	waitHref := "goosie://unresponsive?action=wait&url=" + url.QueryEscape(target)
	body := fmt.Sprintf(`<div class="card">
      <h1>Unresponsive Script</h1>
      <p>%s</p>
      <p>Choose how to reload the page:</p>
      <p style="margin:20px 0;">
        <a id="stop-script" class="btn-action" href="%s">Reload without scripts</a>
        <a id="wait-script" class="btn-action secondary" href="%s">Reload with a %s script limit</a>
      </p>
      <p style="color:#5f6368;font-size:13px;">Reloading without scripts may leave parts of the page missing. The longer limit gives the page's own code more time to finish.</p>
    </div>`, lead, html.EscapeString(stopHref), html.EscapeString(waitHref), unresponsiveRetryBudget.String())
	return internalPageTemplate("", "Unresponsive Script", body)
}

func buildUnresponsivePageHTML(rawURL string) (title, htmlContent string) {
	u, _ := url.Parse(rawURL)
	target := ""
	count := 0
	if u != nil {
		target = u.Query().Get("url")
		count, _ = strconv.Atoi(u.Query().Get("count"))
	}
	return "Unresponsive Script", formatUnresponsivePageHTML(target, count)
}

func (f *framePath) buildUnresponsivePage(target string, count int) (*frame.Layer, *engine.Session, frame.Color) {
	if f.fonts == nil || f.contentWidth() <= 0 {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	w := f.contentWidth()
	h := f.contentHeight()
	if w <= 0 {
		w = 800
	}
	if h <= 0 {
		h = 600
	}
	sess, err := engine.NewSession(formatUnresponsivePageHTML(target, count), nil, float32(w),
		engine.WithMetrics(f.fonts),
		engine.WithViewportH(float32(h)),
		engine.WithDarkMode(f.isDarkMode()),
		engine.WithHighContrast(f.isHighContrast()),
	)
	if err != nil {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	scale := float32(f.config.dpr)
	if scale < 1 {
		scale = 1
	}
	list, err := sess.PaintChecked(scale)
	if err != nil {
		sess.Close()
		return nil, nil, frame.RGB(255, 255, 255)
	}
	dl := list.Build(1)
	extent, boundsErr := sess.DocumentLayerBounds(dl.Extent(), scale, f.documentViewport())
	if boundsErr != nil {
		sess.Close()
		return nil, nil, frame.RGB(255, 255, 255)
	}
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		sess.Close()
		return nil, nil, frame.RGB(255, 255, 255)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	return layer, sess, sess.BackgroundColor()
}

func (f *framePath) installUnresponsivePrompt(tab *tabs.Tab, target string, count int, serial uint64, replaced *engine.Session) bool {
	layer, sess, bg := f.buildUnresponsivePage(target, count)
	if layer == nil || sess == nil {
		return false
	}
	current := tab.Session()
	tab.SetLayer(layer)
	tab.SetSession(sess)
	tab.SetBGColor(bg)
	tab.SetURL(target)
	tab.SetTitle("Unresponsive Script")
	tab.SetFavicon(nil)
	if current != nil && current != sess {
		current.Close()
	}
	if replaced != nil && replaced != sess && replaced != current {
		replaced.Close()
	}
	if f.tabMgr != nil && f.tabMgr.Active() == tab {
		if f.toolbar != nil {
			f.syncTabToToolbar()
		}
		f.sched.SetPlan(frame.FramePlan{
			Serial:     serial,
			Layers:     []*frame.Layer{layer},
			Background: bg,
		})
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{},
			Size:   f.devSize(),
		})
		f.publishAccessibility()
	}
	return true
}
