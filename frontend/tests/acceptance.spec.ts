import {test,expect} from '@playwright/test';

test('AC-01: anonymous home shows the search form even while the catalog is pending',async({page})=>{
 let release!:()=>void;
 const gate=new Promise<void>(resolve=>{release=resolve});
 await page.route('**/api/v1/courses*',async route=>{await gate;await route.fulfill({status:503,json:{error:'unavailable'}})});
 try{
  await page.goto('/');
  await expect(page.getByRole('button',{name:'Найти обучение'})).toBeVisible();
  await expect(page.getByRole('radio',{name:'Go',exact:true})).toBeEnabled();
  await expect(page.getByLabel('Ваша цель')).toBeEnabled();
  await expect(page.locator('input[type=email],input[type=tel],input[type=password]')).toHaveCount(0);
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('input[required]')).toHaveCount(0);
 }finally{release()}
 await expect(page.locator('main').getByRole('alert')).toContainText('Каталог временно недоступен');
 await expect(page.getByRole('button',{name:'Найти обучение'})).toBeEnabled();
});
