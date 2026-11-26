const { chromium } = require('playwright');

const settle = async page => {
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
};

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 320, height: 240 }, deviceScaleFactor: 1, colorScheme: 'light' });
  await page.setContent(`<!doctype html><html><head><style>
    html, body { margin: 0; }
    body { height: 1200px; }
    #box { position: absolute; left: 10px; top: 20px; width: 100px; height: 50px; box-sizing: border-box; border: 10px solid; padding: 5px; }
    #partial { position: absolute; left: 0; top: 200px; width: 100px; height: 100px; }
    #off { position: absolute; left: 0; top: 400px; width: 100px; height: 100px; }
    #edge { position: absolute; left: 0; top: 240px; width: 100px; height: 100px; }
    #root { position: absolute; left: 20px; top: 20px; width: 200px; height: 100px; box-sizing: border-box; border: 10px solid; padding: 5px; }
    #child { position: absolute; left: 0; top: 70px; width: 100px; height: 100px; }
  </style></head><body>
    <div id="box"></div><div id="partial"></div><div id="off"></div><div id="edge"></div>
    <div id="root"><div id="child"></div></div>
  </body></html>`);

  const media = await page.evaluate(() => {
    const queries = [
      '(min-width: 320px)', '(width: 320px)', '(width: 321px)', '(max-width: 319px)', '(min-height: 240px)', '(max-height: 239px)',
      '(orientation: landscape)', '(orientation: portrait)', '(resolution: 1dppx)', '(resolution: 96dpi)',
      '(min-resolution: 2dppx)', '(min-device-pixel-ratio: 1)', '(-webkit-min-device-pixel-ratio: 1)',
      '(prefers-color-scheme: light)', '(prefers-color-scheme: dark)',
      '(hover: hover)', '(hover: none)', '(pointer: fine)', '(pointer: coarse)',
      '(any-hover: hover)', '(any-pointer: fine)',
      '(prefers-reduced-motion: no-preference)', '(prefers-reduced-motion: reduce)',
      '(unknown-feature: yes)', '(min-width: bananas)', 'and', 'not (min-width: 321px)',
      'screen and (min-width: 300px)', 'print', '(min-width: 300px), print',
    ];
    return Object.fromEntries(queries.map(query => [query, matchMedia(query).matches]));
  });

  await page.evaluate(() => {
    globalThis.viewportLog = [];
    globalThis.narrow = matchMedia('(max-width: 350px)');
    globalThis.narrow.addEventListener('change', event => {
      globalThis.viewportLog.push({
        type: event.type, media: event.media, matches: event.matches,
        targetIsList: event.target === globalThis.narrow,
        currentTargetIsList: event.currentTarget === globalThis.narrow,
        width: innerWidth, height: innerHeight,
      });
    });
    addEventListener('resize', event => {
      globalThis.viewportLog.push({ type: event.type, width: innerWidth, height: innerHeight, targetIsWindow: event.target === window });
    });
  });
  await page.setViewportSize({ width: 400, height: 300 });
  await page.waitForFunction(() => globalThis.viewportLog.some(event => event.type === 'change' && event.width === 400));
  const viewportEvents = await page.evaluate(() => globalThis.viewportLog.filter(event => event.type === 'change' || event.width === 400));

  await page.evaluate(() => window.scrollTo(0, 100));
  await settle(page);
  const clientRects = await page.evaluate(() => {
    const box = document.getElementById('box');
    const rect = box.getBoundingClientRect();
    return {
      rect: { x: rect.x, y: rect.y, width: rect.width, height: rect.height, top: rect.top, right: rect.right, bottom: rect.bottom, left: rect.left },
      clientRects: Array.from(box.getClientRects(), rect => [rect.x, rect.y, rect.width, rect.height]),
      offset: [box.offsetLeft, box.offsetTop, box.offsetWidth, box.offsetHeight],
      client: [box.clientLeft, box.clientTop, box.clientWidth, box.clientHeight],
      scroll: [scrollX, scrollY],
    };
  });

  await page.evaluate(() => window.scrollTo(0, 0));
  await page.setViewportSize({ width: 320, height: 240 });
  await settle(page);
  await page.evaluate(() => {
    globalThis.resizeEntries = [];
    globalThis.ro = new ResizeObserver(entries => {
      for (const entry of entries) {
        const size = value => ({ inlineSize: value.inlineSize, blockSize: value.blockSize, width: value.width, height: value.height });
        globalThis.resizeEntries.push({
          contentRect: { x: entry.contentRect.x, y: entry.contentRect.y, width: entry.contentRect.width, height: entry.contentRect.height },
          borderBoxSize: Array.from(entry.borderBoxSize, size),
          contentBoxSize: Array.from(entry.contentBoxSize, size),
          inlineSize: entry.inlineSize, blockSize: entry.blockSize,
        });
      }
    });
    globalThis.ro.observe(document.getElementById('box'));
  });
  await settle(page);
  await page.evaluate(() => {
    const box = document.getElementById('box');
    box.style.width = '140px';
    box.style.height = '70px';
  });
  await settle(page);
  const resizeEntries = await page.evaluate(() => globalThis.resizeEntries);

  await page.evaluate(() => {
    globalThis.intersectionEntries = [];
    globalThis.captureIntersection = (name, target, options) => {
      globalThis[name] = new IntersectionObserver(entries => {
        for (const entry of entries) {
          const rect = value => value && ({ x: value.x, y: value.y, width: value.width, height: value.height, top: value.top, right: value.right, bottom: value.bottom, left: value.left });
          globalThis.intersectionEntries.push({
            name, ratio: entry.intersectionRatio, isIntersecting: entry.isIntersecting,
            boundingClientRect: rect(entry.boundingClientRect), intersectionRect: rect(entry.intersectionRect),
            rootBounds: rect(entry.rootBounds),
          });
        }
      }, options);
      globalThis[name].observe(target);
    };
    globalThis.captureIntersection('viewportObserver', document.getElementById('partial'), { threshold: [0, 0.5, 1] });
    globalThis.captureIntersection('marginObserver', document.getElementById('partial'), { rootMargin: '20px' });
    globalThis.captureIntersection('offObserver', document.getElementById('off'));
    globalThis.captureIntersection('edgeObserver', document.getElementById('edge'));
    globalThis.captureIntersection('rootObserver', document.getElementById('child'), { root: document.getElementById('root') });
    globalThis.captureIntersection('rootMarginObserver', document.getElementById('child'), { root: document.getElementById('root'), rootMargin: '10px' });
  });
  await settle(page);
  const initialIntersections = await page.evaluate(() => globalThis.intersectionEntries);
  await page.evaluate(() => window.scrollTo(0, 20));
  await settle(page);
  const scrolledIntersections = await page.evaluate(() => globalThis.intersectionEntries);

  console.log(JSON.stringify({ media, viewportEvents, clientRects, resizeEntries, initialIntersections, scrolledIntersections }, null, 2));
  await browser.close();
})().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
