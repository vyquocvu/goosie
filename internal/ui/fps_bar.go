package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// FPSBar is a fixed UI bar displaying live frame rate and performance metrics
// outside of the web page canvas.
type FPSBar struct {
	container *fyne.Container
	label     *widget.Label
	bg        *canvas.Rectangle
	visible   bool
	lastText  string
}

// NewFPSBar creates a new fixed FPS bar.
func NewFPSBar() *FPSBar {
	label := widget.NewLabel("FPS: -")
	label.TextStyle = fyne.TextStyle{Monospace: true}
	label.Wrapping = fyne.TextTruncate

	// Dark semi-transparent background bar
	bg := canvas.NewRectangle(color.RGBA{R: 24, G: 28, B: 34, A: 245})

	bar := &FPSBar{
		label: label,
		bg:    bg,
	}

	const barHeight float32 = 26
	barContent := container.NewMax(
		bg,
		container.NewPadded(label),
	)

	bar.container = container.New(&fixedHeightLayout{height: barHeight}, barContent)
	bar.container.Hide()
	bar.visible = false

	return bar
}

// CanvasObject returns the root canvas object for the FPS bar.
func (f *FPSBar) CanvasObject() fyne.CanvasObject {
	return f.container
}

// Show makes the FPS bar visible.
func (f *FPSBar) Show() {
	f.visible = true
	f.container.Show()
	f.container.Refresh()
}

// Hide hides the FPS bar.
func (f *FPSBar) Hide() {
	f.visible = false
	f.container.Hide()
	f.container.Refresh()
}

// Visible returns whether the FPS bar is currently visible.
func (f *FPSBar) Visible() bool {
	return f.visible
}

// Text returns the current text displayed in the FPS bar.
func (f *FPSBar) Text() string {
	return f.label.Text
}

// Update refreshes the metrics displayed on the FPS bar.
func (f *FPSBar) Update(stats renderer.FPSStats, m renderer.FrameMetricsSnapshot) {
	fpsDisplay := stats.CurrentFPS
	if fpsDisplay <= 0 && m.CurrentFPS > 0 {
		fpsDisplay = m.CurrentFPS
	}
	if fpsDisplay <= 0 {
		fpsDisplay = 60.0
	}

	parts := []string{
		fmt.Sprintf("FPS: %.1f", fpsDisplay),
		fmt.Sprintf("latency: i→p %s, q %s", formatFPSDuration(m.MaxInputToPresent), formatFPSDuration(m.MaxUIQueueWait)),
		fmt.Sprintf("frames: %d long, %d drop", m.LongFrames, m.StaleFramesDropped),
		fmt.Sprintf("coalesced: s%d m%d i%d", m.CoalescedScrollEvents, m.CoalescedMutations, m.CoalescedImages),
	}
	newText := strings.Join(parts, "  •  ")

	if f.lastText == newText {
		return
	}
	f.lastText = newText
	f.label.SetText(newText)
	f.label.Refresh()
}

// SetTheme updates background color when the application theme changes.
func (f *FPSBar) SetTheme(_ ThemeType) {
	if f.bg != nil {
		// Use contrasting dark bar background
		f.bg.FillColor = theme.InputBackgroundColor()
		f.bg.Refresh()
	}
}

// formatFPSDuration formats a duration into a compact human-readable string.
func formatFPSDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d >= time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}
