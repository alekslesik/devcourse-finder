import {test,expect} from '@playwright/test';

for(const theme of ['light','dark'] as const)test(`touch input and shortened viewport retain a usable comparison bar: ${theme}`,async({browser},info)=>{
 const context=await browser.newContext({viewport:{width:390,height:844},hasTouch:true,isMobile:true,colorScheme:theme,reducedMotion:'reduce'});
 try{
  const page=await context.newPage();await page.goto('http://127.0.0.1:3091/courses');
  await expect(page.locator('.card')).toHaveCount(12);
  await page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Go',exact:true}).tap();
  await expect(page.getByRole('radio',{name:'Go',exact:true})).toBeChecked();
  const max=page.getByLabel('До, ₽',{exact:true});await max.tap();await max.fill('100000');
  expect(await max.evaluate(n=>parseFloat(getComputedStyle(n).fontSize))).toBeGreaterThanOrEqual(16);
  // A shorter viewport exercises available space; it is not a software keyboard.
  await page.setViewportSize({width:390,height:480});
  await page.getByRole('button',{name:'Найти обучение'}).tap();
  await expect(page.locator('.resultHeading')).toBeFocused();
  await page.getByLabel('Сравнить тариф').first().tap();
  const compare=page.getByRole('button',{name:'Сравнить →'});
  await expect(compare).toBeInViewport();await compare.tap();
  await expect(page.getByRole('region',{name:'Сравнение тарифов, горизонтальная прокрутка'})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.screenshot({path:info.outputPath(`touch-${theme}.png`),fullPage:true});
 }finally{await context.close()}
});
