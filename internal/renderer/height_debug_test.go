package renderer

import (
"fmt"
"os"
"path/filepath"
"testing"
)

func TestDebugHeights(t *testing.T) {
tests := []string{
"test_001_typography.html",
"test_002_typography.html",
"test_003_typography.html",
"test_004_typography.html",
"test_005_typography.html",
"test_009_typography.html",
}
base := "/home/runner/work/goosie/goosie/testdata"
for _, f := range tests {
path := filepath.Join(base, f)
data, _ := os.ReadFile(path)
r := NewRenderer(1280, 800)
_, err := r.RenderHTML(string(data))
if err != nil {
fmt.Printf("%s: ERROR: %v\n", f, err)
continue
}
fmt.Printf("%s: height=%.1f\n", f, r.GetContentHeight())
}
}
