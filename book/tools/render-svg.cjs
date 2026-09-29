// Рендер SVG-схемы в PNG (светлая и тёмная тема) для визуальной проверки.
//   NODE_PATH=$(npm root -g) node book/tools/render-svg.cjs in.svg out-prefix
// → out-prefix-light.png, out-prefix-dark.png  (ширина колонки как в книге: 688px)
const { chromium } = require("playwright");
const fs = require("fs");
(async () => {
  const [inp, out] = process.argv.slice(2);
  const svg = fs.readFileSync(inp, "utf8").replace(/<\?xml[^>]*>/, "");
  const b = await chromium.launch();
  for (const [name, bg, fg] of [["light", "#f6f6f7", "#3c3c43"], ["dark", "#202127", "#dfdfd6"]]) {
    const p = await b.newPage({ viewport: { width: 740, height: 400 }, deviceScaleFactor: 2 });
    await p.setContent(`<html><body style="margin:0;background:${bg};color:${fg};font-family:Inter,system-ui,sans-serif">
      <figure style="margin:0;padding:18px 16px 12px;width:688px;box-sizing:border-box">${svg}</figure>
      <style>svg{display:block;width:100%;height:auto;overflow:visible}</style></body></html>`);
    const el = await p.$("figure");
    await el.screenshot({ path: `${out}-${name}.png` });
    await p.close();
  }
  await b.close();
})();
