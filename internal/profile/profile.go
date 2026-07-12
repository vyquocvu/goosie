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
)

type Options struct {
	Root    string
	Private bool
}

type writeTask struct {
	name     string
	data     []byte
	syncOnly bool
	done     chan error
}

type Profile struct {
	root      string
	private   bool
	locksMu   sync.Mutex
	fileLocks map[string]*sync.Mutex

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
		root:      root,
		private:   options.Private,
		fileLocks: map[string]*sync.Mutex{},
		writeChan: make(chan writeTask, 1024),
		ctx:       ctx,
		cancel:    cancel,
	}

	p.wg.Add(1)
	go p.worker()

	return p, nil
}

func defaultRoot() string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return ".goosie"
	}

	return filepath.Join(configDir, "goosie")
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

func (p *Profile) worker() {
	defer p.wg.Done()
	for {
		select {
		case task, ok := <-p.writeChan:
			if !ok {
				return
			}
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
		case <-p.ctx.Done():
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
	if err := p.Sync(); err != nil {
		return err
	}

	path := filepath.Join(p.root, name)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(file)
	decodeErr := decoder.Decode(target)
	if decodeErr == nil {
		if err := decoder.Decode(new(struct{})); err == nil {
			decodeErr = fmt.Errorf("decode JSON %q trailing data: %w", name, errors.New("unexpected trailing JSON value"))
		} else if !errors.Is(err, io.EOF) {
			decodeErr = fmt.Errorf("decode JSON %q trailing data: %w", name, err)
		}
	}
	closeErr := file.Close()
	if decodeErr != nil {
		if closeErr != nil {
			return fmt.Errorf("decode JSON %q and close corrupt JSON before backup: %w", name, errors.Join(decodeErr, closeErr))
		}
		if backupErr := os.Rename(path, path+".corrupt"); backupErr != nil {
			return fmt.Errorf("decode JSON %q and back up corrupt JSON: %w", name, errors.Join(decodeErr, backupErr))
		}
		return fmt.Errorf("decode JSON %q: %w", name, decodeErr)
	}
	if closeErr != nil {
		return closeErr
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

	return os.Rename(tempPath, path)
}
