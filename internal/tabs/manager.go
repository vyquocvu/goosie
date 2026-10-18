package tabs

import (
	"sync"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

// maxClosedTabs is the maximum number of closed tab snapshots preserved for reopening.
const maxClosedTabs = 25

// ClosedTab holds snapshot state of a closed tab to support ReopenTab (Cmd+Shift+T).
type ClosedTab struct {
	URL     string
	Title   string
	History *toolbar.History
	ScrollY int32
}

// TabManager owns all tabs and tracks which is active.
type TabManager struct {
	mu          sync.Mutex
	tabs        []*Tab
	activeIdx   int
	nextID      uint64
	closedTabs  []ClosedTab
	onCloseLast func()
	OnChange    func()
	// tabScroll is the strip's horizontal offset in logical pixels. The wheel
	// handler, the draw path and hit testing share this one value under mu so
	// a press always lands on the tab the pixels show.
	tabScroll int32
	// drag tracks a pointer tab-reorder: the dragged tab and its current
	// strip X. Draw and hit-testing share it under mu with everything else.
	dragID uint64
	dragX  int32
	dragOn bool
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

// NewIncognitoTab creates a new private incognito tab.
func (m *TabManager) NewIncognitoTab() *Tab {
	m.mu.Lock()
	m.nextID++
	tab := newTab(m.nextID)
	tab.incognito = true
	tab.title = "New Incognito Tab"
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
		if t.ID() == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return
	}
	closingTab := m.tabs[idx]
	var histClone *toolbar.History
	var hasHistory bool
	if h := closingTab.History(); h != nil {
		entries, histIdx := h.Entries()
		if len(entries) > 0 {
			hasHistory = true
			histClone = toolbar.RestoreHistory(entries, histIdx)
		}
	}
	url := closingTab.URL()
	// Incognito tabs are ephemeral and leave zero trace in closed tab history.
	if !closingTab.Incognito() && (url != "" || hasHistory) {
		m.closedTabs = append(m.closedTabs, ClosedTab{
			URL:     url,
			Title:   closingTab.Title(),
			History: histClone,
			ScrollY: closingTab.ScrollY(),
		})
		if len(m.closedTabs) > maxClosedTabs {
			m.closedTabs = m.closedTabs[len(m.closedTabs)-maxClosedTabs:]
		}
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

// ReopenTab creates a new tab restoring the state of the most recently closed tab,
// appends it, activates it, and returns the tab and true. If no closed tab is
// available, it returns nil, false.
func (m *TabManager) ReopenTab() (*Tab, bool) {
	m.mu.Lock()
	if len(m.closedTabs) == 0 {
		m.mu.Unlock()
		return nil, false
	}
	n := len(m.closedTabs) - 1
	rec := m.closedTabs[n]
	m.closedTabs = m.closedTabs[:n]

	m.nextID++
	tab := newTab(m.nextID)
	tab.SetURL(rec.URL)
	if rec.Title != "" {
		tab.SetTitle(rec.Title)
	}
	if rec.History != nil {
		entries, idx := rec.History.Entries()
		tab.SetHistory(toolbar.RestoreHistory(entries, idx))
	}
	tab.SetScrollY(rec.ScrollY)

	m.tabs = append(m.tabs, tab)
	m.activeIdx = len(m.tabs) - 1
	m.mu.Unlock()
	m.fireChange()
	return tab, true
}

// CanReopenTab reports whether there are closed tabs available to reopen.
func (m *TabManager) CanReopenTab() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.closedTabs) > 0
}

// ClosedTabCount reports the number of closed tabs in the reopen stack.
func (m *TabManager) ClosedTabCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.closedTabs)
}

// PopClosedTab pops and returns the most recently closed tab record without creating a tab.
func (m *TabManager) PopClosedTab() (ClosedTab, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.closedTabs) == 0 {
		return ClosedTab{}, false
	}
	n := len(m.closedTabs) - 1
	rec := m.closedTabs[n]
	m.closedTabs = m.closedTabs[:n]
	return rec, true
}

func (m *TabManager) SwitchTo(id uint64) {
	m.mu.Lock()
	for i, t := range m.tabs {
		if t.ID() == id {
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
		if t.ID() == id {
			return t
		}
	}
	return nil
}

// TabScroll reports the strip's horizontal offset in logical pixels.
func (m *TabManager) TabScroll() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tabScroll
}

// SetTabScroll moves the strip, clamped to [0, StripMaxOffset] for the given
// geometry so an over-scroll or a resize can never park it past its content.
func (m *TabManager) SetTabScroll(off, tabCount, contentWidth int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	max := StripMaxOffset(tabCount, contentWidth)
	if off < 0 {
		off = 0
	}
	if off > max {
		off = max
	}
	m.tabScroll = off
}

// MoveTab reorders tabID to final index toIndex (clamped), keeping the
// active tab stable by identity rather than position. It reports whether
// anything moved. Change notification fires after unlock, like every other
// mutator: callbacks reenter the manager.
func (m *TabManager) MoveTab(id uint64, toIndex int) bool {
	m.mu.Lock()
	from := -1
	for i, t := range m.tabs {
		if t.ID() == id {
			from = i
			break
		}
	}
	if from < 0 {
		m.mu.Unlock()
		return false
	}
	to := toIndex
	if to < 0 {
		to = 0
	}
	if to >= len(m.tabs) {
		to = len(m.tabs) - 1
	}
	if from == to {
		m.mu.Unlock()
		return false
	}
	activeID := m.tabs[m.activeIdx].ID()
	tab := m.tabs[from]
	m.tabs = append(m.tabs[:from], m.tabs[from+1:]...)
	if to > len(m.tabs) {
		to = len(m.tabs)
	}
	m.tabs = append(m.tabs, nil)
	copy(m.tabs[to+1:], m.tabs[to:])
	m.tabs[to] = tab
	for i, t := range m.tabs {
		if t.ID() == activeID {
			m.activeIdx = i
			break
		}
	}
	m.mu.Unlock()
	m.fireChange()
	return true
}

// StartDrag begins a pointer reorder of the given tab at strip X.
func (m *TabManager) StartDrag(id uint64, x int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dragID, m.dragX, m.dragOn = id, x, true
}

// MoveDrag moves the in-progress reorder to strip X.
func (m *TabManager) MoveDrag(x int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dragOn {
		m.dragX = x
	}
}

// Dragging reports the in-progress reorder, if any.
func (m *TabManager) Dragging() (id uint64, x int32, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dragID, m.dragX, m.dragOn
}

// EndDrag clears any in-progress reorder.
func (m *TabManager) EndDrag() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dragOn = false
}
