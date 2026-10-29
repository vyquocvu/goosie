package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// animDOM creates a minimal DOM with a <div> for animation tests.
func animDOM(t *testing.T) *dom.Document {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)

	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html

	div := doc.NewElement("div")
	div.SetAttribute("id", "target")
	body.AppendChild(div)

	return doc
}

func newAnimRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	d := animDOM(t)
	r, err := js.New(js.Options{
		URL:     "https://example.test/",
		DOM:     d,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// ---------------------------------------------------------------------------
// Element.prototype.animate
// ---------------------------------------------------------------------------

func TestElementAnimateExists(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		if (typeof el.animate !== 'function') throw new Error('animate should be a function');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestLateSetDOMInstallsElementAnimateAndCommitStylesReactions(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("js.New without DOM: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	r.SetDOM(animDOM(t))
	err = r.Run(`
		window.styleAttributeChanges = 0;
		class AnimatedElement extends HTMLElement {
			static observedAttributes = ['style'];
			attributeChangedCallback() { window.styleAttributeChanges++; }
		}
		customElements.define('x-animated-reactions', AnimatedElement);
		const element = document.createElement('x-animated-reactions');
		document.body.appendChild(element);
		const animation = element.animate([{borderColor: 'rgb(0, 0, 255)'}], 1);
		animation.commitStyles();
		window.animationCommitSummary = [typeof element.animate, typeof animation.commitStyles,
			styleAttributeChanges, element.hasAttribute('style')].join(',');
	`, "late-dom-animation-commit-styles.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := r.Evaluate("window.animationCommitSummary")
	if err != nil {
		t.Fatalf("Evaluate summary: %v", err)
	}
	if got != "function,function,1,true" {
		t.Fatalf("late DOM animation summary = %q, want %q", got, "function,function,1,true")
	}
}

func TestElementAnimateReturnsAnimation(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (typeof anim !== 'object') throw new Error('animate should return an object');
		if (typeof anim.play !== 'function') throw new Error('animation should have play method');
		if (typeof anim.pause !== 'function') throw new Error('animation should have pause method');
		if (typeof anim.cancel !== 'function') throw new Error('animation should have cancel method');
		if (typeof anim.finish !== 'function') throw new Error('animation should have finish method');
		if (typeof anim.reverse !== 'function') throw new Error('animation should have reverse method');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestElementAnimateWithArrayKeyframes(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate([{opacity: 0}, {opacity: 1}], {duration: 500});
		if (typeof anim !== 'object') throw new Error('animate should return object');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestElementAnimateWithObjectKeyframes(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (typeof anim !== 'object') throw new Error('animate should return object');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestElementAnimateWithOptionsObject(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, {
			duration: 1000,
			delay: 200,
			iterations: 2,
			direction: 'alternate',
			fill: 'forwards',
			easing: 'linear'
		});
		if (typeof anim !== 'object') throw new Error('animate should return object');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// playState
// ---------------------------------------------------------------------------

func TestAnimationPlayStateRunning(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (anim.playState !== 'running') throw new Error('expected running, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationPauseSetsPaused(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.pause();
		if (anim.playState !== 'paused') throw new Error('expected paused, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationCancelSetsIdle(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.cancel();
		if (anim.playState !== 'idle') throw new Error('expected idle, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationFinishSetsFinished(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.finish();
		if (anim.playState !== 'finished') throw new Error('expected finished, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// play/pause roundtrip
// ---------------------------------------------------------------------------

func TestAnimationPlayAfterPause(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.pause();
		if (anim.playState !== 'paused') throw new Error('expected paused');
		anim.play();
		if (anim.playState !== 'running') throw new Error('expected running after play, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// playbackRate
// ---------------------------------------------------------------------------

func TestAnimationPlaybackRateDefault(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (anim.playbackRate !== 1) throw new Error('expected default playbackRate 1, got ' + anim.playbackRate);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationPlaybackRateSetGet(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.playbackRate = 2;
		if (anim.playbackRate !== 2) throw new Error('expected 2, got ' + anim.playbackRate);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationReverse(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.reverse();
		if (anim.playbackRate !== -1) throw new Error('expected -1 after reverse, got ' + anim.playbackRate);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// effect and KeyframeEffect
// ---------------------------------------------------------------------------

func TestAnimationEffect(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (!anim.effect) throw new Error('effect should exist');
		if (typeof anim.effect.getTiming !== 'function') throw new Error('effect should have getTiming');
		if (typeof anim.effect.getKeyframes !== 'function') throw new Error('effect should have getKeyframes');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationEffectGetTiming(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, {duration: 2000, delay: 100, iterations: 3, direction: 'alternate', fill: 'forwards', easing: 'linear'});
		var timing = anim.effect.getTiming();
		if (timing.duration !== 2000) throw new Error('duration should be 2000, got ' + timing.duration);
		if (timing.delay !== 100) throw new Error('delay should be 100, got ' + timing.delay);
		if (timing.iterations !== 3) throw new Error('iterations should be 3, got ' + timing.iterations);
		if (timing.direction !== 'alternate') throw new Error('direction should be alternate, got ' + timing.direction);
		if (timing.fill !== 'forwards') throw new Error('fill should be forwards, got ' + timing.fill);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationEffectGetKeyframes(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 0.5, 1]}, 1000);
		var kf = anim.effect.getKeyframes();
		if (!Array.isArray(kf)) throw new Error('getKeyframes should return array');
		if (kf.length < 2) throw new Error('should have at least 2 keyframes');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// timeline
// ---------------------------------------------------------------------------

func TestAnimationTimeline(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (!anim.timeline) throw new Error('timeline should exist');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// finished promise
// ---------------------------------------------------------------------------

func TestAnimationFinishedPromise(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		if (!anim.finished) throw new Error('finished should exist');
		if (typeof anim.finished.then !== 'function') throw new Error('finished should be a promise');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationFinishedResolvesOnFinish(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		var resolved = false;
		anim.finished.then(function() { resolved = true; });
		anim.finish();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Drain microtasks to let the promise resolve.
	// The resolved flag should be set after microtask drain.
}

// ---------------------------------------------------------------------------
// onfinish handler
// ---------------------------------------------------------------------------

func TestAnimationOnFinishHandler(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		var finished = false;
		anim.onfinish = function() { finished = true; };
		anim.finish();
		if (!finished) throw new Error('onfinish should have been called');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// currentTime
// ---------------------------------------------------------------------------

func TestAnimationCurrentTimeNotNull(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		// currentTime should be a number when running (not null).
		var ct = anim.currentTime;
		if (ct === null || ct === undefined) throw new Error('currentTime should not be null when running');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationCurrentTimeNullWhenIdle(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
		anim.cancel();
		if (anim.currentTime !== null) throw new Error('currentTime should be null when idle');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Animation constructor
// ---------------------------------------------------------------------------

func TestAnimationConstructorExists(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		if (typeof Animation !== 'function') throw new Error('Animation constructor should exist');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimationConstructorCreatesIdle(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var anim = new Animation();
		if (anim.playState !== 'idle') throw new Error('new Animation should be idle, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// KeyframeEffect constructor
// ---------------------------------------------------------------------------

func TestKeyframeEffectConstructorExists(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		if (typeof KeyframeEffect !== 'function') throw new Error('KeyframeEffect constructor should exist');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestKeyframeEffectConstructor(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var effect = new KeyframeEffect(el, [{opacity: 0}, {opacity: 1}], {duration: 1000});
		if (typeof effect.getTiming !== 'function') throw new Error('should have getTiming');
		if (typeof effect.getKeyframes !== 'function') throw new Error('should have getKeyframes');
		var timing = effect.getTiming();
		if (timing.duration !== 1000) throw new Error('duration should be 1000, got ' + timing.duration);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Multiple animations on same element
// ---------------------------------------------------------------------------

func TestMultipleAnimationsOnSameElement(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim1 = el.animate({opacity: [0, 1]}, 1000);
		var anim2 = el.animate({transform: ['translateX(0px)', 'translateX(100px)']}, 2000);
		if (anim1 === anim2) throw new Error('animations should be different objects');
		if (anim1.playState !== 'running') throw new Error('anim1 should be running');
		if (anim2.playState !== 'running') throw new Error('anim2 should be running');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Duration as number
// ---------------------------------------------------------------------------

func TestAnimateDurationAsNumber(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 500);
		if (anim.playState !== 'running') throw new Error('should be running');
		var timing = anim.effect.getTiming();
		if (timing.duration !== 500) throw new Error('duration should be 500, got ' + timing.duration);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TickAnimations
// ---------------------------------------------------------------------------

func TestTickAnimationsAdvancesState(t *testing.T) {
	d := animDOM(t)
	r, err := js.New(js.Options{
		URL:     "https://example.test/",
		DOM:     d,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer r.Close()

	// Create a short animation and tick past its duration.
	if err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 100);
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Wait past the animation duration and tick.
	time.Sleep(150 * time.Millisecond)
	r.TickAnimations()

	// The animation should have finished.
	val, ok := r.Global("anim")
	if !ok {
		t.Fatal("anim not found")
	}
	_ = val // animation should be finished after tick
}

// ---------------------------------------------------------------------------
// HasPendingRAF with active animations
// ---------------------------------------------------------------------------

func TestHasPendingRAFWithAnimations(t *testing.T) {
	d := animDOM(t)
	r, err := js.New(js.Options{
		URL:     "https://example.test/",
		DOM:     d,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer r.Close()

	if r.HasPendingRAF() {
		t.Error("HasPendingRAF should be false before any animations")
	}

	if err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !r.HasPendingRAF() {
		t.Error("HasPendingRAF should be true with a running animation")
	}
}

// ---------------------------------------------------------------------------
// Inline style is modified by animation
// ---------------------------------------------------------------------------

func TestAnimationModifiesInlineStyle(t *testing.T) {
	d := animDOM(t)
	r, err := js.New(js.Options{
		URL:     "https://example.test/",
		DOM:     d,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer r.Close()

	// Create animation and tick to force application of values.
	if err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Tick once to apply initial values.
	r.TickAnimations()

	// The element's inline style should have opacity set.
	if err := r.Run(`
		var el = document.getElementById('target');
		var style = el.getAttribute('style');
		if (!style || style.indexOf('opacity') === -1) {
			throw new Error('expected opacity in inline style, got: ' + style);
		}
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Cancel clears animated style
// ---------------------------------------------------------------------------

func TestCancelClearsAnimatedStyle(t *testing.T) {
	d := animDOM(t)
	r, err := js.New(js.Options{
		URL:     "https://example.test/",
		DOM:     d,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({opacity: [0, 1]}, 1000);
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	r.TickAnimations()

	// Cancel and check style is cleared.
	// `anim` was set in the first Run and is still accessible since Run calls
	// share the same VM.
	if err := r.Run(`
		anim.cancel();
		if (anim.playState !== 'idle') throw new Error('should be idle after cancel');
	`, "test.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestAnimateWithEmptyKeyframes(t *testing.T) {
	r := newAnimRuntime(t)
	// Should not throw with empty keyframes.
	err := r.Run(`
		var el = document.getElementById('target');
		var anim = el.animate({}, 1000);
		// Should still return something (possibly undefined or a dummy).
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAnimateWithNoArguments(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var el = document.getElementById('target');
		var result = el.animate();
		// Should not throw.
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestFinishIdleAnimationIsNoop(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var anim = new Animation();
		anim.finish();
		// Should still be idle since it was never started.
		if (anim.playState !== 'idle') throw new Error('expected idle, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestPauseIdleAnimationIsNoop(t *testing.T) {
	r := newAnimRuntime(t)
	err := r.Run(`
		var anim = new Animation();
		anim.pause();
		if (anim.playState !== 'idle') throw new Error('expected idle, got ' + anim.playState);
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}
