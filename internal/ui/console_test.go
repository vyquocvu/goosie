package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestConsolePanel_LogFiltering(t *testing.T) {
	test.NewApp()
	panel := NewConsolePanel(nil)

	panel.AddMessage(js.ConsoleMessage{Level: "error", Data: "test error", Timestamp: time.Now()})
	panel.AddMessage(js.ConsoleMessage{Level: "log", Data: "test log", Timestamp: time.Now()})

	assert.Equal(t, 2, panel.getFilteredMessageCount())

	panel.filterSelect.SetSelected("error")

	assert.Equal(t, 1, panel.getFilteredMessageCount())
	assert.Equal(t, "test error", panel.getFilteredMessage(0).Data)
}

func TestConsolePanel_Execute(t *testing.T) {
	test.NewApp()
	panel := NewConsolePanel(nil)

	executedCmd := ""
	panel.SetExecuteCallback(func(cmd string) {
		executedCmd = cmd
	})

	panel.commandEntry.SetText("test cmd")
	panel.commandEntry.OnSubmitted("test cmd")

	assert.Equal(t, "test cmd", executedCmd)
	assert.Equal(t, "", panel.commandEntry.Text)
}

func TestConsoleEntry_History(t *testing.T) {
	test.NewApp()
	entry := NewConsoleEntry()

	entry.AddHistory("cmd 1")
	entry.AddHistory("cmd 2")

	// up
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	assert.Equal(t, "cmd 2", entry.Text)

	// up
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	assert.Equal(t, "cmd 1", entry.Text)

	// up again (should stay at oldest)
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	assert.Equal(t, "cmd 1", entry.Text)

	// down
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	assert.Equal(t, "cmd 2", entry.Text)

	// down (should clear)
	entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	assert.Equal(t, "", entry.Text)
}

func TestConsoleEntry_DuplicateHistory(t *testing.T) {
	test.NewApp()
	entry := NewConsoleEntry()

	entry.AddHistory("cmd 1")
	entry.AddHistory("cmd 1")
	entry.AddHistory("cmd 2")

	assert.Equal(t, 2, len(entry.history))
	assert.Equal(t, "cmd 1", entry.history[0])
	assert.Equal(t, "cmd 2", entry.history[1])
}
