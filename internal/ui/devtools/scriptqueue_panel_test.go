package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestScriptQueuePanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newScriptQueuePanelContent(func() *TabContext { return &TabContext{} })
	assert.NotNil(t, p)
}

func TestScriptQueuePanel_NilRuntime(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{JSRuntime: nil}
	p := newScriptQueuePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestScriptQueuePanel_WithRuntime(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	rt := js.NewRuntime()
	ctx := &TabContext{JSRuntime: rt}
	p := newScriptQueuePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}
