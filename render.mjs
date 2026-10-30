// node render.mjs page.html            -> page.png (full page, 1400px) + page.pdf
// node render.mjs page.html --og o.jpg -> 1400x735 (1.91:1) OGP jpg of the top of the page (nav hidden)
import { createRequire } from 'module';
import path from 'path';
const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PW_PATH || '/home/box/tools/pw/node_modules/playwright');
const html = path.resolve(process.argv[2]);
const ogi = process.argv.indexOf('--og');
const b = await chromium.launch();
async function open(w, h) {
  const p = await b.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 1 });
  await p.goto('file://' + html, { waitUntil: 'networkidle' }).catch(() => {});
  await p.evaluate(() => document.fonts.ready);
  await p.waitForTimeout(500);
  return p;
}
if (ogi > 0) {
  const p = await open(1400, 1000);
  await p.addStyleTag({ content: '.sitenav{display:none}' });
  await p.screenshot({ path: path.resolve(process.argv[ogi + 1]), type: 'jpeg', quality: 82, clip: { x: 0, y: 0, width: 1400, height: 735 } });
  console.log('og image', process.argv[ogi + 1]);
} else {
  const base = html.replace(/\.html$/, '');
  const p = await open(1400, 1000);
  await p.screenshot({ path: base + '.png', fullPage: true });
  const h = await p.evaluate(() => document.documentElement.scrollHeight);
  await p.emulateMedia({ media: 'screen' });
  await p.pdf({ path: base + '.pdf', width: '1400px', height: (h + 2) + 'px', printBackground: true, pageRanges: '1' });
  console.log('rendered', base + '.png/.pdf', 'height', h);
}
await b.close();
