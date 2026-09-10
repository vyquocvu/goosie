package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/platform"
)

// report is what a run leaves behind: three greppable lines on stderr and, with -out,
// the recorder's own JSON artifact.
//
// The split is deliberate. The JSON is the timing record - the frame marks and the
// summary a pacing gate reads - and it is written by the instrument that produced it,
// so the numbers in the file cannot disagree with the ring. The lines on stderr are
// the counters and the environment, which the frame recorder has no business knowing:
// they describe the whole pipeline and the machine it ran on, and a wall-clock gate on
// one runner is meaningless to the next without them.
func (f *framePath) report(d *driver) error {
	c := f.config
	rep := f.rec.Report()
	st := f.sched.Stats()
	ls := f.loop.Stats()
	ps := f.pool.Stats()

	elapsed := time.Since(f.started)
	name, interactive := platform.Available()
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
	// The window's own geometry, not the one that was configured. A surface that ended
	// up a different shape measured a different frame path, and every other number in
	// this report would look exactly as good either way.
	actual := c.devSize()
	if sz, ok := f.window.(platform.Sizer); ok && !sz.Size().Empty() {
		actual = sz.Size()
	}
	stamped := -1
	if d != nil {
		stamped = d.Frames()
	}

	fmt.Fprintf(os.Stderr, "goosie: run backend=%s requested=%q interactive=%t frames=%d stamped=%d elapsed_s=%.2f fps=%.1f scene=%s size=%dx%d window=%dx%d dpr=%g\n",
		name, backend, interactive, rep.Frames, stamped, elapsed.Seconds(), fps, c.scene,
		c.devSize().W, c.devSize().H, actual.W, actual.H, c.dpr)
	fmt.Fprintf(os.Stderr, "goosie: timings mean_ms=%.3f p50_ms=%.3f p99_ms=%.3f max_ms=%.3f zero_work_frames=%d\n",
		rep.MeanMS, rep.P50MS, rep.P99MS, rep.MaxMS, rep.ZeroWorkFrames)
	fmt.Fprintf(os.Stderr, "goosie: counters rasterized=%d reused=%d failed=%d refused=%d deferred=%d panics=%d writes=%d bytes=%d budget=%d miss_pct=%.2f presents=%d idle=%d dropped_vsyncs=%d plans_published=%d plans_superseded=%d\n",
		st.Rasterized, st.Reused, st.Failed, st.Refused, st.Deferred, ps.Panics, st.Writes,
		st.Bytes, st.Budget, miss, ls.Presents, ls.Idle, dropped,
		f.sched.PlanStats().Published, f.sched.PlanStats().Dropped)
	fmt.Fprintf(os.Stderr, "goosie: env goos=%s goarch=%s go=%s cpu=%d workers=%d\n",
		runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), ps.Workers)
	// What the display did with the frames, told apart from what the loop drew. A run
	// whose commits did not keep up with its presents is a run whose mean is a
	// measurement of work nobody saw.
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
