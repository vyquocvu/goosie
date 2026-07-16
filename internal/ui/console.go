package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/js"
)

// ConsoleEntry is a custom entry widget that supports command history via up/down arrows.
type ConsoleEntry struct {
	widget.Entry
	history      []string
	historyIndex int
}

// NewConsoleEntry creates a new ConsoleEntry.
func NewConsoleEntry() *ConsoleEntry {
	e := &ConsoleEntry{
		history:      make([]string, 0),
		historyIndex: -1,
	}
	e.Wrapping = fyne.TextTruncate
	e.ExtendBaseWidget(e)
	return e
}

// TypedKey handles key events to provide command history navigation.
func (e *ConsoleEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyUp {
		if len(e.history) > 0 {
			if e.historyIndex < 0 {
				e.historyIndex = len(e.history) - 1
			} else if e.historyIndex > 0 {
				e.historyIndex--
			}
			e.SetText(e.history[e.historyIndex])
		}
	} else if k.Name == fyne.KeyDown {
		if len(e.history) > 0 && e.historyIndex >= 0 {
			if e.historyIndex < len(e.history)-1 {
				e.historyIndex++
				e.SetText(e.history[e.historyIndex])
			} else {
				e.historyIndex = -1
				e.SetText("")
			}
		}
	} else {
		e.Entry.TypedKey(k)
	}
}

// AddHistory adds a command to the history if it is not empty.
func (e *ConsoleEntry) AddHistory(cmd string) {
	if cmd == "" {
		return
	}
	if len(e.history) > 0 && e.history[len(e.history)-1] == cmd {
		e.historyIndex = -1
		return
	}
	e.history = append(e.history, cmd)
	e.historyIndex = -1
}

// ConsolePanel represents the developer console panel
type ConsolePanel struct {
	container       *fyne.Container
	messageList     *widget.List
	messages        []js.ConsoleMessage
	clearButton     *widget.Button
	closeButton     *widget.Button
	filterSelect    *widget.Select
	filterLevel     string
	commandEntry    *ConsoleEntry
	onExecute       func(string)
	onRefresh       func()
	onClose         func()
	errorCountLabel *widget.Label
	errorCount      int
}

// NewConsolePanel creates a new console panel
func NewConsolePanel(onClose func()) *ConsolePanel {
	panel := &ConsolePanel{
		messages:    make([]js.ConsoleMessage, 0),
		filterLevel: "all",
		errorCount:  0,
		onClose:     onClose,
	}

	// Create error count label
	panel.errorCountLabel = widget.NewLabel("Errors: 0")

	// Create clear button
	panel.clearButton = widget.NewButton("Clear", func() {
		panel.messages = make([]js.ConsoleMessage, 0)
		panel.errorCount = 0
		panel.errorCountLabel.SetText("Errors: 0")
		panel.messageList.Refresh()
		if panel.onRefresh != nil {
			panel.onRefresh()
		}
	})

	// Create close button
	panel.closeButton = widget.NewButton("X", func() {
		if panel.onClose != nil {
			panel.onClose()
		}
	})

	// Create message list first, so it's available for the filter's OnChange callback
	panel.messageList = widget.NewList(
		func() int {
			return panel.getFilteredMessageCount()
		},
		func() fyne.CanvasObject {
			// Template for list items
			timeLabel := widget.NewLabel("")
			timeLabel.TextStyle.Monospace = true

			levelLabel := widget.NewLabel("")
			levelLabel.TextStyle.Bold = true

			messageLabel := widget.NewLabel("")
			messageLabel.Wrapping = fyne.TextWrapWord

			return container.NewBorder(
				container.NewHBox(timeLabel, levelLabel),
				nil, nil, nil,
				messageLabel,
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			msg := panel.getFilteredMessage(id)
			if msg == nil {
				return
			}

			border := item.(*fyne.Container)
			topContainer := border.Objects[1].(*fyne.Container)
			timeLabel := topContainer.Objects[0].(*widget.Label)
			levelLabel := topContainer.Objects[1].(*widget.Label)
			messageLabel := border.Objects[0].(*widget.Label)

			// Format time
			timeLabel.SetText(msg.Timestamp.Format("15:04:05"))

			// Set level with color
			switch msg.Level {
			case "error":
				levelLabel.SetText("[ERROR]")
				// Use a theme color that indicates an error
				// This requires a custom theme or direct color setting if available
			case "warn":
				levelLabel.SetText("[WARN]")
			case "info":
				levelLabel.SetText("[INFO]")
			case "log":
				levelLabel.SetText("[LOG]")
			case "table":
				levelLabel.SetText("[TABLE]")
			}

			// Set message content
			messageLabel.SetText(fmt.Sprintf("%v", msg.Data))
		},
	)

	// Create filter dropdown
	panel.filterSelect = widget.NewSelect(
		[]string{"all", "log", "error", "warn", "info", "table"},
		func(selected string) {
			panel.filterLevel = selected
			panel.messageList.Refresh()
		},
	)
	panel.filterSelect.SetSelected("all")

	// Create top bar with controls
	topBar := container.NewBorder(
		nil, nil,
		container.NewHBox(panel.clearButton, panel.closeButton),
		container.NewHBox(widget.NewLabel("Filter:"), panel.filterSelect, panel.errorCountLabel),
	)
	panel.commandEntry = NewConsoleEntry()
	panel.commandEntry.SetPlaceHolder("Execute JavaScript")
	panel.commandEntry.OnSubmitted = func(source string) {
		if panel.onExecute != nil && source != "" {
			panel.commandEntry.AddHistory(source)
			panel.onExecute(source)
			panel.commandEntry.SetText("")
		}
	}

	// Create main container
	panel.container = container.NewBorder(
		topBar, panel.commandEntry, nil, nil,
		panel.messageList,
	)

	return panel
}

// GetContainer returns the console panel's container
func (cp *ConsolePanel) GetContainer() *fyne.Container {
	return cp.container
}

// AddMessage adds a new message to the console
func (cp *ConsolePanel) AddMessage(msg js.ConsoleMessage) {
	cp.messages = append(cp.messages, msg)

	// Update error count
	if msg.Level == "error" {
		cp.errorCount++
		cp.errorCountLabel.SetText(fmt.Sprintf("Errors: %d", cp.errorCount))
	}

	cp.messageList.Refresh()
}

// SetMessages replaces all messages in the console
func (cp *ConsolePanel) SetMessages(messages []js.ConsoleMessage) {
	cp.messages = messages

	// Count errors
	cp.errorCount = 0
	for _, msg := range messages {
		if msg.Level == "error" {
			cp.errorCount++
		}
	}
	cp.errorCountLabel.SetText(fmt.Sprintf("Errors: %d", cp.errorCount))

	cp.messageList.Refresh()
}

// Clear clears all console messages
func (cp *ConsolePanel) Clear() {
	cp.messages = make([]js.ConsoleMessage, 0)
	cp.errorCount = 0
	cp.errorCountLabel.SetText("Errors: 0")
	cp.messageList.Refresh()
}

// SetRefreshCallback sets the callback for when console is cleared
func (cp *ConsolePanel) SetRefreshCallback(callback func()) {
	cp.onRefresh = callback
}

func (cp *ConsolePanel) SetExecuteCallback(callback func(string)) { cp.onExecute = callback }

// getFilteredMessageCount returns the count of messages matching the current filter
func (cp *ConsolePanel) getFilteredMessageCount() int {
	if cp.filterLevel == "all" {
		return len(cp.messages)
	}

	count := 0
	for _, msg := range cp.messages {
		if msg.Level == cp.filterLevel {
			count++
		}
	}
	return count
}

// getFilteredMessage returns the message at the given index after filtering
func (cp *ConsolePanel) getFilteredMessage(index int) *js.ConsoleMessage {
	if cp.filterLevel == "all" {
		if index >= 0 && index < len(cp.messages) {
			return &cp.messages[index]
		}
		return nil
	}

	// Find the nth message matching the filter
	currentIndex := 0
	for i := range cp.messages {
		if cp.messages[i].Level == cp.filterLevel {
			if currentIndex == index {
				return &cp.messages[i]
			}
			currentIndex++
		}
	}
	return nil
}

// GetErrorCount returns the current error count
func (cp *ConsolePanel) GetErrorCount() int {
	return cp.errorCount
}

// CanvasObject returns the underlying canvas object for the panel
func (p *ConsolePanel) CanvasObject() fyne.CanvasObject {
	return p.container
}
