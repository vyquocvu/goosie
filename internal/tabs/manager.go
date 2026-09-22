package tabs

import "sync"

// TabManager owns all tabs and tracks which is active.
type TabManager struct {
	mu          sync.Mutex
	tabs        []*Tab
	activeIdx   int
	nextID      uint64
	onCloseLast func()
	OnChange    func()
}

func NewManager(onCloseLast func()) *TabManager {
	return &TabManager{
		onCloseLast: onCloseLast,
	}
}

func (m *TabManager) NewTab() *Tab {
	m.mu.Lock()
	m.nextID++
	tab := newTab(m.nextID)
	m.tabs = append(m.tabs, tab)
	if len(m.tabs) == 1 {
		m.activeIdx = 0
	}
	m.mu.Unlock()
	m.fireChange()
	return tab
}

func (m *TabManager) Active() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tabs) == 0 {
		return nil
	}
	return m.tabs[m.activeIdx]
}

func (m *TabManager) CloseTab(id uint64) {
	m.mu.Lock()
	idx := -1
	for i, t := range m.tabs {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return
	}
	m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
	if len(m.tabs) == 0 {
		m.activeIdx = 0
		m.mu.Unlock()
		m.fireChange()
		if m.onCloseLast != nil {
			go m.onCloseLast()
		}
		return
	}
	if m.activeIdx >= len(m.tabs) {
		m.activeIdx = len(m.tabs) - 1
	} else if m.activeIdx > idx {
		m.activeIdx--
	}
	m.mu.Unlock()
	m.fireChange()
}

func (m *TabManager) SwitchTo(id uint64) {
	m.mu.Lock()
	for i, t := range m.tabs {
		if t.ID == id {
			if i == m.activeIdx {
				m.mu.Unlock()
				return
			}
			m.activeIdx = i
			m.mu.Unlock()
			m.fireChange()
			return
		}
	}
	m.mu.Unlock()
}

func (m *TabManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tabs)
}

func (m *TabManager) Tabs() []*Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Tab, len(m.tabs))
	copy(out, m.tabs)
	return out
}

func (m *TabManager) ActiveIndex() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeIdx
}

func (m *TabManager) fireChange() {
	if m.OnChange != nil {
		m.OnChange()
	}
}

func (m *TabManager) TabByID(id uint64) *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tabs {
		if t.ID == id {
			return t
		}
	}
	return nil
}
