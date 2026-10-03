const { chromium } = require('playwright');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');

const FIXTURES_DIR = path.join(__dirname, '..', 'testdata', 'interaction-parity');
const OUTPUT_DIR = FIXTURES_DIR;

function sha256File(p) {
  return crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
}

async function main() {
  const launchConfig = { viewport: { width: 800, height: 600 }, deviceScaleFactor: 1 };

  const browser = await chromium.launch();
  const context = await browser.newContext(launchConfig);

  const exePath = chromium.executablePath();
  const fixtures = [
    '01-event-order.html', '02-stop-propagation.html', '03-multiple-listeners.html',
    '04-checkbox-radio.html', '05-focus-tab.html', '06-prevent-default.html',
    '07-keyboard.html',
  ];
  const meta = {
    playwrightVersion: require('playwright/package.json').version,
    browserVersion: browser.version(),
    executablePath: exePath,
    executableSha256: fs.existsSync(exePath) ? sha256File(exePath) : null,
    launchConfig,
    fixtureSha256: Object.fromEntries(fixtures.map(f => [f, sha256File(path.join(FIXTURES_DIR, f))])),
    capturedAtConfigNote: 'file:// fixtures, headless chromium, single click/keyboard actions',
  };

  const results = {};

  // 01-event-order: click the button
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '01-event-order.html'));
    await page.click('#btn');
    results['01-event-order'] = await page.evaluate(() => __log);
    await page.close();
  }

  // 02-stop-propagation: click the button
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '02-stop-propagation.html'));
    await page.click('#btn');
    results['02-stop-propagation'] = await page.evaluate(() => __log);
    await page.close();
  }

  // 03-multiple-listeners: click the button
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '03-multiple-listeners.html'));
    await page.click('#btn');
    results['03-multiple-listeners'] = await page.evaluate(() => __log);
    await page.close();
  }

  // 04-checkbox-radio: toggle cb1, toggle cb2, click r1, click r3
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '04-checkbox-radio.html'));
    await page.click('#cb1');
    await page.click('#cb2');
    await page.click('#r1');
    await page.click('#r3');
    results['04-checkbox-radio'] = await page.evaluate(() => __log);
    await page.close();
  }

  // 05-focus-tab: tab through elements
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '05-focus-tab.html'));
    await page.click('#a');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    results['05-focus-tab'] = await page.evaluate(() => __log);
    await page.close();
  }

  // 06-prevent-default: click submit button, click link
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '06-prevent-default.html'));
    await page.click('#btn');
    await page.click('#link');
    results['06-prevent-default'] = await page.evaluate(() => __log);
    await page.close();
  }

  // 07-keyboard: focus #a by clicking, then Tab twice (a->b->c). Captures the
  // real keydown/keyup/focus/blur ordering Chromium produces for keyboard
  // focus traversal (keyup lands on the newly focused element).
  {
    const page = await context.newPage();
    await page.goto('file://' + path.join(FIXTURES_DIR, '07-keyboard.html'));
    await page.click('#a');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    results['07-keyboard'] = await page.evaluate(() => __log);
    await page.close();
  }

  await browser.close();

  fs.writeFileSync(path.join(OUTPUT_DIR, 'reference-events.json'), JSON.stringify(results, null, 2) + '\n');
  fs.writeFileSync(path.join(OUTPUT_DIR, 'reference-meta.json'), JSON.stringify(meta, null, 2) + '\n');
  console.log('Reference events written to', path.join(OUTPUT_DIR, 'reference-events.json'));
  console.log('Reference meta written to', path.join(OUTPUT_DIR, 'reference-meta.json'));
  console.log(JSON.stringify(meta, null, 2));
  console.log(JSON.stringify(results, null, 2));
}

main().catch(err => {
  console.error(err);
  process.exit(1);
});
