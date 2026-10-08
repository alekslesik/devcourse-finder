import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

for(const width of [360,768,1440])for(const theme of ['light','dark'] as const){
 test(`visual and accessibility matrix: ${width}px ${theme}`,async({page},info)=>{
  test.setTimeout(120000);
  await page.setViewportSize({width,height:900});
  await page.emulateMedia({colorScheme:theme,reducedMotion:'reduce'});
  const inspect=async(name:string)=>{
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
   await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
   const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
   expect(axe.violations.map(v=>({id:v.id,targets:v.nodes.map(n=>n.target)}))).toEqual([]);
   await page.screenshot({path:info.outputPath(name+'.png'),fullPage:true,animations:'disabled'});
  };
  for(const [path,name] of [['/courses','catalog'],['/courses/demo-go-1','course'],['/compare?offers=demo-go-1-standard,closed-tariff,removed','comparison'],['/about','about'],['/courses?compare=demo-go-1-standard,removed','dialog']] as const){
   await page.goto(path);
   if(name==='catalog'){
    await expect(page.locator('.card')).toHaveCount(12);
    await expect(page.locator('header').getByRole('link',{name:'DevCourseFinder — на главную'})).toBeVisible();
    await expect(page.locator('.routeIllustration')).toBeVisible({visible:width>700});
   }
   if(name==='dialog')await expect(page.getByRole('dialog').getByRole('table')).toBeVisible();
   await inspect(name);
  }
  await page.evaluate(()=>localStorage.removeItem('devcourse-offers'));
  let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve});
  await page.route('**/api/v1/courses*',async route=>{await gate;await route.fulfill({status:503,json:{error:'unavailable'}})});
  await page.goto('/courses');
  try{await expect(page.getByLabel('Загрузка',{exact:true})).toBeVisible();await inspect('loading');}finally{release()}
  await expect(page.locator('main').getByRole('alert')).toBeVisible();await inspect('error');
  await page.unroute('**/api/v1/courses*');
  await page.route('**/api/v1/courses*',route=>route.fulfill({json:{items:[],total:0,page:1,page_size:12}}));
  await page.goto('/courses');await expect(page.getByText('Программы пока не опубликованы')).toBeVisible();await inspect('empty');
  await page.goto('/courses?language=go');await expect(page.getByText('Подходящая программа ещё не нашлась')).toBeVisible();await inspect('no-matches');
 });
}

test('long course text and duplicate-course offers fit a mobile card',async({page})=>{
 await page.setViewportSize({width:360,height:800});
 await page.route('**/api/v1/courses*',async route=>{
  const body=await (await route.fetch()).json();const item=body.items[0];
  item.course.title='ОченьДлинноеНазваниеПрограммы'.repeat(8);
  item.course.provider='ПоставщикБезПробелов'.repeat(8);item.course.summary='Подробности '.repeat(90);
  item.effective_price=9999999999;
  const second=structuredClone(item);second.offer.id='second-offer';second.offer.name='Другой тариф';
  await route.fulfill({json:{...body,items:[item,second],total:2}});
 });
 await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(2);
 await page.getByLabel('Сравнить тариф').nth(1).check();
 await expect(page.getByLabel('Сравнить тариф').first()).not.toBeChecked();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});
