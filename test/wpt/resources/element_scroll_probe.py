#!/usr/bin/env python3
"""Measure Chromium's element-scroll behavior for the dom/events/scrolling corpus.

Run: python3 test/wpt/resources/element_scroll_probe.py

Answers, per case, what a scroll container reports for its scrollable overflow
area, what scrollTop/scrollLeft accept, and when the resulting scroll event
reaches the page. The numbers here are the expectations the goosie oracles copy.
"""
import asyncio
import json

from playwright.async_api import async_playwright

PAGE = """<!doctype html><html><head><style>
  html,body{margin:0}
  .box{width:100px;height:100px}
  .big{width:200px;height:200px}
  #scroller{overflow:scroll;scrollbar-width:none}
  #clipping{overflow:hidden}
</style></head><body>
  <div id="scroller" class="box"><div class="big"></div></div>
  <div id="plain" class="box"><div class="big"></div></div>
  <div id="clipping" class="box"><div class="big"><div class="big"></div></div></div>
  <div id="nested" class="box" style="overflow:scroll;scrollbar-width:none">
    <div id="inner" class="box" style="overflow:scroll;scrollbar-width:none"><div class="big"></div></div>
  </div>
  <div id="roomy" style="overflow:scroll;scrollbar-width:none;width:100px;height:100px"><div style="width:50px;height:50px"></div></div>
  <div id="padded" style="overflow:scroll;scrollbar-width:none;width:100px;height:100px;padding:0 40px 60px 0"><div class="big"></div></div>
  <div id="margins" style="overflow:scroll;scrollbar-width:none;width:100px;height:100px"><div style="width:100px;height:100px;margin:0 60px 60px 0"></div></div>
  <div id="abspos" style="overflow:scroll;scrollbar-width:none;position:relative;width:100px;height:100px"><div style="position:absolute;left:0;top:0;width:200px;height:200px"></div></div>
  <div id="fixedkid" style="overflow:scroll;scrollbar-width:none;position:relative;width:100px;height:100px"><div style="position:fixed;left:0;top:0;width:300px;height:300px"></div></div>
  <div id="tallpage" style="height:2000px"></div>
  <div id="widepage" style="width:1200px;height:10px"></div>
</body></html>"""

PROBE = r"""
(async () => {
async function record(id, writes) {
  const el = document.getElementById(id);
  const events = [];
  el.addEventListener('scroll', (e) => events.push({
    target: e.target.id, isTrusted: e.isTrusted, bubbles: e.bubbles,
    cancelable: e.cancelable, defaultPrevented: e.defaultPrevented,
  }));
  const rafSeen = [];
  let rafBefore = events.length;
  const before = { scrollTop: el.scrollTop, scrollLeft: el.scrollLeft,
                   clientWidth: el.clientWidth, clientHeight: el.clientHeight,
                   scrollWidth: el.scrollWidth, scrollHeight: el.scrollHeight };
  const frames = [];
  for (const w of writes) {
    w(el);
    // one event-loop turn later the spec has already fired any queued scroll,
    // so record synchronously after the write and again after a task.
    const inline = events.length;
    await new Promise((r) => setTimeout(r, 0));
    const afterTask = events.length;
    await new Promise((r) => requestAnimationFrame(() => r()));
    frames.push({ write: w.name, inline, afterTask, afterFrame: events.length,
                  scrollTop: el.scrollTop, scrollLeft: el.scrollLeft });
  }
  return { id, before, frames, total: events.length, sample: events[0] || null };
}

return {
  scroller: await record('scroller', [
    function setTop10(el) { el.scrollTop = 10; },
    function setTop10Again(el) { el.scrollTop = 10; },
    function setBothAxes(el) { el.scrollTop = 30; el.scrollLeft = 30; },
    function setTop1000(el) { el.scrollTop = 1000; },
    function setNegative(el) { el.scrollTop = -50; el.scrollLeft = -50; },
    function scrollToHuge(el) { el.scrollTo({ top: 1000, left: 1000 }); },
    function scrollByMixed(el) { el.scrollBy({ top: -30, left: 10 }); },
    function scrollToNumeric(el) { el.scrollTo(50, 60); },
    function scrollByNumeric(el) { el.scrollBy(-1000, -1000); },
    function scrollToSmooth(el) { el.scrollTo({ top: 0, behavior: 'smooth' }); },
  ]),
  plain: await record('plain', [
    function setTop50(el) { el.scrollTop = 50; },
    function scrollTo50(el) { el.scrollTo({ top: 50, left: 50 }); },
  ]),
  clipping: await record('clipping', [
    function setTop50(el) { el.scrollTop = 50; },
  ]),
  nested: await record('nested', [
    function setTop50(el) { el.scrollTop = 50; },
  ]),
  roomy: await record('roomy', [
    function setTop50(el) { el.scrollTop = 50; },
  ]),
  padded: await record('padded', [
    function setTop1000(el) { el.scrollTop = 1000; el.scrollLeft = 1000; },
  ]),
  geometry: Object.fromEntries(['margins', 'abspos', 'fixedkid', 'nested'].map((id) => {
    const el = document.getElementById(id);
    el.scrollTop = 1000;
    el.scrollLeft = 1000;
    return [id, { clientWidth: el.clientWidth, clientHeight: el.clientHeight,
                  scrollWidth: el.scrollWidth, scrollHeight: el.scrollHeight,
                  scrollTop: el.scrollTop, scrollLeft: el.scrollLeft }];
  })),
  viewport: await (async () => {
    const events = [];
    const onScroll = (e) => events.push({ target: e.target === document ? 'document' : String(e.target.nodeName), isTrusted: e.isTrusted, bubbles: e.bubbles, cancelable: e.cancelable });
    document.addEventListener('scroll', onScroll);
    document.documentElement.scrollTop = 250;
    const inline = events.length;
    await new Promise((r) => setTimeout(r, 0));
    const afterTask = events.length;
    await new Promise((r) => requestAnimationFrame(() => r()));
    return { inline, afterTask, afterFrame: events.length, scrollTop: document.documentElement.scrollTop,
             scrollY: window.scrollY, sample: events[0] || null,
             scrollHeight: document.scrollingElement.scrollHeight, clientHeight: document.documentElement.clientHeight };
  })(),
  // Viewport scrolls reach which targets, and how many times per turn. The
  // listener is replaced for each case so one case cannot see another's events.
  viewportTargets: await (async () => {
    const reset = () => { window.scrollTo(0, 0); };
    async function once(where, write, opts) {
      const o = opts || {};
      if (!o.keepPosition) reset();
      const seen = [];
      const handler = (e) => seen.push(e.target === document ? 'document' : String(e.target.nodeName));
      const node = where === 'window' ? window : where === 'documentElement' ? document.documentElement : document;
      node.addEventListener('scroll', handler);
      write();
      await new Promise((r) => requestAnimationFrame(() => r()));
      await new Promise((r) => requestAnimationFrame(() => r()));
      node.removeEventListener('scroll', handler);
      return { seen, scrollX: window.scrollX, scrollY: window.scrollY };
    }
    const setTop = (y) => () => { document.documentElement.scrollTop = y; };
    return {
      onDocument: await once('document', setTop(300)),
      onWindow: await once('window', setTop(600)),
      onDocumentElement: await once('documentElement', setTop(900)),
      twoWritesOneTurn: await once('document', () => { document.documentElement.scrollTop = 100; document.documentElement.scrollTop = 200; }),
      noChange: await once('document', () => { document.documentElement.scrollTop = 200; }, { keepPosition: true }),
      windowScrollTo: await once('document', () => { window.scrollTo(0, 400); }),
      // The horizontal axis: does a viewport scrollLeft write fire the event,
      // and do both axes in one turn coalesce into one delivery?
      xAxis: await once('document', () => { document.documentElement.scrollLeft = 40; }),
      bothAxesOneTurn: await once('document', () => {
        document.documentElement.scrollTop = 150;
        document.documentElement.scrollLeft = 80;
      }),
      windowScrollBy: await once('document', () => { window.scrollBy(20, 20); }),
      scrollingElementScrollTo: await once('document', () => { document.scrollingElement.scrollTo({ top: 500, left: 100 }); }),
      elementWindowPhase: await (async () => {
        // A non-bubbling element scroll must not reach a bubble-phase window
        // listener, but the window is on the propagation path, so a capture
        // listener there does see it.
        async function phase(capture) {
          const el = document.getElementById('scroller');
          let seen = 0;
          const h = () => { seen++; };
          window.addEventListener('scroll', h, capture);
          el.scrollTop = capture ? 25 : 10;
          await new Promise((r) => requestAnimationFrame(() => r()));
          await new Promise((r) => requestAnimationFrame(() => r()));
          window.removeEventListener('scroll', h, capture);
          return seen;
        }
        return { bubble: await phase(false), capture: await phase(true) };
      })(),
      twoFrames: await (async () => {
        reset();
        await new Promise((r) => requestAnimationFrame(() => r()));
        const seen = [];
        document.addEventListener('scroll', () => seen.push([window.scrollX, window.scrollY]));
        document.documentElement.scrollTop = 100;
        await new Promise((r) => requestAnimationFrame(() => r()));
        document.documentElement.scrollTop = 300;
        await new Promise((r) => requestAnimationFrame(() => r()));
        await new Promise((r) => requestAnimationFrame(() => r()));
        return { seen };
      })(),
    };
  })(),
};
})()
"""


async def main():
    async with async_playwright() as p:
        browser = await p.chromium.launch()
        page = await browser.new_page(viewport={"width": 400, "height": 400})
        await page.set_content(PAGE)
        out = await page.evaluate(PROBE)
        print(json.dumps(out, indent=2))
        await browser.close()


asyncio.run(main())
