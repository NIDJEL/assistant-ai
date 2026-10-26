import { test, expect, type Locator, type Page } from '@playwright/test';
import { mockSchedule } from './mock-workspace';
import { chooseOption } from './helpers';

async function keyboardFocus(page: Page, control: Locator) { await page.keyboard.press('Tab'); await control.focus(); }
async function ring(control: Locator) {
  await expect.poll(() => control.evaluate(node => ({ width: getComputedStyle(node).outlineWidth, offset: getComputedStyle(node).outlineOffset, style: getComputedStyle(node).outlineStyle }))).toEqual({ width: '2px', offset: '-2px', style: 'solid' });
}
async function settleMenu(menu: Locator) { await menu.evaluate(async root => { await Promise.all(root.getAnimations().map(animation => animation.finished)); }); }

test('every segmented control slides, fits phones and respects reduced motion', async ({ page }, info) => {
  await mockSchedule(page);
  for (const [route, label] of [['/calendar', 'Вид календаря'], ['/integrations', 'Фильтр интеграций'], ['/tasks', 'Фильтр задач'], ['/friends', 'Списки друзей'], ['/reminders', 'Фильтр напоминаний'], ['/archive', 'Раздел архива']]) {
    await page.goto(route);
    const segments = page.getByRole('group', { name: label, exact: true }); await expect(segments).toHaveAttribute('data-ready', 'true');
    const animation = await segments.evaluate(async root => {
      const pill = root.querySelector('.segment-indicator')!; pill.getBoundingClientRect();
      (root.querySelectorAll('button')[1] as HTMLButtonElement).click();
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      return pill.getAnimations().length > 0;
    });
    expect(animation).toBe(true);
    for (const width of [1440, 390, 320]) {
      await page.setViewportSize({ width, height: 950 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      const button = segments.locator('button').last(); await keyboardFocus(page, button); await ring(button);
    }
    if (route === '/calendar' || route === '/integrations') await page.screenshot({ path: info.outputPath(`${route.slice(1)}-mobile.png`) });
    await page.setViewportSize({ width: 1440, height: 950 });
  }
  await page.emulateMedia({ reducedMotion: 'reduce' }); await page.goto('/calendar');
  expect(await page.locator('.segment-indicator').evaluate(node => getComputedStyle(node).transitionDuration)).toBe('0s');
});

test('custom filters select with mouse and keyboard and keep menus inside the viewport', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await mockSchedule(page); await page.goto('/tasks');
  const priority = page.getByRole('combobox', { name: 'Приоритет задач' });
  await chooseOption(priority, 'Высокий'); await expect(page.locator('.task-card')).toHaveCount(1); await expect(page.locator('.task-card')).toContainText('Важная задача');
  await keyboardFocus(page, priority); await ring(priority); await page.keyboard.press('Home'); await page.keyboard.press('Enter'); await expect(page.locator('.task-card')).toHaveCount(2);
  await priority.click(); await priority.dispatchEvent('keydown', { key: 'н' }); await page.keyboard.press('Escape'); await expect(priority).toHaveText('Приоритет');
  await priority.click(); await page.locator('h1').click(); await expect(page.getByRole('listbox')).toHaveCount(0);
  await page.goto('/meetings'); await chooseOption(page.getByRole('combobox', { name: 'Период встреч' }), 'Сегодня'); await expect(page.locator('.meeting-tile')).toHaveCount(1);
  await page.goto('/archive');
  const kind = page.getByRole('combobox', { name: 'Тип в архиве' });
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 720 }); await kind.click(); const menu = page.getByRole('listbox'); await expect(menu).toBeVisible(); await settleMenu(menu);
    expect(await menu.evaluate(node => node.matches(':popover-open'))).toBe(true);
    const box = (await menu.boundingBox())!; expect(box.x).toBeGreaterThanOrEqual(11); expect(box.x + box.width).toBeLessThanOrEqual(width - 11); expect(box.y + box.height).toBeLessThanOrEqual(709);
    await page.screenshot({ path: info.outputPath(`dropdown-${width}.png`) }); await page.keyboard.press('Escape');
  }
  await chooseOption(kind, 'Напоминания'); await expect(kind).toHaveText('Напоминания');
  expect(errors).toEqual([]);
});

test('select menus work above modal forms, preserve values and leave Escape to the form after closing', async ({ page }, info) => {
  await page.addInitScript(() => {
    const errors: string[] = []; Object.assign(window, { layoutErrors: errors });
    window.addEventListener('error', event => errors.push(event.message));
  });
  await mockSchedule(page); await page.goto('/tasks'); await page.getByRole('button', { name: 'Новая задача', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Новая задача', exact: true }); const priority = dialog.getByRole('combobox', { name: 'Приоритет', exact: true });
  await dialog.getByRole('textbox', { name: 'Название', exact: true }).fill('Сохранить мой черновик');
  await priority.click(); await expect(page.getByRole('listbox')).toBeVisible();
  expect(await page.getByRole('listbox').evaluate(node => !!node.closest('dialog[open]') && node.matches(':popover-open'))).toBe(true);
  await page.keyboard.press('Escape'); await expect(page.getByRole('listbox')).toHaveCount(0); await expect(dialog).toBeVisible(); await expect(priority).toBeFocused();
  await chooseOption(priority, 'Высокий'); await expect(priority).toHaveText('Высокий'); await expect(dialog.locator('input[name="priority"]')).toHaveValue('high');
  const date = dialog.getByRole('textbox', { name: 'Срок', exact: true }); await keyboardFocus(page, date); await ring(dialog.locator('.date-field')); expect(await date.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('none');
  await dialog.getByRole('button', { name: 'Выбрать дату', exact: true }).click();
  const calendar = page.getByRole('dialog', { name: 'Выбор даты', exact: true }); await keyboardFocus(page, calendar.locator('button[data-date][tabindex="0"]')); await ring(calendar.locator('button[data-date][tabindex="0"]'));
  await page.keyboard.press('Escape'); await expect(calendar).toHaveCount(0); await expect(dialog).toBeVisible();
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 800 }); await priority.click(); const menu = page.getByRole('listbox'); await expect(menu).toBeVisible(); await settleMenu(menu);
    const box = (await menu.boundingBox())!; expect(box.y).toBeGreaterThanOrEqual(11); expect(box.y + box.height).toBeLessThanOrEqual(789);
    await page.screenshot({ path: info.outputPath(`form-select-${width}.png`) }); await page.keyboard.press('Escape');
  }
  await expect(dialog.getByRole('textbox', { name: 'Название', exact: true })).toHaveValue('Сохранить мой черновик');
  await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0);
  expect(await page.evaluate(() => (window as unknown as { layoutErrors: string[] }).layoutErrors)).toEqual([]);
});

test('focus follows controls and composite fields, and dropdowns inherit light and dark palettes', async ({ page }, info) => {
  await mockSchedule(page); await page.goto('/archive');
  const search = page.getByRole('textbox', { name: 'Поиск в архиве' }); await keyboardFocus(page, search); await ring(page.locator('.search-box'));
  expect(await search.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('none');
  await keyboardFocus(page, page.getByRole('link', { name: 'Главная', exact: true })); await ring(page.getByRole('link', { name: 'Главная', exact: true }));
  const control = page.getByRole('combobox', { name: 'Тип в архиве' });
  const colors: string[] = [];
  for (const surface of ['workspace', 'preview']) {
    // Preview uses the shared light tokens; workspace supplies the navy tokens.
    await page.evaluate(surface => { document.documentElement.dataset.surface = surface; }, surface);
    await control.click(); const menu = page.getByRole('listbox'); await expect(menu).toBeVisible(); await settleMenu(menu);
    const style = await menu.evaluate(node => ({ background: getComputedStyle(node).backgroundColor, text: getComputedStyle(node).color }));
    colors.push(style.background); expect(style.background).not.toBe(style.text);
    await page.screenshot({ path: info.outputPath(`select-theme-${surface}.png`) }); await page.keyboard.press('Escape'); await ring(control);
  }
  expect(colors).toEqual(['rgb(20, 29, 44)', 'rgb(255, 255, 255)']);
  await page.goto('/settings'); const motion = page.getByRole('combobox', { name: 'Анимации интерфейса' });
  await chooseOption(motion, 'Без анимаций'); await expect(page.locator('html')).toHaveAttribute('data-motion', 'reduced');
  await motion.click(); expect(await page.getByRole('listbox').evaluate(node => getComputedStyle(node).animationName)).toBe('none'); await page.keyboard.press('Escape');
  const toggle = page.getByRole('switch', { name: 'Показывать панель на компьютере' }); await keyboardFocus(page, toggle); await ring(toggle);
  await page.goto('/'); const composer = page.getByRole('textbox', { name: 'Сообщение ассистенту' }); await keyboardFocus(page, composer); await ring(page.locator('.composer'));
  expect(await composer.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('none');
});

test('long calendar lists, reminder fields and widget settings all use custom selects', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await mockSchedule(page); await page.goto('/integrations?service=google');
  await page.setViewportSize({ width: 390, height: 844 });
  const integration = page.getByRole('dialog', { name: 'Интеграция', exact: true }); const calendar = integration.getByRole('combobox', { name: 'Календарь Google' });
  await expect(calendar).toContainText('Календарь 1:'); await calendar.click(); await expect(page.getByRole('option')).toHaveCount(46);
  await page.keyboard.press('End'); await expect.poll(() => page.getByRole('listbox').evaluate(node => node.scrollTop)).toBeGreaterThan(0);
  await settleMenu(page.getByRole('listbox')); await page.screenshot({ path: info.outputPath('long-calendar-list.png') });
  await page.keyboard.press('Enter'); await expect(calendar).toContainText('Календарь 45:'); await expect(integration.getByRole('button', { name: 'Сохранить и синхронизировать' })).toBeEnabled();
  await page.keyboard.press('Escape'); await expect(integration).toHaveCount(0);
  await page.goto('/reminders'); await page.locator('.reminder-title').first().click();
  const editor = page.getByRole('form', { name: /Редактор:/ }); await chooseOption(editor.getByRole('combobox', { name: 'Повтор напоминания' }), 'Каждый месяц');
  await chooseOption(editor.getByRole('combobox', { name: 'Когда напомнить' }), 'За 1 ч.'); await expect(editor.getByRole('spinbutton', { name: 'За сколько минут' })).toHaveValue('60');
  await expect(editor.getByRole('combobox', { name: 'Звук напоминания' })).toBeDisabled();
  await page.goto('/meetings'); await page.getByRole('button', { name: 'Открыть виджеты', exact: true }).click();
  const drawer = page.getByRole('dialog', { name: 'Виджеты', exact: true });
  await drawer.getByRole('button', { name: 'Настройки: Сегодня', exact: true }).click();
  const settings = page.getByRole('dialog', { name: 'Настройки: Сегодня', exact: true }); await chooseOption(settings.getByRole('combobox', { name: 'Количество записей' }), '5');
  await settings.getByRole('button', { name: 'Готово', exact: true }).click();
  await chooseOption(drawer.getByRole('combobox', { name: 'Длительность быстрого планирования' }), '1 ч');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(await page.locator('select').count()).toBe(0); expect(errors).toEqual([]);
});

test('sign in fields, password controls and links use the same contained focus ring', async ({ page }, info) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 401, json: { error: { code: 'unauthorized', message: 'Войдите' } } }));
  await page.goto('/auth');
  const email = page.getByRole('textbox', { name: /Почта|Email/i }); await keyboardFocus(page, email); await ring(page.locator('.auth-input').filter({ has: email }));
  expect(await email.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('none');
  const password = page.getByRole('button', { name: 'Показать пароль', exact: true }); await keyboardFocus(page, password); await ring(password);
  for (const checkbox of await page.getByRole('checkbox').all()) await checkbox.check();
  await keyboardFocus(page, page.getByRole('button', { name: 'Войти', exact: true })); await ring(page.getByRole('button', { name: 'Войти', exact: true }));
  await page.screenshot({ path: info.outputPath('sign-in-focus.png') });
});

test.describe('touch controls', () => {
  test.use({ hasTouch: true, isMobile: true, viewport: { width: 390, height: 844 } });
  test('selects with a finger and dismisses without opening the system picker', async ({ page }) => {
    await mockSchedule(page); await page.goto('/tasks'); const control = page.getByRole('combobox', { name: 'Приоритет задач' });
    await control.tap(); await page.getByRole('option', { name: 'Высокий', exact: true }).tap(); await expect(control).toHaveText('Высокий');
    await expect(page.locator('.task-card')).toHaveCount(1);
    await control.tap(); await page.locator('h1').tap(); await expect(page.getByRole('listbox')).toHaveCount(0);
    await page.getByRole('button', { name: 'Новая задача', exact: true }).tap(); const dialog = page.getByRole('dialog', { name: 'Новая задача', exact: true });
    await dialog.getByRole('combobox', { name: 'Приоритет', exact: true }).tap(); await page.getByRole('option', { name: 'Низкий', exact: true }).tap();
    await expect(dialog.getByRole('combobox', { name: 'Приоритет', exact: true })).toHaveText('Низкий');
  });
});
