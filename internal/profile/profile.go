package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	Root    string
	Private bool
}

const CurrentSchemaVersion = 1

type SchemaConfig struct {
	Version int `json:"version"`
}

type writeTask struct {
	name     string
	data     []byte
	syncOnly bool
	done     chan error
}

type Profile struct {
	root       string
	private    bool
	locksMu    sync.Mutex
	fileLocks  map[string]*sync.Mutex
	writeCount int64 // atomic count of file writes on disk

	// In-memory snapshots of files for instant consistent reads
	snapshotsMu      sync.Mutex
	snapshots        map[string][]byte
	snapshotVersions map[string]uint64

	// Background writing
	writeChan chan writeTask
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

func Open(options Options) (*Profile, error) {
	root := options.Root
	if root == "" {
		root = defaultRoot()
	}

	if !options.Private {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Profile{
		root:             root,
		private:          options.Private,
		fileLocks:        map[string]*sync.Mutex{},
		snapshots:        map[string][]byte{},
		snapshotVersions: map[string]uint64{},
		writeChan:        make(chan writeTask, 1024),
		ctx:              ctx,
		cancel:           cancel,
	}

	if !options.Private {
		if err := p.runMigrations(); err != nil {
			cancel()
			return nil, err
		}
		p.wg.Add(1)
		go p.worker()
	}

	return p, nil
}

func (p *Profile) Root() string {
	return p.root
}

func (p *Profile) Private() bool {
	return p.private
}

func (p *Profile) Close() error {
	p.cancel()
	p.wg.Wait()
	return nil
}

func (p *Profile) WriteCount() int64 {
	return atomic.LoadInt64(&p.writeCount)
}

func (p *Profile) Sync() error {
	if p.private {
		return nil
	}

	select {
	case <-p.ctx.Done():
		return errors.New("profile is closed")
	default:
	}

	done := make(chan error, 1)
	task := writeTask{
		syncOnly: true,
		done:     done,
	}

	select {
	case p.writeChan <- task:
		return <-done
	case <-p.ctx.Done():
		return errors.New("profile is closed")
	}
}

func (p *Profile) SnapshotVersion(name string) uint64 {
	p.snapshotsMu.Lock()
	defer p.snapshotsMu.Unlock()
	return p.snapshotVersions[name]
}

func (p *Profile) worker() {
	defer p.wg.Done()

	type pendingItem struct {
		data []byte
		at   time.Time
		done chan error
	}
	pending := make(map[string]pendingItem)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	flushFile := func(name string, item pendingItem) {
		err := p.saveJSONBytes(name, item.data)
		if item.done != nil {
			item.done <- err
			close(item.done)
		}
		delete(pending, name)
	}

	flushAll := func() {
		for name, item := range pending {
			flushFile(name, item)
		}
	}

	for {
		select {
		case task, ok := <-p.writeChan:
			if !ok {
				flushAll()
				return
			}
			if task.syncOnly {
				flushAll()
				if task.done != nil {
					task.done <- nil
					close(task.done)
				}
				continue
			}

			// If there was a previous pending write, complete it silently
			if old, ok := pending[task.name]; ok {
				if old.done != nil {
					old.done <- nil
					close(old.done)
				}
			}

			pending[task.name] = pendingItem{
				data: task.data,
				at:   time.Now(),
				done: task.done,
			}

		case <-ticker.C:
			now := time.Now()
			for name, item := range pending {
				// Coalesce / delay writes by 200ms
				if now.Sub(item.at) >= 200*time.Millisecond {
					flushFile(name, item)
				}
			}

		case <-p.ctx.Done():
			flushAll()
			// Drain writeChan
			for {
				select {
				case task := <-p.writeChan:
					if task.syncOnly {
						if task.done != nil {
							task.done <- nil
							close(task.done)
						}
						continue
					}
					err := p.saveJSONBytes(task.name, task.data)
					if task.done != nil {
						task.done <- err
						close(task.done)
					}
				default:
					return
				}
			}
		}
	}
}

func (p *Profile) withFileLock(name string, fn func() error) error {
	lock := p.fileLock(name)
	lock.Lock()
	defer lock.Unlock()

	return fn()
}

func (p *Profile) fileLock(name string) *sync.Mutex {
	cleanName := filepath.Clean(name)

	p.locksMu.Lock()
	defer p.locksMu.Unlock()

	lock := p.fileLocks[cleanName]
	if lock == nil {
		lock = &sync.Mutex{}
		p.fileLocks[cleanName] = lock
	}

	return lock
}

func (p *Profile) LoadJSON(name string, target any) error {
	if p.private {
		return nil
	}

	// 1. Check in-memory snapshot cache first
	p.snapshotsMu.Lock()
	snapData, ok := p.snapshots[name]
	p.snapshotsMu.Unlock()

	if ok {
		return json.Unmarshal(snapData, target)
	}

	// 2. Load from disk
	path := filepath.Join(p.root, name)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		return err
	}
	closeErr := file.Close()

	decodeErr := json.Unmarshal(data, target)
	if decodeErr == nil {
		if closeErr != nil {
			return closeErr
		}
		// Populate snapshot cache
		p.snapshotsMu.Lock()
		p.snapshots[name] = data
		p.snapshotVersions[name] = 1
		p.snapshotsMu.Unlock()
		return nil
	}

	// If corrupt, handle backup
	if decodeErr != nil {
		if closeErr != nil {
			return fmt.Errorf("decode JSON %q and close corrupt JSON before backup: %w", name, errors.Join(decodeErr, closeErr))
		}
		if backupErr := os.Rename(path, path+".corrupt"); backupErr != nil {
			return fmt.Errorf("decode JSON %q and back up corrupt JSON: %w", name, errors.Join(decodeErr, backupErr))
		}
		return fmt.Errorf("decode JSON %q: %w", name, decodeErr)
	}

	return nil
}

func (p *Profile) SaveJSON(name string, value any) error {
	if p.private {
		return nil
	}

	select {
	case <-p.ctx.Done():
		return errors.New("profile is closed")
	default:
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Cache the snapshot in memory for instant consistent reads
	p.snapshotsMu.Lock()
	p.snapshots[name] = data
	p.snapshotVersions[name]++
	p.snapshotsMu.Unlock()

	task := writeTask{
		name: name,
		data: data,
		done: make(chan error, 1),
	}

	select {
	case p.writeChan <- task:
		return nil
	case <-p.ctx.Done():
		return errors.New("profile is closed")
	}
}

func (p *Profile) saveJSONBytes(name string, data []byte) error {
	if err := os.MkdirAll(p.root, 0o700); err != nil {
		return err
	}

	path := filepath.Join(p.root, name)
	tempFile, err := os.CreateTemp(p.root, "."+filepath.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	err = os.Rename(tempPath, path)
	if err == nil {
		atomic.AddInt64(&p.writeCount, 1)
	}
	return err
}

func defaultRoot() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", ".goosie")
	}
	return filepath.Join(dir, "goosie")
}

func (p *Profile) runMigrations() error {
	schemaPath := filepath.Join(p.root, "schema.json")
	schemaData, err := os.ReadFile(schemaPath)

	var currentVersion int
	if os.IsNotExist(err) {
		// schema.json does not exist.
		// Check if this is an existing legacy profile (version 0) or a new one.
		legacyExists := false
		files := []string{"bookmarks.json", "history.json", "settings.json", "session.json", "storage.json"}
		for _, file := range files {
			if _, err := os.Stat(filepath.Join(p.root, file)); err == nil {
				legacyExists = true
				break
			}
		}

		if legacyExists {
			currentVersion = 0
		} else {
			// Brand new profile directory, set to current version directly
			currentVersion = CurrentSchemaVersion
			if err := p.writeSchemaVersion(CurrentSchemaVersion); err != nil {
				return err
			}
		}
	} else if err != nil {
		return fmt.Errorf("read schema file: %w", err)
	} else {
		var cfg SchemaConfig
		if err := json.Unmarshal(schemaData, &cfg); err != nil {
			// If schema.json is corrupt, back it up and assume version 0
			_ = os.Rename(schemaPath, schemaPath+".corrupt")
			currentVersion = 0
		} else {
			currentVersion = cfg.Version
		}
	}

	for v := currentVersion; v < CurrentSchemaVersion; v++ {
		switch v {
		case 0:
			if err := p.migrate0to1(); err != nil {
				return fmt.Errorf("migration 0 to 1: %w", err)
			}
		}
	}

	if currentVersion < CurrentSchemaVersion {
		if err := p.writeSchemaVersion(CurrentSchemaVersion); err != nil {
			return err
		}
	}

	return nil
}

func (p *Profile) writeSchemaVersion(version int) error {
	cfg := SchemaConfig{Version: version}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return p.saveJSONBytes("schema.json", data)
}

func (p *Profile) migrate0to1() error {
	// Migration 0 to 1: Migrate bookmarks.json if it is in legacy string array format []string
	bookmarksPath := filepath.Join(p.root, "bookmarks.json")
	data, err := os.ReadFile(bookmarksPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// Try to unmarshal as legacy list of strings
	var legacy []string
	if err := json.Unmarshal(data, &legacy); err == nil {
		// Migrate to modern []Bookmark format
		now := time.Now().UTC()
		modern := make([]Bookmark, len(legacy))
		for i, url := range legacy {
			modern[i] = Bookmark{
				URL:       url,
				Title:     url,
				CreatedAt: now,
				UpdatedAt: now,
			}
		}

		newData, err := json.MarshalIndent(modern, "", "  ")
		if err != nil {
			return err
		}
		newData = append(newData, '\n')

		if err := p.saveJSONBytes("bookmarks.json", newData); err != nil {
			return err
		}
	}

	return nil
}
