// GOOSIE-CANVAS-001 anchor: Chromium RGBA measurements for the first safe Canvas 2D operation set.
// Run with NODE_PATH=$(npm root -g) node scripts/verify-canvas-safe-ops.js when Playwright is installed globally.
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 320, height: 240 }, deviceScaleFactor: 1 });
  await page.setContent('<!doctype html><canvas id="c" width="96" height="96"></canvas>');
  const result = await page.evaluate(() => {
    const canvas = document.getElementById('c');
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    const out = {};
    const reset = () => { ctx.reset(); };
    const pixel = (name, x, y) => {
      const d = ctx.getImageData(x, y, 1, 1).data;
      out[name] = [d[0], d[1], d[2], d[3]];
    };
    const rect = (name, x, y, w, h) => {
      const d = ctx.getImageData(x, y, w, h).data;
      out[name] = Array.from(d);
    };

    reset();
    ctx.fillStyle = '#ff0000';
    ctx.fillRect(2, 3, 8, 9);
    ctx.strokeStyle = '#00ff00';
    ctx.lineWidth = 2;
    ctx.strokeRect(20, 3, 12, 10);
    pixel('style.fill.interior', 5, 6);
    pixel('style.fill.outside', 11, 6);
    pixel('style.stroke.top', 25, 3);
    pixel('style.stroke.inside', 25, 6);
    pixel('style.stroke.outside', 18, 6);

    reset();
    ctx.fillStyle = 'rgba(255, 0, 0, 0.5)';
    ctx.fillRect(0, 0, 4, 4);
    ctx.globalAlpha = 0.5;
    ctx.fillStyle = '#ff0000';
    ctx.fillRect(4, 0, 4, 4);
    ctx.fillStyle = '#0000ff';
    ctx.fillRect(8, 0, 4, 4);
    ctx.fillStyle = '#ff0000';
    ctx.fillRect(8, 0, 4, 4);
    ctx.fillStyle = '#ff0000';
    ctx.fillRect(12, 0, 4, 4);
    ctx.fillRect(12, 0, 4, 4);
    pixel('alpha.style-half.transparent', 1, 1);
    pixel('alpha.global-half.transparent', 5, 1);
    pixel('alpha.global-half.over-blue', 9, 1);
    pixel('alpha.overlap.transparent', 13, 1);

    reset();
    ctx.fillStyle = '#123456';
    ctx.translate(10, 6);
    ctx.fillRect(1, 2, 5, 4);
    ctx.scale(2, 1);
    ctx.fillRect(10, 2, 3, 4);
    pixel('transform.translate.interior', 12, 9);
    pixel('transform.translate.outside', 8, 9);
    pixel('transform.scale.interior', 31, 9);
    pixel('transform.scale.outside', 38, 9);

    reset();
    ctx.fillStyle = '#abcdef';
    ctx.setTransform(1, 0, 0, 1, 20, 20);
    ctx.rotate(Math.PI / 2);
    ctx.fillRect(0, 0, 8, 4);
    pixel('transform.rotate.center', 17, 21);
    pixel('transform.rotate.old-origin', 2, 2);

    reset();
    ctx.fillStyle = '#ff8000';
    ctx.beginPath();
    ctx.moveTo(2, 2);
    ctx.lineTo(12, 2);
    ctx.lineTo(2, 12);
    ctx.closePath();
    ctx.fill();
    pixel('path.triangle.interior', 4, 4);
    pixel('path.triangle.outside', 10, 10);
    ctx.strokeStyle = '#0080ff';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(20, 5);
    ctx.lineTo(32, 5);
    ctx.stroke();
    pixel('path.stroke.center', 26, 5);
    pixel('path.stroke.edge-top', 26, 4);
    pixel('path.stroke.outside', 26, 7);
    ctx.fillStyle = '#00ff80';
    ctx.beginPath();
    ctx.arc(50, 20, 10, 0, Math.PI * 2);
    ctx.fill();
    pixel('path.arc.center', 50, 20);
    pixel('path.arc.interior-cardinal', 58, 20);
    pixel('path.arc.outside', 50, 5);

    reset();
    ctx.beginPath();
    ctx.rect(4, 4, 20, 20);
    ctx.clip();
    ctx.fillStyle = '#ff00ff';
    ctx.fillRect(0, 0, 40, 40);
    pixel('clip.rect.inside', 10, 10);
    pixel('clip.rect.edge-inside', 4, 10);
    pixel('clip.rect.outside', 3, 10);

    reset();
    ctx.fillStyle = '#ff0000';
    ctx.globalAlpha = 1;
    ctx.save();
    ctx.fillStyle = '#00ff00';
    ctx.globalAlpha = 0.5;
    ctx.translate(40, 0);
    ctx.fillRect(0, 0, 4, 4);
    ctx.restore();
    ctx.fillRect(4, 0, 4, 4);
    pixel('save.restore.inside', 41, 1);
    pixel('save.restore.after', 5, 1);
    pixel('save.restore.transform-after', 41, 6);

    reset();
    ctx.fillStyle = '#ffff00';
    ctx.fillRect(2, 2, 20, 20);
    ctx.clearRect(5, 5, 8, 8);
    pixel('clear.interior', 6, 6);
    pixel('clear.edge-inside', 5, 6);
    pixel('clear.outside', 4, 6);

    reset();
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(70, 70, 2, 2);
    rect('pending.region', 70, 70, 2, 2);
    return out;
  });
  console.log(JSON.stringify(result, null, 2));
  await browser.close();
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
