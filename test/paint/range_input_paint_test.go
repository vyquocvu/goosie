package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestRangeInputPaintTracksLiveValue(t *testing.T) {
	thumbRight := func(value string) int32 {
		t.Helper()
		html := `<!doctype html><html><head><style>input { display:inline-block; width:280px; height:20px; }</style></head><body style="margin:0"><input type="range" min="0" max="100" step="20" value="` + value + `"></body></html>`
		sess, err := engine.NewSession(html, nil, 400, engine.WithViewportH(100))
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		defer sess.Close()
		dl, err := sess.PaintChecked(1)
		if err != nil {
			t.Fatalf("PaintChecked: %v", err)
		}
		var maxX int32
		found := false
		for _, cmd := range dl.Build(1).All() {
			if cmd.Kind != paint.CmdFill || cmd.Color.R() != 10 || cmd.Color.G() != 132 || cmd.Color.B() != 255 {
				continue
			}
			if !found || cmd.Rect.X1 > maxX {
				maxX = cmd.Rect.X1
				found = true
			}
		}
		if !found {
			t.Fatal("range control emitted no active track or thumb fills")
		}
		return maxX
	}

	low := thumbRight("20")
	high := thumbRight("80")
	if high <= low {
		t.Fatalf("range thumb did not track its live value: x1(20)=%d x1(80)=%d", low, high)
	}
}

func TestRangeInputPaintsEnabledInRangeDatalistTicks(t *testing.T) {
	html := `<!doctype html><html><head><style>input { display:inline-block; width:280px; height:20px; }</style></head><body style="margin:0">
		<input type="range" min="0" max="100" value="50" list="marks">
		<datalist id="marks">
			<option value="20"><option disabled value="30"><option value="40"><option value="70"><option value="120">
		</datalist>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 400, engine.WithViewportH(100))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var tickXs []int32
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdFill && cmd.Color.R() == 16 && cmd.Color.G() == 16 && cmd.Color.B() == 16 && cmd.Rect.Y1-cmd.Rect.Y0 == 4 {
			tickXs = append(tickXs, cmd.Rect.X0)
		}
	}
	if len(tickXs) != 3 {
		t.Fatalf("range datalist tick marks = %v, want enabled in-range options 20, 40, 70", tickXs)
	}
	if !(tickXs[0] < tickXs[1] && tickXs[1] < tickXs[2]) {
		t.Fatalf("range datalist ticks are not ordered by value: %v", tickXs)
	}
}
