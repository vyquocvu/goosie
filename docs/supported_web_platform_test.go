package docs

import (
	"os"
	"strings"
	"testing"
)

func TestSupportedWebPlatformDocumentCoversRoadmapScope(t *testing.T) {
	data, err := os.ReadFile("SUPPORTED_WEB_PLATFORM.md")
	if err != nil {
		t.Fatalf("read support matrix: %v", err)
	}
	doc := string(data)
	lowerDoc := strings.ToLower(doc)

	required := []string{
		"# Supported Web Platform",
		"## Status Categories",
		"### Supported",
		"### Partial",
		"### Planned",
		"### Fallback",
		"### Out of Scope",
		"## Supported HTML Elements",
		"## Supported CSS",
		"## Supported DOM and Browser APIs",
		"## Maximum Resource Limits",
	}
	for _, phrase := range required {
		if !strings.Contains(doc, phrase) {
			t.Fatalf("support matrix missing %q", phrase)
		}
	}
	if !strings.Contains(lowerDoc, "full modern web application compatibility is not a v2 goal") {
		t.Fatalf("support matrix missing v2 compatibility scope statement")
	}

	for _, phrase := range []string{
		"Maximum document size",
		"Maximum stylesheet size",
		"Maximum decoded image size",
		"Maximum script execution budget",
	} {
		if !strings.Contains(doc, phrase) {
			t.Fatalf("support matrix missing resource limit %q", phrase)
		}
	}

	if strings.Count(doc, "| `") < 20 {
		t.Fatalf("support matrix should enumerate concrete features, found too few table rows")
	}
}
