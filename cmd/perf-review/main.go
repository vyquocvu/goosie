// perf-review is a CLI driver that exercises the Goosie browser through
// the browsercontrol.Service interface, measuring every stage of the
// pipeline. It is the in-process equivalent of the MCP server: same
// API, same backing engine, no protocol overhead. Use it to attribute
// performance regressions to a specific subsystem.
//
// Usage:
//
//	perf-review                          # default pages, default iterations
//	perf-review -iterations=5            # repeat each workload
//	perf-review -urls=https://example.com,https://www.iana.org
//	perf-review -fixtures=testdata/perf/ # use offline HTML fixtures
//	perf-review -include-mutations       # run the JS mutation stress test
//	perf-review -include-scroll          # run the scroll-burst test
//	perf-review -json                    # machine-readable output
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ts, fixtures := startFixtureServer()
	defer ts.Close()

	// If -fixtures is set, load HTML files from that directory and serve
	// them via the test server, replacing the default fixtures.
	var fixtureURLs []string
	if opts.fixtures != "" {
		fixtureURLs = loadFixtureDir(ts, opts.fixtures)
	} else {
		fixtureURLs = fixtures
	}

	allURLs := append([]string(nil), fixtureURLs...)
	allURLs = append(allURLs, opts.urls...)

	runner := &review{
		urls:       allURLs,
		iterations: opts.iterations,
		verbose:    opts.verbose,
	}
	results := runner.run(opts.includeMutations, opts.includeScroll)

	// When using offline fixtures, also measure the render pipeline stages
	// directly via the renderer package.
	if opts.fixtures != "" {
		results.RenderStages = measureRenderStages(opts.fixtures, opts.iterations)
	}

	if opts.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}
	printHuman(os.Stdout, results)
}

type options struct {
	urls             []string
	fixtures         string
	iterations       int
	includeMutations bool
	includeScroll    bool
	json             bool
	verbose          bool
}

func parseOptions(args []string) (options, error) {
	const defaultURLs = "https://example.com,https://www.iana.org/help/example-domains"

	fs := flag.NewFlagSet("perf-review", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	urls := fs.String("urls", defaultURLs, "comma-separated URLs to navigate")
	fixtures := fs.String("fixtures", "", "directory of offline HTML fixtures to use instead of default pages")
	iterations := fs.Int("iterations", 3, "number of times to repeat each workload")
	includeMutations := fs.Bool("include-mutations", true, "run the JS mutation stress test")
	includeScroll := fs.Bool("include-scroll", true, "run the scroll-burst test")
	jsonOut := fs.Bool("json", false, "emit a single JSON document instead of human-readable text")
	verbose := fs.Bool("v", false, "print per-iteration results in addition to aggregates")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if *iterations < 1 {
		return options{}, fmt.Errorf("iterations must be positive")
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return options{
		urls:             splitNonEmpty(*urls, ","),
		fixtures:         *fixtures,
		iterations:       *iterations,
		includeMutations: *includeMutations,
		includeScroll:    *includeScroll,
		json:             *jsonOut,
		verbose:          *verbose,
	}, nil
}

type review struct {
	urls       []string
	iterations int
	verbose    bool
}

type stageSample struct {
	Name   string  `json:"name"`
	N      int     `json:"n"`
	MinNS  int64   `json:"min_ns"`
	MaxNS  int64   `json:"max_ns"`
	MeanNS float64 `json:"mean_ns"`
	P50NS  int64   `json:"p50_ns"`
	P95NS  int64   `json:"p95_ns"`
	P99NS  int64   `json:"p99_ns"`
	Errors int     `json:"errors"`
	Notes  string  `json:"notes,omitempty"`
}

type workloadResult struct {
	Name    string        `json:"name"`
	URL     string        `json:"url,omitempty"`
	Samples []stageSample `json:"samples"`
}

type reviewResult struct {
	GoVersion     string           `json:"go_version"`
	StartedAt     time.Time        `json:"started_at"`
	Duration      time.Duration    `json:"duration"`
	Workloads     []workloadResult `json:"workloads"`
	FreezeHealth  map[string]bool  `json:"freeze_health"`
	HealthSummary string           `json:"health_summary,omitempty"`
	RenderStages  []renderStageResult `json:"render_stages,omitempty"`
}

// renderStageResult holds per-stage timing for the render pipeline,
// measured directly via renderer.RenderHTMLToImageWithStages.
type renderStageResult struct {
	Fixture string            `json:"fixture"`
	N       int               `json:"n"`
	Stages  map[string]int64  `json:"stages_ns"` // mean duration per stage in nanoseconds
}

// withContext opens a context, runs fn, and closes the context. The
// service returns the public Context interface so the perf-review
// tool stays decoupled from the engine implementation.
func withContext(svc browsercontrol.Service, fn func(ec browsercontrol.Context) error) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	if err != nil {
		return err
	}
	ec, err := svc.Context(ctx, info.ID)
	if err != nil {
		return err
	}
	defer func() { _ = svc.CloseContext(ctx, info.ID) }()
	return fn(ec)
}

func (r *review) run(includeMutations, includeScroll bool) reviewResult {
	start := time.Now()
	out := reviewResult{
		StartedAt:    start,
		FreezeHealth: map[string]bool{},
	}
	svc := browsercontrol.NewEngineService()

	for _, u := range r.urls {
		w := r.measureNavigation(svc, u)
		out.Workloads = append(out.Workloads, w)
	}

	if len(r.urls) > 0 {
		out.Workloads = append(out.Workloads, r.measureReadStages(svc, r.urls[0]))
	}
	if includeScroll && len(r.urls) > 0 {
		w := r.measureScrollBurst(svc, r.urls[0])
		out.Workloads = append(out.Workloads, w)
		for _, s := range w.Samples {
			if s.Name == "scroll.per_event" {
				out.FreezeHealth["scroll_under_5ms_p50"] = s.P50NS < int64(5*time.Millisecond)
			}
		}
	}
	if includeMutations && len(r.urls) > 0 {
		w := r.measureMutationBurst(svc, r.urls[0])
		out.Workloads = append(out.Workloads, w)
		for _, s := range w.Samples {
			if s.Name == "mutation.cycle" {
				out.FreezeHealth["mutation_under_50ms_p95"] = s.P95NS < int64(50*time.Millisecond)
			}
		}
	}

	out.Duration = time.Since(start)
	out.GoVersion = runtime.Version()
	out.HealthSummary = summarizeHealth(out.FreezeHealth)
	return out
}

func (r *review) measureNavigation(svc browsercontrol.Service, url string) workloadResult {
	w := workloadResult{Name: "navigation", URL: url}
	var samples []int64
	var errs int
	for i := 0; i < r.iterations; i++ {
		err := withContext(svc, func(ec browsercontrol.Context) error {
			start := time.Now()
			_, err := ec.Navigate(context.Background(), url, browsercontrol.WaitComplete, 25000)
			if err != nil {
				return err
			}
			samples = append(samples, int64(time.Since(start)))
			return nil
		})
		if err != nil {
			errs++
			if r.verbose {
				fmt.Fprintf(os.Stderr, "[warn] navigation %d failed: %v\n", i, err)
			}
		}
	}
	w.Samples = append(w.Samples, summarizeSamples("navigate", samples, errs, ""))
	return w
}

func (r *review) measureReadStages(svc browsercontrol.Service, url string) workloadResult {
	w := workloadResult{Name: "read-stages", URL: url}
	snap := make([]int64, 0, r.iterations)
	shot := make([]int64, 0, r.iterations)
	eval := make([]int64, 0, r.iterations)
	var errs [3]int

	for i := 0; i < r.iterations; i++ {
		err := withContext(svc, func(ec browsercontrol.Context) error {
			ctx := context.Background()
			if _, err := ec.Navigate(ctx, url, browsercontrol.WaitComplete, 25000); err != nil {
				return err
			}
			start := time.Now()
			if _, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{MaxDepth: 30}); err != nil {
				return err
			}
			snap = append(snap, int64(time.Since(start)))

			start = time.Now()
			if _, err := ec.Screenshot(ctx, browsercontrol.ScreenshotOptions{}); err != nil {
				return err
			}
			shot = append(shot, int64(time.Since(start)))

			start = time.Now()
			if _, err := ec.Evaluate(ctx, "({a:1, b:'hello', c:[1,2,3]})", browsercontrol.EvaluateOptions{}); err != nil {
				return err
			}
			eval = append(eval, int64(time.Since(start)))
			return nil
		})
		if err != nil {
			errs[0]++
			errs[1]++
			errs[2]++
		}
	}
	w.Samples = append(w.Samples,
		summarizeSamples("snapshot", snap, errs[0], "PageSnapshot extraction"),
		summarizeSamples("screenshot", shot, errs[1], "PNG capture"),
		summarizeSamples("evaluate", eval, errs[2], "JS expression eval"),
	)
	return w
}

// measureScrollBurst issues N rapid scroll commands and reports the
// per-event time. The test uses the public Scroll() method so the
// measure reflects what the MCP server would observe.
func (r *review) measureScrollBurst(svc browsercontrol.Service, url string) workloadResult {
	const burst = 100
	w := workloadResult{Name: "scroll-burst", URL: url}
	var samples []int64
	var errs int
	for i := 0; i < r.iterations; i++ {
		err := withContext(svc, func(ec browsercontrol.Context) error {
			ctx := context.Background()
			if _, err := ec.Navigate(ctx, url, browsercontrol.WaitComplete, 25000); err != nil {
				return err
			}
			start := time.Now()
			for j := 0; j < burst; j++ {
				_, err := ec.Scroll(ctx, browsercontrol.ScrollOptions{DeltaY: 4, DeltaX: 0})
				if err != nil {
					return err
				}
			}
			total := time.Since(start)
			samples = append(samples, int64(total)/burst)
			return nil
		})
		if err != nil {
			errs++
		}
	}
	w.Samples = append(w.Samples, summarizeSamples("scroll.per_event", samples, errs,
		"per-scroll median over 100-event burst"))
	return w
}

func (r *review) measureMutationBurst(svc browsercontrol.Service, url string) workloadResult {
	const cycles = 30
	w := workloadResult{Name: "mutation-burst", URL: url}
	var samples []int64
	var errs int
	expr := buildMutationExpr(cycles)
	for i := 0; i < r.iterations; i++ {
		err := withContext(svc, func(ec browsercontrol.Context) error {
			ctx := context.Background()
			if _, err := ec.Navigate(ctx, url, browsercontrol.WaitComplete, 25000); err != nil {
				return err
			}
			if _, err := ec.Evaluate(ctx, "document.body.innerHTML='<div id=probe></div>'", browsercontrol.EvaluateOptions{}); err != nil {
				return err
			}
			start := time.Now()
			if _, err := ec.Evaluate(ctx, expr, browsercontrol.EvaluateOptions{TimeoutMs: 60000}); err != nil {
				return err
			}
			samples = append(samples, int64(time.Since(start))/cycles)
			return nil
		})
		if err != nil {
			errs++
		}
	}
	w.Samples = append(w.Samples, summarizeSamples("mutation.cycle", samples, errs,
		"per-cycle median over 30-element textContent mutation burst"))
	return w
}

func buildMutationExpr(n int) string {
	var sb strings.Builder
	sb.WriteString("(function(){")
	sb.WriteString("var p=document.getElementById('probe');")
	sb.WriteString("for (var i=0; i<")
	sb.WriteString(fmt.Sprintf("%d", n))
	sb.WriteString("; i++) { p.textContent='iter '+i; }")
	sb.WriteString("return i;})()")
	return sb.String()
}

func summarizeSamples(name string, samples []int64, errs int, notes string) stageSample {
	s := stageSample{Name: name, N: len(samples), Errors: errs, Notes: notes}
	if len(samples) == 0 {
		return s
	}
	sorted := make([]int64, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	s.MinNS = sorted[0]
	s.MaxNS = sorted[len(sorted)-1]
	var sum int64
	for _, v := range sorted {
		sum += v
	}
	s.MeanNS = float64(sum) / float64(len(sorted))
	s.P50NS = sorted[len(sorted)*50/100]
	if len(sorted) > 1 {
		s.P95NS = sorted[imin(len(sorted)-1, len(sorted)*95/100)]
		s.P99NS = sorted[imin(len(sorted)-1, len(sorted)*99/100)]
	} else {
		s.P95NS = s.P50NS
		s.P99NS = s.P50NS
	}
	return s
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func startFixtureServer() (*httptest.Server, []string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/small", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<!doctype html><html><head><title>Small</title></head><body><h1>Small</h1><p>A minimal page used as a performance baseline.</p></body></html>`)
	})
	var sb strings.Builder
	sb.WriteString("<!doctype html><html><head><title>Long</title></head><body>")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "<section><h2>Section %d</h2>", i)
		for j := 0; j < 6; j++ {
			fmt.Fprintf(&sb, "<p>Paragraph %d.%d: the quick brown fox jumps over the lazy dog. ", i, j)
		}
		sb.WriteString("</section>")
	}
	sb.WriteString("</body></html>")
	mux.HandleFunc("/long", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, sb.String())
	})
	var t strings.Builder
	t.WriteString("<!doctype html><html><head><title>Table</title><style>td,th{padding:4px 8px}</style></head><body><table>")
	for r := 0; r < 80; r++ {
		t.WriteString("<tr>")
		for c := 0; c < 12; c++ {
			fmt.Fprintf(&t, "<td>r%d c%d</td>", r, c)
		}
		t.WriteString("</tr>")
	}
	t.WriteString("</table></body></html>")
	mux.HandleFunc("/table", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, t.String())
	})
	mux.HandleFunc("/mutating", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<!doctype html><html><head><title>Mutating</title></head><body><div id=probe>init</div><script>
		(function(){
			var p=document.getElementById('probe');
			for (var i=0;i<10;i++){ p.textContent='phase '+i; }
		})();
		</script></body></html>`)
	})
	ts := httptest.NewServer(mux)
	urls := []string{
		ts.URL + "/small",
		ts.URL + "/long",
		ts.URL + "/table",
		ts.URL + "/mutating",
	}
	return ts, urls
}

// loadFixtureDir reads HTML files from dir, registers them on the test
// server, and returns the URLs. Each file is served at /fix/<filename>.
func loadFixtureDir(ts *httptest.Server, dir string) []string {
	var urls []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".html" {
			return nil
		}
		name := filepath.Base(path)
		htmlContent, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] cannot read fixture %s: %v\n", path, err)
			return nil
		}
		// Register a handler on the test server's mux via the global
		// pattern. Since httptest.Server wraps a ServeMux, we use the
		// underlying handler. For simplicity, we serve via a closure
		// that writes the content directly.
		content := htmlContent // capture
		pattern := "/fix/" + name
		ts.Config.Handler.(*http.ServeMux).HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(content)
		})
		urls = append(urls, ts.URL+pattern)
		return nil
	})
	sort.Strings(urls)
	return urls
}

// measureRenderStages runs the renderer pipeline directly on each HTML
// fixture file and collects per-stage timings. The GOOSIE_PERF_STAGES
// environment variable must be set for stage collection to be active.
func measureRenderStages(dir string, iterations int) []renderStageResult {
	// Respect the GOOSIE_PERF_STAGES env var gate — the user controls
	// whether stage collection is active.
	if os.Getenv("GOOSIE_PERF_STAGES") == "" {
		return nil
	}

	var results []renderStageResult
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".html" {
			return nil
		}
		htmlContent, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] cannot read fixture %s: %v\n", path, err)
			return nil
		}

		// Accumulate stage totals across iterations.
		totals := make(map[string]int64)
		n := 0
		for i := 0; i < iterations; i++ {
			_, stages, err := renderer.RenderHTMLToImageWithStages(
				context.Background(), string(htmlContent), 800, 600)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[warn] render %s iteration %d failed: %v\n",
					filepath.Base(path), i, err)
				continue
			}
			if stages != nil {
				for k, v := range stages {
					totals[k] += int64(v)
				}
			}
			n++
		}

		if n == 0 {
			return nil
		}
		means := make(map[string]int64)
		for k, v := range totals {
			means[k] = v / int64(n)
		}
		results = append(results, renderStageResult{
			Fixture: filepath.Base(path),
			N:       n,
			Stages:  means,
		})
		return nil
	})
	sort.Slice(results, func(i, j int) bool { return results[i].Fixture < results[j].Fixture })
	return results
}

func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func summarizeHealth(h map[string]bool) string {
	if len(h) == 0 {
		return "no checks"
	}
	ok := 0
	for _, v := range h {
		if v {
			ok++
		}
	}
	return fmt.Sprintf("%d/%d checks passed", ok, len(h))
}

func printHuman(w io.Writer, r reviewResult) {
	fmt.Fprintln(w, "Goosie performance review")
	fmt.Fprintln(w, "==========================")
	fmt.Fprintf(w, "Started: %s\n", r.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Total:   %s\n", r.Duration)
	fmt.Fprintf(w, "Go:      %s\n\n", r.GoVersion)

	for _, wl := range r.Workloads {
		if wl.URL != "" {
			fmt.Fprintf(w, "%s  url=%s\n", wl.Name, wl.URL)
		} else {
			fmt.Fprintf(w, "%s\n", wl.Name)
		}
		for _, s := range wl.Samples {
			notes := ""
			if s.Notes != "" {
				notes = "  (" + s.Notes + ")"
			}
			errNote := ""
			if s.Errors > 0 {
				errNote = fmt.Sprintf("  errs=%d", s.Errors)
			}
			fmt.Fprintf(w,
				"  %-18s n=%-3d min=%-8s mean=%-8s p50=%-8s p95=%-8s p99=%-8s max=%-8s%s%s\n",
				s.Name,
				s.N,
				time.Duration(s.MinNS),
				time.Duration(int64(s.MeanNS)),
				time.Duration(s.P50NS),
				time.Duration(s.P95NS),
				time.Duration(s.P99NS),
				time.Duration(s.MaxNS),
				notes,
				errNote,
			)
		}
		fmt.Fprintln(w)
	}

	if len(r.FreezeHealth) > 0 {
		fmt.Fprintln(w, "Freeze-fix health checks")
		fmt.Fprintln(w, "------------------------")
		keys := make([]string, 0, len(r.FreezeHealth))
		for k := range r.FreezeHealth {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			mark := "OK"
			if !r.FreezeHealth[k] {
				mark = "DEGRADED"
			}
			fmt.Fprintf(w, "  %-30s  %s\n", k, mark)
		}
		fmt.Fprintf(w, "\n  Summary: %s\n", r.HealthSummary)
	}

	if len(r.RenderStages) > 0 {
		fmt.Fprintln(w, "Render pipeline stages")
		fmt.Fprintln(w, "----------------------")
		for _, rs := range r.RenderStages {
			fmt.Fprintf(w, "  %s  (n=%d)\n", rs.Fixture, rs.N)
			// Sort stage names for deterministic output.
			names := make([]string, 0, len(rs.Stages))
			for k := range rs.Stages {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Fprintf(w, "    %-20s %s\n", name, time.Duration(rs.Stages[name]))
			}
		}
	}
}
