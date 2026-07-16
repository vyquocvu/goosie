//go:build headless

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func newHeadlessAppWindow() (fyne.App, fyne.Window, error) {
	a := test.NewApp()
	w := a.NewWindow("Goosie Headless")
	w.Resize(fyne.NewSize(1000, 700))
	return a, w, nil
}
