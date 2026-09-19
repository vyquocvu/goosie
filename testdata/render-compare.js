const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

async function renderWithPlaywright() {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 800, height: 600 },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();

  const testDir = 'testdata/render';
  const outputDir = '/tmp/playwright-renders';
  
  if (!fs.existsSync(outputDir)) {
    fs.mkdirSync(outputDir, { recursive: true });
  }

  const htmlFiles = [];
  function findHtmlFiles(dir) {
    const files = fs.readdirSync(dir);
    for (const file of files) {
      const fullPath = path.join(dir, file);
      const stat = fs.statSync(fullPath);
      if (stat.isDirectory()) {
        findHtmlFiles(fullPath);
      } else if (file.endsWith('.html')) {
        htmlFiles.push(fullPath);
      }
    }
  }
  
  findHtmlFiles(testDir);
  
  console.log(`Found ${htmlFiles.length} HTML files`);
  
  for (const htmlFile of htmlFiles) {
    const relativePath = path.relative(testDir, htmlFile);
    const outputPath = path.join(outputDir, relativePath.replace('.html', '.png'));
    const outputDirPath = path.dirname(outputPath);
    
    if (!fs.existsSync(outputDirPath)) {
      fs.mkdirSync(outputDirPath, { recursive: true });
    }
    
    const fileUrl = 'file://' + path.resolve(htmlFile);
    await page.goto(fileUrl, { waitUntil: 'load' });
    await page.screenshot({ path: outputPath, fullPage: false });
    console.log(`Rendered: ${relativePath}`);
  }
  
  await browser.close();
  console.log('Done!');
}

renderWithPlaywright().catch(console.error);
