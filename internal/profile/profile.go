package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Options struct {
	Root    string
	Private bool
}

type Profile struct {
	root    string
	private bool
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

	return &Profile{
		root:    root,
		private: options.Private,
	}, nil
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

func (p *Profile) LoadJSON(name string, target any) error {
	path := filepath.Join(p.root, name)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	decodeErr := json.NewDecoder(file).Decode(target)
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

	if err := os.MkdirAll(p.root, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

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
