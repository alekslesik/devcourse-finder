import {test,expect} from '@playwright/test';

for(const width of [390,1440])test(`hero language shortcuts lead to route filters at ${width}px and preserve search criteria`,async({page})=>{
 await page.setViewportSize({width,height:844});
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses?goal=try&sort=price_asc&page=2');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await expect(page.getByRole('complementary',{name:'Ваш маршрут'})).toBeVisible();
 await expect(page.getByRole('button',{name:/^(Скрыть фильтры|Фильтры)/})).toHaveCount(0);
 await expect(page.getByRole('link',{name:'Посмотреть программы'})).toHaveCount(0);
 await page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true}).click();
 expect(new URL(page.url()).searchParams.has('language')).toBe(false);
 await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
 await expect.poll(()=>page.locator('#catalog-filters').evaluate(node=>Math.round(node.getBoundingClientRect().top))).toBe(24);
 await expect(page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true})).toHaveAttribute('aria-pressed','true');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page).toHaveURL(/language=python/);
 expect(new URL(page.url()).searchParams.get('goal')).toBe('try');
 expect(new URL(page.url()).searchParams.get('sort')).toBe('price_asc');
 expect(new URL(page.url()).searchParams.has('page')).toBe(false);
 await page.goBack();await expect(page.getByRole('radio',{name:'Все языки',exact:true})).toBeChecked();
});

for(const theme of ['light','dark'] as const)test(`forced-color palettes preserve panel boundaries without contrast more: ${theme}`,async({page})=>{
 await page.setViewportSize({width:390,height:900});
 await page.emulateMedia({colorScheme:theme,forcedColors:'active',contrast:'no-preference'});
 expect(await page.evaluate(()=>matchMedia('(forced-colors: active)').matches)).toBe(true);
 expect(await page.evaluate(()=>matchMedia('(prefers-contrast: more)').matches)).toBe(false);
 const boundaries=async(selectors:string[])=>{
  for(const selector of selectors){
   const panel=page.locator(selector).first();await expect(panel).toBeVisible();
   const border=await panel.evaluate(node=>{const s=getComputedStyle(node);return {width:parseFloat(s.borderTopWidth),style:s.borderTopStyle,color:s.borderTopColor}});
   expect(border.width,selector).toBeGreaterThan(0);
   expect(border.style,selector).not.toMatch(/^(none|hidden)$/);
   expect(border.color,selector).not.toBe('rgba(0, 0, 0, 0)');
  }
 };
 await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(12);
 await boundaries(['header','.languageScene','.workspace aside','.card']);
 await page.getByLabel('Сравнить тариф').first().check();await boundaries(['.compareBar']);
 await page.goto('/courses/demo-go-1');await boundaries(['.courseSummary','.detailOffers>section']);
 await page.goto('/courses?compare=demo-go-1-standard,demo-go-2-standard');
 await expect(page.getByRole('dialog').getByRole('table')).toBeVisible();await boundaries(['.modal']);
 await page.route(url=>url.pathname==='/api/v1/courses',route=>route.fulfill({json:{items:[],total:0,page:1,page_size:12}}));
 await page.goto('/courses');await expect(page.getByText('Программы пока не опубликованы')).toBeVisible();await boundaries(['.empty']);
});
