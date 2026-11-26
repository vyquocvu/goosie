package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// qc:BR-330
func TestAsyncGeneratorSyntaxIsAccepted(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`
		var compatible = false;
		new Function('try { async function* x() {} } catch (e) { compatible = false; throw e; }')();
		compatible = true;
	`, ""); err != nil {
		t.Fatalf("async generator syntax must compile: %v", err)
	}
}
