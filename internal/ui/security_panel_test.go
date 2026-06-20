package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

func TestSecurityPanelSummary(t *testing.T) {
	panel := NewSecurityPanel()
	panel.SetSummary(goosienet.SecuritySummary{URL: "https://example.com", Scheme: "https", Secure: true, Subject: "example.com"})
	require.NotNil(t, panel.CanvasObject())
	require.Contains(t, panel.summaryLabel.Text, "example.com")
}
