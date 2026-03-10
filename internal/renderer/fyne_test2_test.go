package renderer

import (
"fmt"
"testing"
"strings"
"image/color"

"fyne.io/fyne/v2"
"fyne.io/fyne/v2/container"
"fyne.io/fyne/v2/test"
"fyne.io/fyne/v2/canvas"
"golang.org/x/net/html"
)

func TestFyneCapture002(t *testing.T) {
a := test.NewApp()
defer a.Quit()

htmlContent := `<!DOCTYPE html><html><body><p><b>Bold</b>, <strong>Strong</strong>, <i>Italic</i>, <em>Emphasized</em>, <u>Underline</u>, <strike>Strike</strike>, <s>Strikethrough</s>, <tt>Teletype</tt>, <code>Code</code>, <small>Small</small>, <sub>Sub</sub>, <sup>Sup</sup>.</p></body></html>`

doc, _ := html.Parse(strings.NewReader(htmlContent))
bodyNode := findBodyNode(doc)
renderTree := BuildRenderTree(bodyNode)

r := NewRenderer(1280, 800)
obj, _ := r.RenderHTML(string([]byte(htmlContent)))
height := r.GetContentHeight()

fmt.Printf("GetContentHeight = %.1f\n", height)
fmt.Printf("obj type = %T\n", obj)
fmt.Printf("obj.MinSize = %v\n", obj.MinSize())
fmt.Printf("obj.Size = %v\n", obj.Size())
_ = bodyNode
_ = renderTree

w := a.NewWindow("Test")
w.Resize(fyne.NewSize(1280, height))

bg := canvas.NewRectangle(color.White)
bg.Resize(fyne.NewSize(1280, height))

content := container.NewMax(bg, obj)
fmt.Printf("content.MinSize = %v\n", content.MinSize())

w.SetContent(content)
img := w.Canvas().Capture()
if img != nil {
fmt.Printf("canvas.Capture size = %v\n", img.Bounds())
}
}
