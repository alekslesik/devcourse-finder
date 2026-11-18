import {test,expect,type Page} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

type Activity={id:string;kind:string;course_id?:string;language?:string;total?:number};
async function captureEvents(page:Page,fail=false){
 const events:Activity[]=[];
 await page.route('**/api/v1/events',async route=>{events.push(route.request().postDataJSON());if(fail)await route.abort();else await route.fulfill({status:204});});
 return events;
}

test('course views count rendered courses once, not tariff changes or missing pages',async({page})=>{
 const events=await captureEvents(page);
 await page.goto('/courses/demo-go-1');
 await expect(page.getByRole('heading',{level:1})).toBeVisible();
 await expect.poll(()=>events.filter(e=>e.kind==='view')).toHaveLength(1);
 expect(events[0]).toMatchObject({kind:'view',course_id:'demo-go-1',language:'go'});
 expect(Object.keys(events[0]).sort()).toEqual(['course_id','id','kind','language']);
 await page.evaluate(()=>history.replaceState(history.state,'',location.pathname+'?offer=changed'));
 expect(events.filter(e=>e.kind==='view')).toHaveLength(1);
 await page.goto('/courses/demo-python-1');
 await expect.poll(()=>events.filter(e=>e.kind==='view')).toHaveLength(2);
 await page.goto('/courses/does-not-exist');
 await expect(page.getByText('404',{exact:true})).toBeVisible();
 expect(events.filter(e=>e.kind==='view')).toHaveLength(2);
});

test('direct and legacy comparisons count available tariffs, never unavailable or empty selections',async({page})=>{
 const events=await captureEvents(page);
 await page.goto('/compare?offers=demo-go-1-standard,removed');
 await expect(page.getByRole('table')).toBeVisible();
 await expect.poll(()=>events.filter(e=>e.kind==='compare')).toHaveLength(1);
 expect(events.find(e=>e.kind==='compare')).toMatchObject({total:1});
 await page.goto('/compare?offers=removed');
 await expect(page.getByText('Предложение недоступно')).toBeVisible();
 await page.goto('/compare');
 await expect(page.getByText('Выберите от одного до трёх тарифов в каталоге.')).toBeVisible();
 expect(events.filter(e=>e.kind==='compare')).toHaveLength(1);
 await page.goto('/courses?compare=demo-go-1-standard,demo-go-2-standard');
 await expect(page.getByRole('dialog').getByRole('table')).toBeVisible();
 await expect.poll(()=>events.filter(e=>e.kind==='compare')).toHaveLength(2);
 expect(events.filter(e=>e.kind==='compare')[1].total).toBe(2);
});

test('failed comparisons do not count, blocked analytics cannot break successful comparison',async({page})=>{
 const events=await captureEvents(page,true);
 await page.route('**/api/v1/compare?*',route=>route.fulfill({status:503,json:{error:'unavailable'}}));
 await page.goto('/courses?compare=demo-go-1-standard');
 await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();
 expect(events.filter(e=>e.kind==='compare')).toHaveLength(0);
 await page.goto('/compare?offers=demo-go-1-standard');
 await expect(page.getByRole('table')).toBeVisible();
 await expect.poll(()=>events.filter(e=>e.kind==='compare')).toHaveLength(1);
 await expect(page.getByRole('link',{name:'Проверить условия'})).toBeEnabled();
});

test('advanced filters retain closed drafts, reopen applied URL values and reset',async({page})=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses');
 const details=page.locator('.advancedFilters'),summary=details.locator('summary');
 await expect(details).not.toHaveAttribute('open');
 await summary.focus();await page.keyboard.press('Enter');
 await expect(details).toHaveAttribute('open','');
 await page.locator('select[name=direction]').selectOption('backend');
 await page.getByLabel('Обратная связь').selectOption('mentor');
 await page.getByLabel('Часов в неделю, не больше').fill('12');
 await summary.click();
 await expect(details).not.toHaveAttribute('open');
 await page.getByRole('radio',{name:'Go',exact:true}).check();
 await expect(details).not.toHaveAttribute('open');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(details).toHaveAttribute('open','');
 expect(new URL(page.url()).searchParams.get('support')).toBe('mentor');
 await expect(page.getByLabel('Часов в неделю, не больше')).toHaveValue('12');
 await page.goBack();await expect(details).not.toHaveAttribute('open');
 await page.goForward();await expect(details).toHaveAttribute('open','');
 await page.reload();
 await expect(details).toHaveAttribute('open','');
 await page.getByRole('button',{name:'Сбросить',exact:true}).click();
 await expect(details).not.toHaveAttribute('open');
 await expect(page).toHaveURL(/\/courses$/);
});

for(const width of [390,1440])for(const theme of ['light','dark'] as const){
 test(`first screen and both filter states fit at ${width}px ${theme}`,async({page})=>{
  await page.setViewportSize({width,height:900});await page.emulateMedia({colorScheme:theme,reducedMotion:'reduce'});
  await page.goto('/');await expect(page.locator('.card').first()).toBeVisible();
  await expect(page.locator('.heroCopy')).toContainText('полную стоимость конкретных тарифов');
  await expect(page.getByRole('navigation',{name:'Основная навигация'}).getByRole('link',{name:'О сервисе',exact:true})).toHaveAttribute('href','/about');
  if(width===1440){
   const summary=(await page.locator('.advancedFilters summary').boundingBox())!;
   const submit=(await page.getByRole('button',{name:'Найти обучение'}).boundingBox())!;
   expect(Math.abs(summary.y-submit.y)).toBeLessThan(4);
  }
  for(const expanded of [false,true]){
   if(expanded)await page.locator('.advancedFilters summary').click();
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
   const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();expect(axe.violations).toEqual([]);
  }
 });
}

 test('invalid workload expands its closed filters so validation can focus the input',async({page})=>{
  await page.goto('/courses');
  const details=page.locator('.advancedFilters'),hours=page.getByLabel('Часов в неделю, не больше');
  await details.locator('summary').click();await hours.fill('169');
  await details.locator('summary').click();
  await page.getByRole('button',{name:'Найти обучение'}).click();
  await expect(details).toHaveAttribute('open','');await expect(hours).toBeFocused();
  expect(new URL(page.url()).searchParams.has('hours')).toBe(false);
 });
