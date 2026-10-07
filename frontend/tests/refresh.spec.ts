import {test, expect} from '@playwright/test';

test('search survives unavailable randomUUID and synchronous analytics failures', async ({page}) => {
  await page.addInitScript(() => {
    Object.defineProperty(window.crypto, 'randomUUID', {value: undefined});
    const nativeFetch = window.fetch.bind(window);
    window.fetch = (input, init) => {
      if (String(input).includes('/api/v1/events')) throw new Error('Analytics unavailable');
      return nativeFetch(input, init);
    };
  });
  await page.goto('/courses');
  await expect(page.locator('.card')).toHaveCount(12);
  await page.getByRole('radio', {name: 'Go', exact: true}).check();
  await page.getByRole('button', {name: 'Найти обучение'}).click();
  await expect(page.locator('.card')).toHaveCount(5);
  await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
});

test('empty catalog, no matches, and malformed service responses are distinct', async ({page}) => {
  await page.route('**/api/v1/courses*', route => route.fulfill({json: {items: [], total: 0, page: 1, page_size: 12}}));
  await page.goto('/courses');
  await expect(page.getByText('Программы пока не опубликованы')).toBeVisible();
  await expect(page.getByRole('button', {name: 'Показать все программы'})).toHaveCount(0);
  await page.getByRole('radio', {name: 'Go', exact: true}).check();
  await page.getByRole('button', {name: 'Найти обучение'}).click();
  await expect(page.getByText('Подходящая программа ещё не нашлась')).toBeVisible();
  await page.unroute('**/api/v1/courses*');
  await page.route('**/api/v1/courses*', route => route.fulfill({body: 'not-json'}));
  await page.getByRole('button', {name: 'Показать все программы'}).click();
  await expect(page.locator('main').getByRole('alert')).toContainText('Каталог временно недоступен');
  await expect(page.locator('main')).not.toContainText('Unexpected token');
});
