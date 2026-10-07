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
 await expect(page.getByRole('button',{name:'Фильтры',exact:true})).toBeFocused();
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
 await expect(page.getByRole('button',{name:'Фильтры',exact:true})).toBeFocused();
});
