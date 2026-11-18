const { chromium } = require('playwright');

const VIEWPORT = { width: 900, height: 640 };

const PAGE = `
<!doctype html>
<meta charset="utf-8">
<style>
  * { box-sizing: border-box; }
  body { margin: 0; font: 16px sans-serif; color: rgb(10, 20, 30); background: #fff; }
  .g input, .g select, .g button, .g textarea { display: block; }
  .inh input, .inh select, .inh button, .inh textarea { font: inherit; }
  .ini input, .ini select, .ini button, .ini textarea { font: initial; }
  .un input, .un button { font: unset; }
  .deep { font: 20px Georgia, serif; }
  .deep input { font: inherit; }
  .loud { font: italic 700 20px/30px Georgia, serif; }
  #pseudoHost::before { content: "P"; font: inherit; }
  #pseudoHost::after { content: "A"; font: initial; }
  #wide { position: absolute; left: 20px; top: 20px; }
  #row { position: absolute; left: 320px; top: 20px; width: 260px; }
  #row input { display: block; }
</style>
<div id="wide">
  <div class="g plain"><input value="Hello"><select><option>Go</option></select><button>Go</button><textarea>Hello</textarea></div>
  <div class="g inh"><input value="Hello"><select><option>Go</option></select><button>Go</button><textarea>Hello</textarea></div>
  <div class="g ini"><input value="Hello"><select><option>Go</option></select><button>Go</button><textarea>Hello</textarea></div>
  <div class="g un"><input value="Hello"><button>Go</button></div>
  <div class="g deep"><input value="Hello"></div>
  <div class="g loud"><input id="loudInherit" style="font: inherit" value="Hello"><input id="loudInitial" style="font: initial" value="Hello"><input id="loudUnset" style="font: unset" value="Hello"></div>
</div>
<div id="row">
  <input id="inheritThenLonghand" style="font: inherit; font-size: 12px" value="Hello">
  <input id="longhandThenInherit" style="font-size: 12px; font: inherit" value="Hello">
  <input id="invalidMix" style="font: inherit bold 12px Arial" value="Hello">
  <input id="invalidTrailing" style="font: 12px Arial inherit" value="Hello">
  <input id="quotedFamily" style="font: 12px Arial, 'inherit'" value="Hello">
  <input id="initialThenLonghand" style="font: initial; font-family: sans-serif" value="Hello">
  <div id="divInitial" style="font: initial"><span id="divInitialKid">text</span></div>
  <div class="loud" id="loudHost"><span id="pseudoHost">text</span></div>
</div>
`;

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(PAGE, { waitUntil: 'load' });

  const report = await page.evaluate(() => {
    const lines = [];
    const say = (s) => lines.push(s);
    const probe = (label, el) => {
      if (!el) { say(`${label.padEnd(26)} MISSING`); return; }
      const cs = getComputedStyle(el);
      const r = el.getBoundingClientRect();
      say(`${label.padEnd(26)} size=${cs.fontSize} family=${cs.fontFamily} weight=${cs.fontWeight} ` +
        `style=${cs.fontStyle} variant=${cs.fontVariant} stretch=${cs.fontStretch} ` +
        `line-height=${cs.lineHeight} box=${r.width}x${r.height}`);
    };
    const group = (cls) => {
      const host = document.querySelector('.' + cls);
      if (!host) { say(`${cls} host MISSING`); return; }
      for (const tag of ['input', 'select', 'button', 'textarea']) {
        const el = host.querySelector(tag);
        if (el) probe(`${cls} ${tag}`, el);
      }
    };
    for (const cls of ['plain', 'inh', 'ini', 'un', 'deep']) group(cls);
    for (const id of ['inheritThenLonghand', 'longhandThenInherit', 'invalidMix', 'invalidTrailing',
      'quotedFamily', 'initialThenLonghand', 'divInitial', 'divInitialKid',
      'loudInherit', 'loudInitial', 'loudUnset']) {
      probe(id, document.getElementById(id));
    }
    probe('loud host div', document.querySelector('.loud'));
    for (const tok of ['::before', '::after']) {
      const el = document.getElementById('pseudoHost');
      const cs = getComputedStyle(el, tok);
      const r = el.getBoundingClientRect();
      say(`pseudoHost ${tok.padEnd(12)} size=${cs.fontSize} family=${cs.fontFamily} weight=${cs.fontWeight} ` +
        `style=${cs.fontStyle} line-height=${cs.lineHeight} hostBox=${r.width}x${r.height}`);
    }
    return lines.join('\n');
  });
  console.log(report);
  await browser.close();
}

main().catch((e) => { console.error(e); process.exit(1); });
