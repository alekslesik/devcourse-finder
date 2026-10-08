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
  await expect(page.locator('main').getByRole('button',{name:'Сбросить фильтры',exact:true})).toHaveCount(0);
});

test('theme persists across routes and follows system changes with blocked storage', async ({page}) => {
  await page.emulateMedia({colorScheme:'dark'});
  await page.goto('/courses');
  await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
  await page.getByRole('radio',{name:'Светлая тема',exact:true}).check();
  await page.goto('/about');
  await expect(page.locator('html')).toHaveAttribute('data-theme','light');
  await expect(page.getByRole('radio',{name:'Светлая тема',exact:true})).toBeChecked();
  await page.getByRole('radio',{name:'Системная тема',exact:true}).focus();
  await page.keyboard.press('Space');
  await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
  await page.keyboard.press('ArrowLeft');
  await expect(page.getByRole('radio',{name:'Светлая тема',exact:true})).toBeChecked();
  await page.addInitScript(()=>{Object.defineProperty(window,'localStorage',{get(){throw new Error('Storage blocked')}})});
  await page.goto('/compare?offers=demo-go-1-standard');
  await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
  await page.emulateMedia({colorScheme:'light'});
  await expect(page.locator('html')).toHaveAttribute('data-theme','light');
  await page.getByRole('radio',{name:'Тёмная тема',exact:true}).check();
  await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
});

test('sorting preserves drafts, budgets validate, and chips/history restore applied filters', async ({page}) => {
  await page.goto('/courses');
  await expect(page.locator('.card')).toHaveCount(12);
  await page.getByRole('radio',{name:'Python',exact:true}).check();
  await page.getByLabel('До, ₽',{exact:true}).fill('1000');
  await page.getByLabel('Сортировка',{exact:true}).selectOption('price_asc');
  await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
  await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('1000');
  expect(new URL(page.url()).searchParams.has('language')).toBe(false);
  await page.getByLabel('От, ₽',{exact:true}).fill('2000');
  await page.getByRole('button',{name:'Найти обучение'}).click();
  expect(new URL(page.url()).searchParams.has('min')).toBe(false);
  await expect(page.getByLabel('До, ₽',{exact:true})).toBeFocused();
  await page.getByLabel('От, ₽',{exact:true}).fill('0');
  await page.getByRole('button',{name:'Найти обучение'}).click();
  await expect(page.getByRole('button',{name:'Убрать фильтр: Язык'})).toBeVisible();
  await page.getByRole('button',{name:'Убрать фильтр: Язык'}).click();
  await expect(page.getByRole('radio',{name:'Все языки',exact:true})).toBeChecked();
  await page.goBack();
  await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
  await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('1000');
});

test('clearing comparison selection stays cleared after reload',async({page})=>{
  await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(12);
  await page.getByLabel('Сравнить тариф').first().check();
  await page.getByRole('button',{name:'Очистить',exact:true}).click();
  await page.reload();await expect(page.locator('.card')).toHaveCount(12);
  await expect(page.getByLabel('Сравнить тариф').first()).not.toBeChecked();
});

test('saved theme is applied before React loads, without hydration errors',async({page})=>{
 const errors:string[]=[];page.on('console',message=>{if(message.type()==='error')errors.push(message.text())});
 await page.emulateMedia({colorScheme:'light'});
 await page.addInitScript(()=>localStorage.setItem('devcourse-theme','dark'));
 await page.route('**/_next/static/**/*.js',route=>route.abort());
 await page.goto('/about');
 await expect(page.locator('html')).toHaveAttribute('data-theme','dark');
 await page.unroute('**/_next/static/**/*.js');
 await page.reload();await expect(page.getByRole('radio',{name:'Тёмная тема',exact:true})).toBeChecked();
 expect(errors.filter(message=>/hydration|didn't match|did not match/i.test(message))).toEqual([]);
 await page.keyboard.press('Tab');
 await expect(page.getByRole('link',{name:'Перейти к содержанию'})).toBeFocused();
 expect(await page.getByRole('link',{name:'Перейти к содержанию'}).evaluate(node=>getComputedStyle(node).outlineStyle)).toBe('solid');
});
