package webapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestLocalStorage(t *testing.T) {
	runtime := js.NewRuntime()

	tests := []struct {
		name     string
		script   string
		expected interface{}
	}{
		{
			name:     "Set and Get Item",
			script:   `localStorage.setItem('key1', 'value1'); localStorage.getItem('key1');`,
			expected: "value1",
		},
		{
			name:     "Remove Item",
			script:   `localStorage.setItem('key2', 'value2'); localStorage.removeItem('key2'); localStorage.getItem('key2');`,
			expected: nil,
		},
		{
			name:     "Clear",
			script:   `localStorage.setItem('key3', 'value3'); localStorage.clear(); localStorage.getItem('key3');`,
			expected: nil,
		},
		{
			name:     "Length",
			script:   `localStorage.clear(); localStorage.setItem('a', '1'); localStorage.setItem('b', '2'); localStorage.length();`,
			expected: int64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := runtime.RunScript(tt.script)
			assert.NoError(t, err)

			if tt.expected == nil {
				// goja.Null or goja.Undefined check
				assert.True(t, val == nil || val.Export() == nil || val.String() == "null" || val.String() == "undefined")
			} else {
				assert.Equal(t, tt.expected, val.Export())
			}
		})
	}
}

func TestSessionStorage(t *testing.T) {
	runtime := js.NewRuntime()

	tests := []struct {
		name     string
		script   string
		expected interface{}
	}{
		{
			name:     "Set and Get Item",
			script:   `sessionStorage.setItem('skey1', 'svalue1'); sessionStorage.getItem('skey1');`,
			expected: "svalue1",
		},
		{
			name:     "Isolation from LocalStorage",
			script:   `localStorage.setItem('common', 'local'); sessionStorage.setItem('common', 'session'); sessionStorage.getItem('common');`,
			expected: "session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := runtime.RunScript(tt.script)
			assert.NoError(t, err)
			if tt.expected == nil {
				assert.True(t, val == nil || val.Export() == nil || val.String() == "null" || val.String() == "undefined")
			} else {
				assert.Equal(t, tt.expected, val.Export())
			}
		})
	}
}
