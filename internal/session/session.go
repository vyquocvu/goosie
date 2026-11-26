package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	MaxTabs              = 50
	MaxHistoryPerTab     = 100
	maxHistoryStateBytes = 256 << 10
)

type TabState struct {
	URL                 string   `json:"url"`
	Title               string   `json:"title,omitempty"`
	History             []string `json:"history,omitempty"`
	HistoryStates       [][]byte `json:"historyStates,omitempty"`
	HistorySameDocument []bool   `json:"historySameDocument,omitempty"`
	HistoryIndex        int      `json:"historyIndex,omitempty"`
	ScrollX             int      `json:"scrollX,omitempty"`
	ScrollY             int      `json:"scrollY,omitempty"`
}

type WindowState struct {
	Tabs   []TabState `json:"tabs"`
	Active int        `json:"active"`
}

type State struct {
	Tabs    []TabState    `json:"tabs"`
	Active  int           `json:"active"`
	Windows []WindowState `json:"windows,omitempty"`
}

func Save(path string, s State) error {
	s = clamp(s)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	return clamp(s), nil
}

func clamp(s State) State {
	s.Tabs, s.Active = clampTabs(s.Tabs, s.Active)
	for i := range s.Windows {
		s.Windows[i].Tabs, s.Windows[i].Active = clampTabs(s.Windows[i].Tabs, s.Windows[i].Active)
	}
	if len(s.Windows) > 0 {
		s.Tabs = s.Windows[0].Tabs
		s.Active = s.Windows[0].Active
	}
	return s
}

func clampTabs(tabs []TabState, active int) ([]TabState, int) {
	if len(tabs) > MaxTabs {
		tabs = tabs[:MaxTabs]
	}
	for i := range tabs {
		t := &tabs[i]
		if len(t.History) > MaxHistoryPerTab {
			t.History = t.History[:MaxHistoryPerTab]
		}
		if len(t.HistoryStates) > len(t.History) {
			t.HistoryStates = t.HistoryStates[:len(t.History)]
		}
		for len(t.HistoryStates) < len(t.History) {
			t.HistoryStates = append(t.HistoryStates, nil)
		}
		for i, state := range t.HistoryStates {
			if len(state) > maxHistoryStateBytes {
				t.HistoryStates[i] = nil
			}
		}
		if len(t.HistorySameDocument) > len(t.History) {
			t.HistorySameDocument = t.HistorySameDocument[:len(t.History)]
		}
		for len(t.HistorySameDocument) < len(t.History) {
			t.HistorySameDocument = append(t.HistorySameDocument, false)
		}
		if t.HistoryIndex >= len(t.History) {
			t.HistoryIndex = len(t.History) - 1
		}
		if t.HistoryIndex < -1 {
			t.HistoryIndex = -1
		}
		if t.URL != "" && t.HistoryIndex >= 0 && t.HistoryIndex < len(t.History) && t.History[t.HistoryIndex] != t.URL {
			t.URL = t.History[t.HistoryIndex]
		}
	}
	if active >= len(tabs) {
		active = len(tabs) - 1
	}
	if active < 0 {
		active = 0
	}
	return tabs, active
}
