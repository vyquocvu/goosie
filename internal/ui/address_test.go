package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAddressInput(t *testing.T) {
	search := "https://duckduckgo.com/?q="
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "absolute https", input: "https://example.com/a", want: "https://example.com/a"},
		{name: "host becomes https", input: "example.com", want: "https://example.com"},
		{name: "localhost keeps http", input: "localhost:8080", want: "http://localhost:8080"},
		{name: "query uses search engine", input: "golang browser engine", want: "https://duckduckgo.com/?q=golang+browser+engine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ResolveAddressInput(tt.input, search))
		})
	}
}
