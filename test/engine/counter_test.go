package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

const counterDoc = `<html><head><style>
body, #test span:first-child { counter-reset: c; }
p, #test span { counter-increment: c; }
#test span::before { content: counters(c, ".", upper-roman); }
</style></head><body><p>x</p><div id="test">` +
	`<span></span><span></span><span></span><span></span><span></span><span></span>` +
	`<span></span><span></span><span></span><span></span><span></span><span></span>` +
	`<span style="counter-reset: c 48"></span><span></span>` +
	`<span style="counter-reset: c 388"></span><span></span>` +
	`<span style="counter-reset: c 3488"></span><span></span><span></span>` +
	`</div></body></html>`

func counterSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return sess
}

func TestCountersJoinNestedScopes(t *testing.T) {
	sess := counterSession(t, counterDoc)
	defer sess.Close()
	for _, want := range []string{"I.I", "II", "XII", "XII.XLIX", "XIII.CCCLXXXIX", "XIV.MMMCDLXXXIX", "XVI"} {
		if got := sess.Find(want); len(got) == 0 {
			t.Errorf("Find(%q) found nothing; generated counters missing", want)
		}
	}
}

func TestCounterFunctionTextNeverLeaks(t *testing.T) {
	sess := counterSession(t, counterDoc)
	defer sess.Close()
	if got := sess.Find("counters("); len(got) != 0 {
		t.Errorf("Find(counters() found %d matches: raw function text leaked", len(got))
	}
}

func TestGeorgianCounters(t *testing.T) {
	html := `<html><head><style>
div span { counter-increment: c; }
div span::before { content: counter(c, georgian); }
</style></head><body><div>` +
		`<span style="counter-reset: c 19"></span>` +
		`<span style="counter-reset: c 59"></span>` +
		`<span style="counter-reset: c 999"></span>` +
		`<span style="counter-reset: c 19998"></span>` +
		`</div></body></html>`
	sess := counterSession(t, html)
	defer sess.Close()
	for _, want := range []string{"კ", "ჲ", "ჩ", "ჵჰშჟთ"} {
		if got := sess.Find(want); len(got) == 0 {
			t.Errorf("Find(%q) found nothing", want)
		}
	}
}

func TestCounterScopesAreIndependent(t *testing.T) {
	html := `<html><head><style>
div.a, div.b { counter-reset: c; }
div span { counter-increment: c; }
div span::before { content: counter(c); }
</style></head><body><div class="a"><span></span></div><div class="b"><span></span></div></body></html>`
	sess := counterSession(t, html)
	defer sess.Close()
	if got := sess.Find("2"); len(got) != 0 {
		t.Errorf("Find(2) found %d matches: sibling scopes leaked", len(got))
	}
	if got := sess.Find("1"); len(got) == 0 {
		t.Error("Find(1) found nothing: independent scopes render 1 each")
	}
}

func TestMissingCounterRendersZero(t *testing.T) {
	html := `<html><head><style>span::before { content: counter(zzz); }</style></head><body><span>q</span></body></html>`
	sess := counterSession(t, html)
	defer sess.Close()
	if got := sess.Find("0"); len(got) == 0 {
		t.Error("Find(0) found nothing: missing counter must render 0")
	}
}
