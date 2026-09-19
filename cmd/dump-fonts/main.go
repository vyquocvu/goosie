package main

import (
	"fmt"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func main() {
	dir := "/System/Library/Fonts/Supplemental"
	for _, f := range os.Args[1:] {
		data, err := os.ReadFile(dir + "/" + f)
		if err != nil { fmt.Println(f, "ERR", err); continue }
		parsed, err := opentype.Parse(data)
		if err != nil { fmt.Println(f, "PARSE ERR", err); continue }
		for _, size := range []float64{16, 12, 24, 14} {
			face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
				Size: size, DPI: 72, Hinting: font.HintingNone,
			})
			if err != nil { fmt.Println(f, "FACE ERR", err); continue }
			m := face.Metrics()
			asc := float64(m.Ascent) / 64
			dsc := float64(m.Descent) / 64
			hgt := float64(m.Height) / 64
			fmt.Printf("%-30s size=%2.0f asc=%.3f desc=%.3f sum=%.3f height=%.3f /size=%.5f\n",
				f, size, asc, dsc, asc+dsc, hgt, hgt/size)
		}
	}
}
