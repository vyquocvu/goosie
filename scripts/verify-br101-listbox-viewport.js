// GOOSIE-LISTBOX-001 anchor: where can a <select> listbox put its rows, and
// which row does Chromium put at the top of a list that does not fit?
//
//   node scripts/verify-br101-listbox-viewport.js
//
// Two facts have to be separated, and this script measures one and proves the
// other cannot be measured here.
//
// (1) A closed select's POPUP is invisible to every headless channel on this
// host. Playwright cannot read it (no page, no element, no AX node) and it is
// not in the captured surface either: POPUP-PIXELS below writes a screenshot
// taken before and after a click on a select at y=540 of a 600px viewport. The
// diff, run with
//   python3 - <<'PY'
//   from PIL import Image
//   a=Image.open('/tmp/lbviewport/before.png').convert('RGB'); b=Image.open('/tmp/lbviewport/after.png').convert('RGB')
//   pa,pb=a.load(),b.load(); xs=[];ys=[]
//   for y in range(a.size[1]):
//     for x in range(a.size[0]):
//       if pa[x,y]!=pb[x,y]: xs.append(x); ys.append(y)
//   print('CHANGED', [min(xs),min(ys),max(xs),max(ys)] if xs else None)
//   PY
// prints CHANGED [40,540,219,560] (measured 2026-11-04, Chromium 1.63): exactly
// the control's own 180x21 rect, i.e. only its focus ring. No row of the popup is
// anywhere in the image, so neither its side choice nor its clamping can be read
// off a screenshot. A screen-region capture is not available either - this host
// answers `screencapture` with "could not create image from display".
//
// (2) The in-page form of the same widget, <select size=N>, IS observable, and
// its scrollTop divided by the row height IS the index of its top visible row.
// ROW-WINDOW below measures it for combinations that all overflow the box.
// Measured 2026-11-04 on this host:
//
//   ROW-WINDOW size 3 of 20, selected 0  -> scrollTop 0    topRow 0.000  sel-visible+1 = 0
//   ROW-WINDOW size 3 of 20, selected 15 -> scrollTop 262  topRow 12.978 sel-visible+1 = 13
//   ROW-WINDOW size 3 of 20, selected 19 -> scrollTop 343  topRow 16.991 sel-visible+1 = 17
//   ROW-WINDOW size 4 of 12, selected 9  -> scrollTop 121  topRow 5.994  sel-visible+1 = 6
//   ROW-WINDOW size 5 of 40, selected 37 -> scrollTop 666  topRow 32.991 sel-visible+1 = 33
//
// One rule covers all five: the box keeps its own height and scrolls by the
// SMALLEST amount that brings the selection into view, which parks the selected
// row on the last visible line (integer part of scrollTop/rowH is exactly
// max(0, sel-visible+1), and scrollTop never exceeds scrollHeight-clientHeight).
//
// The last GEO-CLAMP line is the negative result that shapes the popup design:
// a listbox whose size asks for more rows than the viewport holds does NOT
// clamp - size 60 of 70 measures box [180,1213.25] in a 600px viewport and its
// own selection paints at y 666.63, off screen, because the PAGE scrolls instead.
// So the popup's clamp-to-the-viewport rule is this engine's own design choice,
// forced by the defect (the pointer and the ink must agree on what is visible)
// and is signed off against the engine's viewport, not against a Chromium
// reading. The side choice is recorded the same way in docs/qa-open-defects.json.
const { chromium } = require('playwright');
const fs = require('fs');

const VIEWPORT = { width: 800, height: 600 };

const pageHTML = (top, n, selected, size) => `<!doctype html><html><head><style>
html,body{margin:0;padding:0}
#lb{position:absolute;left:40px;top:${top}px;width:180px;font-size:16px}
</style></head><body>
<select id="lb" size="${size}">${Array.from({ length: n }, (_, i) => `<option value="v${i}"${i === selected ? ' selected' : ''}>opt${i}</option>`).join('')}</select>
</body></html>`;

async function popupInvisible(browser) {
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  fs.mkdirSync('/tmp/lbviewport', { recursive: true });
  await page.setContent(pageHTML(540, 12, 3, 1), { waitUntil: 'load' });
  const rect = await page.evaluate(() => {
    const b = document.getElementById('lb').getBoundingClientRect();
    return [b.x, b.y, b.width, b.height];
  });
  await page.screenshot({ path: '/tmp/lbviewport/before.png' });
  await page.mouse.click(rect[0] + rect[2] / 2, rect[1] + rect[3] / 2);
  await page.waitForTimeout(400);
  await page.screenshot({ path: '/tmp/lbviewport/after.png' });
  console.log(`POPUP-PIXELS control ${JSON.stringify(rect)} at y=540 of a ${VIEWPORT.height}px viewport; ` +
    `screenshots in /tmp/lbviewport/{before,after}.png - the measured diff is the control rect only`);
  await page.close();
}

async function rowWindow(browser, size, n, selected, top) {
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(pageHTML(top, n, selected, size), { waitUntil: 'load' });
  const m = await page.evaluate(() => {
    const lb = document.getElementById('lb');
    const b = lb.getBoundingClientRect();
    const o = lb.options[0].getBoundingClientRect();
    return {
      box: [+b.width.toFixed(2), +b.height.toFixed(2)],
      client: lb.clientHeight,
      scrollH: lb.scrollHeight,
      scrollTop: +lb.scrollTop.toFixed(2),
      rowH: +o.height.toFixed(4),
      index: lb.selectedIndex,
      selTop: +lb.options[lb.selectedIndex].getBoundingClientRect().top.toFixed(2),
      boxBottom: +b.bottom.toFixed(2),
    };
  });
  const visible = Math.floor(m.client / m.rowH);
  const topRow = m.scrollTop / m.rowH;
  const scrolled = m.scrollH - m.client;
  console.log(`ROW-WINDOW size ${size} of ${n}, selected ${selected} -> scrollTop ${m.scrollTop} ` +
    `topRow ${topRow.toFixed(3)} sel-visible+1 = ${Math.max(0, m.index - visible + 1)} ` +
    `visible ${visible} (client ${m.client} rowH ${m.rowH} scrollH ${m.scrollH} maxScroll ${scrolled})`);
  console.log(`GEO-CLAMP size ${size} of ${n} -> box ${JSON.stringify(m.box)} in a ${VIEWPORT.height}px viewport, ` +
    `selection ${m.index} paints at y ${m.selTop} (box bottom ${m.boxBottom})`);
  await page.close();
}

(async () => {
  const browser = await chromium.launch();
  await popupInvisible(browser);
  await rowWindow(browser, 3, 20, 0, 60);
  await rowWindow(browser, 3, 20, 15, 60);
  await rowWindow(browser, 3, 20, 19, 60);
  await rowWindow(browser, 4, 12, 9, 60);
  await rowWindow(browser, 5, 40, 37, 60);
  await rowWindow(browser, 60, 70, 30, 60);
  await browser.close();
  console.log('DONE');
})();
