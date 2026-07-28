#!/usr/bin/env node
/**
 * Convert HTML files to PNG screenshots using Playwright.
 * Usage: node scripts/html_to_png.js [--viewport=1280x800]
 */
const { chromium } = require("/tmp/node_modules/playwright");
const fs = require("fs");
const path = require("path");

const HTML_DIR = path.resolve(__dirname, "../test/mcp-screenshots");
const OUT_DIR = path.resolve(__dirname, "../test/mcp-screenshots/png");
const VIEWPORT = { width: 1280, height: 900 };

if (!fs.existsSync(OUT_DIR)) fs.mkdirSync(OUT_DIR, { recursive: true });

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || "/Users/vyquocvu/Library/Caches/ms-playwright/chromium-1234/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
  });
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 2,
  });

  const files = fs.readdirSync(HTML_DIR).filter((f) => f.endsWith(".html")).sort();
  for (const file of files) {
    const page = await context.newPage();
    const htmlPath = `file://${path.join(HTML_DIR, file)}`;
    await page.goto(htmlPath, { waitUntil: "networkidle" });
    const pngName = file.replace(".html", ".png");
    const pngPath = path.join(OUT_DIR, pngName);
    await page.screenshot({ path: pngPath, fullPage: true });
    console.log(`📸 ${pngName}`);
    await page.close();
  }

  await browser.close();
  console.log("\n✅ PNGs in", OUT_DIR);
})();
