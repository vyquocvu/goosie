package download

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ItemStatus string

const (
	StatusQueued    ItemStatus = "queued"
	StatusActive    ItemStatus = "active"
	StatusPaused    ItemStatus = "paused"
	StatusCompleted ItemStatus = "completed"
	StatusFailed    ItemStatus = "failed"
	StatusCanceled  ItemStatus = "canceled"
)

type Item struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	Filename        string     `json:"filename"`
	LocalPath       string     `json:"local_path"`
	TempPath        string     `json:"temp_path,omitempty"`
	SHA256          string     `json:"sha256,omitempty"`
	TotalBytes      int64      `json:"total_bytes"`
	DownloadedBytes int64      `json:"downloaded_bytes"`
	Status          ItemStatus `json:"status"`
	Error           string     `json:"error,omitempty"`
	StartTime       time.Time  `json:"start_time"`
	EndTime         time.Time  `json:"end_time,omitempty"`
}

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

func (it *Item) FormattedSize() string {
	if it.TotalBytes > 0 {
		return fmt.Sprintf("%s / %s", FormatBytes(it.DownloadedBytes), FormatBytes(it.TotalBytes))
	}
	return FormatBytes(it.DownloadedBytes)
}

func (it *Item) FormattedSpeed() string {
	return FormatSpeed(it.Speed())
}

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

func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return "--"
	}
	return FormatBytes(int64(bytesPerSec)) + "/s"
}

type FileLauncher interface {
	OpenFile(path string) error
	ShowInFolder(path string) error
}

type Manager struct {
	mu        sync.RWMutex
	dir       string
	items     []*Item
	byID      map[string]*Item
	seq       int64
	listeners []func(*Item)
	cancel    map[string]context.CancelFunc
	pause     map[string]context.CancelFunc
	resume    map[string]func()
	launcher  FileLauncher
}

func NewManager(dir string) *Manager {
	return &Manager{
		dir:    dir,
		byID:   make(map[string]*Item),
		cancel: make(map[string]context.CancelFunc),
		pause:  make(map[string]context.CancelFunc),
		resume: make(map[string]func()),
	}
}

func (m *Manager) Dir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dir
}

func (m *Manager) SetDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dir = dir
}

func (m *Manager) SetLauncher(launcher FileLauncher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.launcher = launcher
}

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

func (m *Manager) Complete(id string, localPath string) {
	m.CompleteWithDigest(id, localPath, "")
}

func (m *Manager) CompleteWithDigest(id, localPath, sha256 string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.byID[id]
	if !ok {
		return
	}
	if item.Status == StatusCanceled || item.Status == StatusFailed {
		return
	}
	item.LocalPath = localPath
	item.TempPath = ""
	item.SHA256 = sha256
	item.Status = StatusCompleted
	item.EndTime = time.Now()
	delete(m.cancel, id)
	delete(m.pause, id)
	delete(m.resume, id)
	if item.TotalBytes > 0 {
		item.DownloadedBytes = item.TotalBytes
	}
	m.notify(item)
}

func (m *Manager) Fail(id string, err error) {
	m.mu.Lock()
	item, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if item.Status == StatusCanceled || item.Status == StatusCompleted {
		m.mu.Unlock()
		return
	}
	tempPath := item.TempPath
	item.TempPath = ""
	item.Status = StatusFailed
	item.EndTime = time.Now()
	delete(m.cancel, id)
	delete(m.pause, id)
	delete(m.resume, id)
	if err != nil {
		item.Error = err.Error()
	}
	m.notify(item)
	m.mu.Unlock()
	if tempPath != "" {
		_ = os.Remove(tempPath)
	}
}

func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	item, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if item.Status == StatusActive || item.Status == StatusQueued || item.Status == StatusPaused {
		tempPath := item.TempPath
		item.TempPath = ""
		item.Status = StatusCanceled
		item.EndTime = time.Now()
		cancel := m.cancel[id]
		delete(m.cancel, id)
		delete(m.pause, id)
		delete(m.resume, id)
		m.notify(item)
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
		return
	}
	m.mu.Unlock()
}

func (m *Manager) SetCancel(id string, cancel context.CancelFunc) {
	if cancel == nil {
		return
	}
	m.mu.Lock()
	item, ok := m.byID[id]
	if !ok || item.Status == StatusCanceled || item.Status == StatusCompleted || item.Status == StatusFailed {
		m.mu.Unlock()
		cancel()
		return
	}
	if m.cancel == nil {
		m.cancel = make(map[string]context.CancelFunc)
	}
	m.cancel[id] = cancel
	m.mu.Unlock()
}

func (m *Manager) SetPause(id string, pause context.CancelFunc) {
	if pause == nil {
		return
	}
	m.mu.Lock()
	item, ok := m.byID[id]
	if !ok || (item.Status != StatusActive && item.Status != StatusQueued) {
		m.mu.Unlock()
		pause()
		return
	}
	m.pause[id] = pause
	m.mu.Unlock()
}

func (m *Manager) Pause(id string) bool {
	m.mu.Lock()
	item, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return false
	}
	pause, pausable := m.pause[id]
	if !pausable || item.Status != StatusActive && item.Status != StatusQueued {
		m.mu.Unlock()
		return false
	}
	item.Status = StatusPaused
	item.EndTime = time.Time{}
	delete(m.cancel, id)
	delete(m.pause, id)
	m.notify(item)
	m.mu.Unlock()
	if pause != nil {
		pause()
	}
	return true
}

func (m *Manager) SetResume(id string, resume func()) {
	if resume == nil {
		return
	}
	m.mu.Lock()
	item, ok := m.byID[id]
	if ok && item.Status != StatusCanceled && item.Status != StatusCompleted && item.Status != StatusFailed {
		m.resume[id] = resume
	}
	m.mu.Unlock()
}

func (m *Manager) Resume(id string) bool {
	m.mu.Lock()
	item, ok := m.byID[id]
	if !ok || item.Status != StatusPaused {
		m.mu.Unlock()
		return false
	}
	resume, ok := m.resume[id]
	if !ok || resume == nil {
		m.mu.Unlock()
		return false
	}
	item.Status = StatusActive
	item.EndTime = time.Time{}
	item.StartTime = time.Now()
	delete(m.resume, id)
	m.notify(item)
	m.mu.Unlock()
	go resume()
	return true
}

func (m *Manager) SetTemp(id, path string, downloaded int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.byID[id]
	if !ok || item.Status == StatusCanceled || item.Status == StatusCompleted || item.Status == StatusFailed {
		return
	}
	item.TempPath = path
	item.DownloadedBytes = downloaded
	m.notify(item)
}

func (m *Manager) Restart(id string, totalBytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.byID[id]
	if !ok || item.Status == StatusCanceled || item.Status == StatusCompleted || item.Status == StatusFailed {
		return
	}
	item.TempPath = ""
	item.DownloadedBytes = 0
	item.TotalBytes = totalBytes
	item.StartTime = time.Now()
	m.notify(item)
}

func (m *Manager) Pausable(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.pause[id]
	return ok
}

func (m *Manager) Resumable(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.byID[id]
	_, hasResume := m.resume[id]
	return ok && item.Status == StatusPaused && hasResume
}

func (m *Manager) Get(id string) *Item {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if item, ok := m.byID[id]; ok {
		cp := *item
		return &cp
	}
	return nil
}

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

func (m *Manager) ActiveItems() []*Item {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var res []*Item
	for _, it := range m.items {
		if it.Status == StatusActive || it.Status == StatusQueued || it.Status == StatusPaused {
			cp := *it
			res = append(res, &cp)
		}
	}
	return res
}

func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, it := range m.items {
		if it.Status == StatusActive || it.Status == StatusQueued || it.Status == StatusPaused {
			count++
		}
	}
	return count
}

func (m *Manager) TotalCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

func (m *Manager) OverallProgress() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total, downloaded int64
	active := 0
	for _, it := range m.items {
		if (it.Status == StatusActive || it.Status == StatusPaused) && it.TotalBytes > 0 {
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

func (m *Manager) ClearCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var retained []*Item
	newByID := make(map[string]*Item)
	for _, it := range m.items {
		if it.Status == StatusActive || it.Status == StatusQueued || it.Status == StatusPaused {
			retained = append(retained, it)
			newByID[it.ID] = it
		}
	}
	m.items = retained
	m.byID = newByID
}

func (m *Manager) Subscribe(fn func(*Item)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, l := range m.listeners {
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

type systemFileLauncher struct{}

func (systemFileLauncher) ShowInFolder(path string) error {
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

func (systemFileLauncher) OpenFile(path string) error {
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

func resolveManagedFilePath(dir, path string) (string, error) {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("download path is not configured")
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve download directory: %w", err)
	}
	dirResolved, err := filepath.EvalSymlinks(dirAbs)
	if err != nil {
		return "", fmt.Errorf("resolve download directory: %w", err)
	}
	dirInfo, err := os.Stat(dirResolved)
	if err != nil {
		return "", fmt.Errorf("inspect download directory: %w", err)
	}
	if !dirInfo.IsDir() {
		return "", fmt.Errorf("download directory is not a directory")
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dirAbs, candidate)
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve download path: %w", err)
	}
	candidateResolved, err := filepath.EvalSymlinks(candidateAbs)
	if err != nil {
		return "", fmt.Errorf("resolve download path: %w", err)
	}
	rel, err := filepath.Rel(dirResolved, candidateResolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("download path is outside the configured directory")
	}
	info, err := os.Stat(candidateResolved)
	if err != nil {
		return "", fmt.Errorf("inspect download path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("download path is not a regular file")
	}
	return candidateResolved, nil
}

func (m *Manager) launchManagedFile(path string, reveal bool) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resolved, err := resolveManagedFilePath(m.dir, path)
	if err != nil {
		return err
	}
	launcher := m.launcher
	if launcher == nil {
		launcher = systemFileLauncher{}
	}
	if reveal {
		return launcher.ShowInFolder(resolved)
	}
	return launcher.OpenFile(resolved)
}

func (m *Manager) OpenFile(path string) error {
	return m.launchManagedFile(path, false)
}

func (m *Manager) ShowInFolder(path string) error {
	return m.launchManagedFile(path, true)
}
