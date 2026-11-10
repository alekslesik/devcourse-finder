import {test,expect,type Page} from '@playwright/test';

const origin='https://devcoursefinder.ru';
const tag='https://mc.yandex.ru/metrika/tag.js?id=114139174';
const pixel='https://mc.yandex.ru/watch/114139174';
declare global {interface Window {__metrikaCalls?:unknown[][]}}

// Serve the real app at its production hostname without sending test visits to
// Yandex. The local and real-stack suites otherwise use loopback hostnames.
async function productionHost(page:Page){
 await page.route(origin+'/**',async route=>{
  const url=new URL(route.request().url());
  const response=await route.fetch({url:'http://127.0.0.1:3091'+url.pathname+url.search});
  await route.fulfill({response});
 });
}
const calls=(page:Page)=>page.evaluate(()=>window.__metrikaCalls??window.ym?.a??[]);
const sdk="window.__metrikaCalls=(window.ym?.a||[]).slice();window.ym=(...args)=>window.__metrikaCalls.push(args);";

// A search also posts a keepalive event. Drain the forwarding routes before
// Playwright disposes their request context, including on the slower real stack.
test.afterEach(async({page})=>{await page.unrouteAll({behavior:'wait'})});

test('initial visit, queued SPA views and history initialize exactly once',async({page})=>{
 await productionHost(page);
 let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve});
 let loads=0,pixels=0;
 await page.route('https://mc.yandex.ru/**',async route=>{
  if(route.request().url()===tag){loads++;await gate;await route.fulfill({contentType:'application/javascript',body:sdk});}
  else{pixels++;await route.fulfill({status:204});}
 });
 try{
  await page.goto(origin+'/courses',{waitUntil:'domcontentloaded'});
  await expect(page.locator('.card')).toHaveCount(18);
  await expect.poll(()=>calls(page)).toHaveLength(1);
  const init=(await calls(page))[0];
  expect(init.slice(0,2)).toEqual([114139174,'init']);
  expect(init[2]).toMatchObject({ssr:true,webvisor:true,clickmap:true,ecommerce:'dataLayer',referrer:'',url:origin+'/courses',accurateTrackBounce:true,trackLinks:true});
  await page.getByRole('button',{name:'Далее →',exact:true}).click();
  await expect(page).toHaveURL(/page=2/);
  await expect.poll(()=>calls(page)).toHaveLength(2);
  expect((await calls(page))[1]).toEqual([114139174,'hit',origin+'/courses?page=2',{referer:origin+'/courses'}]);
  await page.goBack();await expect(page).toHaveURL(origin+'/courses');
  await expect.poll(()=>calls(page)).toHaveLength(3);
  expect((await calls(page))[2]).toEqual([114139174,'hit',origin+'/courses',{referer:origin+'/courses?page=2'}]);
  await page.evaluate(()=>history.replaceState(history.state,'',location.href));
  release();await expect.poll(()=>page.evaluate(()=>!!window.__metrikaCalls)).toBe(true);
  expect((await calls(page)).filter(call=>call[1]==='init')).toHaveLength(1);
  expect(await calls(page)).toHaveLength(3);expect(loads).toBe(1);expect(pixels).toBe(0);
 }finally{release()}
});

test('a blocked counter or throwing SDK cannot break catalog navigation',async({page})=>{
 await productionHost(page);
 const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
 await page.route('https://mc.yandex.ru/**',route=>route.abort('blockedbyclient'));
 await page.goto(origin+'/courses');await expect(page.locator('.card')).toHaveCount(18);
 await expect.poll(()=>calls(page)).toHaveLength(1);
 await page.evaluate(()=>{window.ym=()=>{throw Error('SDK unavailable')}});
 await page.getByRole('button',{name:'Далее →',exact:true}).click();await expect(page).toHaveURL(/page=2/);
 await expect(page.getByRole('button',{name:'← Назад',exact:true})).toBeEnabled();
 expect(errors).toEqual([]);
});

test('loopback development does not load the production counter',async({page})=>{
 let requests=0;await page.route('https://mc.yandex.ru/**',route=>{requests++;return route.abort()});
 await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(18);
 await page.getByRole('button',{name:'Далее →',exact:true}).click();await expect(page).toHaveURL(/page=2/);
 expect(await page.evaluate(()=>window.ym)).toBeUndefined();expect(requests).toBe(0);
});

test('JavaScript-disabled visitors receive the supplied fallback pixel',async({browser})=>{
 const context=await browser.newContext({javaScriptEnabled:false});
 try{
  const page=await context.newPage();await productionHost(page);
  const requests:string[]=[];
  await page.route('https://mc.yandex.ru/**',route=>{requests.push(route.request().url());return route.fulfill({status:204})});
  await page.goto(origin+'/about');
  await expect(page.locator('noscript img')).toHaveAttribute('src',pixel);
  await expect(page.locator('noscript img')).toHaveAttribute('alt','');
  await expect.poll(()=>requests).toEqual([pixel]);
 }finally{await context.close()}
});
