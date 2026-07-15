package devtools

import "fyne.io/fyne/v2"

type Panel interface {
	Title() string
	Icon() fyne.Resource
	CanvasObject() fyne.CanvasObject
}
