// Package download decides whether a response must be saved instead of
// rendered and writes it under a collision-free name.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// Digest is the SHA-256 of a download's bytes. It is computed as the stream is
// written, so verification needs no second pass over the file.
type Digest [sha256.Size]byte

// String returns the lowercase hexadecimal form used in download metadata.
func (d Digest) String() string {
	return hex.EncodeToString(d[:])
}

// Sum returns the SHA-256 digest of an already-buffered response body.
func Sum(body []byte) Digest {
	return Digest(sha256.Sum256(body))
}

// ShouldDownload reports whether a main-document response must be saved to
// disk instead of rendered: an attachment disposition always downloads, and
// any content type the engine cannot render downloads too. An absent content
// type preserves the historical render behavior.
func ShouldDownload(contentType, disposition string) bool {
	if strings.Contains(strings.ToLower(disposition), "attachment") {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "text/html", "application/xhtml+xml", "text/plain":
		return false
	case "":
		return false
	default:
		return true
	}
}

// FileName picks a save name from the disposition's filename parameter, then
// the URL path, then a fallback, reduced to one safe path component.
func FileName(disposition, rawURL string) string {
	if name := sanitize(dispositionParam(disposition)); name != "" {
		return name
	}
	if u, err := url.Parse(rawURL); err == nil {
		base := path.Base(u.Path)
		if base != "" && base != "/" && base != "." {
			if name := sanitize(base); name != "" {
				return name
			}
		}
	}
	return "download"
}

// dispositionParam extracts filename="value" (or bare value) from a
// Content-Disposition header value.
func dispositionParam(disposition string) string {
	for _, part := range strings.Split(disposition, ";") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "filename="); ok {
			return strings.Trim(rest, `"`)
		}
	}
	return ""
}

// sanitize strips control characters and keeps only the last path segment.
// Both separators are treated as path boundaries regardless of host OS, and
// the only names that reduce to a directory are discarded rather than joined.
func sanitize(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	fields := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	if last == "." || last == ".." {
		return ""
	}
	return last
}

// Save writes body under dir with the given name, appending (1), (2), ... to
// the base when the name is taken. It returns the written path. The name is
// reduced to a single component here as well as in FileName: an exported
// writer must not be the one call that trusts its caller with a path.
func Save(dir, name string, body []byte) (string, error) {
	name = sanitize(name)
	if name == "" {
		name = "download"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("goosie: download dir: %w", err)
	}
	base, ext := name, filepath.Ext(name)
	if ext != "" {
		base = strings.TrimSuffix(name, ext)
	}
	full := filepath.Join(dir, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(full); os.IsNotExist(err) {
			break
		}
		if i > 999 {
			return "", fmt.Errorf("goosie: download: no free name for %q in %q", name, dir)
		}
		full = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
	return full, os.WriteFile(full, body, 0o644)
}

// ErrPaused reports that a transfer stopped because the user paused it. The
// partial file is preserved so the transfer can resume with a Range request.
var ErrPaused = errors.New("goosie: download paused")

// StreamResult describes where a streamed download landed. On success Path is
// the final file. When ErrPaused is returned TempPath names the preserved
// partial file and Bytes holds how much of it was written.
type StreamResult struct {
	Path     string
	TempPath string
	Bytes    int64
	Digest   Digest
}

// SaveStream writes a response incrementally to a temporary file, reporting
// byte progress as chunks arrive. The completed file is linked into place only
// after the stream succeeds, so cancellation and network errors leave no
// partial download visible in the destination directory.
func SaveStream(ctx context.Context, dir, name string, src io.Reader, progress func(downloaded int64)) (string, int64, Digest, error) {
	res, err := SaveStreamPausable(ctx, dir, name, src, progress)
	return res.Path, res.Bytes, res.Digest, err
}

// SaveStreamPausable is SaveStream with pause support. A paused stream returns
// ErrPaused with the partial file in StreamResult.TempPath; cancellation still
// removes the partial file.
func SaveStreamPausable(ctx context.Context, dir, name string, src io.Reader, progress func(downloaded int64)) (StreamResult, error) {
	if src == nil {
		return StreamResult{}, errors.New("goosie: download: missing response body")
	}
	name = sanitize(name)
	if name == "" {
		name = "download"
	}
	tmpName, total, sum, err := writeStreamTemp(ctx, dir, name, src, progress)
	if err != nil {
		if errors.Is(err, ErrPaused) {
			return StreamResult{TempPath: tmpName, Bytes: total}, err
		}
		return StreamResult{Bytes: total}, err
	}
	defer os.Remove(tmpName)
	fullPath, err := linkTemp(tmpName, dir, name)
	if err != nil {
		return StreamResult{Bytes: total}, err
	}
	return StreamResult{Path: fullPath, Bytes: total, Digest: sum}, nil
}

// SaveStreamTo writes a stream to an explicitly selected path. The temporary
// file is placed beside the destination, then renamed into place after the
// stream succeeds. Callers should only use this after obtaining overwrite
// confirmation from the user's save dialog.
func SaveStreamTo(ctx context.Context, target string, src io.Reader, progress func(downloaded int64)) (int64, Digest, error) {
	res, err := SaveStreamToPausable(ctx, target, src, progress)
	return res.Bytes, res.Digest, err
}

// SaveStreamToPausable is SaveStreamTo with pause support.
func SaveStreamToPausable(ctx context.Context, target string, src io.Reader, progress func(downloaded int64)) (StreamResult, error) {
	if src == nil {
		return StreamResult{}, errors.New("goosie: download: missing response body")
	}
	absTarget, err := resolveTarget(target)
	if err != nil {
		return StreamResult{}, err
	}
	tmpName, total, sum, err := writeStreamTemp(ctx, filepath.Dir(absTarget), filepath.Base(absTarget), src, progress)
	if err != nil {
		if errors.Is(err, ErrPaused) {
			return StreamResult{TempPath: tmpName, Bytes: total}, err
		}
		return StreamResult{Bytes: total}, err
	}
	defer os.Remove(tmpName)
	if err := ctx.Err(); err != nil {
		return StreamResult{Bytes: total}, err
	}
	if err := os.Rename(tmpName, absTarget); err != nil {
		return StreamResult{Bytes: total}, fmt.Errorf("goosie: finalize download: %w", err)
	}
	return StreamResult{Path: absTarget, Bytes: total, Digest: sum}, nil
}

// AppendToTemp continues a paused download into its existing partial file. It
// returns the complete byte count and the SHA-256 of the whole file. ErrPaused
// means the transfer stopped again and its partial file was preserved;
// cancellation removes the partial file.
func AppendToTemp(ctx context.Context, tempPath string, src io.Reader, progress func(downloaded int64)) (int64, Digest, error) {
	if src == nil {
		return 0, Digest{}, errors.New("goosie: download: missing response body")
	}
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, Digest{}, fmt.Errorf("goosie: resume download: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return 0, Digest{}, fmt.Errorf("goosie: resume download: %w", err)
	}
	total := info.Size()
	fail := func(cause error) (int64, Digest, error) {
		_ = file.Close()
		if known := ctx.Err(); known != nil {
			if errors.Is(context.Cause(ctx), ErrPaused) {
				return total, Digest{}, ErrPaused
			}
			_ = os.Remove(tempPath)
			return 0, Digest{}, known
		}
		_ = os.Remove(tempPath)
		return total, Digest{}, cause
	}
	buf := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := file.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return fail(fmt.Errorf("goosie: write download: %w", writeErr))
			}
			if written != n {
				return fail(io.ErrShortWrite)
			}
			if progress != nil {
				progress(total)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fail(readErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := file.Sync(); err != nil {
		return fail(fmt.Errorf("goosie: sync download: %w", err))
	}
	if err := file.Chmod(0o644); err != nil {
		return fail(fmt.Errorf("goosie: set download permissions: %w", err))
	}
	if err := file.Close(); err != nil {
		return total, Digest{}, fmt.Errorf("goosie: close download: %w", err)
	}
	sum, err := hashFile(tempPath)
	if err != nil {
		return total, Digest{}, err
	}
	return total, sum, nil
}

// FinalizeTemp links a completed temporary file into dir under a collision-free
// name.
func FinalizeTemp(tempPath, dir, name string) (string, error) {
	name = sanitize(name)
	if name == "" {
		name = "download"
	}
	fullPath, err := linkTemp(tempPath, dir, name)
	if err != nil {
		return "", err
	}
	_ = os.Remove(tempPath)
	return fullPath, nil
}

// FinalizeTempTo renames a completed temporary file to an explicit destination.
func FinalizeTempTo(tempPath, target string) error {
	absTarget, err := resolveTarget(target)
	if err != nil {
		return err
	}
	if err := os.Rename(tempPath, absTarget); err != nil {
		return fmt.Errorf("goosie: finalize download: %w", err)
	}
	return nil
}

func resolveTarget(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", errors.New("goosie: download: missing destination path")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("goosie: download destination: %w", err)
	}
	base := filepath.Base(absTarget)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "", errors.New("goosie: download: destination must name a file")
	}
	return absTarget, nil
}

func linkTemp(tmpName, dir, name string) (string, error) {
	base, ext := name, filepath.Ext(name)
	if ext != "" {
		base = strings.TrimSuffix(name, ext)
	}
	for i := 0; i <= 999; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		fullPath := filepath.Join(dir, candidate)
		if err := os.Link(tmpName, fullPath); err == nil {
			return fullPath, nil
		} else if !os.IsExist(err) {
			return "", fmt.Errorf("goosie: finalize download: %w", err)
		}
	}
	return "", fmt.Errorf("goosie: download: no free name for %q in %q", name, dir)
}

// HashFile returns the SHA-256 of a file already on disk. Resumed downloads
// use it when the server confirms the partial file already holds every byte.
func HashFile(path string) (Digest, error) {
	return hashFile(path)
}

func hashFile(path string) (Digest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Digest{}, fmt.Errorf("goosie: hash download: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return Digest{}, fmt.Errorf("goosie: hash download: %w", err)
	}
	var sum Digest
	copy(sum[:], hasher.Sum(nil))
	return sum, nil
}

func writeStreamTemp(ctx context.Context, dir, name string, src io.Reader, progress func(downloaded int64)) (string, int64, Digest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, Digest{}, fmt.Errorf("goosie: download dir: %w", err)
	}
	name = sanitize(name)
	if name == "" {
		name = "download"
	}
	tmp, err := os.CreateTemp(dir, name+".goosie-download-*")
	if err != nil {
		return "", 0, Digest{}, fmt.Errorf("goosie: download temp file: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	buf := make([]byte, 64*1024)
	hasher := sha256.New()
	var total int64
	fail := func(cause error) (string, int64, Digest, error) {
		_ = tmp.Close()
		if errors.Is(context.Cause(ctx), ErrPaused) {
			keep = true
			return tmpName, total, Digest{}, ErrPaused
		}
		if known := ctx.Err(); known != nil {
			return "", total, Digest{}, known
		}
		return "", total, Digest{}, cause
	}
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := tmp.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return fail(fmt.Errorf("goosie: write download: %w", writeErr))
			}
			if written != n {
				return fail(io.ErrShortWrite)
			}
			_, _ = hasher.Write(buf[:n])
			if progress != nil {
				progress(total)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fail(readErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(fmt.Errorf("goosie: sync download: %w", err))
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(fmt.Errorf("goosie: set download permissions: %w", err))
	}
	if err := tmp.Close(); err != nil {
		return "", total, Digest{}, fmt.Errorf("goosie: close download: %w", err)
	}
	keep = true
	var sum Digest
	copy(sum[:], hasher.Sum(nil))
	return tmpName, total, sum, nil
}
