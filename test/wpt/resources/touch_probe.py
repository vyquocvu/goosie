# Ground-truth probe for touch-event cancelability and the TouchEvent surface.
#
# This is the touch counterpart of wheel_probe.py, and it is the measurement the
# remaining 21 red files in dom/events/non-cancelable-when-passive must be
# implemented against. Run with:
#
#     pip install playwright && playwright install chromium
#     python3 test/wpt/resources/touch_probe.py
#
# What Chromium prints:
#
#   A-D. A trusted touchstart reports the derived cancelable flag a wheel gesture
#        uses - false when the only listener is default-passive (window, document,
#        documentElement, body registered with no option) or explicitly passive,
#        true for a non-passive listener. Each case here registers one listener, so
#        the one-value-per-dispatch half of the rule is what wheel_probe.py
#        establishes, not this script. The event is a TouchEvent with one touch in
#        `touches` and `changedTouches`.
#   C. Only the touchstart row prints: a stationary tap dispatches no touchmove,
#      so the corpus files that assert on touchmove must move the pointer between
#      down and up the way resources/touching.js does.
#   E. touchend arrives with `touches` empty and one entry in `changedTouches`, and
#      is cancelable for a plain element listener. No touchcancel row printed, so
#      this probe says nothing about it; the DOM's default-passive types are
#      touchstart and touchmove, which the wheel probe's type set already matches.
#   F. A constructed `new TouchEvent("touchstart")` is non-cancelable and
#      non-bubbling, exposes touches/targetTouches/changedTouches, and dispatchEvent
#      returns true because a default-passive window listener cannot prevent it.
from playwright.sync_api import sync_playwright

PAGE = """<!doctype html><html><head></head><body>
<div id="touchDiv" style="width:200px;height:200px;background:#ccc"></div>
<script>
window.__report = [];
function probe(label, target, options, type) {
  target.addEventListener(type, function (event) {
    window.__report.push(label + '/' + type + ' cancelable=' + event.cancelable +
      ' trusted=' + event.isTrusted + ' ctor=' + event.constructor.name +
      ' touches=' + event.touches.length + ' changed=' + event.changedTouches.length);
  }, options);
}
%s
</script>
</body></html>"""

CASES = {
    "A touchstart window {} (default passive)":
      "probe('window', window, {}, 'touchstart');",
    "B touchstart window {passive:false}":
      "probe('window', window, {passive:false}, 'touchstart');",
    "C touchstart div {passive:false} + touchmove":
      "probe('div', document.getElementById('touchDiv'), {passive:false}, 'touchstart');"
      "probe('div', document.getElementById('touchDiv'), {passive:false}, 'touchmove');",
    "D touchstart div {passive:true}":
      "probe('div', document.getElementById('touchDiv'), {passive:true}, 'touchstart');",
    "E touchend/touchcancel listeners, no option":
      "probe('div', document.getElementById('touchDiv'), {}, 'touchend');"
      "probe('div', document.getElementById('touchDiv'), {}, 'touchcancel');",
}

with sync_playwright() as pw:
    browser = pw.chromium.launch()
    for name, setup in CASES.items():
        ctx = browser.new_context(viewport={"width": 800, "height": 600}, has_touch=True)
        page = ctx.new_page()
        page.set_content(PAGE % setup)
        page.touchscreen.tap(100, 100)
        page.wait_for_timeout(300)
        print("=== %-45s %s" % (name, " || ".join(page.evaluate("__report")) or "NO EVENT"))
        ctx.close()
    ctx = browser.new_context(viewport={"width": 800, "height": 600}, has_touch=True)
    page = ctx.new_page()
    page.set_content("<div id=x></div>")
    print("=== F constructed TouchEvent")
    print(page.evaluate("""() => {
      const out = [];
      const e = new TouchEvent('touchstart');
      out.push('cancelable=' + e.cancelable + ' bubbles=' + e.bubbles + ' ctor=' + e.constructor.name);
      out.push('hasTouches=' + ('touches' in e) + ' hasChanged=' + ('changedTouches' in e) +
        ' hasTargetTouches=' + ('targetTouches' in e));
      window.addEventListener('touchstart', ev => {
        out.push('listener prevented=' + ev.defaultPrevented);
      });
      out.push('returned=' + dispatchEvent(e));
      return out.join(' | ');
    }"""))
    browser.close()
