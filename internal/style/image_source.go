package style

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type ImageSource struct {
	Src  string
	W, H float32
}

func SelectedImageSource(n *dom.Node, viewportW float32) ImageSource {
	if n == nil {
		return ImageSource{}
	}
	if n.Parent != nil && n.Parent.Data == "picture" {
		for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
			if c == n {
				break
			}
			if !c.Element() || c.Data != "source" {
				continue
			}
			if typ := c.GetAttribute("type"); typ != "" && !supportedImageType(typ) {
				continue
			}
			if media := c.GetAttribute("media"); media != "" && !css.MatchMedia(media, viewportW) {
				continue
			}
			srcset := strings.TrimSpace(c.GetAttribute("srcset"))
			if srcset == "" {
				continue
			}
			fields := strings.Fields(srcset)
			if len(fields) == 0 {
				continue
			}
			return ImageSource{Src: fields[0], W: imageAttrPx(c, "width"), H: imageAttrPx(c, "height")}
		}
	}
	return ImageSource{Src: n.GetAttribute("src")}
}

func imageAttrPx(n *dom.Node, name string) float32 {
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(n.GetAttribute(name), "px")), 32)
	if err != nil || v <= 0 {
		return 0
	}
	return float32(v)
}

func supportedImageType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp", "image/svg+xml", "image/bmp":
		return true
	}
	return false
}
