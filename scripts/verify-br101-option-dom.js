// BR-101 / GOOSIE-FORMVALUE-002 reference measurement: what does Chromium
// report for the *other* option surfaces - option.text, option.label, the AX
// name of a closed select whose selected option carries a label attribute,
// options inside an optgroup, and a multiple control with two selections?
//
// Run: node scripts/verify-br101-option-dom.js
//
// Every assertion in the Go oracles cites a line of this output. Do not encode
// a rule from memory - re-run and read.
const { chromium } = require('playwright');

function show(label, obj) {
  console.log(label + ' -> ' + JSON.stringify(obj));
}

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } });

  // 1. The IDL surface on one option per shape. Padded attribute values are
  // quoted verbatim so trailing spaces survive JSON.
  await page.setContent(`<!doctype html><html><body>
<select id="s">
  <option id="plain" selected>
    Charcoal
  </option>
  <option id="runs" value="v-runs">  Hi   there </option>
  <option id="lab" label="  My Label  ">text content</option>
  <option id="labempty" label="">empty label attr</option>
  <option id="labonly" label="Label only"></option>
  <option id="blank" value="b">   </option>
</select></body></html>`);
  show('IDL', await page.evaluate(() => {
    const ids = ['plain', 'runs', 'lab', 'labempty', 'labonly', 'blank'];
    return ids.map((id) => {
      const o = document.getElementById(id);
      return {
        id,
        textContent: o.textContent,
        text: o.text,
        label: o.label,
        value: o.value,
        index: o.index,
        nbspFree: o.label === o.text,
      };
    });
  }));

  // Setting the IDL attributes: does label= write the content attribute, and
  // does value= change what .value and FormData report?
  show('IDL-SET', await page.evaluate(() => {
    const o = document.getElementById('runs');
    o.label = '  set  label  ';
    o.value = '  set  value  ';
    return {
      labelAttr: o.getAttribute('label'),
      label: o.label,
      valueAttr: o.getAttribute('value'),
      value: o.value,
      text: o.text,
    };
  }));

  // 2. Display precedence, measured where page script can see it: the
  // accessibility name/value of the closed select and of the option.
  await page.setContent(`<!doctype html><html><body>
<select id="s"><option id="wide" selected label="IIIII">WWWWW</option><option id="other">B</option></select>
<select id="s2"><option id="w2" selected>WWWWW</option></select>
<select id="s3" size="3"><option selected label="IIIII">WWWWW</option></select></body></html>`);
  const ax = async () => {
    const client = await page.context().newCDPSession(page);
    await client.send('Accessibility.enable');
    const { nodes } = await client.send('Accessibility.getFullAXTree');
    const keep = nodes
      .filter((n) => n.role && ['combobox', 'listbox', 'option', 'group'].includes(n.role.value))
      .map((n) => ({
        role: n.role.value,
        name: n.name && n.name.value,
        value: n.value && n.value.value,
      }));
    await client.detach();
    return keep;
  };
  show('AX label-vs-text', await ax());

  // The three shapes the display rule hinges on: is the label attribute used
  // verbatim, collapsed, or only when non-empty? The text content is "WWWWW" in
  // every case so a fallback is distinguishable from the label.
  await page.setContent(`<!doctype html><html><body>
<select id="s1"><option selected label="  My   Label  ">WWWWW</option></select>
<select id="s2"><option selected label="">WWWWW</option></select>
<select id="s3"><option selected label="   ">WWWWW</option></select>
<select id="s4"><option selected>WWWWW</option></select></body></html>`);
  show('AX label shapes', await ax());
  show('IDL label shapes', await page.evaluate(() => {
    return ['s1', 's2', 's3', 's4'].map((id) => {
      const o = document.querySelector('#' + id + ' option');
      return { id, label: o.label, value: o.value, text: o.text };
    });
  }));

  // Does a value attribute ever become the announced name when there is no text
  // and no label to show? And what does a whitespace-only label do to the
  // combobox value when a value is present?
  await page.setContent(`<!doctype html><html><body>
<select id="s5"><option selected value="V"></option></select>
<select id="s6"><option selected value="V" label="   "></option></select>
<select id="s7"><option selected value="V" label=""></option></select></body></html>`);
  show('AX value-only shapes', await ax());
  show('IDL value-only shapes', await page.evaluate(() => {
    return ['s5', 's6', 's7'].map((id) => {
      const o = document.querySelector('#' + id + ' option');
      return { id, label: o.label, value: o.value, text: o.text, selectValue: document.getElementById(id).value };
    });
  }));

  // 3. optgroup: how do .options, .index, .length and the flattened list read?
  await page.setContent(`<!doctype html><html><body>
<select id="s">
  <option id="top" value="top">Top</option>
  <optgroup id="g" label="  Group  Label  ">
    <option id="in1" value="in1">In one</option>
    <option id="in2" selected value="in2">In two</option>
  </optgroup>
  <option id="tail" value="tail">Tail</option>
</select></body></html>`);
  show('OPTGROUP', await page.evaluate(() => {
    const s = document.getElementById('s');
    return {
      length: s.options.length,
      ids: [...s.options].map((o) => o.id),
      indexes: [...s.options].map((o) => o.index),
      groupLabel: document.getElementById('g').label,
      groupLabelAttr: document.getElementById('g').getAttribute('label'),
      inOptionLabel: document.getElementById('in1').label,
      selectedId: s.options[s.selectedIndex].id,
      selectValue: s.value,
      optionTextOfGroup: [...s.options].map((o) => o.text),
    };
  }));
  show('OPTGROUP-DISPATCH', await page.evaluate(() => {
    // Does the <option> interface answer for an <optgroup>, and vice versa?
    const g = document.getElementById('g');
    const o = document.getElementById('in1');
    return {
      groupTag: Object.prototype.toString.call(g),
      groupIsOption: g instanceof HTMLOptionElement,
      groupIsGroup: g instanceof HTMLOptGroupElement,
      optionIsGroup: o instanceof HTMLOptGroupElement,
      groupSelected: 'selected' in g,
      optionValueIn: 'value' in o,
    };
  }));

  show('OPTGROUP-AX', await ax());

  // 4. A multiple control with two authored selections: select.value,
  // selectedIndex, selectedOptions and FormData all at once.
  await page.setContent(`<!doctype html><html><body><form id="f">
<select id="s" name="m" multiple>
  <option id="a" value="a">A</option>
  <option id="b" value="b" selected>B</option>
  <option id="c" value="c" selected>C</option>
</select></form></body></html>`);
  show('MULTIPLE two-selected', await page.evaluate(() => {
    const s = document.getElementById('s');
    return {
      selectValue: s.value,
      selectedIndex: s.selectedIndex,
      selectedOptions: [...s.selectedOptions].map((o) => o.id),
      optionFlags: [...s.options].map((o) => o.selected),
      formdata: [...new FormData(document.getElementById('f')).entries()],
      labels: [...s.selectedOptions].map((o) => o.label),
    };
  }));
  show('MULTIPLE after a third select', await page.evaluate(() => {
    const s = document.getElementById('s');
    s.options[0].selected = true;
    return {
      selectValue: s.value,
      selectedIndex: s.selectedIndex,
      selectedOptions: [...s.selectedOptions].map((o) => o.id),
      formdata: [...new FormData(document.getElementById('f')).entries()],
    };
  }));

  // 5. A single select with two selected read through the same surfaces, so a
  // Go tie-break cannot disagree between value, selectedIndex and AX.
  await page.setContent(`<!doctype html><html><body>
<select id="s">
  <option id="a" value="a" selected>
    A   A
  </option>
  <option id="b" selected>  B   B
  </option>
</select></body></html>`);
  show('TWO-SELECTED single', await page.evaluate(() => {
    const s = document.getElementById('s');
    const o = s.options[s.selectedIndex];
    return {
      selectedIndex: s.selectedIndex,
      selectValue: s.value,
      optionValue: o.value,
      optionText: o.text,
      optionLabel: o.label,
      attrFlags: [...s.querySelectorAll('option')].map((n) => n.hasAttribute('selected')),
    };
  }));

  await browser.close();
})();
