// BR-111 visual anchor: what the live IME preview actually inks.
//
// The catalog row is about what the user *sees* while composing Vietnamese, and
// two of its failure modes are pixel failure modes: a preview that never gets
// laid out (the empty editor types into a blank box) and a preview drawn twice
// over the value it already holds (the word appears as "tiếngtiếng"). Neither
// is visible in a DOM dump, so both are measured here as ink.
//
// The fixture is cmd/goosie/testdata/br111-ime-preview.html - one file, read by
// this script and served by the Go oracle, so the two sides cannot drift.
// deviceScaleFactor 1 keeps one PNG pixel equal to one CSS pixel, which is what
// the ink ruler below assumes.
//
// Run:
//   node scripts/verify-br111-ime-visual.js /tmp/br111-vis
// which writes chrome-empty.png, chrome-input-marked.png,
// chrome-input-committed.png, chrome-editor-marked.png,
// chrome-editor-committed.png. Measure them with the same rule the Go oracle
// uses (cmd/goosie/journey_ime_visual_test.go): dark pixels (all channels
// < 110) inside the element's border box inset by 2px, so neither the 1px
// border nor the neighbouring box is counted - x 22..238, y 22..42 for #i and
// y 72..112 for #c in this 280x160 viewport at dpr 1.
//
// Measured 2026-11-05 on this host (Chromium 153.0.8010.12, dpr 1, Arial 16px,
// steps ti / tie / tiế / tiếng):
//   box  state            dark px  ink columns  x span   non-white px
//   #i   untouched            0              0        -            0
//   #i   composition open   216             34   23..56          879
//   #i   committed          160             27   23..56          539
//   #c   composition open   213             33   22..54          844
//   #c   committed          157             26   22..54          552
// Goosie on the same fixture, same inset rule, from the frame it presents:
//   #i   composition open = committed: 156 dark, 26 columns, x 22..56
//   #c   composition open = committed: 135 dark, 25 columns, x 22..54
//
// One visible difference, recorded rather than papered over: Chromium inks the
// open composition as a *live selection*, so the passage carries its #b3c9ba
// selection tint and the glyph edges darken against it (879 non-white px over
// the same box that holds 539 once the word is committed). Goosie sets the same
// range in the DOM but inks the glyphs with no tint - its marked and committed
// frames are pixel-identical. The word, its column count and its span agree;
// the highlight does not. Tracked as GOOSIE-IMESELECTION-001.
//
// Nothing in test/ quotes a rule from memory that this script has not printed.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const OUT = process.argv[2] || '/tmp/br111-vis';
const FIXTURE = path.join(__dirname, '..', 'cmd', 'goosie', 'testdata', 'br111-ime-preview.html');
const STEPS = ['ti', 'tie', 'tiế', 'tiếng'];

async function compose(page, cdp, sel, steps, commit) {
  await page.click(sel);
  for (const s of steps) {
    await cdp.send('Input.imeSetComposition', { text: s, selectionStart: 0, selectionEnd: s.length });
  }
  const marked = await page.evaluate((s) => {
    const el = document.querySelector(s);
    return { text: el.value === undefined ? el.textContent : el.value, sel: getSelection().toString() };
  }, sel);
  await page.screenshot({ path: path.join(OUT, nameFor(sel, 'marked')) });
  await cdp.send('Input.insertText', { text: commit });
  const committed = await page.evaluate((s) => {
    const el = document.querySelector(s);
    return { text: el.value === undefined ? el.textContent : el.value, sel: getSelection().toString() };
  }, sel);
  await page.screenshot({ path: path.join(OUT, nameFor(sel, 'committed')) });
  return { marked, committed };
}

function nameFor(sel, phase) {
  const el = sel === '#i' ? 'input' : 'editor';
  return `chrome-${el}-${phase}.png`;
}

async function main() {
  const html = fs.readFileSync(FIXTURE, 'utf8');
  fs.mkdirSync(OUT, { recursive: true });
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 280, height: 160 }, deviceScaleFactor: 1 });
  const cdp = await page.context().newCDPSession(page);
  await page.setContent(html);
  await page.screenshot({ path: path.join(OUT, 'chrome-empty.png') });
  console.log(`chromium ${browser.version()} | fixture=${path.relative(process.cwd(), FIXTURE)} | steps=${JSON.stringify(STEPS)}`);

  const input = await compose(page, cdp, '#i', STEPS, 'tiếng');
  console.log('#i  ' + JSON.stringify(input));
  await page.evaluate(() => { document.getElementById('i').value = ''; });
  const editor = await compose(page, cdp, '#c', STEPS, 'tiếng');
  console.log('#c  ' + JSON.stringify(editor));
  console.log('screenshots in ' + OUT);
  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
