import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const providers=['JavaRush','Skypro','Skillfactory','Skillbox','HTML Academy','RS School','PurpleSchool','Нетология','Stepik','OTUS','Яндекс Практикум','Хекслет','CodeBasics','Новая школа'];
const languages=['python','javascript','java','go'];

for(const width of [390,1440])for(const theme of ['light','dark'] as const){
 test(`all school names and language badges remain readable: ${width}px ${theme}`,async({page},info)=>{
  await page.setViewportSize({width,height:900});
  await page.emulateMedia({colorScheme:theme,reducedMotion:'reduce'});
  await page.route('**/api/v1/courses*',async route=>{
   const body=await (await route.fetch()).json();
   const items=providers.map((provider,index)=>{
    const item=structuredClone(body.items[index%body.items.length]);
    item.course.provider=provider;item.course.language=languages[index%4];
    item.course.id=`brand-${index}`;item.offer.id=`brand-${index}-offer`;
    return item;
   });
   await route.fulfill({json:{...body,items,total:items.length,page_size:items.length}});
  });
  await page.goto('/courses');await expect(page.locator('.card')).toHaveCount(providers.length);
  const axe=await new AxeBuilder({page}).include('.cardTop').withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  expect(axe.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>n.target)}))).toEqual([]);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await page.locator('.card').first().scrollIntoViewIfNeeded();
  await page.screenshot({path:info.outputPath(`brands-${width}-${theme}.png`),fullPage:true});
  await page.emulateMedia({forcedColors:'active'});
  const highContrast=await new AxeBuilder({page}).include('.cardTop').withTags(['wcag2a','wcag2aa']).analyze();
  expect(highContrast.violations.map(v=>v.id)).toEqual([]);
 });
}
