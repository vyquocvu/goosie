package gate

import (
	"strings"
	"testing"
)

func TestZeroAllocVerdict(t *testing.T) {
	cases := []struct {
		name     string
		frames   int
		first    allocWindow
		second   allocWindow
		wantFail bool
		want     []string
		wantMsg  bool
	}{
		{
			name:   "a clean first window is the normal case, and it needs no message",
			frames: gateWarmFrames,
			first:  allocWindow{mallocs: 0},
		},
		{
			name:    "one stray that does not recur is the runtime's, not the frame path's",
			frames:  gateWarmFrames,
			first:   allocWindow{mallocs: 1, bytes: 32},
			second:  allocWindow{mallocs: 0},
			want:    []string{"1 allocations (32 bytes)", "and none in a second"},
			wantMsg: true,
		},
		{
			name:     "allocations in both windows are the frame path's own",
			frames:   gateWarmFrames,
			first:    allocWindow{avg: 1, mallocs: 600, bytes: 19200},
			second:   allocWindow{avg: 1, mallocs: 600, bytes: 19200},
			wantFail: true,
			want:     []string{"600 allocations (19200 bytes)", "in a second on the same warm path"},
			wantMsg:  true,
		},
		{
			name:     "a rate too low to average out still recurs, so it is not a stray",
			frames:   gateWarmFrames,
			first:    allocWindow{mallocs: 1, bytes: 32},
			second:   allocWindow{mallocs: 1, bytes: 32},
			wantFail: true,
			want:     []string{"1 allocations (32 bytes)", "in a second on the same warm path"},
			wantMsg:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, fail := zeroAllocVerdict("warm scroll frame", tc.frames, tc.first, tc.second)
			if fail != tc.wantFail {
				t.Errorf("fail = %v, want %v (%s)", fail, tc.wantFail, msg)
			}
			if !tc.wantMsg && msg != "" {
				t.Errorf("a clean window must report nothing, got %q", msg)
			}
			if tc.wantMsg && msg == "" {
				t.Fatal("want a report, got none")
			}
			for _, want := range tc.want {
				if !strings.Contains(msg, want) {
					t.Errorf("message lacks %q:\n%s", want, msg)
				}
			}
			if !tc.wantFail && strings.Contains(msg, "per-frame") {
				t.Errorf("a tolerated stray must not be described as per-frame work: %s", msg)
			}
		})
	}
}

func TestZeroAllocVerdictCatchesPerFrameLeak(t *testing.T) {
	const frames = 200
	leaked := make([][]byte, 0, 2*frames+2)
	path := func() { leaked = append(leaked, make([]byte, 32)) }
	_, first, _ := allocTotals(frames, path)
	if uint64(first) < frames {
		t.Fatalf("the instrument counted %d allocations across %d allocating frames, so it cannot see a leak either", first, frames)
	}
	_, second, _ := allocTotals(frames, path)
	if _, fail := zeroAllocVerdict("leaking path", frames, allocWindow{mallocs: first}, allocWindow{mallocs: second}); !fail {
		t.Errorf("a path that allocated %d and then %d times per window passed the warm criterion", first, second)
	}
}
