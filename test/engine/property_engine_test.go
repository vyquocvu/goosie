package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
)

func TestEngine_PropertyAndStartingStyleSession(t *testing.T) {
	html := `<!DOCTYPE html>
	<html>
	<head>
		<style>
			@property --card-bg {
				syntax: '<color>';
				inherits: false;
				initial-value: rgb(255, 255, 255);
			}

			.card {
				--card-bg: rgb(200, 100, 50);
				background-color: var(--card-bg);
				opacity: 1;
				width: 200px;
				height: 100px;
			}

			.card-inner {
				/* --card-bg has inherits: false, so it falls back to initial-value rgb(255, 255, 255) */
				background-color: var(--card-bg);
				width: 50px;
				height: 50px;
			}

			dialog {
				opacity: 1;
				width: 100px;
				height: 50px;
			}

			@starting-style {
				dialog {
					opacity: 0;
				}
			}
		</style>
	</head>
	<body>
		<div id="card" class="card">
			<div id="inner" class="card-inner"></div>
		</div>
		<dialog id="dlg"></dialog>
	</body>
	</html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("engine.NewSession failed: %v", err)
	}

	cardNode := sess.Doc.ElementByID("card")
	if cardNode == nil {
		t.Fatalf("card element not found")
	}
	innerNode := sess.Doc.ElementByID("inner")
	if innerNode == nil {
		t.Fatalf("inner element not found")
	}
	dlgNode := sess.Doc.ElementByID("dlg")
	if dlgNode == nil {
		t.Fatalf("dlg element not found")
	}

	cardStyle := sess.Styles[cardNode.ID]
	if cardStyle == nil {
		t.Fatalf("cardStyle is nil")
	}
	wantCardBg := css.Color{R: 200, G: 100, B: 50, A: 255}
	if cardStyle.BackgroundColor != wantCardBg {
		t.Errorf("card background-color = %+v, want %+v", cardStyle.BackgroundColor, wantCardBg)
	}

	innerStyle := sess.Styles[innerNode.ID]
	if innerStyle == nil {
		t.Fatalf("innerStyle is nil")
	}
	// Since --card-bg inherits: false, inner receives initial-value white
	wantInnerBg := css.Color{R: 255, G: 255, B: 255, A: 255}
	if innerStyle.BackgroundColor != wantInnerBg {
		t.Errorf("inner background-color = %+v, want %+v (initial-value white)", innerStyle.BackgroundColor, wantInnerBg)
	}

	dlgStyle := sess.Styles[dlgNode.ID]
	if dlgStyle == nil {
		t.Fatalf("dlgStyle is nil")
	}
	if dlgStyle.Starting == nil || !dlgStyle.Starting.HasOpacity || dlgStyle.Starting.Opacity != 0 {
		t.Errorf("dialog starting opacity = %+v, want opacity 0", dlgStyle.Starting)
	}

	// Verify PaintChecked works smoothly with DPR 1.0
	displayList, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	if displayList == nil {
		t.Fatalf("displayList is nil")
	}
}
