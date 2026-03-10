package renderer

import (
"fmt"
"testing"

"fyne.io/fyne/v2"
"fyne.io/fyne/v2/container"
"fyne.io/fyne/v2/test"
"fyne.io/fyne/v2/widget"
"fyne.io/fyne/v2/canvas"
"image/color"
)

func TestFyneMinSize(t *testing.T) {
a := test.NewApp()
defer a.Quit()

w := a.NewWindow("Test")
w.Resize(fyne.NewSize(1280, 48))

label1 := widget.NewLabel("Bold")
label1.Wrapping = fyne.TextWrapWord
label1.Resize(fyne.NewSize(35.7, 19.2))
label1.Move(fyne.NewPos(0, 4.0))

label2 := widget.NewLabel("Strikethrough")
label2.Wrapping = fyne.TextWrapWord
label2.Resize(fyne.NewSize(104.2, 19.2))
label2.Move(fyne.NewPos(352.1, 4.0))

fmt.Printf("label1.MinSize = %v\n", label1.MinSize())
fmt.Printf("label2.MinSize = %v\n", label2.MinSize())

bg := canvas.NewRectangle(color.White)
bg.Resize(fyne.NewSize(1280, 48))

cont := container.NewWithoutLayout(bg, label1, label2)
fmt.Printf("container.MinSize = %v\n", cont.MinSize())

w.SetContent(cont)
img := w.Canvas().Capture()
if img != nil {
fmt.Printf("canvas.Capture size = %v\n", img.Bounds())
}
}
