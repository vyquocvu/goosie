# Ground-truth probe for scroll-blocking event cancelability.
#
# The oracles in test/wpt/testharness_wheel_test.go, and the ones in test/dom and
# test/js/scroll_blocking_cancelable_test.go, are what Chromium prints here. Run
# with:
#
#     pip install playwright && playwright install chromium
#     python3 test/wpt/resources/wheel_probe.py
#
# What it shows (trusted cases are one mouse wheel gesture of deltaY 100 at the
# centre of an 800x600 viewport over a 2000px tall div):
#
#   A. All five default targets plus a plain element, no passive option: every
#      listener reads cancelable=true - one value for the whole dispatch, not one
#      per listener - only "wheel" fires, and scrollY moves by the delta.
#   B. Default-passive targets only, for both wheel and mousewheel: wheel is
#      dispatched with cancelable=false and the legacy event is not dispatched.
#   C. A non-passive listener that calls preventDefault(): cancelable=true and
#      scrollY stays 0.
#   D. Only a mousewheel listener: the legacy event is dispatched, and its own
#      passive flag decides cancelable.
#   E. A passive wheel listener and a non-passive mousewheel listener on the same
#      element: wheel only, cancelable=true - the mousewheel listener makes the
#      gesture preventable even though the wheel listener cannot.
#   F. A default-passive wheel listener on window and a non-passive mousewheel
#      listener on the element: Chromium dispatches BOTH, legacy first. Goosie
#      dispatches only wheel, because it treats any wheel listener in the path as
#      the page having opted into the modern event. No test in the pinned
#      non-cancelable-when-passive corpus registers two types at once, so this is
#      a recorded divergence rather than a gap.
#   G-J. Page-constructed events (dispatchEvent of new Event("wheel",
#      {cancelable:true})) keep cancelable=true even for a passive listener,
#      which simply cannot prevent them. This is the case the engine must NOT
#      apply the generated-event rule to.
from playwright.sync_api import sync_playwright

TRUSTED = """<!doctype html><html><head></head><body>
<div id="box" style="height:2000px"></div>
<script>
window.__report = [];
function probe(label, target, options, type) {
  target.addEventListener(type, function (event) {
    if (window.cancelIn) event.preventDefault();
    window.__report.push(label + '/' + type + ':' + event.isTrusted + ':' + event.deltaY + ':' +
      event.clientX + ':' + event.clientY + ':' + event.cancelable + ':' + event.wheelDelta + ':' +
      event.deltaMode + ':' + event.constructor.name);
  }, options);
}
%s
</script>
</body></html>"""

SYNTHETIC = """<!doctype html><html><head></head><body>
<div id="box" style="height:2000px"></div>
<script>
window.__report = [];
%s
</script>
</body></html>"""

TRUSTED_CASES = {
    "A default-passive globals + non-passive div": """
      probe('window', window, {}, 'wheel');
      probe('document', document, {}, 'wheel');
      probe('root', document.documentElement, {}, 'wheel');
      probe('body', document.body, {}, 'wheel');
      probe('div', document.getElementById('box'), {}, 'wheel');
    """,
    "B default-passive globals only": """
      probe('window', window, {}, 'wheel');
      probe('window', window, {}, 'mousewheel');
    """,
    "C non-passive div cancels the gesture": """
      window.cancelIn = true;
      probe('div', document.getElementById('box'), {passive: false}, 'wheel');
    """,
    "D mousewheel listener only": """
      probe('div', document.getElementById('box'), {passive: false}, 'mousewheel');
    """,
    "E passive wheel + non-passive mousewheel, same div": """
      probe('div', document.getElementById('box'), {passive: true}, 'wheel');
      probe('div', document.getElementById('box'), {}, 'mousewheel');
    """,
    "F default-passive wheel on window + non-passive mousewheel on div": """
      probe('window', window, {}, 'wheel');
      probe('div', document.getElementById('box'), {}, 'mousewheel');
    """,
}

SYNTHETIC_CASES = {
    "G synthetic wheel, div {passive:true}": """
      var e = new Event('wheel', {cancelable:true});
      document.getElementById('box').addEventListener('wheel', function (ev) {
        window.__report.push('cancelable=' + ev.cancelable);
        ev.preventDefault();
        window.__report.push('prevented=' + ev.defaultPrevented);
      }, {passive:true});
      window.__report.push('returned=' + document.getElementById('box').dispatchEvent(e));
    """,
    "H synthetic wheel on window, listener {}": """
      var e = new Event('wheel', {cancelable:true});
      window.addEventListener('wheel', function (ev) {
        window.__report.push('cancelable=' + ev.cancelable);
        ev.preventDefault();
        window.__report.push('prevented=' + ev.defaultPrevented);
      }, {});
      window.__report.push('returned=' + window.dispatchEvent(e));
    """,
    "I synthetic wheel, div {passive:false}": """
      var e = new Event('wheel', {cancelable:true});
      document.getElementById('box').addEventListener('wheel', function (ev) {
        window.__report.push('cancelable=' + ev.cancelable);
        ev.preventDefault();
        window.__report.push('prevented=' + ev.defaultPrevented);
      }, {passive:false});
      window.__report.push('returned=' + document.getElementById('box').dispatchEvent(e));
    """,
    "J synthetic wheel, no listeners": """
      var e = new Event('wheel', {cancelable:true});
      document.getElementById('box').dispatchEvent(e);
      window.__report.push('after=' + e.cancelable);
    """,
}


def report(page, rows, scroll):
    print("  scrollY=%s" % scroll)
    for row in rows:
        print("  %s" % row)


with sync_playwright() as pw:
    browser = pw.chromium.launch()
    for name, setup in TRUSTED_CASES.items():
        page = browser.new_page(viewport={"width": 800, "height": 600})
        page.set_content(TRUSTED % setup)
        page.mouse.move(400, 300)
        page.mouse.wheel(0, 100)
        page.wait_for_timeout(250)
        print("=== %s ===" % name)
        report(page, page.evaluate("__report"), page.evaluate("window.scrollY"))
        page.close()
    for name, script in SYNTHETIC_CASES.items():
        page = browser.new_page(viewport={"width": 800, "height": 600})
        page.set_content(SYNTHETIC % script)
        print("=== %s ===" % name)
        report(page, page.evaluate("__report"), page.evaluate("window.scrollY"))
        page.close()
    browser.close()
