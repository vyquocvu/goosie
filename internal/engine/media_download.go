package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const MaxMediaBytes = 16 << 20

type MediaFetcher func(ctx context.Context, url string) (MediaResponse, error)

type MediaResponse struct {
	StatusCode int
	Body       []byte
	URL        string
}

var errMediaRefused = errors.New("media: source refused")

type mediaDownloader struct {
	fetch MediaFetcher
	sub   *subBudget

	mu        sync.Mutex
	dir       string
	files     int
	downloads int
	closed    bool
}

const maxConcurrentMediaDownloads = 2

func (d *mediaDownloader) Load(ctx context.Context, base, href string) (MediaLease, error) {
	if d == nil {
		return nil, errors.New("media: no loader")
	}
	abs, ok := resolveSubresourceURL(base, href)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not a source this document may fetch", errMediaRefused, href)
	}
	if !d.acquireDownload() {
		return nil, fmt.Errorf("%w: too many media downloads are already in flight", errMediaRefused)
	}
	release := true
	defer func() {
		if release {
			d.releaseDownload()
		}
	}()

	resp, err := d.fetch(ctx, abs)
	if err != nil {
		return nil, err
	}
	final := resp.URL
	if final == "" {
		final = abs
	}
	if err := admitMediaResponse(resp, final, base); err != nil {
		return nil, err
	}
	if d.sub != nil && !d.sub.take(len(resp.Body)) {
		return nil, fmt.Errorf("%w: the document's subresource budget is exhausted", errMediaRefused)
	}
	path, err := d.writeFile(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body = nil
	release = false
	return &mediaFileLease{d: d, path: path}, nil
}

func admitMediaResponse(resp MediaResponse, final, base string) error {
	if resp.StatusCode != 200 {
		return fmt.Errorf("%w: the source answered %d", errMediaRefused, resp.StatusCode)
	}
	if len(resp.Body) == 0 {
		return fmt.Errorf("%w: the source answered with no bytes", errMediaRefused)
	}
	if len(resp.Body) > MaxMediaBytes {
		return fmt.Errorf("%w: the source is %d bytes, over the %d limit", errMediaRefused, len(resp.Body), MaxMediaBytes)
	}
	if !looksLikeISOBaseMedia(resp.Body) {
		return fmt.Errorf("%w: the source is not an MP4 file", errMediaRefused)
	}
	if _, ok := resolveSubresourceURL(base, final); !ok {
		return fmt.Errorf("%w: the source redirected to %s, which this document may not fetch", errMediaRefused, final)
	}
	return nil
}

var isoBaseMediaBoxes = [...]string{"ftyp", "styp", "moov", "mdat", "free", "wide", "skip"}

func looksLikeISOBaseMedia(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	size := uint64(binary.BigEndian.Uint32(data[0:4]))
	if size == 1 {
		if len(data) < 16 {
			return false
		}
		size = binary.BigEndian.Uint64(data[8:16])
	}
	if size < 8 || size > uint64(len(data)) {
		return false
	}
	kind := string(data[4:8])
	for _, box := range isoBaseMediaBoxes {
		if kind == box {
			return true
		}
	}
	return false
}

func (d *mediaDownloader) writeFile(data []byte) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return "", errors.New("media: the document is gone")
	}
	if d.dir == "" {
		dir, err := os.MkdirTemp("", "goosie-media-")
		if err != nil {
			return "", err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			_ = os.Remove(dir)
			return "", err
		}
		d.dir = dir
	}
	path := filepath.Join(d.dir, fmt.Sprintf("media-%d.mp4", d.nextFileLocked()))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (d *mediaDownloader) nextFileLocked() int {
	d.files++
	return d.files
}

func (d *mediaDownloader) acquireDownload() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.downloads >= maxConcurrentMediaDownloads {
		return false
	}
	d.downloads++
	return true
}

func (d *mediaDownloader) releaseDownload() {
	d.mu.Lock()
	if d.downloads > 0 {
		d.downloads--
	}
	d.mu.Unlock()
}

func (d *mediaDownloader) removeAll() {
	d.mu.Lock()
	dir := d.dir
	d.dir = ""
	d.closed = true
	d.mu.Unlock()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

type mediaFileLease struct {
	d    *mediaDownloader
	path string
	once sync.Once
}

func (l *mediaFileLease) Path() string { return l.path }

func (l *mediaFileLease) Release() {
	l.once.Do(func() {
		_ = os.Remove(l.path)
		l.d.releaseDownload()
	})
}
