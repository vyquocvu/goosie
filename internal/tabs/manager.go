package tabs

import (
	"strings"
	"sync"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

const maxClosedTabs = 25

type ClosedTab struct {
	URL     string
	Title   string
	History *toolbar.History
	ScrollY int32
}

type TabManager struct {
	mu          sync.Mutex
	tabs        []*Tab
	activeIdx   int
	nextID      uint64
	closedTabs  []ClosedTab
	onCloseLast func()
	OnChange    func()
	tabScroll   int32
	dragID      uint64
	dragX       int32
	dragOn      bool
	theme       string
}

func (m *TabManager) SetTheme(theme string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "dark", "high-contrast":
		m.theme = strings.ToLower(strings.TrimSpace(theme))
	default:
		m.theme = "light"
	}
}

func (m *TabManager) Theme() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.theme == "" {
		return "light"
	}
	return m.theme
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

func (m *TabManager) archiveClosedTabLocked(closingTab *Tab) {
	if closingTab == nil || closingTab.Incognito() {
		return
	}
	var histClone *toolbar.History
	var hasHistory bool
	if h := closingTab.History(); h != nil {
		entries, _ := h.Entries()
		if len(entries) > 0 {
			hasHistory = true
			histClone = h.Clone()
		}
	}
	url := closingTab.URL()
	if url != "" || hasHistory {
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
	m.archiveClosedTabLocked(closingTab)

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

func (m *TabManager) DuplicateTab(id uint64) (*Tab, bool) {
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
		return nil, false
	}
	src := m.tabs[idx]
	m.nextID++
	dup := newTab(m.nextID)
	dup.SetURL(src.URL())
	dup.SetTitle(src.Title())
	dup.SetIncognito(src.Incognito())
	dup.SetScrollY(src.ScrollY())
	if h := src.History(); h != nil {
		dup.SetHistory(h.Clone())
	}
	newIdx := idx + 1
	m.tabs = append(m.tabs, nil)
	copy(m.tabs[newIdx+1:], m.tabs[newIdx:])
	m.tabs[newIdx] = dup
	m.activeIdx = newIdx
	m.mu.Unlock()
	m.fireChange()
	return dup, true
}

func (m *TabManager) CloseOtherTabs(keepID uint64) {
	m.mu.Lock()
	var remaining []*Tab
	var targetTab *Tab
	for _, t := range m.tabs {
		if t.ID() == keepID {
			remaining = append(remaining, t)
			targetTab = t
		} else if t.Pinned() {
			remaining = append(remaining, t)
		} else {
			m.archiveClosedTabLocked(t)
		}
	}
	m.tabs = remaining
	if len(m.tabs) == 0 {
		m.activeIdx = 0
		m.mu.Unlock()
		m.fireChange()
		if m.onCloseLast != nil {
			go m.onCloseLast()
		}
		return
	}
	m.activeIdx = 0
	if targetTab != nil {
		for i, t := range m.tabs {
			if t.ID() == targetTab.ID() {
				m.activeIdx = i
				break
			}
		}
	}
	m.mu.Unlock()
	m.fireChange()
}

func (m *TabManager) CloseTabsToRight(fromID uint64) {
	m.mu.Lock()
	fromIdx := -1
	for i, t := range m.tabs {
		if t.ID() == fromID {
			fromIdx = i
			break
		}
	}
	if fromIdx < 0 {
		m.mu.Unlock()
		return
	}
	activeID := uint64(0)
	if m.activeIdx >= 0 && m.activeIdx < len(m.tabs) {
		activeID = m.tabs[m.activeIdx].ID()
	}

	var remaining []*Tab
	for i := 0; i <= fromIdx; i++ {
		remaining = append(remaining, m.tabs[i])
	}
	for i := fromIdx + 1; i < len(m.tabs); i++ {
		t := m.tabs[i]
		if t.Pinned() {
			remaining = append(remaining, t)
		} else {
			m.archiveClosedTabLocked(t)
		}
	}
	m.tabs = remaining
	if len(m.tabs) == 0 {
		m.activeIdx = 0
		m.mu.Unlock()
		m.fireChange()
		if m.onCloseLast != nil {
			go m.onCloseLast()
		}
		return
	}
	newActiveIdx := 0
	foundActive := false
	for i, t := range m.tabs {
		if t.ID() == activeID {
			newActiveIdx = i
			foundActive = true
			break
		}
	}
	if !foundActive {
		newActiveIdx = fromIdx
		if newActiveIdx >= len(m.tabs) {
			newActiveIdx = len(m.tabs) - 1
		}
	}
	m.activeIdx = newActiveIdx
	m.mu.Unlock()
	m.fireChange()
}

func (m *TabManager) PinTab(id uint64) bool {
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
		return false
	}
	tab := m.tabs[idx]
	tab.SetPinned(true)
	lastPinnedIdx := -1
	for i, t := range m.tabs {
		if t.Pinned() && t.ID() != id {
			lastPinnedIdx = i
		}
	}
	targetIdx := lastPinnedIdx + 1
	if targetIdx < idx {
		m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
		m.tabs = append(m.tabs, nil)
		copy(m.tabs[targetIdx+1:], m.tabs[targetIdx:])
		m.tabs[targetIdx] = tab
		for i, t := range m.tabs {
			if t.ID() == id {
				m.activeIdx = i
				break
			}
		}
	}
	m.mu.Unlock()
	m.fireChange()
	return true
}

func (m *TabManager) UnpinTab(id uint64) bool {
	m.mu.Lock()
	for _, t := range m.tabs {
		if t.ID() == id {
			t.SetPinned(false)
			m.mu.Unlock()
			m.fireChange()
			return true
		}
	}
	m.mu.Unlock()
	return false
}

func (m *TabManager) TogglePin(id uint64) bool {
	m.mu.Lock()
	for _, t := range m.tabs {
		if t.ID() == id {
			pinned := !t.Pinned()
			m.mu.Unlock()
			if pinned {
				return m.PinTab(id)
			}
			return m.UnpinTab(id)
		}
	}
	m.mu.Unlock()
	return false
}

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
		tab.SetHistory(rec.History.Clone())
	}
	tab.SetScrollY(rec.ScrollY)

	m.tabs = append(m.tabs, tab)
	m.activeIdx = len(m.tabs) - 1
	m.mu.Unlock()
	m.fireChange()
	return tab, true
}

func (m *TabManager) CanReopenTab() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.closedTabs) > 0
}

func (m *TabManager) ClosedTabCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.closedTabs)
}

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

func (m *TabManager) TabScroll() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tabScroll
}

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

func (m *TabManager) StartDrag(id uint64, x int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dragID, m.dragX, m.dragOn = id, x, true
}

func (m *TabManager) MoveDrag(x int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dragOn {
		m.dragX = x
	}
}

func (m *TabManager) Dragging() (id uint64, x int32, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dragID, m.dragX, m.dragOn
}

func (m *TabManager) EndDrag() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dragOn = false
}
