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
