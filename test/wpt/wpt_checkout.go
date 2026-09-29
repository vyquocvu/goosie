package wpt

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Checkout downloads or verifies a WPT repository checkout at the pinned
// commit. It uses the GitHub tarball API to fetch a specific commit without
// requiring git to be installed.
type Checkout struct {
	cfg Config
}

// NewCheckout creates a Checkout for the given configuration.
func NewCheckout(cfg Config) *Checkout {
	return &Checkout{cfg: cfg}
}

// Ensure downloads the WPT repository if it is not already present at the
// expected commit. Returns the path to the checkout directory.
func (c *Checkout) Ensure() (string, error) {
	dir := c.cfg.WPTCheckoutDir()

	// If the directory exists and has a commit marker matching our pin, reuse it.
	markerPath := filepath.Join(dir, ".goosie-wpt-commit")
	if data, err := os.ReadFile(markerPath); err == nil {
		if strings.TrimSpace(string(data)) == c.cfg.Commit {
			return dir, nil
		}
	}

	// Clean up any existing checkout.
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("wpt: clean old checkout: %w", err)
	}

	// Download the tarball.
	if err := c.download(dir); err != nil {
		return "", err
	}

	// Write the commit marker.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("wpt: create checkout dir: %w", err)
	}
	if err := os.WriteFile(markerPath, []byte(c.cfg.Commit+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("wpt: write commit marker: %w", err)
	}

	return dir, nil
}

// download fetches the WPT tarball from GitHub and extracts it.
func (c *Checkout) download(destDir string) error {
	url := fmt.Sprintf("%s/archive/%s.tar.gz", WPTRepo, c.cfg.Commit)
	fmt.Fprintf(os.Stderr, "wpt: downloading %s...\n", url)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("wpt: download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wpt: download %s: HTTP %d", url, resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("wpt: gunzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	// GitHub tarballs have a top-level directory like "wpt-<commit>/".
	// We strip it so files land directly in destDir.
	var prefix string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("wpt: tar read: %w", err)
		}

		// Determine the prefix from the first entry.
		if prefix == "" {
			parts := strings.SplitN(header.Name, "/", 2)
			if len(parts) > 0 {
				prefix = parts[0] + "/"
			}
		}

		// Strip the top-level directory.
		name := header.Name
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		relPath := strings.TrimPrefix(name, prefix)
		if relPath == "" {
			continue
		}

		target := filepath.Join(destDir, relPath)

		// Verify the target stays within destDir.
		absTarget, err := filepath.Abs(target)
		if err != nil {
			continue
		}
		absDest, err := filepath.Abs(destDir)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(absTarget, absDest) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("wpt: mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("wpt: mkdir %s: %w", filepath.Dir(target), err)
			}
			f, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("wpt: create %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("wpt: write %s: %w", target, err)
			}
			f.Close()
		}
	}

	fmt.Fprintf(os.Stderr, "wpt: extracted to %s\n", destDir)
	return nil
}

// MinimalCheckout creates a minimal WPT-like directory with just the resources
// needed for the curated tests. This is used when the full WPT download is not
// available and we want to run against locally-authored test pages that follow
// the WPT format.
func MinimalCheckout(destDir string, testFiles map[string]string) error {
	for relPath, content := range testFiles {
		fullPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
