package main

import (
	"fmt"
	"html"
	"strings"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

// categorizeError classifies a network/navigation failure into a user-friendly code and summary.
func categorizeError(err error) (code, heading, summary string) {
	if err == nil {
		return "ERR_UNKNOWN", "Unknown Error", "An unexpected error occurred."
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "no such host") || strings.Contains(s, "lookup") || strings.Contains(s, "server misbehaving"):
		return "ERR_NAME_NOT_RESOLVED", "Server Not Found", "Goosie cannot find the server at this address."
	case strings.Contains(s, "connection refused"):
		return "ERR_CONNECTION_REFUSED", "Unable to Connect", "The server at this address refused to connect."
	case strings.Contains(s, "certificate") || strings.Contains(s, "tls:") || strings.Contains(s, "x509"):
		return "ERR_CERT_COMMON_NAME_INVALID", "Your Connection is Not Private", "The security certificate presented by the website is invalid or untrusted."
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "ERR_CONNECTION_TIMED_OUT", "Connection Timed Out", "The server took too long to respond."
	default:
		return "ERR_CONNECTION_FAILED", "This Site Can’t Be Reached", "The webpage at this address might be temporarily down or may have moved permanently."
	}
}

// formatErrorPageHTML generates clean, responsive in-viewport HTML for navigation failures.
func formatErrorPageHTML(rawURL string, err error) string {
	code, heading, summary := categorizeError(err)
	escapedURL := html.EscapeString(rawURL)
	escapedErr := html.EscapeString(err.Error())

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
body {
  font-family: sans-serif;
  background-color: #f8f9fa;
  color: #202124;
  margin: 0;
  padding: 40px;
}
.error-card {
  max-width: 640px;
  margin: 40px auto;
  background-color: #ffffff;
  padding: 36px;
  border-radius: 8px;
  border: 1px solid #dadce0;
}
h1 {
  font-size: 24px;
  font-weight: 600;
  color: #d93025;
  margin-top: 0;
  margin-bottom: 8px;
}
.url-bar {
  font-size: 14px;
  color: #5f6368;
  word-break: break-all;
  margin-bottom: 16px;
}
.summary {
  font-size: 15px;
  line-height: 1.5;
  color: #3c4043;
  margin-bottom: 20px;
}
.diag-box {
  font-family: monospace;
  font-size: 13px;
  background-color: #f1f3f4;
  padding: 12px;
  border-radius: 4px;
  color: #202124;
  margin-bottom: 24px;
}
.code {
  font-weight: bold;
  color: #c5221f;
  margin-bottom: 4px;
}
.help-title {
  font-size: 14px;
  font-weight: 600;
  color: #202124;
  margin-bottom: 8px;
}
ul {
  padding-left: 20px;
  margin: 0;
  font-size: 14px;
  color: #3c4043;
  line-height: 1.6;
}
</style>
</head>
<body>
  <div class="error-card">
    <h1>%s</h1>
    <div class="url-bar">%s</div>
    <div class="summary">%s</div>
    <div class="diag-box">
      <div class="code">%s</div>
      <div>%s</div>
    </div>
    <div class="help-title">Try:</div>
    <ul>
      <li>Checking the address for typing errors</li>
      <li>Checking your network connection and cables</li>
      <li>Checking your firewall and proxy settings</li>
    </ul>
  </div>
</body>
</html>`, heading, escapedURL, summary, code, escapedErr)
}

// buildErrorPage renders the error HTML page into a frame Layer for in-viewport display.
func (f *framePath) buildErrorPage(rawURL string, err error) (*frame.Layer, *engine.Session, frame.Color) {
	if f.fonts == nil || f.config.width <= 0 {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	w := f.config.width
	h := f.config.height
	if w <= 0 {
		w = 800
	}
	if h <= 0 {
		h = 600
	}
	htmlStr := formatErrorPageHTML(rawURL, err)
	sess, sessErr := engine.NewSession(htmlStr, nil, float32(w),
		engine.WithMetrics(f.fonts),
		engine.WithViewportH(float32(h)),
	)
	if sessErr != nil {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	scale := float32(f.config.dpr)
	if scale < 1 {
		scale = 1
	}
	list, paintErr := sess.PaintChecked(scale)
	if paintErr != nil {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, budgetErr := engine.TileCacheBudget(extent)
	if budgetErr != nil {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	return layer, sess, sess.BackgroundColor()
}
