package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	MaxTabs          = 50
	MaxHistoryPerTab = 100
)

type TabState struct {
	URL          string   `json:"url"`
	Title        string   `json:"title,omitempty"`
	History      []string `json:"history,omitempty"`
	HistoryIndex int      `json:"historyIndex,omitempty"`
	ScrollX      int      `json:"scrollX,omitempty"`
	ScrollY      int      `json:"scrollY,omitempty"`
}

type State struct {
	Tabs   []TabState `json:"tabs"`
	Active int        `json:"active"`
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
	if len(s.Tabs) > MaxTabs {
		s.Tabs = s.Tabs[:MaxTabs]
	}
	for i := range s.Tabs {
		t := &s.Tabs[i]
		if len(t.History) > MaxHistoryPerTab {
			t.History = t.History[:MaxHistoryPerTab]
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
	if s.Active >= len(s.Tabs) {
		s.Active = len(s.Tabs) - 1
	}
	if s.Active < 0 {
		s.Active = 0
	}
	return s
}
