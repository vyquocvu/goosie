package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// Canvas 2D tests

func TestCanvasConstructor(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D(400, 300);
		var w = ctx.canvas;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasFillRect(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D();
		ctx.fillRect(10, 20, 100, 50);
		ctx.fillStyle = '#ff0000';
		ctx.fillRect(0, 0, 50, 50);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasStrokeRect(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D();
		ctx.strokeStyle = '#00ff00';
		ctx.lineWidth = 2;
		ctx.strokeRect(10, 10, 80, 60);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasPath(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D();
		ctx.beginPath();
		ctx.moveTo(10, 10);
		ctx.lineTo(100, 10);
		ctx.lineTo(100, 100);
		ctx.closePath();
		ctx.fill();
		ctx.stroke();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasArc(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D();
		ctx.beginPath();
		ctx.arc(50, 50, 25, 0, Math.PI * 2, false);
		ctx.fill();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasText(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D();
		ctx.font = '20px Arial';
		ctx.fillText('Hello', 10, 50);
		ctx.strokeText('World', 10, 80);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasTransform(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new CanvasRenderingContext2D();
		ctx.save();
		ctx.translate(50, 50);
		ctx.rotate(Math.PI / 4);
		ctx.scale(2, 2);
		ctx.fillRect(0, 0, 10, 10);
		ctx.restore();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCanvasOnWindow(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ctx = new window.CanvasRenderingContext2D();
		ctx.fillRect(0, 0, 10, 10);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// MutationObserver tests

func TestMutationObserverConstructor(t *testing.T) {
	doc := dom.NewDocument()
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var called = false;
		var observer = new MutationObserver(function(mutations) {
			called = true;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMutationObserverObserve(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.NewElement("body")
	doc.Body = body
	doc.Node.AppendChild(body)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new MutationObserver(function(mutations) {});
		observer.observe(document.body, { childList: true });
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMutationObserverDisconnect(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.NewElement("body")
	doc.Body = body
	doc.Node.AppendChild(body)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new MutationObserver(function(mutations) {});
		observer.observe(document.body, { childList: true });
		observer.disconnect();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMutationObserverTakeRecords(t *testing.T) {
	doc := dom.NewDocument()
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new MutationObserver(function(mutations) {});
		var records = observer.takeRecords();
		var len = records.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("len")
	if !ok {
		t.Fatal("len not found")
	}
	if val.Num != 0 {
		t.Errorf("records.length = %v, want 0", val.Num)
	}
}

func TestMutationObserverOnWindow(t *testing.T) {
	doc := dom.NewDocument()
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new window.MutationObserver(function(mutations) {});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ResizeObserver tests

func TestResizeObserverConstructor(t *testing.T) {
	doc := dom.NewDocument()
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var called = false;
		var observer = new ResizeObserver(function(entries) {
			called = true;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResizeObserverObserve(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.NewElement("body")
	doc.Body = body
	doc.Node.AppendChild(body)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new ResizeObserver(function(entries) {});
		observer.observe(document.body);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResizeObserverUnobserve(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.NewElement("body")
	doc.Body = body
	doc.Node.AppendChild(body)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new ResizeObserver(function(entries) {});
		observer.observe(document.body);
		observer.unobserve(document.body);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResizeObserverDisconnect(t *testing.T) {
	doc := dom.NewDocument()
	body := doc.NewElement("body")
	doc.Body = body
	doc.Node.AppendChild(body)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new ResizeObserver(function(entries) {});
		observer.observe(document.body);
		observer.disconnect();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestResizeObserverOnWindow(t *testing.T) {
	doc := dom.NewDocument()
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
		DOM:     doc,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new window.ResizeObserver(function(entries) {});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}
