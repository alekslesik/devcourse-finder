import {readFileSync} from 'node:fs';
import {test,expect} from '@playwright/test';

test('hero and form edit a draft; removing a chip preserves unrelated edits',async({page})=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses?language=go&goal=try');
 await expect(page.locator('.card')).toHaveCount(1);
 await page.getByLabel('До, ₽',{exact:true}).fill('30000');
 await page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true}).click();
 await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
 expect(new URL(page.url()).searchParams.get('language')).toBe('go');

 await page.getByRole('button',{name:'Убрать фильтр: Цель'}).click();
 await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('30000');
 await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
 await expect(page.getByLabel('Ваша цель')).toHaveValue('');
 await page.getByLabel('Сортировка',{exact:true}).selectOption('price_asc');
 await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('30000');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page.locator('.resultHeading')).toBeFocused();
 expect(new URL(page.url()).searchParams.get('language')).toBe('python');
 expect(new URL(page.url()).searchParams.get('max')).toBe('3000000');

 await page.goBack();
 await expect(page.getByRole('radio',{name:'Go',exact:true})).toBeChecked();
 await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('');
});

test('submitted results receive focus, passive updates announce without moving it',async({page})=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses');
 await expect(page.locator('.srOnly[role=status]')).toContainText('20 программ');
 await page.getByRole('radio',{name:'Go',exact:true}).check();
 await page.getByRole('button',{name:'Найти обучение'}).focus();
 await page.keyboard.press('Enter');
 await expect(page.locator('.card')).toHaveCount(5);
 await expect(page.locator('.resultHeading')).toBeFocused();
 await expect(page.locator('.srOnly[role=status]')).toBeEmpty();
 expect(await page.locator('.resultHeading').evaluate(node=>node.getBoundingClientRect().top)).toBeGreaterThanOrEqual(0);
 await page.getByLabel('Сортировка',{exact:true}).focus();
 await page.getByLabel('Сортировка',{exact:true}).selectOption('price_asc');
 await expect(page.locator('.srOnly[role=status]')).toContainText('5 программ');
 await expect(page.getByLabel('Сортировка',{exact:true})).toBeFocused();
});

test('return from course and comparison restores the exact applied catalog and position',async({page})=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses?sort=price_asc&page=2');
 await expect(page.locator('.card')).toHaveCount(8);
 await page.locator('.card').first().scrollIntoViewIfNeeded();
 const query=new URL(page.url()).search;
 const course=page.locator('.card .titleButton').first();
 await course.click();
 await expect(page.getByRole('navigation',{name:'Основная навигация'}).getByRole('link',{name:'Каталог',exact:true})).toHaveAttribute('href','/courses'+query);
 const snapshot=await page.evaluate(()=>JSON.parse(sessionStorage.getItem('devcourse-catalog-context')!));
 await page.getByRole('navigation',{name:'Основная навигация'}).getByRole('link',{name:'Каталог',exact:true}).click();
 await expect(page.locator('.card')).toHaveCount(8);
 expect(new URL(page.url()).search).toBe(query);
 await expect.poll(()=>page.evaluate(()=>scrollY)).toBeCloseTo(snapshot.scrollY,0);
 await page.getByLabel('Сравнить тариф').first().check();
 await page.getByRole('button',{name:'Сравнить →'}).click();
 await page.getByRole('link',{name:'← Вернуться к подбору'}).click();
 await expect(page.locator('.card')).toHaveCount(8);
 expect(new URL(page.url()).search).toBe(query);
 await page.getByRole('button',{name:'Сравнить →'}).click();
 await page.getByRole('link',{name:'Удалить',exact:true}).click();
 await expect(page.getByText('Выберите от одного до трёх тарифов в каталоге.')).toBeVisible();
 await expect(page.getByRole('link',{name:'← Вернуться к подбору'})).toHaveAttribute('href','/courses'+query);
 await page.getByRole('link',{name:'← Вернуться к подбору'}).click();
 await expect(page.locator('.card')).toHaveCount(8);
 expect(new URL(page.url()).search).toBe(query);
 await page.reload();
 await expect(page.getByLabel('Сортировка',{exact:true})).toHaveValue('price_asc');
});

test('direct entry and untrusted catalog context use the safe fallback',async({page})=>{
 await page.addInitScript(()=>sessionStorage.setItem('devcourse-catalog-context',JSON.stringify({url:'https://example.com/courses',scrollY:20})));
 await page.goto('/courses/demo-go-1');
 await expect(page.locator('main').getByRole('link',{name:'← Каталог',exact:true})).toHaveCount(0);
 await expect(page.getByRole('navigation',{name:'Основная навигация'}).getByRole('link',{name:'Каталог',exact:true})).toHaveAttribute('href','/courses');
 await page.addInitScript(()=>{Object.defineProperty(window,'sessionStorage',{get(){throw new Error('blocked')}})});
 await page.goto('/compare?offers=demo-go-1-standard');
 await expect(page.getByRole('link',{name:'← Каталог',exact:true})).toHaveAttribute('href','/courses');
});

test('mobile comparison keeps both axes of context while scrolling',async({page})=>{
 await page.setViewportSize({width:390,height:900});
 await page.goto('/compare?offers=demo-go-1-standard,closed-tariff,removed');
 const region=page.getByRole('region',{name:'Сравнение тарифов, горизонтальная прокрутка'});
 await region.scrollIntoViewIfNeeded();
 await region.evaluate(node=>{node.scrollLeft=220;node.scrollTop=100;});
 const geometry=await region.evaluate(node=>{
  const wrap=node.getBoundingClientRect(),row=node.querySelector('tbody th')!.getBoundingClientRect(),header=node.querySelector('thead th:nth-child(2)')!.getBoundingClientRect();
  return {rowLeft:row.left-wrap.left,headerTop:header.top-wrap.top,scroll:node.scrollLeft};
 });
 expect(geometry.scroll).toBeGreaterThan(0);
 expect(geometry.rowLeft).toBeLessThan(3);
 expect(geometry.headerTop).toBeLessThan(3);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

for(const width of [390,1440])test(`fixed demo baseline stays within the layout budget at ${width}px`,async({page})=>{
 // This budget compares the exact legacy-response fixture used by the audit.
 // Spacious layout allows up to two mobile viewports before the first card.
 // Full-service diagnostics add explanatory content and are covered separately.
 const courses=JSON.parse(readFileSync('../data/demo-catalog.json','utf8')).courses;
 await page.route('**/api/v1/courses*',route=>route.fulfill({json:{items:courses.slice(0,12).map((course:any)=>({course,offer:course.offers[0],effective_price:course.offers[0].price,reasons:[],stale:false})),total:courses.length,page:1,page_size:12}}));
 await page.setViewportSize({width,height:900});
 await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(12);
 await page.evaluate(()=>document.fonts.ready);
 const first=await page.locator('.card').first().evaluate(node=>node.getBoundingClientRect().top+scrollY);
 await test.info().attach('catalog-layout.json',{body:JSON.stringify({width,height:900,firstCardY:first}),contentType:'application/json'});
 console.log('Catalog layout:',JSON.stringify({width,firstCardY:first}));
 expect(first).toBeLessThan(width===390?1800:1400);
 const scene=(await page.locator('.languageScene').boundingBox())!;
 const filters=(await page.locator('#catalog-filters').boundingBox())!;
 if(width===390){
  expect(Math.abs(scene.x-filters.x)).toBeLessThan(1);
  expect(Math.abs(scene.width-filters.width)).toBeLessThan(1);
 }else{
  const copy=(await page.locator('.heroCopy').boundingBox())!;
  expect(scene.x-copy.x-copy.width).toBeGreaterThanOrEqual(48);
  expect((await page.getByRole('button',{name:'Найти обучение'}).boundingBox())!.width).toBeLessThanOrEqual(240);
 }
 expect(filters.y-scene.y-scene.height).toBeGreaterThanOrEqual(24);
 const nav=(await page.locator('header .siteNav').boundingBox())!;
 const themeControl=(await page.locator('header .themeControl').boundingBox())!;
 expect(nav.x+nav.width+8).toBeLessThanOrEqual(themeControl.x);
 if(width===390){
  expect(await page.getByLabel('До, ₽',{exact:true}).evaluate(node=>parseFloat(getComputedStyle(node).fontSize))).toBeGreaterThanOrEqual(16);
  const theme=page.locator('.themeChoice').first();expect((await theme.boundingBox())!.width).toBeGreaterThanOrEqual(44);
 }
});


test('a later passive action cancels focus transfer from an in-flight submit',async({page})=>{
 let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve;});
 await page.route('**/api/v1/courses*',async route=>{
  const p=new URL(route.request().url()).searchParams;
  if(p.get('language')==='go'&&!p.has('sort')){await gate;try{await route.fulfill({json:{items:[],total:0,page:1,page_size:12}});}catch{}}
  else await route.continue();
 });
 try {
  await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(12);
  await page.getByRole('radio',{name:'Go',exact:true}).check();
  await page.getByRole('button',{name:'Найти обучение'}).click();
  await expect(page.locator('.results')).toHaveAttribute('aria-busy','true');
  const sort=page.getByLabel('Сортировка',{exact:true});await sort.focus();await sort.selectOption('price_asc');
  await expect(page.locator('.card')).toHaveCount(5);
  await expect(sort).toBeFocused();
  await expect(page.locator('.srOnly[role=status]')).toContainText('5 программ');
 } finally {release();}
});
