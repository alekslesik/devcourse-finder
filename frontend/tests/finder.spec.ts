import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
test('search, empty results, reset and API error preserve the form',async({page})=>{
 await page.goto('/');await expect(page.getByRole('button',{name:'Найти обучение'})).toBeVisible();
 await page.getByRole('radio',{name:'Go',exact:true}).check();await page.getByLabel('Ваша цель').selectOption('try');await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page).toHaveURL(/\/courses\?.*language=go/);await expect(page.locator('.card')).toHaveCount(5);
 await page.getByLabel('От, ₽', {exact:true}).fill('1');await page.getByLabel('До, ₽',{exact:true}).fill('1');await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page.getByText('Подходящая программа ещё не нашлась')).toBeVisible();await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('1');
 await page.getByRole('button',{name:'Показать все программы'}).click();await expect(page.locator('.card')).toHaveCount(12);
 await page.route('**/api/v1/courses*',route=>route.fulfill({status:503,json:{error:'unavailable'}}));await page.getByRole('radio',{name:'Python',exact:true}).check();await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page.locator('main').getByRole('alert')).toContainText('Каталог временно недоступен');await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
});
test('three tariffs, fourth rejected, comparison URL in a fresh context and outbound',async({page,browser})=>{
 await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(12);
 const checks=page.getByLabel('Сравнить тариф');for(let i=0;i<3;i++)await checks.nth(i).check();await checks.nth(3).click();
 await expect(page.getByRole('status')).toContainText('уже 3 тарифа');await expect(checks.nth(3)).not.toBeChecked();await page.getByRole('button',{name:'Сравнить →'}).click();
 await expect(page).toHaveURL(/\/compare\?offers=/);await expect(page.locator('thead th')).toHaveCount(4);
 const context=await browser.newContext();const fresh=await context.newPage();await fresh.goto(page.url());await expect(fresh.locator('thead th')).toHaveCount(4);
 await context.route('https://example.com/**',route=>route.fulfill({body:'Provider website'}));const popupPromise=fresh.waitForEvent('popup');await fresh.getByRole('link',{name:'Проверить условия'}).first().click();const popup=await popupPromise;await expect(popup).toHaveURL('https://example.com/course');await context.close();
});
test('course permanent URL and unknown comparison item',async({page})=>{
 await page.goto('/courses/demo-go-1');await expect(page.getByRole('heading',{level:1})).toContainText('Старт в Go');await page.getByRole('button',{name:'Добавить в сравнение'}).click();await expect(page).toHaveURL(/\/compare\?offers=/);
 await page.goto('/compare?offers=removed');await expect(page.getByRole('columnheader',{name:'Предложение недоступно'})).toBeVisible();await page.getByRole('link',{name:'Удалить'}).click();await expect(page.getByText('Выберите от одного до трёх тарифов')).toBeVisible();
});
test('360px, keyboard controls and accessibility',async({page})=>{
 await page.setViewportSize({width:360,height:800});
 for(const path of ['/courses','/courses/demo-go-1','/compare?offers=demo-go-1-standard','/about']){
  await page.goto(path);if(path==='/courses')await expect(page.locator('.card')).toHaveCount(12);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  const results=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();expect(results.violations.filter(v=>v.impact==='critical'||v.impact==='serious').map(v=>({id:v.id,nodes:v.nodes.map(n=>n.target)}))).toEqual([]);
 }
 await page.goto('/courses');await page.getByRole('radio',{name:'Go',exact:true}).focus();await page.keyboard.press('Space');await expect(page.getByRole('radio',{name:'Go',exact:true})).toBeChecked();
 await page.getByRole('button',{name:'Найти обучение'}).focus();await page.keyboard.press('Enter');await expect(page).toHaveURL(/language=go/);
});
