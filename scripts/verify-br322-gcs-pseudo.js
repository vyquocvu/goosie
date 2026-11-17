const { chromium } = require('playwright');

const VIEWPORT = { width: 900, height: 640 };

const PAGE = `
<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; font: 16px/1.5 Arial, Helvetica, sans-serif; color: rgb(10, 20, 30); background: #fff; counter-reset: c 3; }
  #plain { position: absolute; left: 20px; top: 20px; width: 300px; }
  #gen { position: absolute; left: 20px; top: 90px; width: 300px; color: rgb(1, 2, 3); font-size: 20px; }
  #gen::before { content: "PRE"; color: rgb(200, 0, 0); font-size: 12px; display: inline-block; width: 40px; height: 15px; margin-right: 7px; padding: 2px; background-color: rgb(0,0,200); }
  #gen::after { content: ""; color: rgb(7, 8, 9); }
  #line { position: absolute; left: 20px; top: 170px; width: 300px; }
  #line::first-line { color: rgb(40, 50, 60); font-weight: bold; text-transform: uppercase; width: 99px; margin-left: 8px; letter-spacing: 1px; }
  #letter { position: absolute; left: 20px; top: 260px; width: 300px; }
  #letter::first-letter { color: rgb(60, 70, 80); font-size: 32px; float: left; margin-right: 4px; }
  #sh { position: absolute; left: 20px; top: 340px; }
  #sh::placeholder { color: rgb(180, 30, 30); }
  #dh { position: absolute; left: 20px; top: 380px; color: rgb(3, 4, 5); }
  #nh { position: absolute; left: 20px; top: 560px; }
  #mark { position: absolute; left: 20px; top: 420px; }
  #mark li::marker { color: rgb(90, 90, 90); font-size: 18px; }
  #sel { position: absolute; left: 20px; top: 500px; width: 300px; }
  #sel::selection { color: rgb(255, 255, 0); background-color: rgb(0, 128, 0); }
  #empty { position: absolute; left: 380px; top: 20px; width: 300px; }
  #empty::before { content: none; color: rgb(11, 12, 13); }
  #inh { position: absolute; left: 380px; top: 90px; width: 300px; color: rgb(70, 70, 70); font-size: 13px; }
  #none { position: absolute; left: 380px; top: 130px; display: none; }
  #none::before { content: "HIDDEN"; color: rgb(30, 30, 30); }
  #webrtc { position: absolute; left: 380px; top: 160px; }
  #webrtc::-webkit-scrollbar { width: 9px; }
  #host { position: absolute; left: 380px; top: 200px; }
  .c { position: absolute; left: 620px; width: 260px; }
  #cnorm { top: 20px; } #cnorm::before { content: normal; }
  #curl { top: 50px; } #curl::before { content: url(none.png); }
  #ccount { top: 80px; } #ccount::before { content: counter(c); }
  #cattr { top: 110px; } #cattr::before { content: attr(data-x); }
  #cslash { top: 140px; } #cslash::before { content: "a" / "b"; }
  #cempty { top: 170px; } #cempty::before { content: ""; }
  #cnone2 { top: 200px; } #cnone2::before { content: none; }
  #cmark { top: 230px; } #cmark::marker { content: ""; }
  #cfl { top: 260px; } #cfl::first-line { content: none; } #cfl::first-letter { content: "Z"; }
</style>
<div id="plain">plain text</div>
<div id="gen">host text</div>
<div id="line">alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu</div>
<div id="letter">alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu</div>
<input id="sh" placeholder="type here">
<input id="dh">
<input id="nh" placeholder="bare">
<ul id="mark"><li>one</li><li>two</li></ul>
<div id="sel">selectable</div>
<div id="empty">empty content</div>
<div id="inh">inherited host</div>
<div id="none">hidden host</div>
<input id="webrtc">
<div id="host"></div>
<div class="c" id="cnorm">norm</div>
<div class="c" id="curl">url</div>
<div class="c" id="ccount">count</div>
<div class="c" id="cattr" data-x="ATTR">attr</div>
<div class="c" id="cslash">slash</div>
<div class="c" id="cempty">empty</div>
<div class="c" id="cnone2">none</div>
<ul class="c" id="cmark"><li>mark</li></ul>
<div class="c" id="cfl">first line text</div>
`;

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(PAGE, { waitUntil: 'load' });

  const report = await page.evaluate(() => {
    const lines = [];
    const say = (s) => lines.push(s);
    customElements.define('x-part', class extends HTMLElement {
      connectedCallback() {
        const root = this.attachShadow({ mode: 'open' });
        const st = document.createElement('style');
        st.textContent = '::part(widget){color:rgb(123,123,123)}';
        const box = document.createElement('div');
        box.setAttribute('part', 'widget');
        box.textContent = 'w';
        root.append(st, box);
      }
    });
    document.getElementById('host').appendChild(document.createElement('x-part'));

    const listShape = (cs) => {
      const own = Object.getOwnPropertyNames(cs).filter((n) => /^\d+$/.test(n));
      let len = -1;
      try { len = cs.length; } catch (e) { /* noop */ }
      return `len=${len} indexed=${own.length} item0=${(() => { try { return JSON.stringify(cs.item(0)) + '|' + JSON.stringify(cs[0]); } catch (e) { return 'THREW'; } })()}`;
    };
    const reads = (cs) => `color=${JSON.stringify(cs.color)} content=${JSON.stringify(cs.content)} gpv=${JSON.stringify(cs.getPropertyValue('color'))} gpvContent=${JSON.stringify(cs.getPropertyValue('content'))} fontSize=${JSON.stringify(cs.fontSize)} display=${JSON.stringify(cs.display)} width=${JSON.stringify(cs.width)}`;

    const probe = (label, id, pseudo) => {
      let cs;
      try {
        cs = window.getComputedStyle(document.getElementById(id), pseudo);
      } catch (e) {
        say(`${label} THREW ${String(e)}`);
        return;
      }
      if (cs === null) { say(`${label} null`); return; }
      if (cs === undefined) { say(`${label} undefined`); return; }
      say(`${label} typeof=${typeof cs} tag=${Object.prototype.toString.call(cs)} ctor=${cs.constructor && cs.constructor.name} ${listShape(cs)}`);
      say(`${label} ${reads(cs)}`);
    };

    say('=== reference: element path (no argument) ===');
    for (const id of ['plain', 'gen', 'dh', 'nh', 'none']) probe(`REF ${id}`, id, undefined);

    say('=== Q1 non-pseudo tokens ===');
    for (const tok of ['', ' ', 'before', 'null', '::nope', ':not-a-pseudo', '::before extra', '::before;', '::-moz-none', '::-nope', ':hover', '::before:hovert']) probe(`Q1gen ${JSON.stringify(tok)}`, 'gen', tok);
    probe('Q1 null-literal', 'gen', null);
    probe('Q1 undefined-literal', 'gen', undefined);
    let threw = '';
    for (const [tag, tok] of [['number', 42], ['object', {}], ['boolean', true], ['array', ['::before']], ['symbolish', '42']]) {
      threw = '';
      let cs;
      try { cs = window.getComputedStyle(document.getElementById('gen'), tok); } catch (e) { threw = ' THREW ' + e.name; }
      say(`Q1 ${tag}-arg ${threw === '' ? (cs === null ? 'null' : `object len=${cs.length}`) : threw}`);
    }

    say('=== Q2 colon/case/whitespace spellings (host #gen has ::before) ===');
    for (const tok of [':before', '::before', '::BEFORE', '::Before', '::before ', '::before;', ':: before', ' :before', '::\\3: before']) probe(`Q2 ${JSON.stringify(tok)}`, 'gen', tok);

    say('=== Q3 pseudos with no author rule (#plain) ===');
    for (const tok of ['::before', '::after', '::first-line', '::first-letter', '::marker', '::selection', '::placeholder', '::backdrop', '::cue', '::target-text', '::view-transition', '::grammar-error', '::file-selector-button', '::part(widget)', '::part(missing)', '::-webkit-scrollbar']) probe(`Q3 ${tok}`, 'plain', tok);

    say('=== Q3b ungenerated but authored (#empty content:none / #inh / #none display:none) ===');
    for (const id of ['empty', 'inh', 'none']) for (const tok of ['::before', '::after', '::first-line']) probe(`Q3b ${id} ${tok}`, id, tok);

    say('=== Q4 generated ::before / ::after (#gen) ===');
    for (const tok of ['::before', '::after']) probe(`Q4 ${tok}`, 'gen', tok);

    say('=== Q5 ::first-line subset (#line: color/weight/transform/width/margin/letter-spacing authored) ===');
    probe('Q5 first-line', 'line', '::first-line');
    say(`Q5 element ${(() => { const cs = window.getComputedStyle(document.getElementById('line')); return `color=${cs.color} fontWeight=${cs.fontWeight} textTransform=${cs.textTransform} width=${cs.width} marginLeft=${cs.marginLeft} letterSpacing=${cs.letterSpacing}`; })()}`);

    say('=== Q6 ::first-letter / ::placeholder / ::marker / ::selection / webkit ===');
    probe('Q6 first-letter', 'letter', '::first-letter');
    say(`Q6 first-letter extra ${(() => { const cs = window.getComputedStyle(document.getElementById('letter'), '::first-letter'); return `cssFloat=${cs.cssFloat} float=${cs.float} shapeMargin=${cs.shapeMargin}`; })()}`);
    for (const id of ['sh', 'dh', 'nh']) probe(`Q6 placeholder ${id}`, id, '::placeholder');
    probe('Q6 marker-on-ul', 'mark', '::marker');
    probe('Q6 selection', 'sel', '::selection');
    probe('Q6 part', 'host', '::part(widget)');
    probe('Q6 webkit-scrollbar', 'webrtc', '::-webkit-scrollbar');

    say('=== Q7 identity / aliasing / list ===');
    const el = window.getComputedStyle(document.getElementById('gen'));
    const pb = window.getComputedStyle(document.getElementById('gen'), '::before');
    say(`Q7 distinct-objects=${pb !== el} sameLen=${pb.length === el.length} elemColor=${el.color} pseudoColor=${pb.color}`);
    say(`Q7 dotEqGetprop=${pb.color === pb.getPropertyValue('color')} camel=${pb.fontSize} kebab=${pb.getPropertyValue('font-size')} dashed=${pb['font-size']} item=${pb.item(2)} itemOut=${pb.item(99999)} idx=${pb[2]}`);
    say(`Q7 proto=${Object.getPrototypeOf(pb) === Object.getPrototypeOf(el)} ctorName=${pb.constructor.name}`);
    say(`Q7 unknownProp=${JSON.stringify(pb.nope)} hasContent=${('content' in pb)}`);
    const twice = window.getComputedStyle(document.getElementById('gen'), '::before');
    say(`Q7 stable-across-calls=${twice.color === pb.color}`);

    say('=== Q8 mutation write-through (documented, not implemented) ===');
    try {
      pb.color = 'rgb(1, 1, 1)';
      say(`Q8 after-set color=${window.getComputedStyle(document.getElementById('gen'), '::before').color} inlineStyle=${document.getElementById('gen').getAttribute('style')}`);
    } catch (e) {
      say(`Q8 set-assignment THREW ${e.name}: ${e.message}`);
    }

    say('=== Q9 content spellings ===');
    for (const id of ['cnorm', 'curl', 'ccount', 'cattr', 'cslash', 'cempty', 'cnone2']) {
      const cs = window.getComputedStyle(document.getElementById(id), '::before');
      say(`Q9 ${id} content=${JSON.stringify(cs.content)} len=${cs.length} display=${cs.display}`);
    }
    say(`Q9 marker-empty content=${JSON.stringify(window.getComputedStyle(document.getElementById('cmark'), '::marker').content)}`);
    say(`Q9 first-line-none content=${JSON.stringify(window.getComputedStyle(document.getElementById('cfl'), '::first-line').content)}`);
    say(`Q9 first-letter-quote content=${JSON.stringify(window.getComputedStyle(document.getElementById('cfl'), '::first-letter').content)}`);
    say(`Q9 placeholder-content content=${JSON.stringify(window.getComputedStyle(document.getElementById('nh'), '::placeholder').content)}`);

    say('=== Q10 argument grammar edges ===');
    for (const tok of ['nope', 'first-line', ':::before', '::before\t', ':BEFORE', '::first-LINE', '::part( widget )', '::part()', '::part', '::slotted(div)', '::highlight(x)', '::before:after', '::after::before']) probe(`Q10 ${JSON.stringify(tok)}`, 'gen', tok);

    say('=== Q11 ::marker on the box owner ===');
    const markerProbe = (label, el, tok) => {
      const cs = window.getComputedStyle(el, tok);
      say(`${label} content=${JSON.stringify(cs.content)} color=${cs.color} font-size=${cs.fontSize} display=${cs.display}`);
    };
    markerProbe('Q11 mark-li', document.querySelector('#mark li'), '::marker');
    markerProbe('Q11 cmark-li', document.querySelector('#cmark li'), '::marker');
    markerProbe('Q11 mark-li-selection', document.querySelector('#mark li'), '::selection');

    return lines.join('\n');
  });

  console.log(report);
  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
