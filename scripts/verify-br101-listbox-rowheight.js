// BR-101 reference probe: how tall is one row of Chromium's listbox?
//
// A closed <select> opens an OS-native popup that page script cannot observe
// (GOOSIE-LISTBOX-002), so the probe measures the in-page form of the same
// control - <select size="3">, whose option rows Chromium lays out inside the
// page. Row boxes come from getBoundingClientRect, computed styles from
// getComputedStyle.
//
// Every assertion in test/engine/listbox_row_height_test.go cites a line of
// this script's output. Do not encode a rule from memory - re-run and read:
//   node scripts/verify-br101-listbox-rowheight.js
//
// The measured answers (Chromium 153.0.8010.12, macOS, 2026-11-06):
//
//   - Row height is independent of the font family: the UA default face, Arial,
//     "Times New Roman", system-ui and Helvetica all report the same 20.1875px
//     row at 16px. The control face's metrics do not participate.
//   - An authored line-height does not participate either: line-height:30px
//     answers the same 20.1875 row as the default. This is a theme rule, not
//     font metrics.
//   - Row height follows font-size as floor-to-1/64(1.2 x font-size) + a 1px
//     bottom padding: 13px -> 16.59375, 16px -> 20.1875, 20px -> 25,
//     24px -> 29.796875, 32px -> 39.390625. Padding is 0px top / 1px bottom on
//     every option; margin and border are 0.
const { chromium } = require('playwright');

function show(label, obj) {
  console.log(label + ' -> ' + JSON.stringify(obj));
}

function optionHTML() {
  return `<option>Alpha</option><option>Bravo</option><option>Charlie</option>`;
}

// Families at the UA default 16px, then a font-size sweep plus one select with
// an authored line-height, all in Arial so family stays fixed inside the sweep.
const HTML = `<!doctype html><html><head><style>
html,body{margin:0;padding:0}
select{position:absolute;left:40px;width:180px;font-size:16px}
#ua{top:20px}
#arial{top:120px;font-family:Arial}
#times{top:220px;font-family:"Times New Roman"}
#sys{top:320px;font-family:system-ui}
#helve{top:420px;font-family:Helvetica}
#s13{top:560px;font-family:Arial;font-size:13px}
#s16{top:660px;font-family:Arial;font-size:16px}
#s20{top:780px;font-family:Arial;font-size:20px}
#s24{top:920px;font-family:Arial;font-size:24px}
#s32{top:1060px;font-family:Arial;font-size:32px}
#lhfix{top:1220px;font-family:Arial;font-size:16px;line-height:30px}
</style></head><body>
<select id="ua" size="3">${optionHTML()}</select>
<select id="arial" size="3">${optionHTML()}</select>
<select id="times" size="3">${optionHTML()}</select>
<select id="sys" size="3">${optionHTML()}</select>
<select id="helve" size="3">${optionHTML()}</select>
<select id="s13" size="3">${optionHTML()}</select>
<select id="s16" size="3">${optionHTML()}</select>
<select id="s20" size="3">${optionHTML()}</select>
<select id="s24" size="3">${optionHTML()}</select>
<select id="s32" size="3">${optionHTML()}</select>
<select id="lhfix" size="3">${optionHTML()}</select>
</body></html>`;

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 500, height: 1400 }, deviceScaleFactor: 1 });
  await page.setContent(HTML, { waitUntil: 'load' });
  const out = await page.evaluate(() => {
    const ids = ['ua', 'arial', 'times', 'sys', 'helve', 's13', 's16', 's20', 's24', 's32', 'lhfix'];
    const res = {};
    for (const id of ids) {
      const s = document.getElementById(id);
      const cs = getComputedStyle(s);
      const box = s.getBoundingClientRect();
      const rows = [...s.options].map((o) => {
        const r = o.getBoundingClientRect();
        const ocs = getComputedStyle(o);
        return {
          h: r.height,
          font: ocs.fontSize,
          lh: ocs.lineHeight,
          pad: ocs.paddingTop + '/' + ocs.paddingBottom,
          margin: ocs.marginTop + '/' + ocs.marginBottom,
          border: ocs.borderTopWidth + '/' + ocs.borderBottomWidth,
        };
      });
      res[id] = { css: cs.fontSize + ' ' + cs.fontFamily.slice(0, 40), boxH: box.height, rows };
    }
    return res;
  });
  for (const [id, v] of Object.entries(out)) {
    show(id, { css: v.css, boxH: v.boxH });
    for (let i = 0; i < v.rows.length; i++) show(`  row${i}`, v.rows[i]);
  }
  await browser.close();
})();
