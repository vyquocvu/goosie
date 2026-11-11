// GOOSIE-FCCONTAIN-001 anchor: how does Chromium actually contain floats inside
// the various formatting-context roots, and where does a later sibling's clear
// land after a float merged from an earlier, non-containing sibling?
//
//   node scripts/verify-fc-containment-geometry.js
//
// This is the oracle for the SP-4 L2 second landing (authoritative FC roots).
// It loads the fixture Goosie will render once that landing lands and reads the
// box rects straight out of Chromium's own layout, so the "which box establishes
// a formatting context" answer comes from the reference engine, not a spec
// recollection. The fixture is deterministic: fixed px sizes, system sans font,
// no network.
//
// The fixture lives in testdata/fc-containment/ (NOT testdata/render/ or
// testdata/layout-parity/) on purpose: Goosie still diverges on it (the
// inline-block sole-child-of-flow-root lands at the origin, and a flow-root
// group margin is dropped), so promoting it into either gated corpus today
// would break the zero-diff gate. SP-5 promotes it into layout-parity/ with the
// sibling .reference.json once Goosie matches.
const { chromium } = require('playwright');
const path = require('path');

const VIEWPORT = { width: 800, height: 600 };
const FIXTURE = 'file://' + path.resolve(__dirname,
  '../testdata/fc-containment/nested-float-containment.html');

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto(FIXTURE, { waitUntil: 'load' });

  const m = await page.evaluate(() => {
    const r = (n) => Math.round(n * 100) / 100;
    const box = (id) => {
      const b = document.getElementById(id).getBoundingClientRect();
      return { x: r(b.x), y: r(b.y), w: r(b.width), h: r(b.height), bottom: r(b.bottom) };
    };
    const floatIn = (id) => {
      const b = document.querySelector('#' + id + ' > div').getBoundingClientRect();
      return { y: r(b.y), h: r(b.height), bottom: r(b.bottom) };
    };
    return {
      plain: box('plain'), flowroot: box('flowroot'), overflow: box('overflow'),
      clearparent: box('clearparent'), sib1: box('sib1'), sib2: box('sib2'),
      ib: box('ib'), flex: box('flex'), item: box('item'),
      plainFloat: floatIn('plain'), flowFloat: floatIn('flowroot'),
      tallFloat: floatIn('sib1'), ibFloat: floatIn('ib'), itemFloat: floatIn('item'),
    };
  });

  const lines = [];
  for (const k of Object.keys(m)) lines.push(`${k.padEnd(12)} ${JSON.stringify(m[k])}`);
  console.log(lines.join('\n'));

  // Spec-derived containment invariants (CSS 2.1 §9.5, §10.6.7; CSS Display 3
  // flow-root; Flexbox §4.4 independent FC). Tolerances absorb sub-pixel rounding.
  const near = (a, b, t = 1.0) => Math.abs(a - b) <= t;
  const checks = [
    // A plain block does NOT contain its float: height is the text line + borders.
    ['plain collapses to text', near(m.plain.h, 22)],
    // flow-root and overflow:hidden both contain the 80px float: 80 + 2 borders.
    ['flow-root contains float', near(m.flowroot.h, 82)],
    ['overflow contains float', near(m.overflow.h, 82)],
    ['flow-root == overflow', near(m.flowroot.h, m.overflow.h)],
    // The non-containing sibling stays at text height; the float overflows it.
    ['sib1 collapses to text', near(m.sib1.h, 20)],
    ['tall float overflows sib1', m.tallFloat.bottom > m.sib1.bottom + 50],
    // clear:left drops sib2 below the merged float's bottom margin edge.
    ['sib2 clears the float', m.sib2.y >= m.tallFloat.bottom - 0.5],
    // The parent grows to enclose the cleared sibling.
    ['clearparent encloses sib2', near(m.clearparent.bottom, m.sib2.bottom + 1)],
    // inline-block establishes its own FC: contains the 60px float (60 + 2).
    ['inline-block contains float', near(m.ib.h, 62)],
    // A flex item establishes an independent FC: contains the 50px float.
    ['flex item contains float', near(m.item.h, 50)],
    ['flex container hugs item', near(m.flex.h, m.item.h + 2)],
  ];

  let ok = true;
  for (const [name, pass] of checks) {
    console.log(`${pass ? 'OK  ' : 'BAD '} ${name}`);
    if (!pass) ok = false;
  }

  await browser.close();
  if (!ok) { console.error('FC CONTAINMENT ORACLE FAILED'); process.exit(1); }
  console.log('FC CONTAINMENT ORACLE OK');
}

main().catch((e) => { console.error(e); process.exit(1); });