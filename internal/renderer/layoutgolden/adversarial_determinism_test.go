package layoutgolden

import (
	"sync"
	"testing"
)

// TestAdversarialConcurrentLayoutDeterminism tests that parallel layout computations
// of diverse complex layouts produce 100% deterministic identical outputs without race conditions.
func TestAdversarialConcurrentLayoutDeterminism(t *testing.T) {
	complexFixtures := []struct {
		name string
		html string
		css  string
		w, h float32
	}{
		{
			name: "table_with_spacers_and_inlines",
			html: `<!DOCTYPE html><html><body>
				<table width="600" cellspacing="0" cellpadding="0">
					<tr>
						<td width="50">Left</td>
						<td><p>Main content <b>bold</b> <i>italic</i> and <a href="#">link</a>.</p></td>
						<td width="50">Right</td>
					</tr>
				</table>
			</body></html>`,
			css: `
				body { font-size: 16px; margin: 0; }
				table { border: 1px solid black; }
				td { padding: 4px; }
			`,
			w: 600, h: 400,
		},
		{
			name: "nested_flex_with_alignment",
			html: `<!DOCTYPE html><html><body>
				<div class="row">
					<div class="col col-1"><span>Item 1</span></div>
					<div class="col col-2"><span>Item 2</span></div>
					<div class="col col-3"><span>Item 3</span></div>
				</div>
			</body></html>`,
			css: `
				.row { display: flex; flex-direction: row; width: 500px; }
				.col { flex: 1; padding: 10px; margin: 5px; }
				.col-2 { flex-grow: 2; }
			`,
			w: 500, h: 300,
		},
	}

	for _, fix := range complexFixtures {
		fix := fix
		t.Run(fix.name, func(t *testing.T) {
			// Baseline reference run
			ref := renderAndSerialize(t, fix.w, fix.h, fix.html, fix.css)

			// Concurrently compute layout across 20 goroutines
			const concurrency = 20
			var wg sync.WaitGroup
			wg.Add(concurrency)

			for i := 0; i < concurrency; i++ {
				go func(iter int) {
					defer wg.Done()
					got := renderAndSerialize(t, fix.w, fix.h, fix.html, fix.css)
					if got != ref {
						t.Errorf("Goroutine %d produced non-deterministic output: got %s, want %s", iter, got, ref)
					}
				}(i)
			}

			wg.Wait()
		})
	}
}
