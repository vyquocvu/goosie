// Package preferences stores user-facing browser preferences for one profile.
package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const DefaultSearchURLTemplate = "https://www.google.com/search?q={query}"

const (
	StartupRestore = "restore"
	StartupNewTab  = "newtab"
)

// Settings are shared by normal tabs in one profile. Private sessions may read
// them, but callers must not persist changes made from a private window.
type Settings struct {
	SearchURLTemplate   string `json:"searchURLTemplate"`
	DownloadDir         string `json:"downloadDir,omitempty"`
	StartupMode         string `json:"startupMode"`
	AskDownloadLocation bool   `json:"askDownloadLocation,omitempty"`
}

func Defaults() Settings {
	return Settings{SearchURLTemplate: DefaultSearchURLTemplate, StartupMode: StartupRestore}
}

func ValidateStartupMode(mode string) error {
	if mode != StartupRestore && mode != StartupNewTab {
		return errors.New("startup mode must be restore or newtab")
	}
	return nil
}

// ValidateSearchURLTemplate accepts one {query} marker in an HTTP(S) URL.
// The marker is replaced with a harmless value before parsing so that URL
// syntax and the destination origin are checked independently of the query.
func ValidateSearchURLTemplate(template string) error {
	if strings.Count(template, "{query}") != 1 {
		return errors.New("search URL must contain exactly one {query} marker")
	}
	marker := strings.Index(template, "{query}")
	queryStart := strings.IndexByte(template, '?')
	fragmentStart := strings.IndexByte(template, '#')
	if queryStart < 0 || marker < queryStart || (fragmentStart >= 0 && marker > fragmentStart) {
		return errors.New("search URL must place {query} in its query string")
	}
	probe := strings.Replace(template, "{query}", "goosie-query", 1)
	u, err := url.Parse(probe)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return errors.New("search URL must be an HTTP or HTTPS URL without credentials")
	}
	return nil
}

func SearchURL(template, query string) (string, error) {
	if err := ValidateSearchURLTemplate(template); err != nil {
		return "", err
	}
	return strings.Replace(template, "{query}", url.QueryEscape(query), 1), nil
}

// ValidateDownloadDir requires an explicit absolute directory so a profile
// never changes where downloads land based on the process working directory.
func ValidateDownloadDir(dir string) error {
	if dir == "" || !filepath.IsAbs(dir) || strings.IndexByte(dir, 0) >= 0 || strings.ContainsAny(dir, "\r\n") {
		return errors.New("download directory must be an absolute path without control characters")
	}
	return nil
}

// Load reads the profile settings. Missing settings are initialized in memory
// with defaults; malformed or unreadable files are reported to the caller.
func Load(path string) (Settings, error) {
	s := Defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Defaults(), fmt.Errorf("decode preferences: %w", err)
	}
	if err := ValidateSearchURLTemplate(s.SearchURLTemplate); err != nil {
		return Defaults(), fmt.Errorf("invalid saved search URL: %w", err)
	}
	if err := ValidateStartupMode(s.StartupMode); err != nil {
		return Defaults(), fmt.Errorf("invalid saved startup mode: %w", err)
	}
	if s.DownloadDir != "" {
		if err := ValidateDownloadDir(s.DownloadDir); err != nil {
			return Defaults(), fmt.Errorf("invalid saved download directory: %w", err)
		}
	}
	return s, nil
}

// Save atomically replaces the profile settings file.
func Save(path string, s Settings) error {
	if err := ValidateSearchURLTemplate(s.SearchURLTemplate); err != nil {
		return err
	}
	if err := ValidateStartupMode(s.StartupMode); err != nil {
		return err
	}
	if s.DownloadDir != "" {
		if err := ValidateDownloadDir(s.DownloadDir); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
