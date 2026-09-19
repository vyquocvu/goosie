const { chromium } = require('playwright');

(async () => {
  const b = await chromium.launch();
  const p = await b.newPage({ viewport: { width: 800, height: 600 } });
  for (const fam of ['Times New Roman', 'Arial', 'Courier New', 'Georgia', 'Verdana']) {
    const html = `<!doctype html><html><body style="margin:0">
      <div id="d" style="font-family:'${fam}';font-size:16px;width:180px">alpha beta gamma delta epsilon</div>
      <div style="font-family:'${fam}';font-size:16px;line-height:normal"><span id="s">X</span></div>
      </body></html>`;
    await p.setContent(html);
    const r = await p.evaluate(() => {
      const c = document.createElement('canvas').getContext('2d');
      c.font = "16px 'Times New Roman'";
      const m = c.measureText('Xg');
      const range = document.createRange();
      range.selectNodeContents(document.getElementById('d'));
      const lr = [...range.getClientRects()].map(x => [Math.round(x.top * 100) / 100, Math.round(x.height * 100) / 100]);
      const sr = document.getElementById('s').getBoundingClientRect();
      c.font = getComputedStyle(document.getElementById('s')).font;
      const m2 = c.measureText('Xg');
      return {
        lineRects: lr,
        spanH: Math.round(sr.height * 100) / 100,
        spanTop: Math.round(sr.top * 100) / 100,
        fba: Math.round(m.fontBoundingBoxAscent * 100) / 100,
        fbd: Math.round(m.fontBoundingBoxDescent * 100) / 100,
        aba: Math.round(m2.actualBoundingBoxAscent * 100) / 100,
        abd: Math.round(m2.actualBoundingBoxDescent * 100) / 100,
      };
    });
    console.log(fam, JSON.stringify(r));
  }
  await b.close();
})();
