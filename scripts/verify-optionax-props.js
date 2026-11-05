// Oracle for GOOSIE-OPTIONAX-004: what does Chromium expose in the `properties`
// list of a select, its popup container, and its option nodes - and which of
// those keys disappear when the control or the option is disabled?
//
// The previous slice (scripts/verify-optionax-roles.js) pinned the role, the
// name, the value and the presence of a MenuListPopup node. It left the property
// VALUES, the aria-driven overrides, the activedescendant TARGET and the
// disabled-state pruning disclosed rather than measured, because ax.Node had no
// channel to carry them (docs/qa-open-defects.json GOOSIE-OPTIONAX-004). This
// script measures that channel: every case compares the FULL property set, so a
// Chromium answer that adds or drops a key reddens this file instead of passing a
// subset test.
//
// Everything below is copied out of Chromium's dump on this host (153.0.8010.12),
// not out of a specification: 46 fixtures, 324 checks, all green on the run that
// wrote them. Passes of this script falsified these guesses:
//   * `required` was expected on every select carrying `required` or
//     `aria-required`; the closed combobox reports NO required key at all
//     (closed-required-empty, closed-aria-required), so the key belongs to the
//     open list shape only.
//   * `aria-required` was expected to override the content attribute both ways;
//     it only turns the key ON - `required aria-required="false"` still answers
//     required:true (open-aria-required-false-over-attr), so the rule is a
//     disjunction of the two attributes, not an override.
//   * `hasPopup` was expected to repeat whatever aria-haspopup says; only ARIA's
//     popup-kind tokens replace the native "menu" (dialog, listbox, tree, grid),
//     while "true", "false" and an unusable token all leave it at "menu".
//   * `invalid` was expected to be a boolean; it is the STRING "true"/"false", and
//     `aria-invalid="grammar"` and `"spelling"` drop the key entirely rather than
//     answering false.
//   * an empty `aria-invalid` was expected to read as "false"; it reads as ABSENT
//     instead - the same fixture answers "true" when the constraint fails
//     (closed-aria-invalid-empty) and "false" when it holds
//     (closed-aria-invalid-empty-valid), which is why both fixtures are here.
//   * the activedescendant was expected on the control in both shapes; a closed
//     combobox puts it on the MenuListPopup child, and only while that control is
//     focused - the unfocused popup reports no properties at all.
//   * a disabled option was expected to keep `selected`; it keeps nothing but
//     `disabled`, while an enabled option inside a disabled control keeps BOTH
//     `disabled` and `focusable` and loses only `selected`.
//   * an authored role="option" was expected to report no properties; it reports
//     {selected:false}, which no native option in a disabled control does.
//
// Run: node scripts/verify-optionax-props.js
//      (needs playwright chromium: `npm i -D playwright && npx playwright install chromium`)
const { chromium } = require('playwright');

const show = (title, value) => console.log(`${title} -> ${JSON.stringify(value)}`);

// Each fixture states the FULL property set Chromium answers with for the
// control, for the MenuListPopup node that only a closed combobox has, and for
// every option. `ad` says where the activedescendant lives ('ctl' or 'popup')
// and which option it names, or null when no node reports one. `popup: null`
// means the control has no MenuListPopup child at all.
const CASES = [
  {
    id: 'closed-enabled',
    html: '<select id="w"><option value="a">Aye</option><option value="b" selected>Bee</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Aye', props: { focusable: true, selected: false } },
      { name: 'Bee', props: { focusable: true, selected: true } },
    ],
  },
  {
    // A disabled closed box loses `focusable` and keeps the rest, including
    // hasPopup and expanded.
    id: 'closed-disabled',
    html: '<select id="w" disabled><option value="a">Aye</option><option value="b" selected>Bee</option></select>',
    role: 'combobox',
    props: { disabled: true, expanded: false, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Aye', props: { disabled: true, focusable: true } },
      { name: 'Bee', props: { disabled: true, focusable: true } },
    ],
  },
  {
    // A disabled fieldset disables the control the same way the attribute does,
    // and the property answer is byte-identical.
    id: 'fieldset-disabled',
    html: '<fieldset disabled><select id="w"><option value="a">Aye</option><option value="b" selected>Bee</option></select></fieldset>',
    role: 'combobox',
    props: { disabled: true, expanded: false, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Aye', props: { disabled: true, focusable: true } },
      { name: 'Bee', props: { disabled: true, focusable: true } },
    ],
  },
  {
    // `required` never reaches a closed combobox, but the missing selection still
    // makes it invalid: the key set answers the constraint, not the attribute.
    id: 'closed-required-empty',
    html: '<select id="w" required><option value="">Pick</option><option value="a">Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'true' },
    ad: null,
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Pick', props: { focusable: true, selected: true } },
      { name: 'Aye', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'closed-aria-required',
    html: '<select id="w" aria-required="true"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // `multiple` with a used size of 1 is a combobox, and a combobox reports
    // neither multiselectable nor orientation.
    id: 'closed-multiple-size1',
    html: '<select id="w" multiple size="1"><option value="a" selected>Aye</option><option value="b">Bee</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'closed-haspopup-dialog',
    html: '<select id="w" aria-haspopup="dialog"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'dialog', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // ARIA's shorthand for "some kind of popup", and an unusable token: both are
    // read against the "menu" a native select answers by default.
    id: 'closed-haspopup-true',
    html: '<select id="w" aria-haspopup="true"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-haspopup-bogus',
    html: '<select id="w" aria-haspopup="bogus"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // The rest of ARIA's popup-kind tokens, plus "false", measured on a native
    // select so the set that replaces the default "menu" is complete rather than
    // inferred from dialog alone.
    id: 'closed-haspopup-listbox',
    html: '<select id="w" aria-haspopup="listbox"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'listbox', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-haspopup-tree',
    html: '<select id="w" aria-haspopup="tree"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'tree', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-haspopup-grid',
    html: '<select id="w" aria-haspopup="grid"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'grid', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-haspopup-false',
    html: '<select id="w" aria-haspopup="false"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // The native popup never opens here, so an author-set aria-expanded cannot
    // move the reported state.
    id: 'closed-aria-expanded-true',
    html: '<select id="w" aria-expanded="true"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-orientation-horizontal',
    html: '<select id="w" aria-orientation="horizontal"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false', orientation: 'horizontal' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // A closed box with no aria-orientation reports NO orientation key at all, so
    // the authored token has to be the only thing that can add one.
    id: 'closed-orientation-vertical',
    html: '<select id="w" aria-orientation="vertical"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false', orientation: 'vertical' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // The listbox branch of this rule was measured (an unusable token leaves it at
    // "vertical"); this fixture measures the closed branch, where ignoring the
    // token leaves no key at all because a closed box has no default orientation.
    id: 'closed-orientation-bogus',
    html: '<select id="w" aria-orientation="bogus"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-custom-validity',
    html: '<select id="w"><option value="a" selected>Aye</option></select>',
    eval: 'document.getElementById("w").setCustomValidity("nope")',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'true' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // An empty select still has a popup container: the layer is part of the
    // closed widget, not of its contents.
    id: 'closed-no-options',
    html: '<select id="w"></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: [] },
    options: [],
  },
  {
    id: 'closed-optgroup',
    html: '<select id="w"><optgroup label="G"><option value="a" selected>Aye</option><option value="b">Bee</option></optgroup></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['group'] },
    groups: [{ name: 'G', props: {}, children: ['option', 'option'] }],
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // Focus adds `focused` to the control and an activedescendant to the popup,
    // naming the SELECTED option - not the first one.
    id: 'focused-closed-selected',
    html: '<select id="w"><option value="a">Aye</option><option value="b" selected>Bee</option></select>',
    focus: true,
    role: 'combobox',
    props: { expanded: false, focusable: true, focused: true, hasPopup: 'menu', invalid: 'false' },
    ad: { on: 'popup', name: 'Bee' },
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Aye', props: { focusable: true, selected: false } },
      { name: 'Bee', props: { focusable: true, selected: true } },
    ],
  },
  {
    // Nothing authored as selected, so the closed box's default selection - the
    // first enabled option - is what the popup names.
    id: 'focused-closed-default',
    html: '<select id="w"><option value="a">Aye</option><option value="b">Bee</option></select>',
    focus: true,
    role: 'combobox',
    props: { expanded: false, focusable: true, focused: true, hasPopup: 'menu', invalid: 'false' },
    ad: { on: 'popup', name: 'Aye' },
    popup: { name: '', props: {}, children: ['option', 'option'] },
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // Nothing is selected at all - the only option is disabled, so even the
    // closed box's default selection cannot land - and the focused popup then
    // reports no activedescendant.
    id: 'focused-closed-all-disabled',
    html: '<select id="w"><option value="a" disabled>Aye</option></select>',
    focus: true,
    role: 'combobox',
    props: { expanded: false, focusable: true, focused: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { disabled: true } }],
  },
  {
    id: 'open-single',
    html: '<select id="w" size="2"><option value="a" selected>Aye</option><option value="b">Bee</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // Two authored selections: the activedescendant names the FIRST one.
    id: 'open-multiple-two',
    html: '<select id="w" multiple><option value="a" selected>Aye</option><option value="b" selected>Bee</option><option value="c">Cee</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: true, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: true } },
      { name: 'Cee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // An open single select with nothing selected keeps selectedIndex at -1, so
    // there is no activedescendant to report.
    id: 'open-nothing-selected',
    html: '<select id="w" size="2"><option value="a">Aye</option><option value="b">Bee</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: null,
    popup: null,
    options: [
      { name: 'Aye', props: { focusable: true, selected: false } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'open-required',
    html: '<select id="w" size="2" required><option value="">Pick</option><option value="a">Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'true', multiselectable: false, orientation: 'vertical', required: true },
    ad: null,
    popup: null,
    options: [
      { name: 'Pick', props: { focusable: true, selected: false } },
      { name: 'Aye', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'open-required-selected',
    html: '<select id="w" size="2" required><option value="">Pick</option><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: true },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Pick', props: { focusable: true, selected: false } },
      { name: 'Aye', props: { focusable: true, selected: true } },
    ],
  },
  {
    // aria-required reaches the open list as `required`, the same key the content
    // attribute uses.
    id: 'open-aria-required',
    html: '<select id="w" size="2" aria-required="true"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: true },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
    ],
  },
  {
    // Which of the two wins when they disagree: the content attribute asks for
    // required, the ARIA attribute asks for the opposite. Measured the attribute
    // wins - aria-required="true" can turn the key on (open-aria-required) but
    // "false" cannot turn it off, so the answer is a disjunction, not an override.
    id: 'open-aria-required-false-over-attr',
    html: '<select id="w" size="2" required aria-required="false"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: true },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // A disabled open list loses `focusable` but KEEPS its activedescendant.
    id: 'open-disabled',
    html: '<select id="w" size="2" disabled><option value="a" selected>Aye</option><option value="b">Bee</option></select>',
    role: 'listbox',
    props: { disabled: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Aye', props: { disabled: true, focusable: true } },
      { name: 'Bee', props: { disabled: true, focusable: true } },
    ],
  },
  {
    id: 'open-multiple-required-empty',
    html: '<select id="w" multiple required><option value="a">Aye</option><option value="b">Bee</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'true', multiselectable: true, orientation: 'vertical', required: true },
    ad: null,
    popup: null,
    options: [
      { name: 'Aye', props: { focusable: true, selected: false } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    id: 'open-orientation-horizontal',
    html: '<select id="w" size="2" aria-orientation="horizontal"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'horizontal', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // An unusable token leaves the listbox at its own default.
    id: 'open-orientation-bogus',
    html: '<select id="w" size="2" aria-orientation="bogus"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // aria-invalid overrides the constraint: this select is perfectly valid.
    id: 'open-aria-invalid-true',
    html: '<select id="w" size="2" aria-invalid="true"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'true', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // "grammar" is a recognised token that means something other than true/false,
    // so the key is dropped instead of answered.
    id: 'open-aria-invalid-grammar',
    html: '<select id="w" size="2" aria-invalid="grammar"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // "spelling" is the other token that means neither true nor false. It was an
    // inference until this fixture measured it: the key is dropped here too.
    id: 'open-aria-invalid-spelling',
    html: '<select id="w" size="2" aria-invalid="spelling"><option value="a" selected>Aye</option></select>',
    role: 'listbox',
    props: { focusable: true, multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // This select IS invalid by its constraint, and "false" still says otherwise.
    id: 'closed-aria-invalid-false',
    html: '<select id="w" required aria-invalid="false"><option value="">Pick</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Pick', props: { focusable: true, selected: true } }],
  },
  {
    id: 'closed-aria-invalid-empty',
    html: '<select id="w" required aria-invalid=""><option value="">Pick</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'true' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Pick', props: { focusable: true, selected: true } }],
  },
  {
    // The fixture above cannot tell "an empty aria-invalid is ignored, so the
    // constraint shows through" from "an empty aria-invalid means true": both
    // read "true" on an invalid select. This one is valid, so the two readings
    // finally answer differently.
    id: 'closed-aria-invalid-empty-valid',
    html: '<select id="w" aria-invalid=""><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'false' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // An unusable token is read as true, the same way an invalid attribute value
    // is elsewhere in ARIA.
    id: 'closed-aria-invalid-bogus',
    html: '<select id="w" aria-invalid="bogus"><option value="a" selected>Aye</option></select>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'menu', invalid: 'true' },
    ad: null,
    popup: { name: '', props: {}, children: ['option'] },
    options: [{ name: 'Aye', props: { focusable: true, selected: true } }],
  },
  {
    // A disabled option keeps nothing but `disabled` - not `focusable`, and not
    // `selected` even though it is the selected one - while the activedescendant
    // still names it.
    id: 'open-option-disabled',
    html: '<select id="w" size="2"><option value="a" selected disabled>Aye</option><option value="b">Bee</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Aye', props: { disabled: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // The optgroup's disabledness reaches its options; the group node itself
    // reports no properties in either shape.
    id: 'open-optgroup-disabled',
    html: '<select id="w" size="2"><optgroup label="G" disabled><option value="a" selected>Aye</option></optgroup><option value="b">Bee</option></select>',
    role: 'listbox',
    props: { focusable: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    groups: [{ name: 'G', props: {}, children: ['option'] }],
    options: [
      { name: 'Aye', props: { disabled: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // Focus on the open list lands on the control, which is where its
    // activedescendant already lives.
    id: 'focused-open',
    html: '<select id="w" size="2"><option value="a" selected>Aye</option><option value="b">Bee</option></select>',
    focus: true,
    role: 'listbox',
    props: { focusable: true, focused: true, invalid: 'false', multiselectable: false, orientation: 'vertical', required: false },
    ad: { on: 'ctl', name: 'Aye' },
    popup: null,
    options: [
      { name: 'Aye', props: { focusable: true, selected: true } },
      { name: 'Bee', props: { focusable: true, selected: false } },
    ],
  },
  {
    // Authored widgets are a different answer: no focusable, no invalid, no
    // activedescendant, and no MenuListPopup layer.
    id: 'authored-listbox',
    html: '<div id="w" role="listbox" aria-label="Authored"><div role="option" aria-label="O">x</div></div>',
    role: 'listbox',
    props: { multiselectable: false, orientation: 'vertical', required: false },
    ad: null,
    popup: null,
    options: [{ name: 'O', props: { selected: false } }],
  },
  {
    id: 'authored-combobox',
    html: '<div id="w" role="combobox" aria-haspopup="listbox" aria-expanded="false" tabindex="0">x</div>',
    role: 'combobox',
    props: { expanded: false, focusable: true, hasPopup: 'listbox', required: false },
    ad: null,
    popup: null,
    options: [],
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

// Accessibility.getFullAXTree answers a FLAT list, so ordering claims have to be
// read off the childId graph, splicing through ignored nodes - a listbox parents
// its options under an ignored `none` wrapper while a closed combobox parents
// them under a MenuListPopup node that is NOT ignored. axOf returns the control
// and popup shapes with their activedescendant resolved to the NAME of the option
// it points at: the idref itself is a node id that changes per run, so a test
// that compared it would be comparing noise.
const axOf = async (page) => {
  const client = await page.context().newCDPSession(page);
  await client.send('Accessibility.enable');
  const { nodes } = await client.send('Accessibility.getFullAXTree');
  await client.detach();
  const byId = new Map(nodes.map((n) => [n.nodeId, n]));
  const byBackend = new Map(nodes.filter((n) => n.backendDOMNodeId).map((n) => [n.backendDOMNodeId, n]));
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
  // Which option an activedescendant names, resolved through the related node's
  // backendDOMNodeId, with the related node's own name as a fallback.
  const adName = (n) => {
    const p = (n.properties || []).find((x) => x.name === 'activedescendant');
    if (!p) return undefined;
    const rel = (p.value && p.value.relatedNodes && p.value.relatedNodes[0]) || {};
    const target = byBackend.get(rel.backendDOMNodeId);
    if (target && target.name) return target.name.value;
    return rel.name;
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
      ad: ad ? adName(n) : undefined,
      ignored: !!n.ignored,
    };
    const kids = kidsOf(n);
    if (kids.length) out.children = kids.map(shape);
    return out;
  };
  const root = nodes.find((n) => n.role && ['combobox', 'listbox'].includes(n.role.value));
  if (!root) return { ctl: undefined, popup: undefined, options: [], groups: [] };
  const ctl = shape(root);
  const popupNode = kidsOf(root).find((k) => k.role && k.role.value === 'MenuListPopup');
  const popup = popupNode ? shape(popupNode) : undefined;
  const flat = [];
  const walk = (n) => { for (const k of n.children || []) { flat.push(k); walk(k); } };
  walk(ctl);
  return {
    ctl,
    popup,
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
    if (c.eval) await page.evaluate(c.eval);
    if (c.focus) await page.evaluate(() => document.getElementById('w').focus());
    await page.waitForTimeout(30);
    const { ctl, popup, options: opts, groups } = await axOf(page);
    show(`AX ${c.id}`, { ctl, popup, options: opts, groups });

    check(`${c.id}: role`, ctl && ctl.role, c.role);
    check(`${c.id}: control properties`, (ctl || {}).props, sorted(c.props));
    // Where the activedescendant lives is part of the answer: a closed combobox
    // puts it on the popup and only while focused, an open list puts it on the
    // control and keeps it even when disabled.
    check(`${c.id}: activedescendant`,
      ctl && ctl.ad !== undefined ? { on: 'ctl', name: ctl.ad }
        : popup && popup.ad !== undefined ? { on: 'popup', name: popup.ad }
          : null,
      c.ad);
    check(`${c.id}: MenuListPopup layer`,
      popup === undefined ? null : { name: popup.name, props: popup.props, children: (popup.children || []).map((k) => k.role) },
      c.popup === null ? null : { name: c.popup.name, props: sorted(c.popup.props), children: c.popup.children });
    check(`${c.id}: popup node is not ignored`, popup === undefined ? null : popup.ignored, popup === undefined ? null : false);
    // No option node in the reference carries a value (GOOSIE-OPTIONAX-003), so
    // the property slice has to keep that claim too.
    check(`${c.id}: option nodes carry no value`, opts.map((o) => o.value), opts.map(() => undefined));
    check(`${c.id}: option names and properties`,
      opts.map((o) => ({ name: o.name, props: o.props })),
      c.options.map((o) => ({ name: o.name, props: sorted(o.props) })));
    if (c.groups !== undefined) {
      check(`${c.id}: group names, properties and children`,
        groups.map((g) => ({ name: g.name, props: g.props, children: (g.children || []).map((k) => k.role) })),
        c.groups.map((g) => ({ name: g.name, props: sorted(g.props), children: g.children })));
    }
    await page.close();
  }

  await browser.close();
  const failed = checks.filter((x) => !x.ok);
  failed.forEach((x) => console.log(`\nFAILED ${x.label}: got ${JSON.stringify(x.got)} want ${JSON.stringify(x.want)}`));
  console.log(`\n${checks.length} checks, ${failed.length} failed`);
  console.log(failed.length ? '\nOPTIONAX PROP ORACLE FAILED' : '\nOPTIONAX PROP ORACLE OK');
  process.exit(failed.length ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(1); });
