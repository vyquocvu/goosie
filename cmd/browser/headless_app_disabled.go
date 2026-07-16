//go:build !headless

package main

import (
	"fmt"

	"fyne.io/fyne/v2"
)

func newHeadlessAppWindow() (fyne.App, fyne.Window, error) {
	return nil, nil, fmt.Errorf("the -headless browser mode requires a headless-tag build; use `make build-headless` for URL screenshots or `make build-headless-cli` for stdin/local HTML rendering")
}
