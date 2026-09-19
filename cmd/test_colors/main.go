package main

import (
	"fmt"
	"github.com/vyquocvu/goosie/internal/css"
)

func main() {
	tests := []string{
		"#f00",
		"#0f0",
		"#00ff00",
		"#0000ff",
		"rgb(255, 255, 0)",
		"rgb(128, 0, 128)",
		"rgba(0, 255, 255, 0.5)",
		"hsl(0, 100%, 50%)",
		"hsl(120, 100%, 50%)",
		"hsla(60, 100%, 50%, 0.5)",
		"hsla(240, 100%, 50%, 1)",
		"red",
		"yellow",
		"transparent",
	}
	for _, t := range tests {
		c, ok := css.ParseColor(t)
		if !ok {
			fmt.Printf("%-30s → PARSE FAILED\n", t)
			continue
		}
		fmt.Printf("%-30s → R=%3d G=%3d B=%3d A=%3d\n", t, c.R, c.G, c.B, c.A)
	}
}
