package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

func TestNetworkPanelAcceptsEntries(t *testing.T) {
	panel := NewNetworkPanel()
	panel.SetEntries([]goosienet.RequestLogEntry{{Method: "GET", URL: "https://example.com", Status: 200, Bytes: 42, StartedAt: time.Now()}})
	require.NotNil(t, panel.CanvasObject())
	require.Len(t, panel.entries, 1)
}
