import { chromium } from '../e2e/node_modules/playwright/index.mjs';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const destination = new URL('../website/social/', import.meta.url);
await mkdir(destination, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1200, height: 630 }, deviceScaleFactor: 1 });
  await page.goto(new URL('./website-og.html', import.meta.url).href);
  await page.evaluate(() => document.fonts.ready);
  await page.locator('img').evaluateAll(images => Promise.all(images.map(image => image.decode())));
  await page.screenshot({ path: fileURLToPath(new URL('og.png', destination)) });
  await writeFile(new URL('og.png.json', destination), JSON.stringify({
    source: 'scripts/website-og.html',
    prompt: 'Origin: native 1200x630 browser render using the existing CodeXArtifacts mark, local font and actual library screenshot with demonstration content. Rendered by scripts/render-website-og.mjs; no reconstructed product interface.',
  }, null, 2) + '\n');
  console.log('Wrote website/social/og.png (1200×630)');
} finally {
  await browser.close();
}
