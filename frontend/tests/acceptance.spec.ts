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

test('AC-09: copied search URL restores filters, sort and page in an independent browser context',async({page,browser})=>{
 await page.addInitScript(()=>{
  Object.defineProperty(navigator,'clipboard',{value:{writeText:async(value:string)=>{sessionStorage.setItem('copied-search',value)}}});
 });
 await page.goto('/courses?language=go&sort=price_asc&page=2');
 await expect(page.getByRole('region',{name:'Результаты поиска'})).toHaveAttribute('aria-busy','false');
 await page.getByRole('button',{name:'Скопировать ссылку',exact:true}).click();
 await expect(page.getByRole('status')).toHaveText('Ссылка скопирована');
 const copied=await page.evaluate(()=>sessionStorage.getItem('copied-search'));
 expect(copied).toBe(page.url());
 const fresh=await browser.newContext();
 try{
  const shared=await fresh.newPage();
  const received=shared.waitForResponse(response=>new URL(response.url()).pathname==='/api/v1/courses');
  await shared.goto(copied!);
  const response=await received;
  const query=new URL(response.url()).searchParams;
  expect(Object.fromEntries(query)).toMatchObject({language:'go',sort:'price_asc',page:'2'});
  const body=await response.json();
  await expect(shared.getByRole('region',{name:'Результаты поиска'})).toHaveAttribute('aria-busy','false');
  await expect(shared.getByRole('radio',{name:'Go',exact:true})).toBeChecked();
  await expect(shared.getByLabel('Сортировка',{exact:true})).toHaveValue('price_asc');
  await expect(shared.locator('.card')).toHaveCount(body.items.length);
  expect(await shared.locator('.card .titleButton').evaluateAll(nodes=>nodes.map(node=>node.getAttribute('href')))).toEqual(body.items.map((item:{course:{slug:string}})=>'/courses/'+item.course.slug));
 }finally{await fresh.close()}
});

test('AC-09: clipboard refusal offers the current URL for manual copying',async({page})=>{
 await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:{writeText:async()=>{throw Error('Denied')}}}));
 await page.goto('/courses?language=python&sort=duration&page=2');
 await page.getByRole('button',{name:'Скопировать ссылку',exact:true}).click();
 await expect(page.getByLabel('Ссылка на поиск',{exact:true})).toHaveValue(page.url());
 await expect(page.getByLabel('Ссылка на поиск',{exact:true})).toBeFocused();
});
