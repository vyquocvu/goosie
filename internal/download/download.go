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

type Digest [sha256.Size]byte

func (d Digest) String() string {
	return hex.EncodeToString(d[:])
}

func Sum(body []byte) Digest {
	return Digest(sha256.Sum256(body))
}

func ShouldDownload(contentType, disposition string) bool {
	if strings.Contains(strings.ToLower(disposition), "attachment") {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexAny(ct, ";,"); i >= 0 {
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

func dispositionParam(disposition string) string {
	for _, part := range strings.Split(disposition, ";") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "filename="); ok {
			return strings.Trim(rest, `"`)
		}
	}
	return ""
}

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

var ErrPaused = errors.New("goosie: download paused")

type StreamResult struct {
	Path     string
	TempPath string
	Bytes    int64
	Digest   Digest
}

func SaveStream(ctx context.Context, dir, name string, src io.Reader, progress func(downloaded int64)) (string, int64, Digest, error) {
	res, err := SaveStreamPausable(ctx, dir, name, src, progress)
	return res.Path, res.Bytes, res.Digest, err
}

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

func SaveStreamTo(ctx context.Context, target string, src io.Reader, progress func(downloaded int64)) (int64, Digest, error) {
	res, err := SaveStreamToPausable(ctx, target, src, progress)
	return res.Bytes, res.Digest, err
}

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
