// TASK-037 reference measurement: what does Chromium do with whitespace in
// <option> values, the value attribute, and multiple authored `selected`
// options? Run: node scripts/verify-task037-option-value.js
//
// Every assertion in test/engine/option_value_test.go cites a line of this
// script's output. Do not encode a rule from memory - re-run and read.
const { chromium } = require('playwright');

function show(label, obj) {
  console.log(label + ' -> ' + JSON.stringify(obj));
}

// Each case gets its own select + name so FormData reports the value of every
// option instead of only the one the tie-break picked for a shared select.
const NBSP = ' ';
const CASES = [
  ['charcoal', '\n    Charcoal\n  '],
  ['hiruns', '  Hi   there '],
  ['tabinside', 'Tab\tin\tside'],
  ['crlfinside', 'a\r\n   b'],
  ['formfeedinside', 'a\f\fb'],
  ['vtabinside', 'a\u000B\u000Bb'],
  ['edgevtab', '\u000Bleading vt\u000B'],
  ['nbsp', NBSP + 'nbsp edge' + NBSP],
  ['blankonly', '   '],
  ['elementkids', 'a <b>bold</b>   c'],
];
const ATTR_CASES = [
  ['attrpad', '  pad  me  '],
  ['attrnewline', 'a\n b\tc'],
  ['attrempty', ''],
  ['attrnbsp', NBSP + 'x' + NBSP],
];

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } });

  const textMarkup = CASES.map(([name, body]) =>
    `<select name="${name}"><option selected>${body}</option></select>`).join('\n');
  const attrMarkup = ATTR_CASES.map(([name, v]) =>
    `<select name="${name}"><option selected value="${v}">ignored text</option></select>`).join('\n');

  await page.setContent(`<!doctype html><html><body><form id="f">
${textMarkup}
<select name="labelattr"><option selected label="  My Label  ">text content</option></select>
${attrMarkup}
</form></body></html>`);

  const read = await page.evaluate(() => {
    const out = [];
    for (const sel of document.querySelectorAll('select')) {
      const o = sel.options[0];
      out.push({ name: sel.name, textContent: o.textContent, value: o.value, label: o.label });
    }
    return out;
  });
  for (const r of read) show('OPTION name=' + r.name, { textContent: r.textContent, value: r.value, label: r.label });

  const encoded = await page.evaluate(() => {
    const fd = new FormData(document.getElementById('f'));
    const list = [];
    for (const [k, v] of fd.entries()) list.push([k, v]);
    return { entries: list, encoded: new URLSearchParams(fd).toString() };
  });
  show('FORMDATA', encoded.entries);
  show('ENCODED', encoded.encoded);

  // Tie-break: exactly two options carry `selected`. Which one wins?
  await page.setContent(`<!doctype html><html><body><form id="f">
<select id="s" name="c">
  <option id="a" value="a" selected>A</option>
  <option id="b" value="b" selected>B</option>
  <option id="cc" value="c">C</option>
</select></form></body></html>`);
  show('TIEBREAK two-selected', await page.evaluate(() => {
    const s = document.getElementById('s');
    return {
      selectedIndex: s.selectedIndex, value: s.value,
      optionFlags: [...s.options].map(o => o.selected),
      attrFlags: [...s.querySelectorAll('option')].map(o => o.hasAttribute('selected')),
      formdata: [...new FormData(document.getElementById('f')).entries()],
    };
  }));

  // Keyboard on that two-selected document. A *closed* select ignores arrow keys
  // in headless Chromium, so the tie-break is measured on the inline listbox
  // (size=3), which honours them: active records that the keys really went to it.
  await page.focus('#s');
  await page.keyboard.press('ArrowDown');
  show('CLOSED two-selected after ArrowDown', await page.evaluate(() => {
    const s = document.getElementById('s');
    return {
      active: document.activeElement && document.activeElement.id,
      selectedIndex: s.selectedIndex, value: s.value,
    };
  }));

  await page.setContent(`<!doctype html><html><body>
<select id="s" name="c" size="3">
  <option id="a" value="a" selected>A</option>
  <option id="b" value="b" selected>B</option>
  <option id="cc" value="c">C</option>
</select></body></html>`);
  await page.focus('#s');
  const listbox = () => page.evaluate(() => {
    const s = document.getElementById('s');
    return {
      active: document.activeElement && document.activeElement.id,
      selectedIndex: s.selectedIndex, value: s.value,
      optionFlags: [...s.options].map(o => o.selected),
      attrFlags: [...s.querySelectorAll('option')].map(o => o.hasAttribute('selected')),
    };
  });
  show('LISTBOX two-selected initial', await listbox());
  await page.keyboard.press('ArrowDown');
  show('LISTBOX two-selected after ArrowDown', await listbox());
  await page.keyboard.press('ArrowUp');
  show('LISTBOX two-selected after ArrowUp', await listbox());

  // Control: one authored selection, the real-world shape. A move steps from it.
  await page.setContent(`<!doctype html><html><body>
<select id="s" name="c" size="3">
  <option id="a" value="a">A</option>
  <option id="b" value="b" selected>B</option>
  <option id="cc" value="c">C</option>
</select></body></html>`);
  await page.focus('#s');
  show('LISTBOX single-selected initial', await listbox());
  await page.keyboard.press('ArrowDown');
  show('LISTBOX single-selected after ArrowDown', await listbox());

  // The indented real-world markup (the GOOSIE-FORMVALUE-001 case), closed.
  await page.setContent(`<!doctype html><html><body><form id="f">
<select id="s" name="c">
  <option selected>
    Charcoal
  </option>
  <option value="b">B</option>
</select></form></body></html>`);
  await page.focus('#s');
  const indented = () => page.evaluate(() => {
    const s = document.getElementById('s');
    const o = s.options[s.selectedIndex];
    return { selectedIndex: s.selectedIndex, value: s.value, optionValue: o.value, optionLabel: o.label };
  });
  show('INDENTED initial', await indented());
  await page.keyboard.press('ArrowDown');
  show('INDENTED after ArrowDown', await indented());

  await browser.close();
})();
