package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShortcutRegistryDispatchesCommands(t *testing.T) {
	registry := NewShortcutRegistry()
	calls := []string{}
	registry.Register("focus-address", func() { calls = append(calls, "focus-address") })
	registry.Register("new-tab", func() { calls = append(calls, "new-tab") })

	require.True(t, registry.Dispatch("focus-address"))
	require.True(t, registry.Dispatch("new-tab"))
	require.False(t, registry.Dispatch("missing"))
	require.Equal(t, []string{"focus-address", "new-tab"}, calls)
}
