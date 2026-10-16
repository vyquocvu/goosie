/**
 * Gap Analysis: goosie vs Real-World Web Requirements
 * Based on Playwright audit of 14 major websites (Google, GitHub, Wikipedia, MDN,
 * StackOverflow, X/Twitter, YouTube, Amazon, Netflix, Apple, NYTimes, eBay, DuckDuckGo, HackerNews)
 */

const data = require('../audit-results.json');
const results = data.results.filter(r => r.success);

// goosie capability matrix
const goosie = {
  // CSS Properties
  'display': true, 'position': true, 'flex': true, 'grid': true,
  'transition': true, 'border-radius': true, 'background': true,
  'opacity': true, 'overflow': true, 'object-fit': true,
  'z-index': true, 'box-shadow': true, 'transform': true,
  'float': true, 'clip-path': false, 'filter': false,
  'backdrop-filter': false, 'text-shadow': true,
  'text-decoration': true, 'white-space': true,
  'animation': true, 'gap': true,

  // CSS At-Rules
  '@media': true, '@supports': true, '@keyframes': true, '@font-face': true,
  '@layer': false, '@container': false,

  // CSS Functions
  'var()': true, 'calc()': true, 'min()/max()/clamp()': true,
  'rgb()/rgba()': true, 'hsl()/hsla()': true,
  'linear-gradient()': true, 'radial-gradient()': false,
  'url()': true,

  // Selectors
  ':not()': true, ':nth-child': true, ':first-child': true,
  ':last-child': true, ':hover': false, ':focus': false,
  ':checked': false, ':disabled': false, ':empty': true,
  ':root': true, ':is()': false, ':where()': false, ':has()': false,
  '::before': true, '::after': true, '::placeholder': false,
  '::selection': false,
  '[data-*]': true, '[aria-*]': true,
  '> child': true, '~ sibling': true, '+ adjacent': true,

  // DOM Elements (all HTML5 atoms parsed)
  'html/head/body': true, 'div/span/p': true, 'a/img/br': true,
  'form/input/button': true, 'select/textarea/label': true,
  'table/tr/td/th': true, 'ul/ol/li': true,
  'h1-h6': true, 'header/footer/nav/main/section': true,
  'svg/path/rect/circle': true, 'canvas': true,
  'video/audio': true, 'iframe': true, 'picture/source': true,

  // JS APIs
  'console.log/warn/error': true,
  'document.querySelector/querySelectorAll': true,
  'document.createElement': true,
  'document.getElementById/getElementsBy*': true,
  'element.getAttribute/setAttribute': true,
  'element.addEventListener/removeEventListener': true,
  'element.classList': true,
  'element.textContent': true,
  'element.innerHTML': true,
  'element.appendChild/removeChild': true,
  'setTimeout/setInterval': true,
  'fetch()': true,
  'Promise': false,
  'localStorage': false,
  'sessionStorage': false,
  'IntersectionObserver': false,
  'ResizeObserver': false,
  'MutationObserver': false,
  'ServiceWorker': false,
  'WebWorker': false,
  'requestAnimationFrame': false,
  'History.pushState': false,
  'CustomElements': false,
  'ShadowDOM': false,
  'WebSocket': false,
  'crypto': false,
  'URL/URLSearchParams': false,
  'FormData': false,
  'AbortController': false,
  'performance': false,
  'TextEncoder/TextDecoder': false,

  // Rendering
  'SVG rendering': false,
  'Canvas 2D': false,
  'Video/Audio playback': false,
  'Web fonts (loading)': true,
  'Image decode (PNG/JPEG/GIF)': true,

  // Layout
  'Block layout': true,
  'Inline layout': true,
  'Flexbox': true,
  'Grid': true,
  'Table layout': true,
  'Positioning (rel/abs/fixed/sticky)': true,
  'Float': true,

  // Networking
  'HTTP/1.1 GET/POST': true,
  'HTTP/2': false,
  'WebSocket protocol': false,
  'Cookie jar': true,
  'HTTP cache': true,

  // Security
  'CSP': false,
  'CORS': false,
  'SRI': false,
  'Address guard (SSRF)': true,
  'URL validation': true,
  'TLS': true,

  // Events
  'click': true,
  'focus/blur': true,
  'input/change': true,
  'submit': true,
  'keydown/keyup': true,
  'mouseenter/mouseleave': false,
  'scroll': false,
  'resize': false,
  'touchstart/touchend': false,
  'drag/drop': false,
};

console.log('╔══════════════════════════════════════════════════════════════════════════════╗');
console.log('║           GOOSIE vs REAL-WEB GAP ANALYSIS                                  ║');
console.log('║   Based on Playwright audit of 14 major websites                           ║');
console.log('╚══════════════════════════════════════════════════════════════════════════════╝');

console.log('\n┌──────────────────────────────────────────────────────────────────────────────┐');
console.log('│ TIER 1: CRITICAL GAPS (blocks most real websites from working)              │');
console.log('├──────────────────────────────────────────────────────────────────────────────┤');

const critical = [
  { feature: 'Promise', usage: '100%', note: 'Every modern site uses Promises; goosie uses thenables instead' },
  { feature: 'localStorage / sessionStorage', usage: '100%', note: 'All sites use Web Storage for state persistence' },
  { feature: 'IntersectionObserver', usage: '100%', note: 'Lazy loading, infinite scroll, analytics — ubiquitous' },
  { feature: 'MutationObserver', usage: '100%', note: 'SPA frameworks rely on this for DOM change detection' },
  { feature: 'requestAnimationFrame', usage: '100%', note: 'Animation loop primitive; no alternative exists' },
  { feature: 'History API (pushState)', usage: '100%', note: 'Client-side routing for all SPAs' },
  { feature: ':hover pseudo-class', usage: '86%', note: '12/14 sites use hover styles; essential for desktop UX' },
  { feature: ':focus pseudo-class', usage: '93%', note: '13/14 sites style focused elements for accessibility' },
  { feature: 'SVG rendering', usage: '64%', note: '9/14 sites embed SVG icons/illustrations; parsed but not drawn' },
  { feature: 'ResizeObserver', usage: '100%', note: 'Responsive components need element-level resize detection' },
];

critical.forEach((item, i) => {
  console.log(`│ ${String(i+1).padStart(2)}. ${item.feature.padEnd(35)} ${item.usage.padEnd(6)}               │`);
  console.log(`│     ${item.note.substring(0, 76).padEnd(76)} │`);
});

console.log('├──────────────────────────────────────────────────────────────────────────────┤');
console.log('│ TIER 2: HIGH-IMPACT GAPS (visible degradation on many sites)                │');
console.log('├──────────────────────────────────────────────────────────────────────────────┤');

const highImpact = [
  { feature: ':is() selector', usage: '36%', note: '5/14 sites; simplifies complex selectors in modern CSS' },
  { feature: ':has() selector', usage: '14%', note: '2/14 sites; parent selection — growing rapidly in adoption' },
  { feature: 'CSS filter (blur, brightness...)', usage: '7%', note: 'Visual effects on images, backgrounds, overlays' },
  { feature: 'clip-path', usage: '7%', note: 'Non-rectangular clipping for creative layouts' },
  { feature: 'radial-gradient()', usage: 'est. 30%+', note: 'Common in hero sections and decorative backgrounds' },
  { feature: 'WebSocket', usage: 'real-time apps', note: 'Chat, live updates, collaborative editing' },
  { feature: 'Canvas 2D API', usage: '14%', note: 'Charts, games, image manipulation, data visualization' },
  { feature: '::placeholder pseudo-element', usage: 'est. 50%+', note: 'Styling input placeholder text' },
  { feature: '::selection pseudo-element', usage: 'est. 30%+', note: 'Custom text selection highlight colors' },
  { feature: 'scroll event', usage: 'est. 80%+', note: 'Scroll-based animations, sticky headers, lazy loading' },
  { feature: 'mouseenter/mouseleave events', usage: 'est. 70%+', note: 'Dropdowns, tooltips, hover cards' },
  { feature: 'Service Worker', usage: 'est. 40%+', note: 'Offline support, push notifications, background sync' },
  { feature: 'Web Workers', usage: 'est. 30%+', note: 'Background computation without blocking UI' },
  { feature: 'CSP (Content Security Policy)', usage: 'security req.', note: 'XSS prevention; required for enterprise sites' },
  { feature: 'CORS', usage: 'all cross-origin', note: 'API calls to different domains fail without CORS' },
];

highImpact.forEach((item, i) => {
  console.log(`│ ${String(i+1).padStart(2)}. ${item.feature.padEnd(35)} ${item.usage.padEnd(14)}           │`);
  console.log(`│     ${item.note.substring(0, 76).padEnd(76)} │`);
});

console.log('├──────────────────────────────────────────────────────────────────────────────┤');
console.log('│ TIER 3: NICE-TO-HAVE (polish, edge cases, progressive enhancement)          │');
console.log('├──────────────────────────────────────────────────────────────────────────────┤');

const niceToHave = [
  { feature: '@layer', note: 'CSS cascade layers for managing specificity' },
  { feature: '@container queries', note: 'Element-level responsive design' },
  { feature: ':where() selector', note: 'Zero-specificity selector grouping' },
  { feature: 'Custom Elements v1', note: 'Web Components registration' },
  { feature: 'Shadow DOM', note: 'Style encapsulation for components' },
  { feature: 'HTTP/2', note: 'Multiplexed connections, header compression' },
  { feature: 'SRI (Subresource Integrity)', note: 'Script/style tampering prevention' },
  { feature: 'FormData API', note: 'Programmatic form data construction' },
  { feature: 'AbortController', note: 'Cancelling fetch requests' },
  { feature: 'URL / URLSearchParams', note: 'URL parsing and query string manipulation' },
  { feature: 'crypto (Web Crypto)', note: 'Encryption, hashing, random values' },
  { feature: 'TextEncoder / TextDecoder', note: 'String ↔ binary encoding conversion' },
  { feature: 'Video / Audio playback', note: 'Media streaming and playback' },
  { feature: 'touch events', note: 'Mobile touch interaction' },
  { feature: 'drag and drop API', note: 'Native drag-drop for file uploads, reordering' },
  { feature: 'BroadcastChannel', note: 'Cross-tab communication' },
  { feature: 'resize event', note: 'Window resize handling' },
];

niceToHave.forEach((item, i) => {
  console.log(`│ ${String(i+1).padStart(2)}. ${item.feature.padEnd(35)}                           │`);
  console.log(`│     ${item.note.substring(0, 76).padEnd(76)} │`);
});

console.log('└──────────────────────────────────────────────────────────────────────────────┘');

console.log('\n┌──────────────────────────────────────────────────────────────────────────────┐');
console.log('│ GOOSIE STRENGTHS (already implemented, competitive)                          │');
console.log('├──────────────────────────────────────────────────────────────────────────────┤');

const strengths = [
  'Full HTML5 parser with bounded allocation and foster parenting',
  'CSS Flexbox + Grid + Table + Block + Inline layout (all major models)',
  'CSS positioning: static, relative, absolute, fixed, sticky',
  'CSS custom properties (var()) with cycle detection',
  'CSS calc(), min(), max(), clamp() with full recursive evaluation',
  'CSS transitions with cubic-bezier timing',
  'CSS @keyframes animations with direction/fill modes',
  'CSS transforms (translate, rotate, scale, skew, matrix)',
  'CSS box-shadow, text-shadow (multi-layer)',
  'CSS border-radius, linear-gradient, background-image',
  '@media queries with range syntax and feature queries (@supports)',
  '@font-face with custom font loading',
  'DOM traversal: querySelector, getElementById, getElementsBy*',
  'DOM mutation: appendChild, removeChild, innerHTML, textContent',
  'classList: add/remove/toggle/contains',
  'Event system: capture/bubble phases, stopPropagation, preventDefault',
  'Form controls: checkbox, radio, select, text input with edit actions',
  'Form data collection and URL-encoded submission',
  'Tab navigation (FocusNext/FocusPrev) with proper tabbable filtering',
  'Focus/blur/input/change/submit event wiring',
  'fetch() with thenable chaining, concurrent limit, timeout',
  'Cookie jar with domain matching, secure flags, persistence',
  'HTTP cache with Cache-Control/Expires compliance',
  'Image decode: PNG, JPEG, GIF with bounded allocation',
  'Address guard: DNS rebinding prevention, SSRF protection',
  'URL validation: scheme whitelist, hostname validation',
  'Bounded HTML/CSS parsing with hostile-input protection',
  'Tile-based rasterizer with worker pool (256px tiles)',
];

strengths.forEach((s, i) => {
  console.log(`│  ${String(i+1).padStart(2)}. ${s.substring(0, 73).padEnd(73)} │`);
});

console.log('└──────────────────────────────────────────────────────────────────────────────┘');

console.log('\n┌──────────────────────────────────────────────────────────────────────────────┐');
console.log('│ NETWORK RESOURCE PROFILE (avg per site from 14-site audit)                  │');
console.log('├──────────────────────────────────────────────────────────────────────────────┤');

const ns = data.summary.networkStats;
const n = results.length;
console.log(`│  Images:       ${(ns.totalImages / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalImages).padStart(4)})                       │`);
console.log(`│  Scripts:      ${(ns.totalScripts / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalScripts).padStart(4)})                       │`);
console.log(`│  Stylesheets:  ${(ns.totalStylesheets / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalStylesheets).padStart(4)})                        │`);
console.log(`│  Fonts:        ${(ns.totalFonts / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalFonts).padStart(4)})                        │`);
console.log(`│  XHR:          ${(ns.totalXHR / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalXHR).padStart(4)})                        │`);
console.log(`│  Fetch:        ${(ns.totalFetch / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalFetch).padStart(4)})                       │`);
console.log(`│  WebSocket:    ${(ns.totalWebSocket / n).toFixed(0).padStart(4)} avg/site   (total: ${String(ns.totalWebSocket).padStart(4)})                         │`);
console.log('└──────────────────────────────────────────────────────────────────────────────┘');

console.log('\n┌──────────────────────────────────────────────────────────────────────────────┐');
console.log('│ RECOMMENDED PRIORITY ORDER                                                   │');
console.log('├──────────────────────────────────────────────────────────────────────────────┤');
console.log('│                                                                              │');
console.log('│  Phase 4a: Promise + :hover/:focus + scroll/resize events                   │');
console.log('│           → Unlocks interactive behavior on virtually all sites              │');
console.log('│                                                                              │');
console.log('│  Phase 4b: IntersectionObserver + requestAnimationFrame                     │');
console.log('│           → Unlocks lazy loading, smooth animations, SPAs                    │');
console.log('│                                                                              │');
console.log('│  Phase 4c: SVG rendering + :is()/:where() + CSS filter                      │');
console.log('│           → Visual completeness for 64%+ of sites                            │');
console.log('│                                                                              │');
console.log('│  Phase 5a: localStorage/sessionStorage + History API                        │');
console.log('│           → State persistence and client-side routing                        │');
console.log('│                                                                              │');
console.log('│  Phase 5b: WebSocket + CORS + CSP                                           │');
console.log('│           → Real-time apps and security compliance                           │');
console.log('│                                                                              │');
console.log('│  Phase 5c: Canvas 2D + MutationObserver + ResizeObserver                    │');
console.log('│           → Data visualization and responsive components                     │');
console.log('│                                                                              │');
console.log('│  Phase 6:  Web Workers + Service Workers + HTTP/2                           │');
console.log('│           → Performance and offline capability                               │');
console.log('│                                                                              │');
console.log('└──────────────────────────────────────────────────────────────────────────────┘');
