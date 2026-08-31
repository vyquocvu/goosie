package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestConsolePanel_LogFiltering(t *testing.T) {
	test.NewApp()
	panel := ui.NewConsolePanel(nil)

	panel.AddMessage(js.ConsoleMessage{Level: "error", Data: "test error", Timestamp: time.Now()})
	panel.AddMessage(js.ConsoleMessage{Level: "log", Data: "test log", Timestamp: time.Now()})

	assert.Equal(t, 2, panel.GetFilteredMessageCount())

	panel.FilterSelect().SetSelected("error")

	assert.Equal(t, 1, panel.GetFilteredMessageCount())
	assert.Equal(t, "test error", panel.GetFilteredMessage(0).Data)
}

func TestConsolePanel_Execute(t *testing.T) {
	test.NewApp()
	panel := ui.NewConsolePanel(nil)

	executedCmd := ""
	panel.SetExecuteCallback(func(cmd string) {
		executedCmd = cmd
	})

	panel.CommandEntry().SetText("test cmd")
	panel.CommandEntry().OnSubmitted("test cmd")

	assert.Equal(t, "test cmd", executedCmd)
	assert.Equal(t, "", panel.CommandEntry().Text)
}

func TestConsoleEntry_History(t *testing.T) {
	test.NewApp()
	entry := ui.NewConsoleEntry()

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
	entry := ui.NewConsoleEntry()

	entry.AddHistory("cmd 1")
	entry.AddHistory("cmd 1")
	entry.AddHistory("cmd 2")

	assert.Equal(t, 2, len(entry.History()))
	assert.Equal(t, "cmd 1", entry.History()[0])
	assert.Equal(t, "cmd 2", entry.History()[1])
}
