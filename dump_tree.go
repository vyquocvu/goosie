package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/vyquocvu/goosie/internal/css"
)

func main() {
	resp, err := http.Get("https://www.iana.org/static/css/iana_website.968be078325a.css")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	parser := css.NewParser(string(bodyBytes))
	stylesheet, err := parser.Parse()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Parsed %d rules\n", len(stylesheet.Rules))
	for _, rule := range stylesheet.Rules {
		for _, sel := range rule.Selectors {
			// Find if any selector has ID "header"
			hasHeader := false
			for s := &sel; s != nil; s = s.Next {
				if s.Simple.ID == "header" {
					hasHeader = true
					break
				}
			}
			if hasHeader {
				fmt.Printf("Selector sequence: ")
				for s := &sel; s != nil; s = s.Next {
					fmt.Printf("%+v (comb: %q) -> ", s.Simple, s.Combinator)
				}
				fmt.Println("nil")
			}
		}
	}
}
