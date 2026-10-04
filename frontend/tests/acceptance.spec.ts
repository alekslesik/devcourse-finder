import {test,expect,type Route} from '@playwright/test';

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

test('AC-08: empty results and API failure remain distinct without relaxing any filters',async({page})=>{
 await page.goto('/courses?language=go&experience=none&goal=try&min=100&max=100&support=review&schedule=flexible&hours=10&include_closed=true&sort=price_asc');
 await expect(page.getByText('Подходящая программа ещё не нашлась')).toBeVisible();
 await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
 const filters=()=>page.evaluate(()=>Object.fromEntries(new URLSearchParams(location.search)));
 const before=await filters();
 const checkForm=async()=>{
  await expect(page.getByRole('radio',{name:'Go',exact:true})).toBeChecked();
  await expect(page.getByLabel('Текущий опыт')).toHaveValue('none');
  await expect(page.getByLabel('Ваша цель')).toHaveValue('try');
  await expect(page.getByLabel('От, ₽',{exact:true})).toHaveValue('1');
  await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('1');
  await expect(page.getByLabel('Обратная связь')).toHaveValue('review');
  await expect(page.getByLabel('Расписание')).toHaveValue('flexible');
  await expect(page.getByLabel('Часов в неделю, не больше')).toHaveValue('10');
  await expect(page.getByLabel('Показать закрытый и неизвестный набор')).toBeChecked();
  await expect(page.getByLabel('Сортировка',{exact:true})).toHaveValue('price_asc');
  expect(await filters()).toEqual(before);
 };
 await checkForm();
 const unavailable=(route:Route)=>route.fulfill({status:503,json:{error:'unavailable'}});
 await page.route('**/api/v1/courses*',unavailable);
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page.locator('main').getByRole('alert')).toContainText('Каталог временно недоступен');
 await expect(page.getByText('Подходящая программа ещё не нашлась')).toHaveCount(0);
 await checkForm();
 await page.unroute('**/api/v1/courses*',unavailable);
 await page.getByRole('button',{name:'Повторить',exact:true}).click();
 await expect(page.getByText('Подходящая программа ещё не нашлась')).toBeVisible();
 await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
 await checkForm();
});
