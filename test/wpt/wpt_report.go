package wpt

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Reporter writes WPT results in JSON and HTML formats.
type Reporter struct {
	result *SuiteResult
}

// NewReporter creates a Reporter for the given suite result.
func NewReporter(result *SuiteResult) *Reporter {
	return &Reporter{result: result}
}

// WriteJSON writes the results as a JSON file.
func (r *Reporter) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.result)
}

// WriteJSONFile writes the results to a JSON file at the given path.
func (r *Reporter) WriteJSONFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return r.WriteJSON(f)
}

// WriteHTML writes an HTML dashboard of the results.
func (r *Reporter) WriteHTML(w io.Writer) error {
	return htmlTmpl.Execute(w, r.result)
}

// WriteHTMLFile writes the HTML dashboard to a file at the given path.
func (r *Reporter) WriteHTMLFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return r.WriteHTML(f)
}

// PrintSummary writes a human-readable summary to w.
func (r *Reporter) PrintSummary(w io.Writer) {
	res := r.result
	fmt.Fprintf(w, "\n=== WPT Results Summary ===\n")
	fmt.Fprintf(w, "Commit:    %s\n", res.Commit)
	fmt.Fprintf(w, "Timestamp: %s\n", res.Timestamp)
	fmt.Fprintf(w, "Duration:  %.1fs\n", res.Duration)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Total:     %d\n", res.Total)
	fmt.Fprintf(w, "Passed:    %d\n", res.Passed)
	fmt.Fprintf(w, "Failed:    %d\n", res.Failed)
	fmt.Fprintf(w, "Skipped:   %d\n", res.Skipped)
	fmt.Fprintf(w, "Errored:   %d\n", res.Errored)
	fmt.Fprintf(w, "Pass Rate: %.1f%%\n", res.PassRate)

	// Group results by directory for a per-area breakdown.
	byDir := make(map[string]*dirSummary)
	for _, tr := range res.Results {
		dir := filepath.Dir(tr.Path)
		ds, ok := byDir[dir]
		if !ok {
			ds = &dirSummary{dir: dir}
			byDir[dir] = ds
		}
		ds.total++
		switch tr.Status {
		case "pass":
			ds.passed++
		case "fail":
			ds.failed++
		case "skip":
			ds.skipped++
		case "error", "timeout":
			ds.errored++
		}
	}

	// Sort directories by name.
	var dirNames []string
	for name := range byDir {
		dirNames = append(dirNames, name)
	}
	sort.Strings(dirNames)

	fmt.Fprintf(w, "\n--- Per-Directory Breakdown ---\n")
	for _, name := range dirNames {
		ds := byDir[name]
		runnable := ds.total - ds.skipped
		rate := 0.0
		if runnable > 0 {
			rate = float64(ds.passed) / float64(runnable) * 100
		}
		fmt.Fprintf(w, "  %-50s %3d/%3d pass (%5.1f%%)  [fail=%d skip=%d err=%d]\n",
			name, ds.passed, runnable, rate, ds.failed, ds.skipped, ds.errored)
	}

	// List failures.
	var failures []TestResult
	for _, tr := range res.Results {
		if tr.Status == "fail" {
			failures = append(failures, tr)
		}
	}
	if len(failures) > 0 {
		fmt.Fprintf(w, "\n--- Failures (%d) ---\n", len(failures))
		for _, tr := range failures {
			fmt.Fprintf(w, "  FAIL %-60s score=%.1f%% %s\n", tr.Path, tr.Score, tr.Message)
		}
	}
}

type dirSummary struct {
	dir     string
	total   int
	passed  int
	failed  int
	skipped int
	errored int
}

// htmlTmpl is the HTML dashboard template.
var htmlTmpl = template.Must(template.New("wpt-report").Funcs(template.FuncMap{
	"statusClass": func(status string) string {
		switch status {
		case "pass":
			return "pass"
		case "fail":
			return "fail"
		case "skip":
			return "skip"
		default:
			return "error"
		}
	},
	"pct": func(f float64) string {
		return fmt.Sprintf("%.1f%%", f)
	},
	"trimPath": func(p string) string {
		if len(p) > 80 {
			return "..." + p[len(p)-77:]
		}
		return p
	},
	"join": strings.Join,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>WPT Results - {{.Commit}}</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
         padding: 2rem; background: #f5f5f5; color: #333; }
  h1 { margin-bottom: 0.5rem; }
  .meta { color: #666; margin-bottom: 2rem; }
  .summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
             gap: 1rem; margin-bottom: 2rem; }
  .card { background: white; border-radius: 8px; padding: 1.5rem; text-align: center;
          box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  .card .number { font-size: 2rem; font-weight: bold; }
  .card .label { font-size: 0.85rem; color: #666; margin-top: 0.25rem; }
  .pass .number { color: #22c55e; }
  .fail .number { color: #ef4444; }
  .skip .number { color: #f59e0b; }
  .error .number { color: #8b5cf6; }
  .rate .number { color: #3b82f6; }
  table { width: 100%; border-collapse: collapse; background: white;
          border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  th { background: #1e293b; color: white; padding: 0.75rem 1rem; text-align: left;
       font-weight: 500; font-size: 0.85rem; }
  td { padding: 0.5rem 1rem; border-bottom: 1px solid #e5e7eb; font-size: 0.85rem; }
  tr:last-child td { border-bottom: none; }
  tr:hover { background: #f9fafb; }
  .status { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 4px;
            font-size: 0.75rem; font-weight: 600; text-transform: uppercase; }
  .status.pass { background: #dcfce7; color: #166534; }
  .status.fail { background: #fef2f2; color: #991b1b; }
  .status.skip { background: #fef9c3; color: #854d0e; }
  .status.error { background: #f3e8ff; color: #6b21a8; }
  .score { font-family: monospace; }
  .path { font-family: monospace; font-size: 0.8rem; word-break: break-all; }
  .message { color: #666; font-size: 0.8rem; }
  .filter-bar { margin-bottom: 1rem; }
  .filter-bar input { padding: 0.5rem 1rem; border: 1px solid #d1d5db; border-radius: 6px;
                      width: 300px; font-size: 0.9rem; }
</style>
</head>
<body>
<h1>WPT Results</h1>
<div class="meta">
  Commit: {{.Commit}} | {{.Timestamp}} | Duration: {{printf "%.1f" .Duration}}s
</div>

<div class="summary">
  <div class="card rate"><div class="number">{{pct .PassRate}}</div><div class="label">Pass Rate</div></div>
  <div class="card"><div class="number">{{.Total}}</div><div class="label">Total</div></div>
  <div class="card pass"><div class="number">{{.Passed}}</div><div class="label">Passed</div></div>
  <div class="card fail"><div class="number">{{.Failed}}</div><div class="label">Failed</div></div>
  <div class="card skip"><div class="number">{{.Skipped}}</div><div class="label">Skipped</div></div>
  <div class="card error"><div class="number">{{.Errored}}</div><div class="label">Errored</div></div>
</div>

<div class="filter-bar">
  <input type="text" id="filter" placeholder="Filter by path or status..." onkeyup="filterTable()">
</div>

<table id="results">
<thead>
<tr>
  <th>Status</th>
  <th>Test Path</th>
  <th>Type</th>
  <th>Score</th>
  <th>Duration</th>
  <th>Message</th>
</tr>
</thead>
<tbody>
{{range .Results}}
<tr>
  <td><span class="status {{statusClass .Status}}">{{.Status}}</span></td>
  <td class="path">{{trimPath .Path}}</td>
  <td>{{.Type}}</td>
  <td class="score">{{if .Score}}{{pct .Score}}{{else}}-{{end}}</td>
  <td>{{if .DurationMS}}{{.DurationMS}}ms{{else}}-{{end}}</td>
  <td class="message">{{.Message}}</td>
</tr>
{{end}}
</tbody>
</table>

<script>
function filterTable() {
  var input = document.getElementById('filter').value.toLowerCase();
  var rows = document.querySelectorAll('#results tbody tr');
  rows.forEach(function(row) {
    var text = row.textContent.toLowerCase();
    row.style.display = text.includes(input) ? '' : 'none';
  });
}
</script>
</body>
</html>`))
