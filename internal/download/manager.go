package download

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ItemStatus distinguishes the lifecycle phases of a download.
type ItemStatus string

const (
	StatusQueued    ItemStatus = "queued"
	StatusActive    ItemStatus = "active"
	StatusCompleted ItemStatus = "completed"
	StatusFailed    ItemStatus = "failed"
	StatusCanceled  ItemStatus = "canceled"
)

// Item tracks an in-flight or completed download.
type Item struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	Filename        string     `json:"filename"`
	LocalPath       string     `json:"local_path"`
	TotalBytes      int64      `json:"total_bytes"`
	DownloadedBytes int64      `json:"downloaded_bytes"`
	Status          ItemStatus `json:"status"`
	Error           string     `json:"error,omitempty"`
	StartTime       time.Time  `json:"start_time"`
	EndTime         time.Time  `json:"end_time,omitempty"`
}

// Percent reports progress from 0.0 to 100.0, or -1.0 if total size is unknown.
func (it *Item) Percent() float64 {
	if it.TotalBytes <= 0 {
		if it.Status == StatusCompleted {
			return 100.0
		}
		return -1.0
	}
	p := float64(it.DownloadedBytes) / float64(it.TotalBytes) * 100.0
	if p > 100.0 {
		return 100.0
	}
	return p
}

// Speed calculates instantaneous transfer rate in bytes per second.
func (it *Item) Speed() float64 {
	if it.Status != StatusActive {
		return 0
	}
	elapsed := time.Since(it.StartTime).Seconds()
	if elapsed <= 0.01 {
		return 0
	}
	return float64(it.DownloadedBytes) / elapsed
}

// FormattedSize returns a human-readable size string (e.g. "12.4 MB / 50.0 MB" or "12.4 MB").
func (it *Item) FormattedSize() string {
	if it.TotalBytes > 0 {
		return fmt.Sprintf("%s / %s", FormatBytes(it.DownloadedBytes), FormatBytes(it.TotalBytes))
	}
	return FormatBytes(it.DownloadedBytes)
}

// FormattedSpeed returns formatted transfer rate (e.g. "2.4 MB/s").
func (it *Item) FormattedSpeed() string {
	return FormatSpeed(it.Speed())
}

// FormatBytes converts raw bytes into a human-readable unit string.
func FormatBytes(b int64) string {
	if b < 0 {
		return "Unknown"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatSpeed converts bytes per second into human-readable speed.
func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return "--"
	}
	return FormatBytes(int64(bytesPerSec)) + "/s"
}

// Manager orchestrates download items, progress updates, and OS interactions.
type Manager struct {
	mu        sync.RWMutex
	dir       string
	items     []*Item
	byID      map[string]*Item
	seq       int64
	listeners []func(*Item)
}

// NewManager creates a download manager saving by default to dir.
func NewManager(dir string) *Manager {
	return &Manager{
		dir:  dir,
		byID: make(map[string]*Item),
	}
}

// Dir returns the target download directory.
func (m *Manager) Dir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dir
}

// SetDir updates the target download directory.
func (m *Manager) SetDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dir = dir
}

// CreateItem registers a new download item.
func (m *Manager) CreateItem(rawURL, filename string, totalBytes int64) *Item {
	m.mu.Lock()
	defer m.mu.Unlock()

	idNum := atomic.AddInt64(&m.seq, 1)
	id := fmt.Sprintf("dl-%d", idNum)
	item := &Item{
		ID:         id,
		URL:        rawURL,
		Filename:   filename,
		TotalBytes: totalBytes,
		Status:     StatusActive,
		StartTime:  time.Now(),
	}
	m.items = append(m.items, item)
	m.byID[id] = item
	m.notify(item)
	return item
}

// UpdateProgress updates the downloaded byte count for an active item.
func (m *Manager) UpdateProgress(id string, downloaded int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.byID[id]
	if !ok || item.Status != StatusActive {
		return
	}
	item.DownloadedBytes = downloaded
	m.notify(item)
}

// Complete marks an item as successfully completed.
func (m *Manager) Complete(id string, localPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.byID[id]
	if !ok {
		return
	}
	item.LocalPath = localPath
	item.Status = StatusCompleted
	item.EndTime = time.Now()
	if item.TotalBytes > 0 {
		item.DownloadedBytes = item.TotalBytes
	}
	m.notify(item)
}

// Fail marks an item as failed with an error message.
func (m *Manager) Fail(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.byID[id]
	if !ok {
		return
	}
	item.Status = StatusFailed
	item.EndTime = time.Now()
	if err != nil {
		item.Error = err.Error()
	}
	m.notify(item)
}

// Cancel terminates and marks an item as canceled.
func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.byID[id]
	if !ok {
		return
	}
	if item.Status == StatusActive || item.Status == StatusQueued {
		item.Status = StatusCanceled
		item.EndTime = time.Now()
		m.notify(item)
	}
}

// Get finds an item by ID.
func (m *Manager) Get(id string) *Item {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if item, ok := m.byID[id]; ok {
		cp := *item
		return &cp
	}
	return nil
}

// Items returns a chronological slice copy of all tracked items.
func (m *Manager) Items() []*Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]*Item, len(m.items))
	for i, it := range m.items {
		cp := *it
		res[i] = &cp
	}
	return res
}

// ActiveItems returns all currently downloading or queued items.
func (m *Manager) ActiveItems() []*Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var res []*Item
	for _, it := range m.items {
		if it.Status == StatusActive || it.Status == StatusQueued {
			cp := *it
			res = append(res, &cp)
		}
	}
	return res
}

// ActiveCount returns the number of in-flight downloads.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, it := range m.items {
		if it.Status == StatusActive || it.Status == StatusQueued {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of items tracked.
func (m *Manager) TotalCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// OverallProgress calculates the combined 0.0..100.0 progress across active downloads.
func (m *Manager) OverallProgress() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total, downloaded int64
	active := 0
	for _, it := range m.items {
		if it.Status == StatusActive && it.TotalBytes > 0 {
			active++
			total += it.TotalBytes
			downloaded += it.DownloadedBytes
		}
	}
	if active == 0 || total <= 0 {
		return 0
	}
	p := float64(downloaded) / float64(total) * 100.0
	if p > 100.0 {
		return 100.0
	}
	return p
}

// ClearCompleted purges completed, failed, and canceled downloads from memory.
func (m *Manager) ClearCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var retained []*Item
	newByID := make(map[string]*Item)
	for _, it := range m.items {
		if it.Status == StatusActive || it.Status == StatusQueued {
			retained = append(retained, it)
			newByID[it.ID] = it
		}
	}
	m.items = retained
	m.byID = newByID
}

// Subscribe adds an observer notified whenever any item state changes.
func (m *Manager) Subscribe(fn func(*Item)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, l := range m.listeners {
			// Compare address of function isn't trivial in Go, so we can clean if needed
			_ = i
			_ = l
		}
	}
}

func (m *Manager) notify(item *Item) {
	for _, l := range m.listeners {
		if l != nil {
			cp := *item
			go l(&cp)
		}
	}
}

// ShowInFolder reveals the file at path in the native file manager (Finder / Explorer / Files).
func ShowInFolder(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", abs).Start()
	case "windows":
		return exec.Command("explorer", "/select,", abs).Start()
	default:
		dir := filepath.Dir(abs)
		return exec.Command("xdg-open", dir).Start()
	}
}

// OpenFile launches the downloaded file with its system default associated application.
func OpenFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", abs).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}
