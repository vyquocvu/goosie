const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');

const DOC_DIR = '/tmp/goosie-gcs-content';
const DOC = path.join(DOC_DIR, 'index.html');

const VIEWPORT = { width: 900, height: 700 };

const CASES = [
  ['rel', '::before', 'content: url(none.png);'],
  ['unq', '::before', 'content: url(https://example.test/y.png);'],
  ['absq', '::before', 'content: url("https://example.test/x.png");'],
  ['spc', '::before', 'content: url("a b.png");'],
  ['mixurl', '::before', 'content: url(none.png) "T";'],
  ['relq', '::before', 'content: url("none.png");'],
  ['relroot', '::before', 'content: url(/none.png);'],
  ['relabsname', '::before', 'content: url(https://example.test/none.png);'],
  ['relafter', '::after', 'content: url(none.png);'],
  ['attrv', '::before', 'content: attr(data-x);'],
  ['attrnum', '::before', 'content: attr(data-num);'],
  ['attrspace', '::before', 'content: attr(data-sp);'],
  ['attrquote', '::before', 'content: attr(data-q);'],
  ['attrmissing', '::before', 'content: attr(data-nope);'],
  ['attrfallback', '::before', 'content: attr(data-nope, "FB");'],
  ['attrint', '::before', 'content: attr(data-nope, 42);'],
  ['counter', '::before', 'content: counter(c);'],
  ['counterstyle', '::before', 'content: counter(c, lower-roman);'],
  ['counters', '::before', 'content: counters(c, "-");'],
  ['openquote', '::before', 'content: open-quote;'],
  ['quotesfn', '::before', 'content: quotes("\u00ab" "\u00bb");'],
  ['bogusfn', '::before', 'content: bogus(1);'],
  ['leaderfn', '::before', 'content: leader(".") "x";'],
  ['namedstring', '::before', 'content: string(x);'],
  ['imageset', '::before', 'content: image-set(url(none.png) 1x);'],
  ['elementfn', '::before', 'content: element(#curl);'],
  ['allmix', '::before', 'content: "a" counter(c) url(none.png) attr(data-x) open-quote;'],
  ['mixtext', '::before', 'content: "A" counter(c) "B";'],
  ['mixattr', '::before', 'content: "x" attr(data-x) "y";'],
  ['mixattrcounter', '::before', 'content: attr(data-x) counter(c);'],
  ['mixempty', '::before', 'content: "" attr(data-x);'],
  ['normal', '::before', 'content: normal;'],
  ['none', '::before', 'content: none;'],
  ['emptystr', '::before', 'content: "";'],
  ['str', '::before', 'content: "PRE";'],
  ['slash', '::before', 'content: "a" / "b";'],
  ['afterattr', '::after', 'content: attr(data-x);'],
  ['aftercounter', '::after', 'content: counter(c);'],
  ['markerattr', '::marker', 'content: attr(data-x);'],
  ['firstlineattr', '::first-line', 'content: attr(data-x);'],
  ['firstletterattr', '::first-letter', 'content: attr(data-x);'],
  ['placeholderattr', '::placeholder', 'content: attr(data-x);'],
  ['attrupper', '::before', 'content: attr(DATA-X);'],
  ['attrmissupper', '::before', 'content: attr(DATA-NOPE);'],
  ['escapetab', '::before', 'content: "a\\9 b";'],
  ['escapecr', '::before', 'content: "a\\d b";'],
  ['escapeone', '::before', 'content: "a\\1 b";'],
  ['escapeuni', '::before', 'content: "a\\e9 b";'],
  ['escquote', '::before', 'content: "a\\"b";'],
  ['escback', '::before', 'content: "a\\\\b";'],
  ['escape6', '::before', 'content: "a\\000041 b";'],
  ['escapesur', '::before', 'content: "a\\d800b";'],
  ['escapezero', '::before', 'content: "a\\0 b";'],
  ['ctltab', '::before', 'content: attr(data-c);'],
  ['ctllf', '::before', 'content: attr(data-c);'],
  ['ctlcr', '::before', 'content: attr(data-c);'],
  ['ctlback', '::before', 'content: attr(data-c);'],
  ['ctldq', '::before', 'content: attr(data-c);'],
  ['ctluni', '::before', 'content: attr(data-c);'],
];

const CTL = {
  ctltab: 'a\tb',
  ctllf: 'a\nb',
  ctlcr: 'a\rb',
  ctlback: 'a\\b',
  ctldq: 'a"b',
  ctluni: 'aéb\u0001c\u00a0d',
};

const HEAD = `
<!doctype html><meta charset="utf-8">
body { margin: 0; font: 16px Arial, Helvetica, sans-serif; counter-reset: c 3; }
.c { position: absolute; left: 20px; width: 300px; }
input { position: absolute; left: 360px; width: 200px; }
`;

function body() {
  let css = '';
  let html = '';
  CASES.forEach(([id, pseudo, decl], i) => {
    const top = 20 + i * 22;
    css += `#${id}${pseudo} { ${decl} }\n`;
    if (pseudo === '::placeholder') {
      html += `<input id="${id}" placeholder="type ${id}" style="top:${top}px">\n`;
    } else if (pseudo === '::marker') {
      html += `<ul id="${id}" class="c" style="top:${top}px" data-x="ATTR"><li>mark ${id}</li></ul>\n`;
    } else {
      html += `<div id="${id}" class="c" style="top:${top}px" data-x="ATTR" data-num="42" data-sp="a b" data-q='say "hi"'>host ${id}</div>\n`;
    }
  });
  return HEAD + '<style>' + css + '</style>\n' + html;
}

async function main() {
  if (process.env.GCS_DUMP) fs.writeFileSync('/tmp/gcs-anchor-dump.html', body());
  fs.mkdirSync(DOC_DIR, { recursive: true });
  fs.writeFileSync(DOC, body());
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + DOC, { waitUntil: 'load' });
  await page.evaluate((ctl) => {
    for (const id of Object.keys(ctl)) document.getElementById(id).setAttribute('data-c', ctl[id]);
  }, CTL);
  await page.evaluate(() => document.body.offsetHeight);

  const rows = await page.evaluate((list) => list.map(([id, pseudo]) => {
    const el = document.getElementById(id);
    const cs = getComputedStyle(el, pseudo);
    return { id, pseudo, prop: cs.content, gpv: cs.getPropertyValue('content'), len: cs.length, raw: el.getAttribute('data-c') };
  }), CASES);

  const urlRow = await page.evaluate(() => {
    const el = document.getElementById('rel');
    return { doc: document.baseURI, content: getComputedStyle(el, '::before').content };
  });

  await browser.close();

  console.log(`chromium content table, baseURI=${urlRow.doc}`);
  console.log('id'.padEnd(14) + 'pseudo'.padEnd(15) + 'cs.content'.padEnd(52) + 'getPropertyValue');
  for (const r of rows) {
    console.log(r.id.padEnd(14) + r.pseudo.padEnd(15) +
      JSON.stringify(r.prop).padEnd(52) + JSON.stringify(r.gpv) +
      (r.prop === r.gpv ? ' same' : ' DIFFERENT') + ` len=${r.len}`);
  }
  console.log('');
  console.log(`rel-case answer=${JSON.stringify(urlRow.content)} (authored url(none.png), relative to the document)`);
  const agree = rows.every((r) => r.prop === r.gpv);
  console.log(`cs.content === getPropertyValue('content') for every case: ${agree}`);
}

main().catch((e) => { console.error(e); process.exit(1); });
