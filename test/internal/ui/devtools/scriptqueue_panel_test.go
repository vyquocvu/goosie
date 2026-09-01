package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestScriptQueuePanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := devtools.NewScriptQueuePanelContent(func() *devtools.TabContext { return &devtools.TabContext{} })
	assert.NotNil(t, p)
}

func TestScriptQueuePanel_NilRuntime(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{JSRuntime: nil}
	p := devtools.NewScriptQueuePanelContent(func() *devtools.TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestScriptQueuePanel_WithRuntime(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	rt := js.NewRuntime()
	ctx := &devtools.TabContext{JSRuntime: rt}
	p := devtools.NewScriptQueuePanelContent(func() *devtools.TabContext { return ctx })
	assert.NotNil(t, p)
}

// TestScriptQueuePanel_FormatConsoleMessageLite exercises every
// console-message level so the panel's recent-output list keeps
// rendering all four levels plus the unknown-level fallback.
func TestScriptQueuePanel_FormatConsoleMessageLite(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"log", "[LOG] hello"},
		{"info", "[INFO] hello"},
		{"warn", "[WARN] hello"},
		{"error", "[ERROR] hello"},
		{"trace", "[TRACE] hello"},
		{"", "hello"},
		{"   ", "hello"}, // whitespace-only falls back to plain text
	}
	for _, c := range cases {
		c := c
		t.Run(c.level, func(t *testing.T) {
			got := devtools.FormatConsoleMessageLite(c.level, "hello")
			assert.Equal(t, c.want, got)
		})
	}
}

// TestScriptQueuePanel_StringifyConsoleData covers the type
// coercion for console-message payloads. Strings pass through;
// nil becomes ""; numbers, bools, and other types fall back to
// fmt.Sprintf("%v", ...).
func TestScriptQueuePanel_StringifyConsoleData(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"string", "hello", "hello"},
		{"emptyString", "", ""},
		{"nil", nil, ""},
		{"int", 42, "42"},
		{"bool", true, "true"},
		{"float", 3.14, "3.14"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := devtools.StringifyConsoleData(c.in)
			assert.Equal(t, c.want, got)
		})
	}
}