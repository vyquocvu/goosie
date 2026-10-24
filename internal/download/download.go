// Package download decides whether a response must be saved instead of
// rendered and writes it under a collision-free name.
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

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
func sanitize(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, name)
	fields := strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' })
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// Save writes body under dir with the given name, appending (1), (2), ... to
// the base when the name is taken. It returns the written path.
func Save(dir, name string, body []byte) (string, error) {
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

// SaveStream writes a response incrementally to a temporary file, reporting
// byte progress as chunks arrive. The completed file is linked into place only
// after the stream succeeds, so cancellation and network errors leave no
// partial download visible in the destination directory.
func SaveStream(ctx context.Context, dir, name string, src io.Reader, progress func(downloaded int64)) (string, int64, error) {
	if src == nil {
		return "", 0, errors.New("goosie: download: missing response body")
	}
	name = sanitize(name)
	if name == "" {
		name = "download"
	}
	tmpName, total, err := writeStreamTemp(ctx, dir, src, progress)
	if err != nil {
		return "", total, err
	}
	defer os.Remove(tmpName)
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
			return fullPath, total, nil
		} else if !os.IsExist(err) {
			return "", total, fmt.Errorf("goosie: finalize download: %w", err)
		}
	}
	return "", total, fmt.Errorf("goosie: download: no free name for %q in %q", name, dir)
}

// SaveStreamTo writes a stream to an explicitly selected path. The temporary
// file is placed beside the destination, then renamed into place after the
// stream succeeds. Callers should only use this after obtaining overwrite
// confirmation from the user's save dialog.
func SaveStreamTo(ctx context.Context, target string, src io.Reader, progress func(downloaded int64)) (int64, error) {
	if src == nil {
		return 0, errors.New("goosie: download: missing response body")
	}
	if strings.TrimSpace(target) == "" {
		return 0, errors.New("goosie: download: missing destination path")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return 0, fmt.Errorf("goosie: download destination: %w", err)
	}
	base := filepath.Base(absTarget)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return 0, errors.New("goosie: download: destination must name a file")
	}
	tmpName, total, err := writeStreamTemp(ctx, filepath.Dir(absTarget), src, progress)
	if err != nil {
		return total, err
	}
	defer os.Remove(tmpName)
	if err := ctx.Err(); err != nil {
		return total, err
	}
	if err := os.Rename(tmpName, absTarget); err != nil {
		return total, fmt.Errorf("goosie: finalize download: %w", err)
	}
	return total, nil
}

func writeStreamTemp(ctx context.Context, dir string, src io.Reader, progress func(downloaded int64)) (string, int64, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, fmt.Errorf("goosie: download dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".goosie-download-*")
	if err != nil {
		return "", 0, fmt.Errorf("goosie: download temp file: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpName)
		}
	}()
	buf := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			_ = tmp.Close()
			return "", total, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := tmp.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				_ = tmp.Close()
				return "", total, fmt.Errorf("goosie: write download: %w", writeErr)
			}
			if written != n {
				_ = tmp.Close()
				return "", total, io.ErrShortWrite
			}
			if progress != nil {
				progress(total)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = tmp.Close()
			return "", total, readErr
		}
	}
	if err := ctx.Err(); err != nil {
		_ = tmp.Close()
		return "", total, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", total, fmt.Errorf("goosie: sync download: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return "", total, fmt.Errorf("goosie: set download permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", total, fmt.Errorf("goosie: close download: %w", err)
	}
	keep = true
	return tmpName, total, nil
}
