import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
const comparison='/courses?compare=demo-go-1-standard,demo-go-2-standard';

test('legacy comparison traps focus, blocks the background and restores focus on Escape',async({page})=>{
 await page.setViewportSize({width:360,height:800});
 await page.addInitScript(()=>localStorage.setItem('devcourse-offers',JSON.stringify(['demo-python-1-standard'])));
 await page.goto(comparison);
 const dialog=page.getByRole('dialog',{name:'Сравнение тарифов'});
 const close=dialog.getByRole('button',{name:'Закрыть',exact:true});
 await expect(dialog.getByRole('table')).toBeVisible();
 await expect(dialog.getByRole('link',{name:'Проверить условия'})).toHaveCount(2);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
 expect(axe.violations.filter(v=>v.impact==='critical'||v.impact==='serious').map(v=>v.id)).toEqual([]);
 await expect(close).toBeFocused();
 const last=dialog.getByRole('link',{name:'Проверить условия'}).last();
 await page.keyboard.press('Shift+Tab');await expect(last).toBeFocused();
 await page.keyboard.press('Tab');await expect(close).toBeFocused();
 await page.locator('input[name=language][value=go]').evaluate(node=>(node as HTMLElement).focus());
 await expect(close).toBeFocused();
 await page.keyboard.press('Escape');
 await expect(dialog).toHaveCount(0);
 await expect(page.getByRole('button',{name:'Найти обучение'})).toBeFocused();
 expect(new URL(page.url()).searchParams.has('compare')).toBe(false);
 expect(await page.evaluate(()=>document.body.style.overflow)).not.toBe('hidden');
});

test('history-opened comparison restores its previous control on button and backdrop close',async({page})=>{
 await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(12);
 const sort=page.getByLabel('Сортировка',{exact:true});
 const open=async()=>{
  await sort.focus();
  await page.evaluate(url=>{history.pushState(null,'',url);window.dispatchEvent(new PopStateEvent('popstate'))},comparison);
  await expect(page.getByRole('dialog').getByRole('table')).toBeVisible();
 };
 await open();await page.getByRole('dialog').getByRole('button',{name:'Закрыть',exact:true}).click();
 await expect(sort).toBeFocused();
 await open();await page.getByRole('dialog').click({position:{x:2,y:2}});
 await expect(page.getByRole('dialog')).toHaveCount(0);await expect(sort).toBeFocused();
});

test('dialog keeps keyboard focus during loading and error, and passes axe at 360px',async({page})=>{
 await page.setViewportSize({width:360,height:800});
 let release!:()=>void;
 const gate=new Promise<void>(resolve=>{release=resolve});
 await page.route('**/api/v1/compare*',async route=>{await gate;await route.fulfill({status:503,json:{error:'unavailable'}})});
 await page.goto(comparison);
 const dialog=page.getByRole('dialog'),close=dialog.getByRole('button',{name:'Закрыть',exact:true});
 try{
  await expect(dialog.getByRole('status')).toContainText('Загрузка');
  await expect(close).toBeFocused();
  await page.keyboard.press('Tab');await expect(close).toBeFocused();
 }finally{release()}
 await expect(dialog.getByRole('alert')).toContainText('Не удалось загрузить сравнение');
 await page.keyboard.press('Shift+Tab');await expect(close).toBeFocused();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
 expect(result.violations.filter(v=>v.impact==='serious'||v.impact==='critical').map(v=>v.id)).toEqual([]);
 await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);
 await expect(page.getByRole('button',{name:'Найти обучение'})).toBeFocused();
});

for(const scenario of [
 {name:'portrait cutout',width:390,height:900,top:47,right:0,bottom:34,left:0},
 {name:'landscape cutouts',width:844,height:390,top:0,right:47,bottom:21,left:47},
])test(`long comparison stays within synthetic safe areas: ${scenario.name}`,async({page})=>{
 await page.setViewportSize({width:scenario.width,height:scenario.height});
 await page.goto(comparison);
 const dialog=page.getByRole('dialog',{name:'Сравнение тарифов'});
 await expect(dialog.getByRole('table')).toBeVisible();
 // Chromium has no iPhone cutout; substitute only the platform inset tokens.
 await page.addStyleTag({content:`:root {--safe-top:${scenario.top}px;--safe-right:${scenario.right}px;--safe-bottom:${scenario.bottom}px;--safe-left:${scenario.left}px;}`});
 const modal=(await dialog.locator('.modal').boundingBox())!;
 expect(modal.y).toBeGreaterThanOrEqual(scenario.top);
 expect(modal.y+modal.height).toBeLessThanOrEqual(scenario.height-scenario.bottom);
 expect(modal.x).toBeGreaterThanOrEqual(scenario.left);
 expect(modal.x+modal.width).toBeLessThanOrEqual(scenario.width-scenario.right);
 const close=dialog.getByRole('button',{name:'Закрыть',exact:true});
 const box=(await close.boundingBox())!;
 expect(box.y).toBeGreaterThanOrEqual(scenario.top);
 expect(box.x+box.width).toBeLessThanOrEqual(scenario.width-scenario.right);
 await close.click();await expect(dialog).toHaveCount(0);
});
