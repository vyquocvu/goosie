/**
 * Web Feature Audit Script
 * Uses Playwright to visit real websites and catalog what web platform features they use.
 * This helps define the gap between goosie's capabilities and real-world requirements.
 */

const { chromium } = require('playwright');

const SITES = [
  { name: 'Google', url: 'https://www.google.com' },
  { name: 'GitHub', url: 'https://github.com' },
  { name: 'Wikipedia', url: 'https://en.wikipedia.org/wiki/Web_browser' },
  { name: 'MDN', url: 'https://developer.mozilla.org/en-US/docs/Web' },
  { name: 'StackOverflow', url: 'https://stackoverflow.com/questions' },
  { name: 'Reddit', url: 'https://www.reddit.com' },
  { name: 'Twitter/X', url: 'https://x.com' },
  { name: 'YouTube', url: 'https://www.youtube.com' },
  { name: 'Amazon', url: 'https://www.amazon.com' },
  { name: 'Netflix', url: 'https://www.netflix.com' },
  { name: 'LinkedIn', url: 'https://www.linkedin.com' },
  { name: 'Apple', url: 'https://www.apple.com' },
  { name: 'Microsoft', url: 'https://www.microsoft.com' },
  { name: 'BBC', url: 'https://www.bbc.com' },
  { name: 'CNN', url: 'https://www.cnn.com' },
  { name: 'NYTimes', url: 'https://www.nytimes.com' },
  { name: 'eBay', url: 'https://www.ebay.com' },
  { name: 'Pinterest', url: 'https://www.pinterest.com' },
  { name: 'DuckDuckGo', url: 'https://duckduckgo.com' },
  { name: 'HackerNews', url: 'https://news.ycombinator.com' },
];

async function auditSite(page, site) {
  const result = {
    name: site.name,
    url: site.url,
    success: false,
    error: null,
    css: {
      properties: new Set(),
      selectors: new Set(),
      atRules: new Set(),
      customProperties: new Set(),
      functions: new Set(),
    },
    dom: {
      elements: new Set(),
      APIs: new Set(),
    },
    js: {
      APIs: new Set(),
      globals: new Set(),
    },
    features: {
      forms: false,
      fetch: false,
      websocket: false,
      localStorage: false,
      sessionStorage: false,
      cookies: false,
      canvas: false,
      svg: false,
      video: false,
      audio: false,
      webfonts: false,
      mediaQueries: false,
      animations: false,
      transitions: false,
      transforms: false,
      flexbox: false,
      grid: false,
      customElements: false,
      shadowDOM: false,
      intersectionObserver: false,
      resizeObserver: false,
      mutationObserver: false,
      serviceWorker: false,
      webWorkers: false,
      requestAnimationFrame: false,
      history: false,
      pushState: false,
    },
    network: {
      xhrCount: 0,
      fetchCount: 0,
      websocketCount: 0,
      fontCount: 0,
      imageCount: 0,
      stylesheetCount: 0,
      scriptCount: 0,
    },
  };

  try {
    // Intercept network requests to count resource types
    page.on('request', (req) => {
      const type = req.resourceType();
      if (type === 'xhr') result.network.xhrCount++;
      if (type === 'fetch') result.network.fetchCount++;
      if (type === 'websocket') result.network.websocketCount++;
      if (type === 'font') result.network.fontCount++;
      if (type === 'image') result.network.imageCount++;
      if (type === 'stylesheet') result.network.stylesheetCount++;
      if (type === 'script') result.network.scriptCount++;
    });

    await page.goto(site.url, { waitUntil: 'networkidle', timeout: 30000 });

    // Wait a bit for dynamic content
    await page.waitForTimeout(2000);

    result.success = true;

    // Extract feature usage via page evaluation
    const features = await page.evaluate(() => {
      const data = {
        cssProperties: new Set(),
        cssSelectors: new Set(),
        cssAtRules: new Set(),
        cssCustomProps: new Set(),
        cssFunctions: new Set(),
        domElements: new Set(),
        domAPIs: new Set(),
        jsAPIs: new Set(),
        jsGlobals: new Set(),
        features: {},
      };

      // Catalog all elements in the DOM
      document.querySelectorAll('*').forEach((el) => {
        data.domElements.add(el.tagName.toLowerCase());
      });

      // Check for specific features
      data.features.forms = document.querySelectorAll('form').length > 0;
      data.features.canvas = document.querySelectorAll('canvas').length > 0;
      data.features.svg = document.querySelectorAll('svg').length > 0;
      data.features.video = document.querySelectorAll('video').length > 0;
      data.features.audio = document.querySelectorAll('audio').length > 0;
      data.features.webfonts = document.fonts.size > 0;

      // Check storage
      try {
        localStorage.setItem('__test__', '1');
        localStorage.removeItem('__test__');
        data.features.localStorage = true;
      } catch (e) {}
      try {
        sessionStorage.setItem('__test__', '1');
        sessionStorage.removeItem('__test__');
        data.features.sessionStorage = true;
      } catch (e) {}

      data.features.cookies = navigator.cookieEnabled;

      // Check for modern APIs
      data.features.customElements = 'customElements' in window;
      data.features.shadowDOM = !!document.querySelector('*')?.shadowRoot;
      data.features.intersectionObserver = 'IntersectionObserver' in window;
      data.features.resizeObserver = 'ResizeObserver' in window;
      data.features.mutationObserver = 'MutationObserver' in window;
      data.features.serviceWorker = 'serviceWorker' in navigator;
      data.features.webWorkers = 'Worker' in window;
      data.features.requestAnimationFrame = 'requestAnimationFrame' in window;
      data.features.history = 'history' in window;
      data.features.pushState = 'pushState' in history;

      // Analyze stylesheets
      for (const sheet of document.styleSheets) {
        try {
          for (const rule of sheet.cssRules) {
            if (rule.selectorText) {
              data.cssSelectors.add(rule.selectorText);
            }
            if (rule.type === CSSRule.MEDIA_RULE) {
              data.cssAtRules.add('@media');
            }
            if (rule.type === CSSRule.KEYFRAMES_RULE) {
              data.cssAtRules.add('@keyframes');
            }
            if (rule.type === CSSRule.SUPPORTS_RULE) {
              data.cssAtRules.add('@supports');
            }
            if (rule.type === CSSRule.FONT_FACE_RULE) {
              data.cssAtRules.add('@font-face');
            }
            if (rule.type === CSSRule.LAYER_BLOCK_RULE || rule.type === CSSRule.LAYER_STATEMENT_RULE) {
              data.cssAtRules.add('@layer');
            }
            if (rule.type === CSSRule.CONTAINER_RULE) {
              data.cssAtRules.add('@container');
            }
          }
        } catch (e) {
          // Cross-origin stylesheets can't be read
        }
      }

      // Sample computed styles to find CSS properties in use
      const sampleElements = document.querySelectorAll('*');
      const sampleSize = Math.min(sampleElements.length, 100);
      for (let i = 0; i < sampleSize; i++) {
        const style = getComputedStyle(sampleElements[i]);
        // Check for key properties
        const propsToCheck = [
          'display', 'position', 'flex', 'grid', 'transform', 'transition',
          'animation', 'filter', 'backdrop-filter', 'box-shadow', 'text-shadow',
          'border-radius', 'background', 'gradient', 'opacity', 'z-index',
          'overflow', 'float', 'clear', 'clip-path', 'object-fit',
        ];
        propsToCheck.forEach((prop) => {
          const val = style.getPropertyValue(prop);
          if (val && val !== 'none' && val !== 'normal' && val !== 'auto') {
            data.cssProperties.add(prop);
          }
        });

        // Check for custom properties
        const inlineStyle = sampleElements[i].getAttribute('style') || '';
        const customPropMatches = inlineStyle.match(/--[\w-]+/g);
        if (customPropMatches) {
          customPropMatches.forEach((p) => data.cssCustomProps.add(p));
        }
      }

      // Check inline styles for CSS functions
      document.querySelectorAll('[style]').forEach((el) => {
        const style = el.getAttribute('style');
        if (style.includes('var(')) data.cssFunctions.add('var()');
        if (style.includes('calc(')) data.cssFunctions.add('calc()');
        if (style.includes('rgb')) data.cssFunctions.add('rgb()/rgba()');
        if (style.includes('hsl')) data.cssFunctions.add('hsl()/hsla()');
        if (style.includes('linear-gradient')) data.cssFunctions.add('linear-gradient()');
        if (style.includes('radial-gradient')) data.cssFunctions.add('radial-gradient()');
        if (style.includes('url(')) data.cssFunctions.add('url()');
      });

      // Check JS APIs
      const apiChecks = [
        ['fetch', 'fetch'],
        ['XMLHttpRequest', 'XHR'],
        ['WebSocket', 'WebSocket'],
        ['Promise', 'Promise'],
        ['async/await', 'AsyncAwait'],
        ['Symbol', 'Symbol'],
        ['Map', 'Map'],
        ['Set', 'Set'],
        ['WeakMap', 'WeakMap'],
        ['WeakSet', 'WeakSet'],
        ['Proxy', 'Proxy'],
        ['Reflect', 'Reflect'],
        ['Intl', 'Intl'],
        ['TextEncoder', 'TextEncoder'],
        ['TextDecoder', 'TextDecoder'],
        ['URL', 'URL'],
        ['URLSearchParams', 'URLSearchParams'],
        ['Blob', 'Blob'],
        ['FileReader', 'FileReader'],
        ['FormData', 'FormData'],
        ['Headers', 'Headers'],
        ['Request', 'Request'],
        ['Response', 'Response'],
        ['AbortController', 'AbortController'],
        ['crypto', 'crypto'],
        ['performance', 'performance'],
        ['Notification', 'Notification'],
        ['Geolocation', 'Geolocation'],
        ['Storage', 'Storage'],
        ['IndexedDB', 'IndexedDB'],
        ['Cache', 'CacheAPI'],
        ['BroadcastChannel', 'BroadcastChannel'],
        ['SharedWorker', 'SharedWorker'],
      ];

      apiChecks.forEach(([global, name]) => {
        if (global in window) {
          data.jsGlobals.add(name);
        }
      });

      // Check for animation/transition usage
      data.features.animations = data.cssAtRules.has('@keyframes');
      data.features.transitions = Array.from(data.cssProperties).some((p) =>
        p.includes('transition')
      );
      data.features.transforms = Array.from(data.cssProperties).some((p) =>
        p.includes('transform')
      );
      data.features.flexbox = Array.from(data.cssProperties).some((p) =>
        p.includes('flex')
      );
      data.features.grid = Array.from(data.cssProperties).some((p) =>
        p.includes('grid')
      );
      data.features.mediaQueries = data.cssAtRules.has('@media');

      // Convert Sets to Arrays for serialization
      return {
        cssProperties: Array.from(data.cssProperties),
        cssSelectors: Array.from(data.cssSelectors).slice(0, 50),
        cssAtRules: Array.from(data.cssAtRules),
        cssCustomProps: Array.from(data.cssCustomProps).slice(0, 20),
        cssFunctions: Array.from(data.cssFunctions),
        domElements: Array.from(data.domElements),
        domAPIs: Array.from(data.domAPIs),
        jsAPIs: Array.from(data.jsAPIs),
        jsGlobals: Array.from(data.jsGlobals),
        features: data.features,
      };
    });

    Object.assign(result.css.properties, features.cssProperties);
    Object.assign(result.css.selectors, features.cssSelectors);
    Object.assign(result.css.atRules, features.cssAtRules);
    Object.assign(result.css.customProperties, features.cssCustomProps);
    Object.assign(result.css.functions, features.cssFunctions);
    Object.assign(result.dom.elements, features.domElements);
    Object.assign(result.dom.APIs, features.domAPIs);
    Object.assign(result.js.APIs, features.jsAPIs);
    Object.assign(result.js.globals, features.jsGlobals);
    Object.assign(result.features, features.features);
  } catch (err) {
    result.error = err.message;
  }

  return result;
}

async function main() {
  console.log('Starting web feature audit...\n');

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
  });

  const results = [];
  const summary = {
    totalSites: SITES.length,
    successfulAudits: 0,
    failedAudits: 0,
    featureUsage: {},
    cssPropertyUsage: {},
    domElementUsage: {},
    jsGlobalUsage: {},
    networkStats: {
      totalXHR: 0,
      totalFetch: 0,
      totalWebSocket: 0,
      totalFonts: 0,
      totalImages: 0,
      totalStylesheets: 0,
      totalScripts: 0,
    },
  };

  for (const site of SITES) {
    console.log(`Auditing ${site.name} (${site.url})...`);
    const page = await context.newPage();
    const result = await auditSite(page, site);
    await page.close();

    results.push(result);

    if (result.success) {
      summary.successfulAudits++;
      console.log(`  ✓ Success`);

      // Aggregate feature usage
      for (const [feature, used] of Object.entries(result.features)) {
        if (used) {
          summary.featureUsage[feature] = (summary.featureUsage[feature] || 0) + 1;
        }
      }

      // Aggregate CSS properties
      for (const prop of result.css.properties) {
        summary.cssPropertyUsage[prop] = (summary.cssPropertyUsage[prop] || 0) + 1;
      }

      // Aggregate DOM elements
      for (const elem of result.dom.elements) {
        summary.domElementUsage[elem] = (summary.domElementUsage[elem] || 0) + 1;
      }

      // Aggregate JS globals
      for (const global of result.js.globals) {
        summary.jsGlobalUsage[global] = (summary.jsGlobalUsage[global] || 0) + 1;
      }

      // Aggregate network stats
      summary.networkStats.totalXHR += result.network.xhrCount;
      summary.networkStats.totalFetch += result.network.fetchCount;
      summary.networkStats.totalWebSocket += result.network.websocketCount;
      summary.networkStats.totalFonts += result.network.fontCount;
      summary.networkStats.totalImages += result.network.imageCount;
      summary.networkStats.totalStylesheets += result.network.stylesheetCount;
      summary.networkStats.totalScripts += result.network.scriptCount;
    } else {
      summary.failedAudits++;
      console.log(`  ✗ Failed: ${result.error}`);
    }
  }

  await browser.close();

  // Generate report
  console.log('\n' + '='.repeat(80));
  console.log('WEB FEATURE AUDIT REPORT');
  console.log('='.repeat(80));
  console.log(`\nSites audited: ${summary.successfulAudits}/${summary.totalSites}`);
  console.log(`Failed: ${summary.failedAudits}\n`);

  console.log('FEATURE USAGE (across all sites):');
  console.log('-'.repeat(80));
  const featureEntries = Object.entries(summary.featureUsage).sort((a, b) => b[1] - a[1]);
  featureEntries.forEach(([feature, count]) => {
    const pct = ((count / summary.successfulAudits) * 100).toFixed(0);
    console.log(`  ${feature.padEnd(30)} ${count}/${summary.successfulAudits} (${pct}%)`);
  });

  console.log('\nTOP CSS PROPERTIES:');
  console.log('-'.repeat(80));
  const cssEntries = Object.entries(summary.cssPropertyUsage)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 30);
  cssEntries.forEach(([prop, count]) => {
    const pct = ((count / summary.successfulAudits) * 100).toFixed(0);
    console.log(`  ${prop.padEnd(30)} ${count}/${summary.successfulAudits} (${pct}%)`);
  });

  console.log('\nTOP DOM ELEMENTS:');
  console.log('-'.repeat(80));
  const domEntries = Object.entries(summary.domElementUsage)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 30);
  domEntries.forEach(([elem, count]) => {
    const pct = ((count / summary.successfulAudits) * 100).toFixed(0);
    console.log(`  <${elem}>${' '.repeat(Math.max(0, 28 - elem.length))} ${count}/${summary.successfulAudits} (${pct}%)`);
  });

  console.log('\nJS GLOBALS/APIs:');
  console.log('-'.repeat(80));
  const jsEntries = Object.entries(summary.jsGlobalUsage).sort((a, b) => b[1] - a[1]);
  jsEntries.forEach(([api, count]) => {
    const pct = ((count / summary.successfulAudits) * 100).toFixed(0);
    console.log(`  ${api.padEnd(30)} ${count}/${summary.successfulAudits} (${pct}%)`);
  });

  console.log('\nNETWORK RESOURCES (total across all sites):');
  console.log('-'.repeat(80));
  console.log(`  Images:       ${summary.networkStats.totalImages}`);
  console.log(`  Scripts:      ${summary.networkStats.totalScripts}`);
  console.log(`  Stylesheets:  ${summary.networkStats.totalStylesheets}`);
  console.log(`  Fonts:        ${summary.networkStats.totalFonts}`);
  console.log(`  XHR:          ${summary.networkStats.totalXHR}`);
  console.log(`  Fetch:        ${summary.networkStats.totalFetch}`);
  console.log(`  WebSocket:    ${summary.networkStats.totalWebSocket}`);

  // Save detailed results
  const fs = require('fs');
  const outputPath = 'audit-results.json';
  fs.writeFileSync(outputPath, JSON.stringify({ summary, results }, null, 2));
  console.log(`\nDetailed results saved to: ${outputPath}`);

  console.log('\n' + '='.repeat(80));
}

main().catch(console.error);
