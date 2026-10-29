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

// checkoutReusable reports whether dir already holds the pinned checkout: a
// marker matching commit plus content behind it. A marker with nothing else
// in the directory (interrupted download, external cleanup) is not reusable:
// reusing it runs an empty suite that reports zero tests as a pass. Pure
// filesystem so unit tests never touch the network.
func checkoutReusable(dir, commit string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".goosie-wpt-commit"))
	if err != nil || strings.TrimSpace(string(data)) != commit {
		return false
	}
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 1
}

// NewCheckout creates a Checkout for the given configuration.
func NewCheckout(cfg Config) *Checkout {
	return &Checkout{cfg: cfg}
}

// Ensure downloads the WPT repository if it is not already present at the
// expected commit. Returns the path to the checkout directory.
func (c *Checkout) Ensure() (string, error) {
	dir := c.cfg.WPTCheckoutDir()

	// If the directory holds the pinned checkout, reuse it (see
	// checkoutReusable). Anything else falls through to a fresh download.
	if checkoutReusable(dir, c.cfg.Commit) {
		return dir, nil
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
	if err := os.WriteFile(filepath.Join(dir, ".goosie-wpt-commit"), []byte(c.cfg.Commit+"\n"), 0o644); err != nil {
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
	if err := extractWPTTarball(resp.Body, destDir); err != nil {
		return fmt.Errorf("wpt: extract %s: %w", url, err)
	}
	fmt.Fprintf(os.Stderr, "wpt: extracted to %s\n", destDir)
	return nil
}

func extractWPTTarball(source io.Reader, destDir string) error {
	gz, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	// GitHub tarballs have a top-level directory like "wpt-<commit>/".
	// We strip it so files land directly in destDir.
	var prefix string
	extractedFiles := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		// A GitHub archive can start with a PAX global header whose name has
		// no path separator. It is metadata, not the archive's root prefix.
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}

		// Determine the prefix from the first path-bearing entry rather than
		// the first tar header, which may be PAX metadata.
		if prefix == "" {
			parts := strings.SplitN(header.Name, "/", 2)
			if len(parts) < 2 {
				continue
			}
			prefix = parts[0] + "/"
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

		// Verify the target stays within destDir. filepath.Rel avoids the
		// sibling-prefix escape that a plain strings.HasPrefix check permits.
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("resolve archive path %q: %w", header.Name, err)
		}
		absDest, err := filepath.Abs(destDir)
		if err != nil {
			return fmt.Errorf("resolve destination %q: %w", destDir, err)
		}
		relative, err := filepath.Rel(absDest, absTarget)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("archive path %q escapes extraction directory", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("wpt: mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			fallthrough
		case tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
			}
			f, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("write %s: %w", target, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("close %s: %w", target, err)
			}
			extractedFiles++
		}
	}
	if extractedFiles == 0 {
		return fmt.Errorf("archive contained no regular files")
	}
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
