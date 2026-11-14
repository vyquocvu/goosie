package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/vyquocvu/goosie/internal/platform"
)

func (f *framePath) report(d *driver) error {
	c := f.config
	grid := f.sched.GridStats()
	f.rec.SetTileLedger(grid)
	f.rec.SetContent(f.document, int64(f.docHeight))
	rep := f.rec.Report()
	st := f.sched.Stats()
	ls := f.loop.Stats()
	ps := f.pool.Stats()

	elapsed := time.Since(f.started)
	name := platform.WindowName(f.window)
	interactive := name != platform.Headless
	backend := c.backend
	if backend == "" {
		backend = "auto"
	}
	fps := 0.0
	if elapsed > 0 {
		fps = float64(rep.Frames) / elapsed.Seconds()
	}
	miss := 0.0
	if st.Rasterized+st.Reused > 0 {
		miss = 100 * float64(st.Rasterized) / float64(st.Rasterized+st.Reused)
	}
	dropped := int64(-1)
	if dv, ok := f.window.(platform.DroppedVsyncer); ok {
		dropped = dv.DroppedVsyncs()
	}
	actual := c.devSize()
	if sz, ok := f.window.(platform.Sizer); ok && !sz.Size().Empty() {
		actual = sz.Size()
	}
	stamped := -1
	if d != nil {
		stamped = d.Frames()
	}
	startup := "startup_ms=not_measured"
	if rep.StartupMS != nil {
		startup = fmt.Sprintf("startup_ms=%.3f", *rep.StartupMS)
	}
	doc := ""
	if f.document != "" {
		doc = fmt.Sprintf(" document=%q", f.document)
	}

	fmt.Fprintf(os.Stderr, "goosie: run backend=%s requested=%q interactive=%t frames=%d stamped=%d elapsed_s=%.2f fps=%.1f scene=%s size=%dx%d window=%dx%d dpr=%g %s%s\n",
		name, backend, interactive, rep.Frames, stamped, elapsed.Seconds(), fps, c.scene,
		c.devSize().W, c.devSize().H, actual.W, actual.H, c.dpr, startup, doc)
	fmt.Fprintf(os.Stderr, "goosie: timings mean_ms=%.3f p50_ms=%.3f p99_ms=%.3f max_ms=%.3f present_mean_ms=%.3f present_p99_ms=%.3f zero_work_frames=%d\n",
		rep.MeanMS, rep.P50MS, rep.P99MS, rep.MaxMS, rep.PresentMeanMS, rep.PresentP99MS, rep.ZeroWorkFrames)
	fmt.Fprintf(os.Stderr, "goosie: counters rasterized=%d reused=%d failed=%d refused=%d deferred=%d panics=%d writes=%d bytes=%d budget=%d evictions=%d miss_pct=%.2f presents=%d idle=%d dropped_vsyncs=%d plans_published=%d plans_superseded=%d\n",
		st.Rasterized, st.Reused, st.Failed, st.Refused, st.Deferred, ps.Panics, st.Writes,
		grid.Bytes, grid.Budget, grid.Evictions, miss, ls.Presents, ls.Idle, dropped,
		f.sched.PlanStats().Published, f.sched.PlanStats().Dropped)
	fmt.Fprintf(os.Stderr, "goosie: env goos=%s goarch=%s go=%s cpu=%d workers=%d\n",
		runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), ps.Workers)
	if pc, ok := f.window.(platform.PresentCounter); ok {
		queued, pdropped, committed := pc.Presents()
		fmt.Fprintf(os.Stderr, "goosie: present queued=%d dropped=%d committed=%d\n", queued, pdropped, committed)
	}

	if c.out == "" {
		return nil
	}
	if c.out == "-" {
		return f.rec.WriteJSON(os.Stdout)
	}
	fh, err := os.Create(c.out)
	if err != nil {
		return fmt.Errorf("goosie: -out: %w", err)
	}
	if err := f.rec.WriteJSON(fh); err != nil {
		_ = fh.Close()
		return fmt.Errorf("goosie: -out: %w", err)
	}
	return fh.Close()
}
