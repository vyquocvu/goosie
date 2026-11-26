#!/usr/bin/env node
// Measured with Playwright 1.63.0 / Chromium 153.0.8010.12 at dpr 1.
// Run: node scripts/verify-border-bevel.js
const { chromium } = require('playwright');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const ROOT = path.resolve(__dirname, '..');
const OUT = fs.mkdtempSync(path.join(os.tmpdir(), 'goosie-border-bevel-'));
const DOC = path.join(OUT, 'document.html');
const REF_PNG = path.join(OUT, 'chromium.png');
const GOO_PNG = path.join(OUT, 'goosie.png');
const EVIDENCE = path.join(OUT, 'evidence.json');
const TOLERANCE = 2;

const MEASURED = {
  'rgb(118,118,118)': { dark: 'rgb(33,33,33)', light: 'rgb(202,202,202)' },
  'rgb(200,80,40)': { dark: 'rgb(116,46,23)', light: 'rgb(255,102,51)' },
  'rgb(40,80,200)': { dark: 'rgb(23,46,116)', light: 'rgb(51,102,255)' },
  'rgb(0,0,0)': { dark: 'rgb(84,84,84)', light: 'rgb(168,168,168)' },
  'rgb(255,255,255)': { dark: 'rgb(171,171,171)', light: 'rgb(255,255,255)' },
};

const CASES = [
  { id: 'button-inset', tag: 'button', style: 'inset', width: 2, color: 'rgb(118,118,118)' },
  { id: 'button-outset', tag: 'button', style: 'outset', width: 2, color: 'rgb(118,118,118)' },
  { id: 'text-inset', tag: 'input', type: 'text', style: 'inset', width: 2, color: 'rgb(118,118,118)' },
  { id: 'text-outset', tag: 'input', type: 'text', style: 'outset', width: 2, color: 'rgb(118,118,118)' },
  { id: 'color-inset', tag: 'input', type: 'color', style: 'inset', width: 1, color: 'rgb(118,118,118)' },
  { id: 'color-outset', tag: 'input', type: 'color', style: 'outset', width: 1, color: 'rgb(118,118,118)' },
  { id: 'div-inset', tag: 'div', style: 'inset', width: 8, color: 'rgb(118,118,118)' },
  { id: 'div-outset', tag: 'div', style: 'outset', width: 8, color: 'rgb(118,118,118)' },
  { id: 'red-inset', tag: 'div', style: 'inset', width: 8, color: 'rgb(200,80,40)' },
  { id: 'red-outset', tag: 'div', style: 'outset', width: 8, color: 'rgb(200,80,40)' },
  { id: 'blue-inset', tag: 'div', style: 'inset', width: 8, color: 'rgb(40,80,200)' },
  { id: 'blue-outset', tag: 'div', style: 'outset', width: 8, color: 'rgb(40,80,200)' },
  { id: 'black-inset', tag: 'div', style: 'inset', width: 8, color: 'rgb(0,0,0)' },
  { id: 'black-outset', tag: 'div', style: 'outset', width: 8, color: 'rgb(0,0,0)' },
  { id: 'white-inset', tag: 'div', style: 'inset', width: 8, color: 'rgb(255,255,255)' },
  { id: 'white-outset', tag: 'div', style: 'outset', width: 8, color: 'rgb(255,255,255)' },
  { id: 'alpha-inset', tag: 'div', style: 'inset', width: 8, color: 'rgba(118,118,118,0.5)' },
  { id: 'alpha-outset', tag: 'div', style: 'outset', width: 8, color: 'rgba(118,118,118,0.5)' },
  { id: 'opacity-inset', tag: 'div', style: 'inset', width: 8, color: 'rgb(118,118,118)', opacity: 0.5 },
  { id: 'ridge', tag: 'div', style: 'ridge', width: 8, color: 'rgb(118,118,118)' },
  { id: 'groove', tag: 'div', style: 'groove', width: 8, color: 'rgb(118,118,118)' },
  { id: 'solid-button', tag: 'button', style: 'solid', width: 2, color: 'rgb(118,118,118)' },
  { id: 'solid-text', tag: 'input', type: 'text', style: 'solid', width: 2, color: 'rgb(118,118,118)' },
  { id: 'solid-color', tag: 'input', type: 'color', style: 'solid', width: 1, color: 'rgb(118,118,118)' },
  { id: 'solid-div', tag: 'div', style: 'solid', width: 8, color: 'rgb(118,118,118)' },
  {
    id: 'mixed', tag: 'div', style: 'inset outset ridge groove', width: '1px 2px 3px 4px',
    color: 'rgb(200,80,40) rgb(40,80,200) rgb(80,160,60) rgb(160,40,120)',
  },
];

const VIEWPORT = { width: 420, height: 40 + Math.ceil(CASES.length / 2) * 44 };

function markup(testCase) {
  if (testCase.tag === 'input') {
    if (testCase.type === 'color') return `<input class="case" id="${testCase.id}" type="color" value="#ff0000">`;
    return `<input class="case" id="${testCase.id}" type="text" value="Text">`;
  }
  if (testCase.tag === 'button') return `<button class="case" id="${testCase.id}">Button</button>`;
  return `<div class="case" id="${testCase.id}"></div>`;
}

const PAGE = `<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;background:#fff;font:16px Arial;color:#000}
.case{position:absolute;box-sizing:border-box;width:80px;height:30px;padding:0;border-radius:0;background:#fff;color:#000;font:16px Arial;appearance:none;-webkit-appearance:none}
${CASES.map((testCase, i) => `#${testCase.id}{left:${20 + (i % 2) * 200}px;top:${20 + Math.floor(i / 2) * 44}px;border-width:${typeof testCase.width === 'number' ? `${testCase.width}px` : testCase.width};border-style:${testCase.style};border-color:${testCase.color};${testCase.opacity ? `opacity:${testCase.opacity}` : ''}}`).join('\n')}
</style>${CASES.map(markup).join('\n')}`;

function parseRGB(value) {
  const nums = value.match(/[\d.]+/g).map(Number);
  return { r: nums[0], g: nums[1], b: nums[2], a: nums.length > 3 ? Math.round(nums[3] * 255) : 255 };
}

function linear(channel) {
  const value = channel / 255;
  return value <= 0.04045 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4);
}

function luminance(rgb) {
  return 0.2126 * linear(rgb.r) + 0.7152 * linear(rgb.g) + 0.0722 * linear(rgb.b);
}

function shadeRGB(rgb, darken) {
  const maxChannel = Math.max(rgb.r, rgb.g, rgb.b);
  if (maxChannel === 0) return darken ? { r: 0, g: 0, b: 0 } : { r: 84, g: 84, b: 84 };
  const value = maxChannel / 255;
  const multiplier = darken ? Math.max(0, (value - 0.33) / value) : Math.min(1, value + 0.33) / value;
  const quantize = (channel) => Math.max(0, Math.min(255, Math.floor(channel * multiplier * 255.9999847 / 255)));
  return { r: quantize(rgb.r), g: quantize(rgb.g), b: quantize(rgb.b) };
}

function adjustBevel(rgb, darken) {
  const lum = luminance(rgb);
  if (lum <= 0.014443844) return darken ? shadeRGB(rgb, false) : shadeRGB(shadeRGB(rgb, false), false);
  if (darken) return shadeRGB(rgb, true);
  return lum > 0.83077 ? rgb : shadeRGB(rgb, false);
}

function compositeWhite(rgb) {
  return [0, 1, 2].map((i) => Math.round(rgb[['r', 'g', 'b'][i]] * rgb.a / 255 + 255 * (1 - rgb.a / 255)));
}

function modelPixel(testCase, side, depth) {
  const colors = testCase.color.split(' ').map(parseRGB);
  const styles = testCase.style.split(' ');
  const index = { top: 0, right: 1, bottom: 2, left: 3 }[side];
  const rgb = colors.length === 1 ? colors[0] : colors[index];
  const style = styles.length === 1 ? styles[0] : styles[index];
  if (style === 'solid') return compositeWhite(rgb);
  const widths = String(testCase.width).split(' ').map(parseFloat);
  const width = widths.length === 1 ? widths[0] : widths[index];
  const outer = side === 'top' || side === 'left' ? depth === 0 : depth === width - 1;
  let darken;
  if (style === 'inset') darken = side === 'top' || side === 'left';
  else if (style === 'outset') darken = side === 'right' || side === 'bottom';
  else if (style === 'ridge') darken = outer ? side === 'right' || side === 'bottom' : side === 'top' || side === 'left';
  else darken = outer ? side === 'top' || side === 'left' : side === 'right' || side === 'bottom';
  const adjusted = adjustBevel(rgb, darken);
  const alpha = testCase.opacity ? Math.round(rgb.a * testCase.opacity) : rgb.a;
  return compositeWhite({ ...adjusted, a: alpha });
}

async function decode(page, file) {
  const base64 = fs.readFileSync(file).toString('base64');
  return page.evaluate(async (base64) => {
    const image = new Image();
    await new Promise((resolve, reject) => {
      image.onload = resolve;
      image.onerror = reject;
      image.src = 'data:image/png;base64,' + base64;
    });
    const canvas = document.createElement('canvas');
    canvas.width = image.width;
    canvas.height = image.height;
    const context = canvas.getContext('2d', { willReadFrequently: true });
    context.drawImage(image, 0, 0);
    return { width: image.width, height: image.height, data: Array.from(context.getImageData(0, 0, image.width, image.height).data) };
  }, base64);
}

function pixel(png, x, y) {
  const index = (y * png.width + x) * 4;
  return [png.data[index], png.data[index + 1], png.data[index + 2]];
}

function close(got, want, tolerance = TOLERANCE) {
  return got.every((value, i) => Math.abs(value - want[i]) <= tolerance);
}

function samples(testCase, box) {
  const x0 = Math.round(box.x), y0 = Math.round(box.y), x1 = Math.round(box.x + box.width), y1 = Math.round(box.y + box.height);
  const midX = Math.floor((x0 + x1) / 2), midY = Math.floor((y0 + y1) / 2);
  const widths = String(testCase.width).split(' ').map(parseFloat);
  const width = (side) => widths.length === 1 ? widths[0] : widths[{ top: 0, right: 1, bottom: 2, left: 3 }[side]];
  return {
    topOuter: { x: midX, y: y0, side: 'top', depth: 0 },
    topInner: { x: midX, y: y0 + Math.max(0, Math.round(width('top')) - 1), side: 'top', depth: Math.max(0, Math.round(width('top')) - 1) },
    rightOuter: { x: x1 - 1, y: midY, side: 'right', depth: Math.max(0, Math.round(width('right')) - 1) },
    rightInner: { x: x1 - Math.max(1, Math.round(width('right'))), y: midY, side: 'right', depth: 0 },
    bottomOuter: { x: midX, y: y1 - 1, side: 'bottom', depth: Math.max(0, Math.round(width('bottom')) - 1) },
    bottomInner: { x: midX, y: y1 - Math.max(1, Math.round(width('bottom'))), side: 'bottom', depth: 0 },
    leftOuter: { x: x0, y: midY, side: 'left', depth: 0 },
    leftInner: { x: x0 + Math.max(0, Math.round(width('left')) - 1), y: midY, side: 'left', depth: Math.max(0, Math.round(width('left')) - 1) },
  };
}

function runGoosie() {
  let binary = process.env.GOOSIE_BIN;
  if (!binary) {
    binary = path.join(OUT, 'goosie');
    execFileSync('go', ['build', '-o', binary, './cmd/goosie'], { cwd: ROOT, stdio: 'inherit' });
  }
  execFileSync(binary, ['-backend', 'headless', '-width', String(VIEWPORT.width), '-height', String(VIEWPORT.height),
    '-dpr', '1', '-screenshot', '-out', GOO_PNG, '-url', 'file://' + DOC], { stdio: 'inherit' });
}

async function main() {
  fs.writeFileSync(DOC, PAGE);
  const browser = await chromium.launch();
  const chromiumVersion = browser.version();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + DOC, { waitUntil: 'load' });
  const boxes = await page.evaluate((ids) => Object.fromEntries(ids.map((id) => {
    const element = document.getElementById(id);
    const box = element.getBoundingClientRect();
    const style = getComputedStyle(element);
    return [id, {
      x: box.x, y: box.y, width: box.width, height: box.height,
      widths: [style.borderTopWidth, style.borderRightWidth, style.borderBottomWidth, style.borderLeftWidth],
      styles: [style.borderTopStyle, style.borderRightStyle, style.borderBottomStyle, style.borderLeftStyle],
      colors: [style.borderTopColor, style.borderRightColor, style.borderBottomColor, style.borderLeftColor],
    }];
  })), CASES.map((testCase) => testCase.id));
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });
  const reference = await decode(page, REF_PNG);
  await browser.close();

  runGoosie();
  const probe = await chromium.launch();
  const decodePage = await probe.newPage();
  const goosie = await decode(decodePage, GOO_PNG);
  await probe.close();

  const rows = [];
  let modelFailures = 0;
  let parityFailures = 0;
  for (const testCase of CASES) {
    const measured = {};
    for (const [name, sample] of Object.entries(samples(testCase, boxes[testCase.id]))) {
      const ref = pixel(reference, sample.x, sample.y);
      const got = pixel(goosie, sample.x, sample.y);
      const expected = modelPixel(testCase, sample.side, sample.depth);
      const modelOK = close(ref, expected);
      const parityOK = close(got, ref);
      if (!modelOK) modelFailures++;
      if (!parityOK) parityFailures++;
      measured[name] = { ref, got, expected, modelOK, parityOK };
    }
    rows.push({ id: testCase.id, computed: boxes[testCase.id], pixels: measured });
    const summary = Object.entries(measured).map(([name, value]) => `${name}=${value.ref.join(',')}${value.parityOK ? '' : `!=${value.got.join(',')}`}`).join(' ');
    console.log(`${Object.values(measured).every((value) => value.modelOK && value.parityOK) ? 'MATCH' : 'DIFFER'} ${testCase.id.padEnd(14)} ${summary}`);
  }

  const evidence = {
    chromium: chromiumVersion,
    playwright: require('playwright/package.json').version,
    tolerance: TOLERANCE,
    arithmetic: 'Y=0.2126*linear(R)+0.7152*linear(G)+0.0722*linear(B); dark multiplier=max(0,(max(RGB)/255-0.33)/(max(RGB)/255)); light multiplier=min(1,max(RGB)/255+0.33)/(max(RGB)/255); Y<=0.014443844 uses light once for dark and twice for light; Y>0.83077 keeps the light edge unchanged.',
    measuredTable: MEASURED,
    modelFailures,
    parityFailures,
    document: DOC,
    chromiumPng: REF_PNG,
    goosiePng: GOO_PNG,
    rows,
  };
  fs.writeFileSync(EVIDENCE, JSON.stringify(evidence, null, 2));
  console.log(`\nmodel-failures=${modelFailures} parity-failures=${parityFailures} tolerance=${TOLERANCE}`);
  console.log(`evidence=${EVIDENCE}`);
  if (modelFailures || parityFailures) process.exitCode = 1;
}

main().catch((error) => { console.error(error); process.exit(1); });
