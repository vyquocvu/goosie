// BR-087 / GOOSIE-BIDIORDER-001 anchor: what visual order does Chromium give
// the characters of an RTL line, and where does it anchor them?
//
//   node scripts/verify-br087-bidi.js
//
// Goosie reverses a *word list* for RTL lines and never reverses the glyphs
// inside a word, and its neutral resolution puts every space at the base level.
// The QA case says characters must not be reordered wrong, so the rule has to
// come from the reference's own geometry: for every fixture below the script
// reports the rect Chromium gives each character, in logical order and sorted
// into visual order.
//
// Coordinates are device px at deviceScaleFactor 1. Each character's rect comes
// from a Range over the text node (startOffset=i, endOffset=i+1) because a
// Range is the only thing that reports a line fragment; the element's own
// getBoundingClientRect is the border box and says nothing about order.
//
// Nothing here is a rule we recall: if a Goosie oracle disagrees with a number
// below, the number is the evidence and the oracle is the claim.
const { chromium } = require('playwright');

const VIEWPORT = { width: 900, height: 1300 };

// Every fixture is its own 200px box at x=600 so no measurement can overlap
// another element's pixels, and each is given the same font.
const PAGE = `
<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; font: 16px/1.2 Arial, Helvetica, sans-serif; color: #000; background: #fff; }
  #flow { position: absolute; top: 0; left: 600px; width: 200px; }
  #flow div { margin-bottom: 10px; }
</style>
<div id="flow">
  <div id="a_mixed" dir="rtl">مرحبا abc 123</div>
  <div id="b_mixed_ltr">مرحبا abc 123</div>
  <div id="c_arabic_only" dir="rtl">مرحبا</div>
  <div id="d_an" dir="rtl">مرحبا ١٢٣</div>
  <div id="e_en_an" dir="rtl">مرحبا 123 ٤٥٦ abc</div>
  <div id="f_pure_latin_rtl" dir="rtl">abc def</div>
  <div id="g_he_ar" dir="rtl">שלום مرحبا</div>
  <div id="h_en_in_ltr">abc 123 ٤٥٦</div>
  <div id="i_nested_ltr" dir="rtl">مرحبا <span dir="ltr">abc 123</span> ختام</div>
  <div id="j_paren_rtl" dir="rtl">(abc)</div>
  <div id="k_paren_ltr">(مرحبا)</div>
  <div id="l_comma_en" dir="rtl">مرحبا 12,34</div>
  <div id="m_plaintext_ltr_first" dir="rtl" style="unicode-bidi: plaintext">abc مرحبا</div>
  <div id="n_override" dir="rtl" style="unicode-bidi: bidi-override">مرحبا abc</div>
  <div id="o_wrap" dir="rtl">مرحبا بك في متصفح صغير جدا لهذا السطر</div>
  <div id="p_trailing_space" dir="rtl">مرحبا </div>
  <div id="q_isolate" dir="rtl">مرحبا <bdi>abc</bdi> ختام</div>
  <div id="r_align_left" dir="rtl" style="text-align:left">مرحبا abc</div>
  <div id="s_align_center" dir="rtl" style="text-align:center">مرحبا abc</div>
  <div id="t_align_right" dir="rtl" style="text-align:right">مرحبا abc</div>
  <div id="u_padding" dir="rtl" style="padding:0 30px">مرحبا abc</div>
  <div id="v_overflow" dir="rtl">مرحبابالعربيةهذاكلمةطويلةجدا</div>
  <div id="w_ltr_align_right" style="text-align:right">مرحبا abc</div>
  <div id="x_cs_default_ltr">مرحبا abc</div>
  <div id="y_cs_default_rtl" dir="rtl">مرحبا abc</div>
  <div id="z_cs_start_rtl" dir="rtl" style="text-align:start">مرحبا abc</div>
  <div id="aa_cs_end_rtl" dir="rtl" style="text-align:end">مرحبا abc</div>
  <div id="ab_cs_left_rtl" dir="rtl" style="text-align:left">مرحبا abc</div>
  <div id="ac_cs_start_ltr" style="text-align:start">مرحبا abc</div>
  <div id="ad_latin_logical_first" dir="rtl">hello שלום</div>
  <div id="ae_latin_digits_logical_first" dir="rtl">abc 123 שלום</div>
</div>
`;

// Each fixture's text is described by its element id; the script never has to
// know the string to walk it, it reads the text node it finds.
async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(PAGE, { waitUntil: 'load' });

  const { report, geometry } = await page.evaluate(() => {
    const lines = [];
    const say = (s) => lines.push(s);
    const r3 = (n) => Math.round(n * 1000) / 1000;

    const ids = Array.from(document.querySelectorAll('#flow div[id]'), (d) => d.id);
    for (const id of ids) {
      const el = document.getElementById(id);
      const box = el.getBoundingClientRect();
      const cs = getComputedStyle(el);

      // One Range per character of the element's first text node, plus the
      // nested spans' text, walked in document order so logical order is the
      // order the source bytes have.
      const chars = [];
      const walk = (node) => {
        for (const child of node.childNodes) {
          if (child.nodeType === 3) {
            const text = child.nodeValue;
            const ownDir = getComputedStyle(child.parentElement || el).direction;
            for (let i = 0; i < text.length; i++) {
              const range = document.createRange();
              range.setStart(child, i);
              range.setEnd(child, i + 1);
              const rects = range.getClientRects();
              const rect = rects.length ? rects[0] : null;
              chars.push({
                ch: text[i],
                cp: text.codePointAt(i) || 0,
                x: rect ? r3(rect.x) : null,
                w: rect ? r3(rect.width) : null,
                y: rect ? r3(rect.y) : null,
                host: ownDir,
              });
              if (!Number.isNaN(text.charCodeAt(i)) && text.charCodeAt(i) >= 0xd800 && text.charCodeAt(i) <= 0xdbff) i++;
            }
          } else if (child.nodeType === 1) walk(child);
        }
      };
      walk(el);

      const placed = chars.filter((c) => c.x !== null);
      const visual = placed.slice().sort((a, b) => a.x - b.x || a.y - b.y).map((c) => c.ch).join('');
      const logical = chars.map((c) => c.ch).join('');
      const monotonic = (arr) => arr.every((v, i) => i === 0 || v > arr[i - 1]);
      const strictlyDescending = (arr) => arr.every((v, i) => i === 0 || v < arr[i - 1]);

      // Maximal same-script stretches in logical order, each reported with the
      // x order the reference gave it: that is the whole question - which
      // stretches come out reversed and which keep their logical order.
      const isArabicLetter = (cp) => (cp >= 0x0600 && cp <= 0x06ff) || (cp >= 0xfb50 && cp <= 0xfdff);
      const isHebrew = (cp) => cp >= 0x0590 && cp <= 0x05ff;
      const isLatin = (cp) => cp >= 0x41 && cp <= 0x7a;
      const kind = (c) => {
        if (isHebrew(c.cp)) return 'HE';
        if (isArabicLetter(c.cp)) return 'AR';
        if (isLatin(c.cp)) return 'LA';
        if (c.cp >= 0x0660 && c.cp <= 0x0669) return 'AN';
        if (c.cp >= 0x30 && c.cp <= 0x39) return 'EN';
        return 'OT';
      };
      const stretches = [];
      for (let i = 0; i < chars.length; i++) {
        const k = kind(chars[i]);
        if (k === 'OT') continue;
        const run = { kind: k, xs: [chars[i].x], chars: chars[i].ch };
        let j = i + 1;
        while (j < chars.length && kind(chars[j]) === k) {
          run.xs.push(chars[j].x);
          run.chars += chars[j].ch;
          j++;
        }
        stretches.push(run);
        i = j - 1;
      }

      say(`---- ${id}  dir=${cs.direction} unicode-bidi=${cs.unicodeBidi} text-align=${cs.textAlign} ` +
        `box=x${r3(box.x)}..x${r3(box.right)} w=${r3(box.width)} lines=${new Set(placed.map((c) => c.y)).size}`);
      say(`  logical="${logical}"`);
      say(`  visual ="${visual}"  visual==logical=${visual === logical}`);
      for (const s of stretches) {
        const asc = monotonic(s.xs), desc = strictlyDescending(s.xs);
        say(`  stretch ${s.kind} "${s.chars}" x=[${s.xs.join(' ')}] ascending=${asc} descending=${desc}`);
      }
      say(`  per-char: ${placed.map((c) => `${c.ch}=${c.x}+${c.w}`).join(' ')}`);
      const first = placed[0], last = placed[placed.length - 1];
      if (first && last) {
        say(`  span: leftmost x=${r3(Math.min(...placed.map((c) => c.x)))} ` +
          `rightmost edge x=${r3(Math.max(...placed.map((c) => c.x + c.w)))} ` +
          `right-anchored=${r3(box.right - Math.max(...placed.map((c) => c.x + c.w))) < 2}`);
      }
    }

    // The table alignment reset: Goosie drops an inherited `text-align: center`
    // at a table boundary, so the value it puts back has to be the one the
    // reference reports. This asks Chromium directly.
    const probe = document.createElement('div');
    probe.innerHTML = '<div style="text-align:center">' +
      '<table id="tbl-probe"><tbody><tr><td id="cell-probe">TEXT</td></tr></tbody></table></div>';
    document.body.appendChild(probe);
    const tbl = probe.querySelector('#tbl-probe');
    const cell = probe.querySelector('#cell-probe');
    const tableRect = tbl.getBoundingClientRect();
    const cellTextRect = (() => {
      const node = cell.firstChild;
      const range = document.createRange();
      range.setStart(node, 0);
      range.setEnd(node, 4);
      return range.getBoundingClientRect();
    })();
    say('---- table alignment reset (inherited centre)');
    say(`  computed table text-align = "${getComputedStyle(tbl).textAlign}" dir=${getComputedStyle(tbl).direction}`);
    say(`  computed td text-align = "${getComputedStyle(cell).textAlign}"`);
    say(`  table box x=${r3(tableRect.x)} w=${r3(tableRect.width)}; TEXT rect x=${r3(cellTextRect.x)} w=${r3(cellTextRect.width)}`);

    const geo = {};
    for (const id of ids) {
      const b = document.getElementById(id).getBoundingClientRect();
      geo[id] = { x: r3(b.x), y: r3(b.y), w: r3(b.width), h: r3(b.height) };
    }
    return { report: lines.join('\n'), geometry: geo };
  });

  console.log(report);
  console.log('\nGEOMETRY ' + JSON.stringify(geometry));
  await browser.close();
}

main().catch((e) => { console.error(e); process.exit(1); });
