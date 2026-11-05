// Oracle for GOOSIE-OPTIONAX-002 and GOOSIE-OPTIONAX-003: which role does a
// <select> get, and what does Chromium expose on an option node?
//
// Goosie calls every select a combobox and fills an option node's value with the
// form value. The previous slice measured the *spelling* of the strings and left
// both claims as disclosed divergences (docs/qa-open-defects.json
// GOOSIE-OPTIONAX-002, GOOSIE-OPTIONAX-003). This script measures them: it dumps
// the whole AX property list Chromium reports - role, name, value and every
// `properties` entry - over the widget forms that decide a role (size absent,
// size="1", size="0", size="2", multiple, multiple+size="1", multiple+size="0",
// multiple+size="2", an unusable size, an open select with two selected options,
// an open select inside an optgroup, a disabled control, a disabled option, and
// ARIA-authored roles) and then holds the rules it read up against the same dump,
// shape by shape.
//
// Every expectation below is copied out of Chromium's dump on this host, not
// written from a specification, and the property sets are compared IN FULL, so a
// Chromium answer that adds or drops a key reddens this file rather than passing a
// subset test. Four passes of this script falsified four of its own guesses:
// `select.size` reads 0 when the attribute is absent, not the 1 and 4 the used
// value renders at, so the role rule cannot be written against the IDL getter;
// `<select multiple size="1">` is a combobox, so `multiple` alone does not decide
// the role either, and the answer is the used size (1 closed, 2+ open) - which
// makes `size="0"`, `size="less"` and `size="-1"` on a multiple control listboxes
// and on a single control comboboxes; a disabled open control loses `focusable`
// but keeps its activedescendant; and a closed combobox hangs its options off a
// `MenuListPopup` node while a listbox hangs them off an ignored wrapper, so an
// engine that emits the same child list for both shapes diverges either way.
//
// Run: node scripts/verify-optionax-roles.js
//      (needs playwright chromium: `npm i -D playwright && npx playwright install chromium`)
const { chromium } = require('playwright');

const show = (title, value) => console.log(`${title} -> ${JSON.stringify(value)}`);

// The fixtures, each with the role, the FULL property set, and the option nodes'
// names and property sets Chromium answers with. `value: undefined` means the
// node reports no value at all - the reference distinguishes that from "".
// `ad` is whether the control reports an activedescendant (its value names a
// node id that changes per run, so it is checked for presence only) and `popup`
// whether a MenuListPopup node stands between the control and its options.
const CASES = [
  {
    id: 'closed',
    html: '<select id="w"><option value="v1">Alpha</option><option value="v2" selected>Second choice</option></select>',
    size: 0, multiple: false, form: 'v2',
    role: 'combobox', name: '', value: 'Second choice', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [
      { name: 'Alpha', props: { focusable: true, selected: false } },
      { name: 'Second choice', props: { focusable: true, selected: true } },
    ],
  },
  {
    id: 'size1',
    html: '<select id="w" size="1"><option value="s1" selected>One</option><option value="s2">Two</option></select>',
    size: 1, multiple: false, form: 's1',
    role: 'combobox', name: '', value: 'One', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [
      { name: 'One', props: { focusable: true, selected: true } },
      { name: 'Two', props: { focusable: true, selected: false } },
    ],
  },
  {
    // size="0" is not a zero-height list: the widget is still the closed box.
    id: 'size0',
    html: '<select id="w" size="0"><option value="z0" selected>Zero</option></select>',
    size: 0, multiple: false, form: 'z0',
    role: 'combobox', name: '', value: 'Zero', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [{ name: 'Zero', props: { focusable: true, selected: true } }],
  },
  {
    id: 'size2',
    html: '<select id="w" size="2"><option value="v4" selected>Eve Lyn</option><option value="v5">Five</option></select>',
    size: 2, multiple: false, form: 'v4',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    options: [
      { name: 'Eve Lyn', props: { focusable: true, selected: true } },
      { name: 'Five', props: { focusable: true, selected: false } },
    ],
  },
  {
    // multiple with no size attribute: IDL size reads 0, yet the control is the
    // open list, because the used value is 4.
    id: 'multi',
    html: '<select id="w" multiple><option value="v6" selected>Ex Aequo</option><option value="v7">Seven</option></select>',
    size: 0, multiple: true, form: 'v6',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    options: [
      { name: 'Ex Aequo', props: { focusable: true, selected: true } },
      { name: 'Seven', props: { focusable: true, selected: false } },
    ],
  },
  {
    // The case an engine that reads `multiple` alone gets wrong: a multiple
    // control whose used size is 1 is answered as a combobox, with the closed
    // box's full property set, and answers with the selected option's display
    // string as its value.
    id: 'multi-size1',
    html: '<select id="w" multiple size="1"><option value="m1" selected>Mm One</option><option value="m2">Mm Two</option></select>',
    size: 1, multiple: true, form: 'm1',
    role: 'combobox', name: '', value: 'Mm One', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [
      { name: 'Mm One', props: { focusable: true, selected: true } },
      { name: 'Mm Two', props: { focusable: true, selected: false } },
    ],
  },
  {
    // An unusable size on a multiple control is not the closed box: the used
    // value falls back to 4, and the answer is the open list with no value. The
    // used size decides the role - not `multiple` alone (multi-size1 above) and
    // not the IDL getter, which reads 0 here just as it does with no attribute.
    id: 'multi-size0',
    html: '<select id="w" multiple size="0"><option value="n1" selected>Nn One</option><option value="n2">Nn Two</option></select>',
    size: 0, multiple: true, form: 'n1',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    options: [
      { name: 'Nn One', props: { focusable: true, selected: true } },
      { name: 'Nn Two', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'multi-size2',
    html: '<select id="w" multiple size="2"><option value="p1" selected>Pp One</option><option value="p2" selected>Pp Two</option></select>',
    size: 2, multiple: true, form: 'p1',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    options: [
      { name: 'Pp One', props: { focusable: true, selected: true } },
      { name: 'Pp Two', props: { focusable: true, selected: true } },
    ],
  },
  {
    // The two unusable spellings of a multiple control's size, so the rule is
    // measured against the used value and not against a parsed integer.
    id: 'multi-size-bogus',
    html: '<select id="w" multiple size="less"><option value="r1" selected>Rr One</option></select>',
    size: 0, multiple: true, form: 'r1',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    options: [{ name: 'Rr One', props: { focusable: true, selected: true } }],
  },
  {
    id: 'multi-size-minus1',
    html: '<select id="w" multiple size="-1"><option value="t1" selected>Tt One</option></select>',
    size: 0, multiple: true, form: 't1',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    options: [{ name: 'Tt One', props: { focusable: true, selected: true } }],
  },
  {
    // An unusable size attribute is not a zero-height list either.
    id: 'size-bogus',
    html: '<select id="w" size="less"><option value="b1" selected>Bogus</option></select>',
    size: 0, multiple: false, form: 'b1',
    role: 'combobox', name: '', value: 'Bogus', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [{ name: 'Bogus', props: { focusable: true, selected: true } }],
  },
  {
    id: 'size-minus1',
    html: '<select id="w" size="-1"><option value="q1" selected>Minus</option></select>',
    size: 0, multiple: false, form: 'q1',
    role: 'combobox', name: '', value: 'Minus', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [{ name: 'Minus', props: { focusable: true, selected: true } }],
  },
  {
    // A multiple control keeps more than one option selected in the tree, so a
    // single-selection model cannot fill the selected flags.
    id: 'multi-two',
    html: '<select id="w" multiple><option value="a" selected>Aye</option><option value="b" selected>Bee</option><option value="c">Cee</option></select>',
    size: 0, multiple: true, form: 'a',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: true } },
      { name: 'Cee', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'group-open',
    html: '<select id="w" size="3"><optgroup label="Group Label"><option value="g1">In one</option><option value="g2" selected>In two</option></optgroup><option value="g3">Tail</option></select>',
    size: 3, multiple: false, form: 'g2',
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    group: 'Group Label',
    options: [
      { name: 'In one', props: { focusable: true, selected: false } },
      { name: 'In two', props: { focusable: true, selected: true } },
      { name: 'Tail', props: { focusable: true, selected: false } },
    ],
  },
  {
    // The closed box in its disabled state, so the disabled property set is
    // measured for both shapes rather than inferred from the open one. What the
    // run showed is that the control's disabled state reaches the option as
    // `disabled: true` WITHOUT taking away its `focusable`, while an option that
    // is disabled itself inside an enabled control loses `focusable` entirely
    // (option-disabled below).
    id: 'closed-disabled',
    html: '<select id="w" disabled><option value="c1" selected>Closed Dee</option></select>',
    size: 0, multiple: false, form: 'c1', disabled: true,
    role: 'combobox', name: '', value: 'Closed Dee', ad: false, popup: true,
    props: { disabled: true, expanded: false, hasPopup: 'menu', invalid: 'false' },
    options: [{ name: 'Closed Dee', props: { disabled: true, focusable: true } }],
  },
  {
    // A disabled open control keeps its role, its activedescendant and every
    // other property, but loses `focusable` - and its option, which is not
    // disabled itself, inherits `disabled: true` and keeps `focusable: true`.
    id: 'size2-disabled',
    html: '<select id="w" size="2" disabled><option value="d1" selected>Dee</option></select>',
    size: 2, multiple: false, form: 'd1', disabled: true,
    role: 'listbox', name: '', value: undefined, ad: true, popup: false,
    props: { disabled: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    options: [{ name: 'Dee', props: { disabled: true, focusable: true } }],
  },
  {
    // A disabled option reports no `selected` flag at all, even when it is the
    // selected one, and it reports nothing but `disabled` while its control is
    // enabled - the control's own disabled state, measured above, is what adds
    // `focusable` back to an option.
    id: 'option-disabled',
    html: '<select id="w"><option value="e1" disabled selected>Off</option><option value="e2">On</option></select>',
    size: 0, multiple: false, form: 'e1',
    role: 'combobox', name: '', value: 'Off', ad: false, popup: true,
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    options: [
      { name: 'Off', props: { disabled: true } },
      { name: 'On', props: { focusable: true, selected: false } },
    ],
  },
  {
    // The authored roles answer the same way, which is how the engine's ARIA
    // token map is checked against the reference rather than guessed: an
    // authored list reports no activedescendant of its own.
    id: 'aria-listbox',
    html: '<div id="w" role="listbox" aria-label="Authored listbox"><div role="option" aria-label="Authored option">x</div></div>',
    role: 'listbox', name: 'Authored listbox', value: undefined, ad: false, popup: false,
    props: { multiselectable: false, orientation: 'vertical', required: false },
    options: [{ name: 'Authored option', props: { selected: false } }],
  },
];

const propsOf = (n) => {
  const out = {};
  for (const p of n.properties || []) {
    out[p.name] = p.value && (p.value.value !== undefined ? p.value.value : p.value);
  }
  return out;
};

// Object.entries().sort() so a key order that is Chromium's serialisation detail
// cannot turn a correct answer into a failure.
const sorted = (o) => Object.fromEntries(Object.entries(o).sort(([a], [b]) => (a < b ? -1 : 1)));

// Accessibility.getFullAXTree answers a FLAT list whose order is not document
// order (measured: the optgroup fixture's tail option came first), so ordering
// claims have to be read off the childId graph. That graph does not hang options
// straight off the control in either shape: a listbox parents them under an
// ignored `none` wrapper (measured: listbox -> none -> [group, option "Tail"]),
// while a closed combobox parents them under a `MenuListPopup` node that is NOT
// ignored. kidsOf therefore keeps every non-ignored child and walks through an
// ignored one, splicing its children into its place, so the order the tree
// states is the order in the document. axOf returns the nested control, its
// option and group descendants, and whether a MenuListPopup node stands between
// the control and its options.
const axOf = async (page) => {
  const client = await page.context().newCDPSession(page);
  await client.send('Accessibility.enable');
  const { nodes } = await client.send('Accessibility.getFullAXTree');
  await client.detach();
  const byId = new Map(nodes.map((n) => [n.nodeId, n]));
  const kidsOf = (n) => {
    const out = [];
    for (const id of n.childIds || []) {
      const k = byId.get(id);
      if (!k) continue;
      if (k.ignored) out.push(...kidsOf(k));
      else out.push(k);
    }
    return out;
  };
  const shape = (n) => {
    const props = propsOf(n);
    const ad = 'activedescendant' in props;
    delete props.activedescendant;
    const out = {
      role: n.role ? n.role.value : undefined,
      name: n.name ? n.name.value : undefined,
      value: n.value ? n.value.value : undefined,
      props: sorted(props),
      ad,
    };
    const kids = kidsOf(n);
    if (kids.length) out.children = kids.map(shape);
    return out;
  };
  const root = nodes.find((n) => n.role && ['combobox', 'listbox'].includes(n.role.value));
  if (!root) return { ctl: undefined, options: [], groups: [], popup: false };
  const ctl = shape(root);
  const flat = [];
  const walk = (n) => { for (const k of n.children || []) { flat.push(k); walk(k); } };
  walk(ctl);
  return {
    ctl,
    // Whether the options descend from a `MenuListPopup` node: measured true for
    // every closed combobox and false for every listbox, whose options sit under
    // an ignored wrapper instead.
    popup: flat.some((n) => n.role === 'MenuListPopup'),
    options: flat.filter((n) => n.role === 'option'),
    groups: flat.filter((n) => n.role === 'group'),
  };
};

const checks = [];
const check = (label, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  checks.push({ label, ok, got, want });
  console.log(`${ok ? '  ok  ' : ' FAIL '} ${label}: ${JSON.stringify(got)}${ok ? '' : ' != ' + JSON.stringify(want)}`);
};

(async () => {
  const browser = await chromium.launch();
  for (const c of CASES) {
    const page = await browser.newPage();
    await page.setContent(`<!doctype html><html><body>${c.html}</body></html>`);
    await page.waitForTimeout(30);
    const { ctl, options: opts, groups, popup } = await axOf(page);
    show(`AX ${c.id}`, { ctl, options: opts, groups, popup });

    if (c.size !== undefined) {
      const dom = await page.evaluate((id) => {
        const el = document.getElementById(id);
        return { size: el.size, multiple: el.multiple, form: el.value, disabled: el.disabled };
      }, 'w');
      check(`${c.id}: IDL size/multiple as written`, { size: dom.size, multiple: dom.multiple }, { size: c.size, multiple: c.multiple });
      check(`${c.id}: select.value stays the form value`, dom.form, c.form);
      if (c.disabled !== undefined) check(`${c.id}: disabled flag`, dom.disabled, c.disabled);
    }

    check(`${c.id}: role`, ctl && ctl.role, c.role);
    check(`${c.id}: name`, ctl && ctl.name, c.name);
    check(`${c.id}: value`, ctl && ctl.value, c.value);
    check(`${c.id}: properties`, (ctl || {}).props, sorted(c.props));
    check(`${c.id}: activedescendant`, (ctl || {}).ad, c.ad);
    check(`${c.id}: options descend from a MenuListPopup node`, popup, c.popup);
    // GOOSIE-OPTIONAX-003: no option node in the reference carries a value, not
    // even one whose value attribute differs from its text.
    check(`${c.id}: option nodes carry no value`, opts.map((o) => o.value), opts.map(() => undefined));
    check(`${c.id}: option names and properties`,
      opts.map((o) => ({ name: o.name, props: o.props })),
      c.options.map((o) => ({ name: o.name, props: sorted(o.props) })));
    if (c.group !== undefined) {
      const group = groups[0];
      check(`${c.id}: optgroup names a group`, group && group.name, c.group);
      check(`${c.id}: optgroup holds its options in document order`,
        (group || {}).children && group.children.map((n) => n.name),
        c.options.slice(0, 2).map((o) => o.name));
    }
    await page.close();
  }

  await browser.close();
  const failed = checks.filter((x) => !x.ok);
  failed.forEach((x) => console.log(`\nFAILED ${x.label}: got ${JSON.stringify(x.got)} want ${JSON.stringify(x.want)}`));
  console.log(`\n${checks.length} checks, ${failed.length} failed`);
  console.log(failed.length ? '\nOPTIONAX ROLE ORACLE FAILED' : '\nOPTIONAX ROLE ORACLE OK');
  process.exit(failed.length ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(1); });
