// BR-111 oracle: what does a real IME composition look like to the page?
//
// Vietnamese input is typed through a host IME that *replaces* the passage it
// marked on the previous step (`ti` -> `tie` -> `tiế` -> `tiếng`), so the two
// failure modes the catalog names - a duplicated base letter and a lost tone
// mark - are both visible in the value after each replacement. To record them
// honestly the behaviour has to be measured from Chromium, not recalled:
// Playwright cannot drive the macOS IME, but CDP `Input.imeSetComposition` and
// `Input.insertText` inject the same composition the native text input client
// receives, and Chromium then dispatches trusted composition/input events on
// its own.
//
// Sections, each printing the unfiltered event stream (type, inputType, data,
// isComposing, isTrusted, cancelable) plus the field state (value,
// selectionStart/End, Selection.toString()) after every step:
//   1. <input>, <textarea>, an empty contenteditable and a pre-filled one.
//   2. a commit whose text differs from the marked passage (`te` -> `ê`), the
//      shape a real candidate window produces.
//   3. a cancelled composition - an empty marked passage.
//   4. blur, to see whether `change` is trusted.
//   5. the same word with combining tone marks, which reads whether the engine
//      stores the bytes verbatim and counts offsets in UTF-16 code units.
//
// Seam limits, recorded rather than papered over: `Input.insertText` finalises
// a composition through its own path, so the commit arrives with
// inputType=insertCompositionText and compositionend.isTrusted=false. A native
// candidate commit is expected to report insertFromComposition; this seam
// cannot show that, so no test here asserts the commit inputType or its trust.
//
// Run: node scripts/verify-br111-ime-composition.js
//      (needs playwright chromium: `npm i -D playwright && npx playwright install chromium`)
const { chromium } = require('playwright');

const NFC = ['ti', 'tie', 'tiế', 'tiếng'];
// Same word with the accents as combining marks: e + U+0302 + U+0301 then ng.
// That is 7 code points where the precomposed spelling is 5, and every one of
// them is in the BMP - so this run reads whether Chromium stores the bytes it
// was handed verbatim (and counts offsets in code units) or normalises the
// field to the precomposed form underneath.
const NFD = ['ti', 'tie', 'tie\u0302\u0301', 'tie\u0302\u0301ng'];
const TARGETS = ['i', 't', 'c', 'c2'];
const TAGS = { i: 'input', t: 'textarea', c: 'empty contenteditable div', c2: 'contenteditable div holding "x"', b: 'input' };

const HTML =
  '<!doctype html><html><head><meta charset="utf-8"><style>' +
  'body{margin:0;font:16px Arial,sans-serif}' +
  'input,textarea,div[contenteditable]{position:absolute;width:220px;height:24px}' +
  '#i{left:10px;top:10px}#t{left:10px;top:50px}#c{left:10px;top:90px;height:40px}' +
  '#c2{left:10px;top:140px;height:40px}#b{left:10px;top:190px}#z{left:10px;top:230px}' +
  '</style></head><body>' +
  '<input id="i"><textarea id="t"></textarea>' +
  '<div id="c" contenteditable="true"></div>' +
  '<div id="c2" contenteditable="true">x</div>' +
  '<input id="b"><input id="z" value="other">' +
  '<script>' +
  'window.LOG=[];' +
  'window.NOTE=function(e){var el=e.target;' +
  ' var editable=el.value===undefined;' +
  ' var sel=window.getSelection();' +
  ' LOG.push({type:e.type,id:el.id,inputType:e.inputType||null,' +
  ' data:e.data===undefined?null:e.data,comp:e.isComposing===undefined?null:e.isComposing,' +
  ' trust:e.isTrusted,cancel:e.cancelable,' +
  ' value:editable?el.textContent:el.value,' +
  ' ss:el.selectionStart===undefined?-1:el.selectionStart,' +
  ' se:el.selectionEnd===undefined?-1:el.selectionEnd,' +
  ' sdir:(el.selectionDirection===undefined?null:el.selectionDirection),anchor:(window.getSelection()&&document.activeElement&&document.activeElement.selectionStart!==undefined?document.activeElement.selectionStart:null),' +
  ' selstr:sel?sel.toString():null});};' +
  "for (var _a=['i','t','c','c2','b','z'],_i=0;_i<_a.length;_i++){var el=document.getElementById(_a[_i]);" +
  " for (var _b=['compositionstart','compositionupdate','compositionend'," +
  "'beforeinput','input','change','blur','focusout'],_j=0;_j<_b.length;_j++)(function(ty){" +
  '  el.addEventListener(ty,function(e){window.NOTE(e)});}).call(null,_b[_j]);}' +
  '/* Does Chromium treat the preview it puts in an empty editor as a real DOM ' +
  'insertion a page can observe? Observe #c and print what arrives. */' +
  'window.MO=[];' +
  '(function(){var t=document.getElementById("c");' +
  ' new MutationObserver(function(rs){for(var k=0;k<rs.length;k++){var r=rs[k];' +
  '  MO.push({type:r.type,target:r.target.id,added:r.addedNodes.length,' +
  '  first:r.addedNodes.length?r.addedNodes[0].nodeName:null,' +
  '  value:r.type==="characterData"?r.target.textContent:null});}})' +
  '  .observe(t,{childList:true,characterData:true,subtree:true});})();' +
  '</script></body></html>';

function show(label, rows) {
  console.log('--- ' + label);
  for (const r of rows) {
    console.log(
      `  ${r.type.padEnd(17)} id=${r.id} inputType=${String(r.inputType).padEnd(21)} ` +
        `data=${JSON.stringify(r.data)} comp=${r.comp} trusted=${r.trust} cancelable=${r.cancel} ` +
        `value=${JSON.stringify(r.value)} ss=${r.ss} se=${r.se} sdir=${r.sdir} sel=${JSON.stringify(r.selstr)}`
    );
  }
}

async function clear(page) {
  await page.evaluate(() => (window.LOG = []));
}

async function logOf(page) {
  return page.evaluate(() => window.LOG.slice());
}

async function drive(page, cdp, sel, steps, commit, mark = true) {
  await clear(page);
  for (const s of steps) {
    if (!mark) break;
    await cdp.send('Input.imeSetComposition', { text: s, selectionStart: 0, selectionEnd: s.length });
  }
  const during = await logOf(page);
  await clear(page);
  if (commit !== null) await cdp.send('Input.insertText', { text: commit });
  const after = await logOf(page);
  return { during, after };
}

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 400, height: 300 } });
  const cdp = await page.context().newCDPSession(page);
  await page.setContent(HTML);
  console.log(`chromium ${browser.version()} | steps=${JSON.stringify(NFC)}`);

  const results = {};
  for (const id of TARGETS) {
    await page.click('#' + id);
    if (id === 'c2') {
      // Put the caret after the existing "x" so the composition has a place to
      // land: the interesting question is whether the marked text is previewed
      // live inside an existing passage.
      await page.evaluate(() => {
        const el = document.getElementById('c2');
        const r = document.createRange();
        r.setStart(el.firstChild, 1);
        r.collapse(true);
        const s = getSelection();
        s.removeAllRanges();
        s.addRange(r);
      });
    }
    const r = await drive(page, cdp, '#' + id, NFC, NFC[NFC.length - 1]);
    results[id] = r;
    show(`${TAGS[id]} (#${id}) — composing ${NFC.length} steps`, [...r.during, ...r.after]);
    if (id === 'c') {
      r.mo = await page.evaluate(() => {
        const m = window.MO.slice();
        window.MO.length = 0;
        return m;
      });
      console.log('  mutation records ' + JSON.stringify(r.mo));
    }
    const state = await page.evaluate(
      ([s]) => {
        const el = document.querySelector(s);
        const editable = el.value === undefined;
        const v = editable ? el.textContent : el.value;
        return { value: v, utf16: v.length, runes: [...v].length, ss: el.selectionStart, sel: getSelection().toString() };
      },
      ['#' + id]
    );
    console.log('  final ' + JSON.stringify(state));
    await page.evaluate(
      ([s]) => {
        const el = document.querySelector(s);
        if (el.value !== undefined) el.value = '';
        else el.textContent = s === '#c2' ? 'x' : '';
      },
      ['#' + id]
    );
  }

  // 2. commit text differs from the marked passage.
  await page.click('#b');
  const diff = await drive(page, cdp, '#b', ['te'], 'ê');
  show('#b — marked "te", committed "ê"', [...diff.during, ...diff.after]);
  const diffState = await page.evaluate(() => {
    const el = document.getElementById('b');
    return { value: el.value, ss: el.selectionStart, se: el.selectionEnd };
  });
  console.log('  final ' + JSON.stringify(diffState));

  // 3. a cancelled composition: the host marks nothing and drops the passage.
  await page.evaluate(() => { document.getElementById('b').value = ''; });
  await clear(page);
  await cdp.send('Input.imeSetComposition', { text: 'te', selectionStart: 0, selectionEnd: 2 });
  await cdp.send('Input.imeSetComposition', { text: '', selectionStart: 0, selectionEnd: 0 });
  const cancelLog = await logOf(page);
  show('#b — marked "te" then an empty marked passage (cancel)', cancelLog);
  const cancelState = await page.evaluate(() => {
    const el = document.getElementById('b');
    return { value: el.value, ss: el.selectionStart, se: el.selectionEnd };
  });
  console.log('  final ' + JSON.stringify(cancelState));

  // 4. blur: is `change` trusted, and does it carry the composed value?
  // Focus has to move back to #i first: section 3 left it on #b, and a
  // transcript that attributes #b's events to #i is not a measurement.
  await page.evaluate(() => { document.getElementById('i').value = ''; });
  await page.click('#i');
  await drive(page, cdp, '#i', NFC, NFC[NFC.length - 1]);
  await clear(page);
  await page.click('#z');
  const blurLog = await logOf(page);
  show('#i — blur after a committed composition', blurLog);

  // 5. combining marks.
  await page.evaluate(() => { document.getElementById('i').value = ''; });
  await page.click('#i');
  const nfd = await drive(page, cdp, '#i', NFD, NFD[NFD.length - 1]);
  show('#i — combining-mark (NFD) composition', [...nfd.during, ...nfd.after]);
  const nfdState = await page.evaluate(() => {
    const el = document.getElementById('i');
    return { value: el.value, utf16: el.value.length, runes: [...el.value].length, ss: el.selectionStart, se: el.selectionEnd };
  });
  console.log('  final ' + JSON.stringify(nfdState));

  const inputs = (r) => r.during.filter((e) => e.type === 'input');
  const checks = [
    ['compositionstart carries empty data', results.i.during[0].type === 'compositionstart' && results.i.during[0].data === ''],
    ['each step replaces the marked passage, never concatenates', inputs(results.i).every((r) => NFC.includes(r.value))],
    ['the marked range is selected while composition is live', inputs(results.i).every((r) => r.ss === 0 && r.se === r.value.length)],
    ['input arrives with isComposing true, beforeinput is not cancelable', results.i.during.filter((r) => r.type === 'beforeinput').every((r) => r.cancel === false && r.comp === true)],
    ['compositionupdate is cancelable', results.i.during.filter((r) => r.type === 'compositionupdate').every((r) => r.cancel === true)],
    ['an empty contenteditable previews every step live', inputs(results.c).length === NFC.length && inputs(results.c).every((r) => NFC.includes(r.value))],
    ['the empty editor gains its preview node as an observed insertion', (results.c.mo || []).some((m) => m.type === 'childList' && m.first === '#text')],
    ['the steps that only rewrite the preview are characterData changes', (results.c.mo || []).filter((m) => m.type === 'characterData').length > 0],
    ['each marked step selects exactly the marked text', inputs(results.c).every((r) => r.selstr === r.value)],
    ['the commit collapses the selection', results.c.after.filter((r) => r.type === 'input').every((r) => r.selstr === '')],
    ['a filled contenteditable keeps its text and appends the preview', inputs(results.c2).every((r) => r.value.indexOf('x') === 0)],
    ['commit inserts the committed text, not the marked one', diff.after.some((r) => r.type === 'input' && r.value === 'ê')],
    ['a cancel removes the preview and ends composition', cancelState.value === '' && cancelLog.some((r) => r.type === 'compositionend') && cancelLog.some((r) => r.type === 'input' && r.value === '')],
    ['blur fires a trusted change carrying the value', blurLog.some((r) => r.type === 'change' && r.trust === true && r.value === 'tiếng')],
    ['combining marks are stored verbatim (no NFC folding)', nfdState.utf16 === 7 && nfdState.runes === 7 && nfdState.se === 7],
  ];
  let ok = true;
  for (const [name, pass] of checks) {
    console.log(`${pass ? ' OK  ' : ' FAIL'} ${name}`);
    ok = ok && pass;
  }
  console.log(ok ? 'BR-111 ORACLE OK' : 'BR-111 ORACLE INCOMPLETE - do not encode these rules as tests');
  await browser.close();
  process.exitCode = ok ? 0 : 1;
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
