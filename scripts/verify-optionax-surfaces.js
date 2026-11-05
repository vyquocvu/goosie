// Oracle for GOOSIE-OPTIONAX-001: which whitespace survives on which surface?
//
// Goosie answers one normalised string for an option everywhere - painted
// control, listbox row, option's accessible name, combobox's value - because
// one function owns all of them. Chromium does not: the same label attribute
// spells "  My   Label  " as " My Label " in the option's accessible name and
// as "My   Label" in the combobox's value, and a whitespace-only label leaves
// the combobox with no value at all. The BR-101 slice recorded that as a
// divergence instead of guessing a rule, so the rule has to be measured here,
// shape by shape, before any of it can be encoded.
//
// The anchor holds the candidate rules in JS and compares them against
// Chromium's own answer for 11 whitespace shapes, then measures the combobox
// NAME model (nothing from the selected option, aria-label and <label for> do)
// on the same seam. CDP Accessibility.getFullAXTree is the only headless seam
// that sees the announced name; the IDL getters come from the page.
//
// Run: node scripts/verify-optionax-surfaces.js
//      (needs playwright chromium: `npm i -D playwright && npx playwright install chromium`)
const { chromium } = require('playwright');

// The set Chromium collapses in an option string - HTML's ASCII whitespace,
// measured to exclude U+000B (a vertical tab survives inside a name) and U+00A0.
const WS = ' \\t\\n\\f\\r';
// The value edge set is one wider: a vertical tab is trimmed off the display
// string (measured on label="My" with U+000B on both edges, value "My") while
// the name keeps it, and U+00A0 is in neither.
const WSV = `${WS}\\u000b`;
const LEAD = new RegExp(`^[${WS}]+`);
const TRAIL = new RegExp(`[${WS}]+$`);
const RUN = new RegExp(`[${WS}]+`, 'g');
const LEADV = new RegExp(`^[${WSV}]+`);
const TRAILV = new RegExp(`[${WSV}]+$`);

// axName is the accessible-name spelling: every run becomes one space, and a
// run touching an edge contributes that one space too - which is why a
// whitespace-only label announces two spaces, not one and not zero.
const axName = (s) => {
  const lead = LEAD.test(s) ? ' ' : '';
  const trail = TRAIL.test(s) ? ' ' : '';
  const core = s.replace(LEAD, '').replace(TRAIL, '').replace(RUN, ' ');
  return lead + core + trail;
};

// comboValue is the combobox-value spelling: the edges go, the interior stays
// verbatim - "A  B" keeps two spaces here and one space in the name. A label
// attribute is used as written; text content has already been normalised by the
// option's own text rule, so the value path trims that result.
const comboValue = (s) => s.replace(LEADV, '').replace(TRAILV, '');
const comboValueFromText = (s) => comboValue(normalise(s));

// normalise is what an option's TEXT CONTENT costs on both surfaces: strip the
// edges and collapse the interior, the rule the BR-101 slice already shipped.
const normalise = (s) => comboValuePlain(s).replace(RUN, ' ');
const comboValuePlain = (s) => s.replace(LEAD, '').replace(TRAIL, '');

const SHAPES = [
  { name: 'edges+inner', ws: '  My   Label  ' },
  { name: 'three-spaces', ws: '   ' },
  { name: 'two-spaces', ws: '  ' },
  { name: 'one-space', ws: ' ' },
  { name: 'leading-only', ws: ' A' },
  { name: 'trailing-only', ws: 'A ' },
  { name: 'inner-only', ws: 'A  B' },
  { name: 'newlines', ws: '\n My \n' },
  { name: 'tabs', ws: '\tMy\tLabel\t' },
  { name: 'nbsp-edges', ws: '\u00a0My\u00a0' },
  { name: 'vt-edges', ws: '\u000bMy\u000b' },
  { name: 'vt-inner', ws: 'My\u000b\u000bLabel' },
  { name: 'vt-mixed', ws: ' \u000bA\u000b B ' },
  { name: 'cr-inner', ws: 'A\r\rB' },
];

// Newline, carriage return, vertical tab and form feed are written as
// character references: the HTML parser normalises a raw U+000D in the input
// stream to U+000A before attributes are read, so a reference is the only way
// to hand Chromium the byte the fixture claims.
const esc = (s) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
   .replace(/\n/g, '&#10;').replace(/\r/g, '&#13;').replace(/\t/g, '&#9;').replace(/\f/g, '&#12;')
   .replace(/\u000b/g, '&#11;');

const show = (title, value) => console.log(`${title} -> ${JSON.stringify(value)}`);

const axOf = async (page, roles) => {
  const client = await page.context().newCDPSession(page);
  await client.send('Accessibility.enable');
  const { nodes } = await client.send('Accessibility.getFullAXTree');
  await client.detach();
  return nodes
    .filter((n) => n.role && roles.includes(n.role.value))
    .map((n) => {
      const out = { role: n.role.value, name: n.name ? n.name.value : undefined };
      if (n.value) out.value = n.value.value;
      return out;
    });
};

const checks = [];
const check = (label, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  checks.push({ label, ok, got, want });
  console.log(`${ok ? '  ok  ' : ' FAIL '} ${label}: ${JSON.stringify(got)}${ok ? '' : ' != ' + JSON.stringify(want)}`);
};

(async () => {
  const browser = await chromium.launch();

  // 1. The two spellings, for both name sources.
  for (const shape of SHAPES) {
    const page = await browser.newPage();
    await page.setContent('<!doctype html><html><body>' +
      `<select id="a"><option selected label="${esc(shape.ws)}">WWWWW</option></select>` +
      `<select id="b"><option selected>${esc(shape.ws)}</option></select>` +
      '</body></html>');
    await page.waitForTimeout(30);
    const ax = await axOf(page, ['combobox', 'option']);
    const idl = await page.evaluate(() => ({
      rawLabel: document.querySelector('#a option').getAttribute('label'),
      rawText: document.querySelector('#b option').firstChild && document.querySelector('#b option').firstChild.data,
    }));
    // Guard the fixture itself: if the parser rewrote a byte, the shape measures
    // something other than what the table says.
    check(`${shape.name}: label attribute bytes reach Chromium`, idl.rawLabel, shape.ws);
    check(`${shape.name}: text content bytes reach Chromium`, idl.rawText, shape.ws);
    const comboLabel = ax.filter((n) => n.role === 'combobox')[0];
    const optionLabel = ax.filter((n) => n.role === 'option')[0];
    const comboText = ax.filter((n) => n.role === 'combobox')[1];
    const optionText = ax.filter((n) => n.role === 'option')[1];
    show(`AX ${shape.name}`, ax);
    // A label attribute is the absent one when it is the empty string, so the
    // name falls back to the text content; a whitespace-only label is NOT
    // absent - it names the option with nothing but spaces. A control whose
    // display string is empty reports no value at all (undefined), which the
    // Chromium AX tree distinguishes from an empty string.
    const absent = (s) => (s === '' ? undefined : s);
    const fromLabel = shape.ws !== '' ? axName(shape.ws) : 'WWWWW';
    check(`${shape.name}: option name from label attribute`, optionLabel && optionLabel.name, fromLabel);
    check(`${shape.name}: combobox value from label attribute`, comboLabel && comboLabel.value,
      absent(shape.ws !== '' ? comboValue(shape.ws) : 'WWWWW'));
    check(`${shape.name}: option name from text content`, optionText && optionText.name, normalise(shape.ws));
    check(`${shape.name}: combobox value from text content`, comboText && comboText.value, absent(comboValueFromText(shape.ws)));
    await page.close();
  }

  // 2. The combobox NAME model: whose name is the closed control's? One
  // control per page, because the AX tree's node order follows layout rather
  // than the document, so an index into it is not a fixture.
  const CONTROLS = [
    { id: 'bare', html: '<select id="bare"><option value="v1" selected>Alpha Beta</option></select>', role: 'combobox', name: '', value: 'Alpha Beta', form: 'v1' },
    { id: 'labeled', html: '<label for="labeled">Author label</label><select id="labeled"><option value="v2" selected>Gam Ma</option></select>', role: 'combobox', name: 'Author label', value: 'Gam Ma', form: 'v2' },
    { id: 'aria', html: '<select id="aria" aria-label="Aria name"><option value="v3" selected>Delta</option></select>', role: 'combobox', name: 'Aria name', value: 'Delta', form: 'v3' },
    { id: 'size2', html: '<select id="size2" size="2"><option value="v4" selected>Eve Lyn</option><option value="v5">Five</option></select>', role: 'listbox', name: '', value: undefined, form: 'v4' },
    { id: 'multi', html: '<select id="multi" multiple><option value="v6" selected>Ex Aequo</option><option value="v7">Seven</option></select>', role: 'listbox', name: '', value: undefined, form: 'v6' },
    { id: 'spaced', html: '<select id="spaced"><option selected label="  My   Label  " value="sub">WWWWW</option></select>', role: 'combobox', name: '', value: 'My   Label', form: 'sub' },
    { id: 'blank', html: '<select id="blank"><option selected label="   " value="sub">WWWWW</option></select>', role: 'combobox', name: '', value: undefined, form: 'sub' },
    { id: 'nobody', html: '<select id="nobody"></select>', role: 'combobox', name: '', value: undefined, form: '' },
  ];
  for (const control of CONTROLS) {
    const page = await browser.newPage();
    await page.setContent(`<!doctype html><html><body>${control.html}</body></html>`);
    await page.waitForTimeout(30);
    const ax = await axOf(page, ['combobox', 'listbox', 'option']);
    show(`AX ${control.id}`, ax);
    const form = await page.evaluate((id) => document.getElementById(id).value, control.id);
    show(`IDL ${control.id}`, { selectValue: form });
    const node = ax.filter((n) => n.role === 'combobox' || n.role === 'listbox')[0];
    check(`${control.id}: role`, node && node.role, control.role);
    check(`${control.id}: name`, node && node.name, control.name);
    check(`${control.id}: value`, node && node.value, control.value);
    // The form answer is a different surface from the announced one, and the
    // anchor records both so the engine cannot quietly merge them.
    check(`${control.id}: select.value stays the form value`, form, control.form);
    check(`${control.id}: options stay children`, ax.filter((n) => n.role === 'option').length,
      (control.html.match(/<option/g) || []).length);
    await page.close();
  }

  await browser.close();

  const failed = checks.filter((c) => !c.ok);
  failed.forEach((c) => console.log(`\nFAILED ${c.label}: got ${JSON.stringify(c.got)} want ${JSON.stringify(c.want)}`));
  console.log(`\n${checks.length} checks, ${failed.length} failed`);
  console.log(failed.length ? '\nOPTIONAX ORACLE FAILED' : '\nOPTIONAX ORACLE OK');
  process.exit(failed.length ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(1); });
