package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/tabs"
)

const (
	maxPickedFileCount  = 32
	maxPickedFileBytes  = 64 << 20
	maxPickedTotalBytes = 64 << 20
)

func (f *framePath) serviceFilePicker(tab *tabs.Tab) bool {
	if tab == nil || tab.Session() == nil {
		return false
	}
	sess := tab.Session()
	node, ok := sess.PopFilePickerRequest()
	if !ok {
		return false
	}
	picker, ok := f.window.(platform.FilePicker)
	if !ok {
		return true
	}
	paths, chosen, err := picker.ChooseFiles(node.GetAttribute("accept"), node.HasAttribute("multiple"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: file picker:", err)
		return true
	}
	if !chosen {
		return true
	}
	files, err := readPickedFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: file picker:", err)
		return true
	}
	if sess.SetInputFiles(node, files) {
		f.reflowAfterControlChange(tab)
	}
	return true
}

func readPickedFiles(paths []string) ([]js.FileSelection, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > maxPickedFileCount {
		return nil, fmt.Errorf("selection of %d files exceeds the limit of %d", len(paths), maxPickedFileCount)
	}
	files := make([]js.FileSelection, 0, len(paths))
	total := int64(0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", path)
		}
		if info.Size() > maxPickedFileBytes {
			return nil, fmt.Errorf("%s exceeds the %d-byte file limit", path, maxPickedFileBytes)
		}
		total += info.Size()
		if total > maxPickedTotalBytes {
			return nil, fmt.Errorf("selection exceeds the %d-byte total limit", maxPickedTotalBytes)
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxPickedFileBytes+1))
		file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if len(data) > maxPickedFileBytes {
			return nil, fmt.Errorf("%s exceeds the %d-byte file limit", path, maxPickedFileBytes)
		}
		files = append(files, js.FileSelection{
			Name:        filepath.Base(path),
			ContentType: fileContentType(path, data),
			Data:        data,
		})
	}
	return files, nil
}

func fileContentType(path string, data []byte) string {
	typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if typ == "" {
		if len(data) == 0 {
			return "application/octet-stream"
		}
		typ = http.DetectContentType(data)
	}
	if i := strings.IndexByte(typ, ';'); i >= 0 {
		typ = typ[:i]
	}
	return strings.ToLower(strings.TrimSpace(typ))
}

func (f *framePath) reflowAfterControlChange(tab *tabs.Tab) {
	sess := tab.Session()
	if sess == nil {
		return
	}
	if !sess.CanDeferControlEditReflow() {
		var err error
		if sess.HasPseudoClassRule("checked") || sess.HasPseudoClassRule("focus") || sess.HasPseudoClassRule("focus-visible") || sess.HasPseudoClassRule("active") || sess.HasValidityClassRules() {
			err = sess.RestyleAndReflow(float32(f.contentWidth()))
		} else {
			err = sess.Reflow(float32(f.contentWidth()))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		}
	}
	f.repaintTab(tab, frame.Rect{})
}
