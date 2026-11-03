// BR-087 anchor: what does Chromium actually do to the geometry of the four
// text surfaces the QA case names - Vietnamese, CJK, emoji, RTL - plus a long
// unbroken word?
//
//   node scripts/verify-br087-text-parity.js
//
// The case is "ký tự không bị cắt/đảo thứ tự ngoài giới hạn font đã biết":
// characters must not be cut or reordered, outside the font limits already on
// record. So this script measures *geometry and order*, not glyph shapes:
// whether a sequence occupies one advance or several, whether a line box is
// produced where the reference produces one, and the left-to-right order of the
// rect each character gets.
//
// Every number comes from Chromium's own layout, through getBoundingClientRect
// and Range.getClientRects on the same document, at deviceScaleFactor 1 so a
// CSS px and a device px are the same thing. Ink is counted from Chromium's own
// screenshot, inside the rect Chromium reported for that very element, so a
// "no pixels" claim cannot come from a stale coordinate.
//
// Nothing here is a rule we recall: if a Goosie oracle disagrees with a number
// below, the number is the evidence and the oracle is the claim.
const { chromium } = require('playwright');

const VIEWPORT = { width: 900, height: 950 };

// Three columns: Vietnamese measures at x 0..250, emoji measures at 260..560,
// and the flow boxes (CJK, kinsoku, long word, bidi) live at x 600..800 so no
// absolute overlay can ever sit on top of another element's pixels.
const PAGE = `
<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; font: 16px/1.2 Arial, Helvetica, sans-serif; color: #000; background: #fff; }
  #viet, #emo { position: absolute; top: 0; white-space: pre; }
  #viet { left: 0; }
  #emo  { left: 260px; }
  #flow { position: absolute; top: 0; left: 600px; width: 200px; }
  #flow div { margin-bottom: 8px; }
  #cjk, #kin, #long, #wrap { width: 200px; }
</style>
<div id="viet"></div>
<div id="emo"></div>
<div id="flow">
  <div id="cjk">中文文本需要换行并且继续写入更多字符以填满这个宽度</div>
  <div id="kin">文本文本文本文本文本文本。文本</div>
  <div id="mix">文本abc中文文本def字符更多文字here</div>
  <div id="idal">中aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa</div>
  <div id="alid">aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa中</div>
  <div id="kana">カタカナの文章がここで折り返されるかどうかを確かめます</div>
  <div id="hangul">한국어텍스트가이 상자안에서 줄바꿈되는지 확인합니다</div>
  <div id="long">aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa</div>
  <div id="shy">aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&shy;bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb</div>
  <div id="zws">aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&#8203;bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb</div>
  <div id="ka" style="word-break: keep-all">中文文本需要换行并且继续写入更多字符以填满这个宽度</div>
  <div id="wrap">aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb</div>
  <div id="rtl" dir="rtl">مرحبا abc 123</div>
  <div id="ltr">مرحبا abc 123</div>
</div>
`;

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(PAGE, { waitUntil: 'load' });

  const { report, boxes } = await page.evaluate(() => {
    const lines = [];
    const say = (s) => lines.push(s);
    const r = (n) => Math.round(n * 1000) / 1000;
    const rect = (el) => {
      const b = el.getBoundingClientRect();
      return { x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.width), h: Math.round(b.height) };
    };
    const measure = (el) => el.getBoundingClientRect().width;

    // ---- Vietnamese: precomposed vs decomposed ----------------------
    // A decomposed vowel is one base letter plus combining marks. If the marks
    // cost advance, the word gets wider than the same word typed precomposed.
    const mk = (text) => {
      const d = document.createElement('div');
      const s = document.createElement('span');
      s.textContent = text;
      d.appendChild(s);
      document.getElementById('viet').appendChild(d);
      return s;
    };
    const vcases = [
      ['nfc-1EBF', '\u1EBF'],
      ['nfd-e-0302-0301', 'e\u0302\u0301'],
      ['nfd-u-0302-0301', 'u\u0302\u0301'],
      ['nfc-word', 'Người Việt'],
      ['nfd-word', 'Người Việt'.normalize('NFD')],
      ['ascii-word', 'Nguoi Viet'],
      ['lone-mark', '\u0301'],
      ['two-marks', '\u0302\u0301'],
      ['space-then-mark', 'a \u0301'],
      ['n-tilde', 'n\u0303'],
      ['e-circ', '\u00EA'],
      ['e-circ-decomposed', 'e\u0302'],
      ['u-horn', '\u01B0'],
      ['u-horn-decomposed', 'u\u031B'],
      ['o-circ-grave', '\u1EDD'],
      ['o-circ-grave-decomposed', 'o\u0302\u0300'],
      ['nfd-sentence', 'Tiếng Việt Nam 2026'.normalize('NFD')],
      ['nfc-sentence', 'Tiếng Việt Nam 2026'],
    ];
    const vw = {};
    for (const [name, text] of vcases) vw[name] = r(measure(mk(text)));
    say('VIET widths: ' + vcases.map(([n]) => `${n}=${vw[n]}`).join(' '));
    say(`VIET precomposed==decomposed single=${vw['nfc-1EBF'] === vw['nfd-e-0302-0301']}`);
    say(`VIET precomposed==decomposed word=${vw['nfc-word'] === vw['nfd-word']}`);
    say(`VIET precomposed==decomposed sentence=${vw['nfc-sentence'] === vw['nfd-sentence']}`);
    say(`VIET decomposed-word vs ascii diff=${r(vw['nfd-word'] - vw['ascii-word'])}`);
    say(`VIET e-circ==decomposed=${vw['e-circ'] === vw['e-circ-decomposed']}`);
    say(`VIET u-horn==decomposed=${vw['u-horn'] === vw['u-horn-decomposed']}`);
    say(`VIET o-circ-grave(two marks)==decomposed=${vw['o-circ-grave'] === vw['o-circ-grave-decomposed']}`);
    say(`VIET lone combining mark advance=${vw['lone-mark']} two marks=${vw['two-marks']}`);
    say(`VIET 'a'+mark advance=${vw['space-then-mark']} ('a'=8.906, space=4.453)`);

    // ---- Emoji / default-ignorables / sequences ---------------------
    const mkE = (text) => {
      const d = document.createElement('div');
      const s = document.createElement('span');
      s.textContent = text;
      d.appendChild(s);
      document.getElementById('emo').appendChild(d);
      return s;
    };
    const ecases = [
      ['a', 'a'],
      ['a+ZWJ', 'a\u200D'],
      ['a+ZWSP', 'a\u200B'],
      ['a+SHY', 'a\u00AD'],
      ['a+RLM', 'a\u200F'],
      ['a+LRM', 'a\u200E'],
      ['a+VS16', 'a\uFE0F'],
      ['man', '\u{1F468}'],
      ['family-zwj', '\u{1F468}\u200D\u{1F469}\u200D\u{1F467}\u200D\u{1F466}'],
      ['family-no-zwj', '\u{1F468}\u{1F469}\u{1F467}\u{1F466}'],
      ['vn-flag', '\u{1F1FB}\u{1F1F3}'],
      ['single-ri', '\u{1F1FB}'],
      ['vs16-only', '\uFE0F'],
      ['zwj-only', '\u200D'],
    ];
    const ew = {};
    for (const [name, text] of ecases) ew[name] = r(measure(mkE(text)));
    say('EMOJI widths: ' + ecases.map(([n]) => `${n}=${ew[n]}`).join(' '));
    say(`EMOJI zero-advance controls add nothing=${['a+ZWJ', 'a+ZWSP', 'a+SHY', 'a+RLM', 'a+LRM', 'a+VS16'].every((k) => ew[k] === ew['a'])}`);
    say(`EMOJI zwj-only=${ew['zwj-only']} vs16-only=${ew['vs16-only']}`);
    say(`EMOJI family-zwj=${ew['family-zwj']} family-no-zwj=${ew['family-no-zwj']} one-man=${ew['man']}`);
    say(`EMOJI flag=${ew['vn-flag']} single-RI=${ew['single-ri']}`);

    // ---- CJK line breaking ------------------------------------------
    // A block element's own getClientRects() returns its border box, not one
    // rect per line, so line geometry has to come from Range rects over the
    // text node. lineBoxes walks the string and cuts it wherever the prefix
    // starts occupying another line.
    const lineBoxes = (el) => {
      const s = el.firstChild;
      const chars = Array.from(s.nodeValue);
      const rng = document.createRange();
      const out = [];
      let start = 0;
      let prev = 1;
      const flush = (end) => {
        if (end <= start) return;
        rng.setStart(s, start);
        rng.setEnd(s, end);
        const rs = rng.getClientRects();
        if (!rs.length) return;
        const b = rs[0];
        out.push({ x: r(b.x), y: r(b.y), w: r(b.width), text: chars.slice(start, end).join('') });
      };
      for (let i = 1; i <= chars.length; i++) {
        rng.setStart(s, 0);
        rng.setEnd(s, i);
        const n = rng.getClientRects().length;
        // The character at i-1 is the one that spilled: the previous line ends
        // before it, and the next line starts at it.
        if (n > prev) { flush(i - 1); start = i - 1; prev = n; }
      }
      flush(chars.length);
      return out;
    };
    const show = (tag, el, extra) => {
      const ls = lineBoxes(el);
      say(`${tag} lines=${ls.length}${extra ? ' ' + extra : ''}`);
      say(`${tag} boxes: ` + ls.map((l, i) => `#${i} x=${l.x} y=${l.y} w=${l.w} text=${JSON.stringify(l.text)}`).join(' | '));
      return ls;
    };

    const cjk = document.getElementById('cjk');
    say(`CJK chars=${Array.from(cjk.textContent.trim()).length}`);
    const cjkLines = show('CJK', cjk, `box=${cjk.clientWidth} scrollWidth=${cjk.scrollWidth}`);
    const oneChar = (() => {
      const s = document.createElement('span');
      s.textContent = '中';
      const d = document.createElement('div');
      d.appendChild(s);
      document.getElementById('emo').appendChild(d);
      return r(measure(s));
    })();
    say(`CJK one ideograph advance=${oneChar} chars on line 0=${cjkLines.length ? Array.from(cjkLines[0].text).length : 0}`);

    // Kinsoku: does a full stop get left alone at the start of a line?
    const kin = document.getElementById('kin');
    const kinChars = Array.from(kin.firstChild.nodeValue);
    const stopAt = kinChars.findIndex((c) => c === '。');
    const kinLines = show('KINOKU', kin, `stop at char index ${stopAt} of ${kinChars.length}`);
    say(`KINOKU stop leads a line=${kinLines.some((l) => Array.from(l.text)[0] === '。')}`);

    // ---- Long unbroken word and the break-opportunity cases ----------
    const long = document.getElementById('long');
    show('LONG', long, `chars=${long.textContent.length} clientWidth=${long.clientWidth} scrollWidth=${long.scrollWidth}`);
    const wrap = document.getElementById('wrap');
    show('WRAP', wrap, `clientWidth=${wrap.clientWidth} scrollWidth=${wrap.scrollWidth}`);
    for (const id of ['shy', 'zws', 'ka']) {
      const el = document.getElementById(id);
      show(`BREAK-${id}`, el, `clientWidth=${el.clientWidth} scrollWidth=${el.scrollWidth}`);
    }
    for (const id of ['mix', 'kana', 'hangul', 'idal', 'alid']) {
      const el = document.getElementById(id);
      const ls = show(id.toUpperCase(), el, `clientWidth=${el.clientWidth} scrollWidth=${el.scrollWidth}`);
      say(`${id.toUpperCase()} chars per line=${ls.map((l) => Array.from(l.text).length).join(',')}`);
    }
    say(`CJK chars per line=${cjkLines.map((l) => Array.from(l.text).length).join(',')}`);

    // ---- RTL order ---------------------------------------------------
    const orderOf = (el) => {
      const s = el.firstChild;
      const chars = Array.from(s.nodeValue);
      const rng = document.createRange();
      const seen = [];
      for (let i = 0; i < chars.length; i++) {
        rng.setStart(s, i);
        rng.setEnd(s, i + 1);
        const rs = rng.getClientRects();
        if (!rs.length) continue;
        seen.push({ i, ch: chars[i], x: r(rs[0].x), y: r(rs[0].y), w: r(rs[0].width) });
      }
      return seen;
    };
    const descending = (a) => a.every((v, i) => i === 0 || v <= a[i - 1]);
    const ascending = (a) => a.every((v, i) => i === 0 || v >= a[i - 1]);
    for (const id of ['rtl', 'ltr']) {
      const cs = orderOf(document.getElementById(id));
      say(`${id.toUpperCase()} per-char: ` + cs.map((c) => `#${c.i} ${JSON.stringify(c.ch)} x=${c.x} y=${c.y} w=${c.w}`).join(' '));
      const arab = cs.filter((c) => /[\u0600-\u06FF]/.test(c.ch));
      const dig = cs.filter((c) => /[0-9]/.test(c.ch));
      const lat = cs.filter((c) => /[a-z]/.test(c.ch));
      say(`${id.toUpperCase()} summary: arabic-descending=${descending(arab.map((c) => c.x))} ` +
        `digits-ascending=${ascending(dig.map((c) => c.x))} latin-ascending=${ascending(lat.map((c) => c.x))} ` +
        `line-boxes=${lineBoxes(document.getElementById(id)).length} all-descending=${descending(cs.map((c) => c.x))}`);
    }

    const boxes = {};
    for (const id of ['cjk', 'kin', 'mix', 'idal', 'alid', 'kana', 'hangul', 'long', 'shy', 'zws', 'ka', 'wrap', 'rtl', 'ltr']) boxes[id] = rect(document.getElementById(id));
    return { report: lines.join('\n'), boxes };
  });

  console.log(report);

  const png = (await page.screenshot()).toString('base64');
  const counts = await page.evaluate(async ({ b64, boxes }) => {
    const img = new Image();
    await new Promise((res, rej) => {
      img.onload = res;
      img.onerror = rej;
      img.src = 'data:image/png;base64,' + b64;
    });
    const cv = document.createElement('canvas');
    cv.width = img.width;
    cv.height = img.height;
    const ctx = cv.getContext('2d', { willReadFrequently: true });
    ctx.drawImage(img, 0, 0);
    const out = {};
    for (const [k, b] of Object.entries(boxes)) {
      const d = ctx.getImageData(b.x, b.y, b.w, b.h).data;
      let n = 0;
      for (let i = 0; i < d.length; i += 4) {
        if (d[i + 3] > 0 && (d[i] < 200 || d[i + 1] < 200 || d[i + 2] < 200)) n++;
      }
      out[k] = n;
    }
    return { out, size: [img.width, img.height] };
  }, { b64: png, boxes });
  console.log(`INK (dark pixels inside each box's own rect, capture ${counts.size}) ` +
    Object.entries(counts.out).map(([k, v]) => `${k}=${v} in ${boxes[k].w}x${boxes[k].h}@(${boxes[k].x},${boxes[k].y})`).join(' '));

  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
