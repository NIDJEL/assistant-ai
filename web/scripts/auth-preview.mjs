import { chromium } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
const browser = await chromium.launch();
const directory = '../artifacts/auth-premium';
await mkdir(directory, { recursive: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 });
page.on('pageerror', error => console.error(error));
await page.route('**/api/auth/me', route => route.fulfill({ status: 401, json: { error: { message: 'Войдите' } } }));
await page.goto(`${process.env.P1_BASE_URL || 'http://127.0.0.1:5173'}/auth`);
await page.getByRole('heading', { name: 'Добро пожаловать! Я Гена' }).waitFor();
await page.evaluate(async () => {
  await Promise.allSettled(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished));
  await Promise.all([...document.images].map(image => image.decode().catch(() => {})));
});
await page.screenshot({ path: `${directory}/desktop.png`, fullPage: true, animations: 'disabled' });
console.log('desktop', await page.locator('.auth-card').boundingBox());
if (process.env.AUTH_PROFILE) {
  const profile = page.evaluate(async () => {
    const longTasks = [];
    const observer = new PerformanceObserver(list => longTasks.push(...list.getEntries().map(e => e.duration)));
    observer.observe({ type: 'longtask' });
    const intervals = [];
    let previous = 0;
    await new Promise(resolve => {
      function sample(time) {
        if (previous) intervals.push(time - previous);
        previous = time;
        if (intervals.length < 180) requestAnimationFrame(sample); else resolve();
      }
      requestAnimationFrame(sample);
    });
    observer.disconnect();
    const sorted = intervals.toSorted((a, b) => a - b);
    return { samples: intervals.length, medianFrameMs: sorted[Math.floor(sorted.length / 2)], p95FrameMs: sorted[Math.floor(sorted.length * .95)], longTasksOver50Ms: longTasks, viewport: { width: innerWidth, height: innerHeight } };
  });
  await page.mouse.move(100, 600);
  await page.mouse.move(800, 700, { steps: 60 });
  await page.mouse.move(500, 400, { steps: 60 });
  const result = await profile;
  await writeFile(`${directory}/performance.json`, JSON.stringify(result, null, 2));
  console.log('profile', result);
}
for (const [name, width, height] of [['laptop', 1366, 768], ['tablet', 1024, 768]]) {
  await page.setViewportSize({ width, height });
  await page.screenshot({ path: `${directory}/${name}.png`, fullPage: true, animations: 'disabled' });
}
await page.setViewportSize({ width: 390, height: 844 });
await page.screenshot({ path: `${directory}/mobile.png`, fullPage: true, animations: 'disabled' });
await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished)); });
await page.screenshot({ path: `${directory}/mobile-register.png`, fullPage: true, animations: 'disabled' });
console.log('mobile', await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth })));
await browser.close();
