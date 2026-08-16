// Command html-audit regenerates the HTML element conformance tracker
// (HTML_CONFORMANCE.md) at the repository root. Run via `make html-audit`.
package main

import (
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/conformance"
)

func main() {
	tracker := conformance.RenderTracker(conformance.AuditAll())
	path := "HTML_CONFORMANCE.md"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if err := os.WriteFile(path, []byte(tracker), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "html-audit: write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("html-audit: wrote %s\n", path)
}
