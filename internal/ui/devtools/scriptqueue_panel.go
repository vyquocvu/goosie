package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// scriptQueuePanel shows the JavaScript task queue and timer
// statistics for the active tab. The previous implementation
// dumped everything to a single label; this version gives each
// metric its own labelled card so the user can scan the
// dashboard at a glance. The label values are bound to a
// refreshable source so they update without recreating the
// widget tree on every poll.
//
// The panel does not start an internal refresh ticker: that
// pattern caused a goroutine leak across tests where the
// ticker kept accessing browser state that the test had torn
// down. Refresh now happens explicitly via the Refresh button
// and any TabContext-driven refresh the dock wires up later.
type scriptQueuePanel struct {
	fyne.Container

	timerLabel    *widget.Label
	consoleLabel  *widget.Label
	errorLabel    *widget.Label
	runningLabel  *widget.Label
	lastErrorView *widget.Label
	errorList     *widget.List
	consoleList   *widget.List
}

func newScriptQueuePanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	timerBinding := binding.NewString()
	consoleBinding := binding.NewString()
	errorBinding := binding.NewString()
	runningBinding := binding.NewString()
	errorsBinding := binding.NewStringList()
	consoleMessagesBinding := binding.NewStringList()

	timerLabel := widget.NewLabelWithData(timerBinding)
	consoleLabel := widget.NewLabelWithData(consoleBinding)
	errorLabel := widget.NewLabelWithData(errorBinding)
	runningLabel := widget.NewLabelWithData(runningBinding)
	lastErrorView := widget.NewLabel("No JavaScript errors recorded.")
	lastErrorView.Wrapping = fyne.TextWrapWord
	lastErrorView.TextStyle.Monospace = true

	errorList := widget.NewListWithData(errorsBinding,
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			obj.(*widget.Label).Bind(item.(binding.String))
		},
	)
	consoleList := widget.NewListWithData(consoleMessagesBinding,
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapWord
			return label
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			obj.(*widget.Label).Bind(item.(binding.String))
		},
	)

	p := &scriptQueuePanel{
		timerLabel:    timerLabel,
		consoleLabel:  consoleLabel,
		errorLabel:    errorLabel,
		runningLabel:  runningLabel,
		lastErrorView: lastErrorView,
		errorList:     errorList,
		consoleList:   consoleList,
	}

	// Stat cards: one row per metric. Each card is a label + a
	// value widget. The structure stays the same across refreshes;
	// only the bound string values change.
	makeCard := func(title string, value fyne.CanvasObject) fyne.CanvasObject {
		header := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		return container.NewBorder(header, nil, nil, nil, value)
	}

	cards := container.NewGridWithColumns(2,
		makeCard("Active Timers", timerLabel),
		makeCard("Running Scripts", runningLabel),
		makeCard("Console Messages", consoleLabel),
		makeCard("JS Errors", errorLabel),
	)

	errorPanel := container.NewBorder(
		widget.NewLabelWithStyle("Recent JavaScript Errors", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewVSplit(errorList, container.NewVScroll(lastErrorView)),
	)
	consolePanel := container.NewBorder(
		widget.NewLabelWithStyle("Recent Console Output", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		consoleList,
	)

	// Refresh pulls fresh data from the active tab context and
	// pushes it into the bindings. Each metric is broken out
	// into its own bound string so a small change to one (e.g.
	// timers count) does not invalidate the others.
	refresh := func() {
		ctx := activeTab()
		if ctx == nil {
			_ = timerBinding.Set("0")
			_ = consoleBinding.Set("0")
			_ = errorBinding.Set("0")
			_ = runningBinding.Set("No tab")
			_ = errorsBinding.Set(nil)
			_ = consoleMessagesBinding.Set(nil)
			lastErrorView.SetText("No active tab.")
			return
		}
		rt := ctx.JSRuntime
		if rt == nil {
			_ = timerBinding.Set("0")
			_ = consoleBinding.Set("0")
			_ = errorBinding.Set("0")
			_ = runningBinding.Set("Not available")
			_ = errorsBinding.Set(nil)
			_ = consoleMessagesBinding.Set(nil)
			lastErrorView.SetText("JavaScript task queue monitoring requires an active JS runtime on the current tab.")
			return
		}

		timers := rt.ActiveTimersCount()
		consoleMsgs := rt.GetConsoleMessages()
		errs := rt.GetJavaScriptErrors()
		_ = timerBinding.Set(fmt.Sprintf("%d", timers))
		_ = consoleBinding.Set(fmt.Sprintf("%d", len(consoleMsgs)))
		_ = errorBinding.Set(fmt.Sprintf("%d", len(errs)))
		if rt.RunningScriptCount() > 0 {
			_ = runningBinding.Set(fmt.Sprintf("%d running", rt.RunningScriptCount()))
		} else {
			_ = runningBinding.Set("Idle")
		}

		// Most-recent errors first, capped at 50 to keep the list
		// responsive even on pages with hundreds of errors.
		start := len(errs)
		if start > 50 {
			start = len(errs) - 50
		}
		lines := make([]string, 0, len(errs)-start)
		for i := len(errs) - 1; i >= start; i-- {
			lines = append(lines, errs[i])
		}
		_ = errorsBinding.Set(lines)
		if len(errs) > 0 {
			lastErrorView.SetText(errs[len(errs)-1])
		} else {
			lastErrorView.SetText("No JavaScript errors recorded.")
		}

		// Console messages: tail 50 in chronological order so
		// users see the most recent output at the bottom of the
		// scrollable list.
		start = len(consoleMsgs)
		if start > 50 {
			start = len(consoleMsgs) - 50
		}
		var recent []string
		for _, m := range consoleMsgs[start:] {
			recent = append(recent, formatConsoleMessageLite(m.Level, stringifyConsoleData(m.Data)))
		}
		_ = consoleMessagesBinding.Set(recent)
	}

	refreshBtn := widget.NewButton("Refresh", refresh)

	// Refresh once at construction time so the panel shows
	// current values the moment the user opens the tab; further
	// refreshes happen via the Refresh button.
	refresh()

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("JavaScript Task Queue", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	body := container.NewVSplit(
		cards,
		container.NewVSplit(errorPanel, consolePanel),
	)

	p.Container = *container.NewBorder(topBar, nil, nil, nil, body)
	// Return *p (not &p.Container) so callers can type-assert
	// back to *scriptQueuePanel if they need access to the
	// underlying fields. The embedded fyne.Container still
	// satisfies the fyne.CanvasObject contract via the
	// promoted methods.
	return p
}

// formatConsoleMessageLite formats a single console message for
// the queue's recent-output list. The format is intentionally
// compact because the list is dense.
func formatConsoleMessageLite(level, text string) string {
	switch level {
	case "log":
		return fmt.Sprintf("[LOG] %s", text)
	case "info":
		return fmt.Sprintf("[INFO] %s", text)
	case "warn":
		return fmt.Sprintf("[WARN] %s", text)
	case "error":
		return fmt.Sprintf("[ERROR] %s", text)
	}
	if strings.TrimSpace(level) != "" {
		return fmt.Sprintf("[%s] %s", strings.ToUpper(level), text)
	}
	return text
}

// stringifyConsoleData converts the interface{} payload of a
// ConsoleMessage into a renderable string. Strings pass through
// unchanged; everything else uses fmt.Sprintf("%v", ...) so a
// number or boolean still surfaces usefully.
func stringifyConsoleData(data interface{}) string {
	if data == nil {
		return ""
	}
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}