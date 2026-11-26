const { chromium } = require('playwright');

const PAGE = `<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;font:16px Arial,Helvetica,sans-serif;color:#000;background:#fff}
p{margin:0 0 20px;padding:0;font-size:16px;line-height:20px}
#grow{width:150px}#grow::first-line{font-size:32px}
#shrink{width:95px}#shrink::first-line{font-size:8px}
#nested{width:150px}#nested::first-line{font-size:32px}#nested span{font-size:16px}
#float{width:200px}#float::first-line{font-size:32px;line-height:40px}#float .f{float:right;width:60px;height:40px}
#leading{width:200px}#leading::first-line{font-size:32px;line-height:48px}
#ratio{width:400px}#ratio::first-line{line-height:1.5;font-size:32px}
</style>
<p id="grow">Alpha beta gamma delta</p>
<p id="shrink">Alpha beta gamma delta</p>
<p id="nested"><span>Alpha</span> beta gamma</p>
<p id="float"><span class="f"></span>Alpha beta gamma delta</p>
<p id="leading"><br>Alpha beta</p>
<p id="ratio">Alpha beta<br>gamma</p>`;

const EXPECTED = {
  grow: [['Alpha'], ['beta', 'gamma', 'delta']],
  shrink: [['Alpha', 'beta', 'gamma', 'delta']],
  nested: [['Alpha', 'beta'], ['gamma']],
  float: [['Alpha'], ['beta', 'gamma', 'delta']],
  leading: [['Alpha', 'beta']],
  ratio: [['Alpha', 'beta'], ['gamma']],
};

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 600 }, deviceScaleFactor: 1 });
  await page.setContent(PAGE, { waitUntil: 'load' });
  const measured = await page.evaluate(() => {
    const round = (value) => Math.round(value * 1000) / 1000;
    const measure = (id) => {
      const el = document.getElementById(id);
      const box = el.getBoundingClientRect();
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      const words = [];
      let node;
      while ((node = walker.nextNode())) {
        const text = node.nodeValue;
        const pattern = /\S+/g;
        let match;
        while ((match = pattern.exec(text))) {
          const range = document.createRange();
          range.setStart(node, match.index);
          range.setEnd(node, match.index + match[0].length);
          const rect = range.getClientRects()[0];
          if (!rect) continue;
          words.push({ text: match[0], x: rect.x, bottom: rect.bottom, top: rect.top });
        }
      }
      words.sort((a, b) => a.bottom - b.bottom || a.x - b.x);
      const rows = [];
      for (const word of words) {
        let row = rows[rows.length - 1];
        if (!row || Math.abs(row.bottom - word.bottom) > 6) {
          row = { bottom: word.bottom, words: [] };
          rows.push(row);
        }
        row.words.push(word.text);
      }
      return {
        rows: rows.map((row) => row.words),
        box: { x: round(box.x), y: round(box.y), width: round(box.width), height: round(box.height) },
        textTop: words.length ? round(words[0].top - box.top) : null,
      };
    };
    return Object.fromEntries(['grow', 'shrink', 'nested', 'float', 'leading', 'ratio'].map((id) => [id, measure(id)]));
  });

  const failures = [];
  const same = (got, want) => JSON.stringify(got) === JSON.stringify(want);
  for (const [id, want] of Object.entries(EXPECTED)) {
    const got = measured[id];
    console.log(`${id.padEnd(8)} rows=${JSON.stringify(got.rows)} box=${got.box.width}x${got.box.height} textTop=${got.textTop}`);
    if (!same(got.rows, want)) failures.push(`${id} rows ${JSON.stringify(got.rows)} != ${JSON.stringify(want)}`);
  }
  const heights = { grow: 40, shrink: 20, float: 60, leading: 68, ratio: 68 };
  for (const [id, height] of Object.entries(heights)) {
    if (measured[id].box.height !== height) failures.push(`${id} height ${measured[id].box.height} != ${height}`);
  }
  if (measured.leading.textTop !== 49) failures.push(`leading textTop ${measured.leading.textTop} != 49`);
  await browser.close();
  if (failures.length) {
    for (const failure of failures) console.log(`FAIL ${failure}`);
    throw new Error(`${failures.length} first-line breaking oracle checks failed`);
  }
  console.log('FIRST-LINE BREAKING ANCHOR OK');
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
