// BR-101 reference probe: what does Chromium report when a POINTER chooses an
// option in a <select>?
//
// A closed <select> opens an OS-native popup that Playwright cannot observe, so
// the probe drives the in-page form of the same control - <select size="3">, a
// listbox whose rows Chromium lays out and paints inside the page. Row rects
// come from getBoundingClientRect, the choice is made with page.mouse.click at a
// measured row centre, and the outcome is read from value, selectedIndex, the
// `selected` content attribute, activeElement and the event log.
//
// Every assertion in test/engine/select_popup_test.go and
// test/paint/select_popup_test.go cites a line of this script's output. Do not
// encode a rule from memory - re-run and read:
//   node scripts/verify-br101-select-popup.js
const { chromium } = require('playwright');

function show(label, obj) {
  console.log(label + ' -> ' + JSON.stringify(obj));
}

const HTML = `<!doctype html><html><head><style>
html,body{margin:0;padding:0}
#lb{position:absolute;left:40px;top:60px;width:180px;font-size:16px}
#other{position:absolute;left:40px;top:300px;width:200px;height:40px;background:#ddd}
</style></head><body>
<select id="lb" name="pick" size="3">
  <option id="a" value="a">Alpha</option>
  <option id="b" value="b" selected>Bravo</option>
  <option id="c" value="c" disabled>Charlie</option>
</select>
<div id="other"></div>
</body></html>`;

async function probe(page) {
  const rects = await page.evaluate(() => {
    const out = {};
    for (const id of ['lb', 'a', 'b', 'c', 'other']) {
      const r = document.getElementById(id).getBoundingClientRect();
      out[id] = [r.x, r.y, r.width, r.height];
    }
    return out;
  });
  show('RECTS', rects);

  const snap = () =>
    page.evaluate(() => ({
      value: document.getElementById('lb').value,
      index: document.getElementById('lb').selectedIndex,
      selectedAttr: ['a', 'b', 'c'].map((id) =>
        document.getElementById(id).hasAttribute('selected')),
      active: document.activeElement.id,
      activeTag: document.activeElement.tagName.toLowerCase(),
    }));
  show('INITIAL', await snap());

  // Row B is already selected: clicking it must be a no-op selection that does
  // not fire change, because the value did not change.
  await page.evaluate(() => {
    window.__ev = [];
    const lb = document.getElementById('lb');
    const tag = (e) => window.__ev.push(
      `${e.currentTarget.id || e.currentTarget.tagName.toLowerCase()}:${e.type}` +
      (e.eventPhase === 1 ? '(capture)' : '') + ` value=${lb.value}`);
    for (const t of ['input', 'change', 'focus', 'blur']) {
      lb.addEventListener(t, tag);
    }
    for (const t of ['mousedown', 'mouseup', 'click']) {
      lb.addEventListener(t, tag);
      lb.addEventListener(t, tag, true);
      for (const id of ['a', 'b', 'c']) {
        document.getElementById(id).addEventListener(t, tag);
      }
    }
  });
  const rowY = (id) => rects[id][1] + rects[id][3] / 2;
  const rowX = rects.lb[0] + 10;

  await page.mouse.click(rowX, rowY('a'));
  show('CLICK-A', await snap());
  show('EVENTS-CLICK-A', await page.evaluate(() => window.__ev));

  await page.evaluate(() => (window.__ev = []));
  await page.mouse.click(rowX, rowY('c'));
  show('CLICK-C-DISABLED', await snap());
  show('EVENTS-CLICK-C', await page.evaluate(() => window.__ev));

  await page.evaluate(() => (window.__ev = []));
  await page.mouse.click(rowX, rowY('b'));
  show('CLICK-B-ALREADY-SELECTED', await snap());
  show('EVENTS-CLICK-B', await page.evaluate(() => window.__ev));

  // Pointer down outside the control: where does focus go?
  await page.mouse.click(rects.other[0] + 20, rects.other[1] + 20);
  show('CLICK-OUTSIDE', await snap());

  // Escape on a focused listbox: does it change the value or move focus?
  await page.mouse.click(rowX, rowY('a'));
  const beforeEscape = await snap();
  await page.keyboard.press('Escape');
  show('BEFORE-ESCAPE', beforeEscape);
  show('AFTER-ESCAPE', await snap());

  // Does a key press that types a label select by prefix, and does the pointer
  // path share the event sequence? Compare with a keyboard-only change.
  await page.evaluate(() => {
    window.__ev = [];
    document.getElementById('lb').focus();
  });
  await page.keyboard.press('ArrowDown');
  show('KEYDOWN-ARROWDOWN', await snap());
  show('EVENTS-ARROWDOWN', await page.evaluate(() => window.__ev));
}

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 600 }, deviceScaleFactor: 1 });
  await page.setContent(HTML, { waitUntil: 'load' });
  await probe(page);

  // A closed select: what is observable when it is clicked?
  const closedHTML = `<!doctype html><html><head><style>
html,body{margin:0}
#s{position:absolute;left:40px;top:60px;font-size:16px}
</style></head><body><select id="s"><option value="1">One</option><option value="2" selected>Two</option></select></body></html>`;
  const p2 = await browser.newPage({ viewport: { width: 800, height: 600 }, deviceScaleFactor: 1 });
  await p2.setContent(closedHTML, { waitUntil: 'load' });
  await p2.evaluate(() => {
    window.__ev = [];
    const s = document.getElementById('s');
    for (const t of ['input', 'change', 'mousedown', 'mouseup', 'click', 'focus']) {
      s.addEventListener(t, () => window.__ev.push(t));
    }
  });
  const r = await p2.evaluate(() => {
    const b = document.getElementById('s').getBoundingClientRect();
    return [b.x, b.y, b.width, b.height];
  });
  show('CLOSED-RECT', r);
  await p2.mouse.click(r[0] + r[2] / 2, r[1] + r[3] / 2);
  show('CLOSED-CLICK', await p2.evaluate(() => ({
    active: document.activeElement.id,
    value: document.getElementById('s').value,
    events: window.__ev,
  })));
  await p2.keyboard.press('Escape');
  show('CLOSED-CLICK-THEN-ESCAPE', await p2.evaluate(() => ({
    active: document.activeElement.id,
    value: document.getElementById('s').value,
    events: window.__ev,
  })));
  await browser.close();
})();
