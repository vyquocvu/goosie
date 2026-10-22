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

// MaxMediaBytes bounds one media source. It is the same 16 MiB the browser's
// transport already enforces rather than a second opinion about it: a file that
// could not arrive through the HTTP client should not arrive through a host that
// supplies its own fetcher either. The value is repeated here because the engine
// may not import internal/net, and the alternative — no cap at all on the
// injected path — is worse than a constant that has to be kept in step.
const MaxMediaBytes = 16 << 20

// MediaFetcher fetches one media source completely. It is the browser's HTTP
// client seen from the engine: the host owns the transport, the cookies, the
// redirect limit and the dial-time address guard, and the engine owns the URL
// policy, the size caps and the file the bytes land in.
//
// The fetcher must return the whole body or an error. A media element in this
// engine does not stream: playback starts when the file is complete, which is
// what lets a native decoder be handed a local path instead of a URL it would
// fetch behind every policy the browser has.
type MediaFetcher func(ctx context.Context, url string) (MediaResponse, error)

// MediaResponse is one media fetch's result.
type MediaResponse struct {
	StatusCode int
	Body       []byte
	// URL is the final URL after redirects. It is re-checked against the
	// subresource policy, because a redirect can land somewhere the original
	// href was not allowed to go. Empty means "the URL that was asked for".
	URL string
}

// errMediaRefused marks a source the policy or the container check turned down,
// as opposed to one the network failed to deliver. The distinction is what a
// page sees: a refused source is MEDIA_ERR_SRC_NOT_SUPPORTED and a failed fetch
// is MEDIA_ERR_NETWORK, and telling a developer to check their network about a
// source the browser will never load sends them looking in the wrong place.
var errMediaRefused = errors.New("media: source refused")

// mediaDownloader turns a source URL into a local file a backend can open. One
// per session, so its counters and its temporary directory are per-document.
type mediaDownloader struct {
	fetch MediaFetcher
	sub   *subBudget

	mu        sync.Mutex
	dir       string
	files     int
	downloads int
	closed    bool
}

// maxConcurrentMediaDownloads bounds how many media files one document may be
// fetching at once. Without it a page that names a hundred sources and calls
// load() on each would hold a hundred sockets and a hundred files open; with it
// the extra requests fail fast instead of queueing behind the display.
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
	// The bytes are on disk; the response body is not needed again, and a
	// 16 MiB slice held for the life of a playback is 16 MiB held for nothing.
	resp.Body = nil
	// Ownership of the download slot passes to the lease: it is released when
	// the file is, not here.
	release = false
	return &mediaFileLease{d: d, path: path}, nil
}

// admitMediaResponse is the whole admission rule for a media body. It is
// deliberately about the bytes and the URL rather than about the container's
// internals: the backend parses the boxes anyway, and a second parser here would
// be a second place to be wrong.
func admitMediaResponse(resp MediaResponse, final, base string) error {
	// A partial answer is not a playable file. Range support is deferred, so
	// anything other than a complete 200 is refused rather than truncated.
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
	// The redirect target is judged by the same rule as the href, so a public
	// page cannot be sent to a private address by a server it did not choose.
	if _, ok := resolveSubresourceURL(base, final); !ok {
		return fmt.Errorf("%w: the source redirected to %s, which this document may not fetch", errMediaRefused, final)
	}
	return nil
}

// isoBaseMediaBoxes are the top-level box types a self-contained MP4 may begin
// with. ftyp is the ordinary one; the rest turn up in files with a padding box
// first or with the brand box stripped by a transcoder.
var isoBaseMediaBoxes = [...]string{"ftyp", "styp", "moov", "mdat", "free", "wide", "skip"}

// looksLikeISOBaseMedia reports whether data starts with a plausible ISO base
// media box. Its job is to catch the common failure — a server answering a media
// request with an HTML error page — with a reason a person can act on, rather
// than handing that page to a decoder and reporting "unsupported".
func looksLikeISOBaseMedia(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	size := uint64(binary.BigEndian.Uint32(data[0:4]))
	if size == 1 {
		// A 64-bit extended size: the box header is 16 bytes and the real
		// length follows the type.
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

// writeFile puts the bytes somewhere only this process can read and returns the
// path. It is a file rather than a buffer because the decoder is native code:
// the file is the handoff, and it is also what keeps a 16 MiB body out of the Go
// heap for the whole time a video is paused.
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
		// MkdirTemp creates 0700 already; the chmod is here so the directory's
		// permissions do not depend on that staying true.
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
	// Closed before the path is handed on: a decoder that opens a file this
	// process is still writing would read a partial asset.
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

// removeAll drops the whole temporary directory. It runs when the session
// closes, which is after every lease has been released or abandoned; unlinking
// a file a decoder still has open is harmless on Unix, and the directory itself
// goes only once nothing can create another file in it.
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

// mediaFileLease is ownership of one downloaded media file.
type mediaFileLease struct {
	d    *mediaDownloader
	path string
	once sync.Once
}

func (l *mediaFileLease) Path() string { return l.path }

// Release removes the file and gives the download slot back. It is safe to call
// more than once, because the controller calls it from several paths that can
// all be reached for one file — a superseded source, a close, a session going
// away — and removing a file twice must not remove someone else's.
func (l *mediaFileLease) Release() {
	l.once.Do(func() {
		_ = os.Remove(l.path)
		l.d.releaseDownload()
	})
}
