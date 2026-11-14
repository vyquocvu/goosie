package engine_test

import (
	"sync"
	"testing"
	"time"
)

func TestPumpJSCloseIsSerialized(t *testing.T) {
	html := `<html><body><script>
	var n = 0;
	setInterval(function() {
		var acc = 0;
		for (var i = 0; i < 20000; i++) { acc = (acc + i) % 100000; }
		n++;
		window.__raceProbe = acc + n;
	}, 0);
</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				sess.PumpJS(800)
			}
		}
	}()

	time.Sleep(5 * time.Millisecond)
	sess.Close()
	close(stop)
	wg.Wait()
}

func TestAccessibilityTreeSerializesSelectNormalization(t *testing.T) {
	html := `<html><body>
<select id="s">
  <option value="a">Alpha</option>
  <option value="b" selected>Beta</option>
</select>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	sel := sess.Doc.ElementByID("s")
	if sel == nil || !sess.FocusNode(sel) {
		t.Fatal("setup: could not focus the select")
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			sess.AccessibilityTree()
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			sess.FocusedSelectValue()
		}
	}()
	close(start)
	wg.Wait()
}
