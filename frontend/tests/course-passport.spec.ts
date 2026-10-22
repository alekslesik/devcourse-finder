import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

for(const width of [390,1440])for(const theme of ['light','dark'] as const){
 test(`tariff selection stays accessible: ${width}px ${theme}`,async({page},info)=>{
  test.skip(process.env.E2E_REAL_API==='1','Synthetic multi-offer fixture is local to the frontend API fixture.');
  await page.setViewportSize({width,height:1000});await page.emulateMedia({colorScheme:theme,reducedMotion:'reduce'});
  await page.goto('/courses/passport-multi');
  const conditions=page.getByRole('link',{name:'Условия',exact:true});
  await conditions.click();await expect(conditions).toHaveAttribute('aria-current','location');
  const choices=page.locator('.courseTariffList').getByRole('radio');
  await choices.first().focus();await page.keyboard.press('ArrowDown');
  await expect(choices.nth(1)).toBeChecked();await expect(choices.nth(1)).toBeFocused();
  await expect(page.locator('.coursePurchase[data-selected=true]')).toHaveAttribute('data-offer','passport-free');
  await expect(page.locator('.courseConditionsTariff')).toContainText('Вводный тариф');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  expect(axe.violations.map(v=>({id:v.id,targets:v.nodes.map(n=>n.target)}))).toEqual([]);
  await page.screenshot({path:info.outputPath(`tariff-selection-${width}-${theme}.png`),fullPage:true});
 });
}

for(const width of [390,1440])for(const theme of ['light','dark'] as const){
 test(`course passport layout, anchors and actions: ${width}px ${theme}`,async({page},info)=>{
  await page.setViewportSize({width,height:1000});await page.emulateMedia({colorScheme:theme,reducedMotion:'reduce'});
  await page.goto('/courses/demo-go-1');
  const purchase=page.locator('.coursePurchase');await expect(purchase).toBeVisible();
  await expect(purchase.getByRole('link',{name:'На сайт курса'})).toHaveAttribute('href','/out/demo-go-1-standard');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  const intro=(await page.locator('.courseIntro').boundingBox())!,panel=(await purchase.boundingBox())!;
  if(width===1440){
   expect(panel.x).toBeGreaterThan(intro.x+intro.width);
   await page.evaluate(()=>scrollTo(0,350));
   await expect.poll(async()=>Math.round((await purchase.boundingBox())!.y)).toBe(24);
   await page.evaluate(()=>scrollTo(0,0));
   await expect(page.getByRole('link',{name:'Обзор',exact:true})).toHaveAttribute('aria-current','location');
  }else{
   expect(panel.y).toBeGreaterThan(intro.y+intro.height);
   await expect(page.locator('.courseDock')).toHaveCount(0);
  }
  const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
  expect(axe.violations.map(v=>({id:v.id,targets:v.nodes.map(n=>n.target)}))).toEqual([]);
  await page.screenshot({path:info.outputPath(`passport-${width}-${theme}.png`),fullPage:true});
  await page.getByRole('link',{name:'Условия',exact:true}).focus();await page.keyboard.press('Enter');
  await expect(page.locator('#course-conditions')).toBeFocused();
  await expect(page.locator('#course-conditions')).toHaveCSS('outline-style','none');
  await expect(page.getByRole('link',{name:'Источник',exact:true})).toHaveCount(0);
  await expect(page.locator('#course-sources')).toHaveCount(0);
  await page.keyboard.press('Tab');
  await expect(page.locator('.courseProgramLink')).toBeFocused();
  await expect(page.locator('.courseProgramLink')).toHaveCSS('outline-style','solid');
  if(width===390){
   await page.locator('.courseProgram').scrollIntoViewIfNeeded();
   const dock=page.getByRole('region',{name:'Быстрые действия по тарифу'});
   await expect(dock).toBeInViewport();
   await expect(dock.getByRole('link',{name:'На сайт курса'})).toHaveAttribute('href','/out/demo-go-1-standard');
   const source=page.locator('.courseProgramLink');await source.focus();
   await expect.poll(async()=>((await source.boundingBox())!.y+(await source.boundingBox())!.height)<(await dock.boundingBox())!.y).toBe(true);
   await page.screenshot({path:info.outputPath(`passport-dock-${theme}.png`)});
   const dockAxe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();
   expect(dockAxe.violations.map(v=>v.id)).toEqual([]);
   await page.evaluate(()=>scrollTo(0,0));await expect(dock).toHaveCount(0);
  }
 });
}

test('selected tariff keeps its price, conditions, outbound target and comparison identity',async({page})=>{
 test.skip(process.env.E2E_REAL_API==='1','Synthetic multi-offer fixture is local to the frontend API fixture.');
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses/passport-multi');
 const purchase=page.locator('.coursePurchase[data-selected=true]');
 const choose=(id:string)=>page.locator('input[name=course-offer][value="'+id+'"]');
 await expect(page.locator('.coursePurchase')).toHaveCount(4);
 await expect(page.locator('select')).toHaveCount(0);
 for(const id of ['passport-standard','passport-free','passport-from','passport-closed'])await expect(page.locator('.coursePurchase[data-offer="'+id+'"] .coursePrimaryAction')).toHaveAttribute('href','/out/'+id);
 const fact=(label:string)=>page.locator('.courseFact').filter({has:page.locator('dt',{hasText:label})}).locator('dd');
 await expect(fact('Кому подходит')).toHaveText('Без опыта · Программирую на другом языке');
 await expect(fact('Требования к опыту')).toHaveText('Знание основ на языке курса');
 await expect(purchase.locator('.coursePurchasePrice')).toContainText('70');
 await expect(fact('Проверка кода')).toHaveText('Уточните у школы');
 await choose('passport-free').check();
 await expect(purchase.locator('.coursePurchasePrice')).toHaveText('Бесплатно');
 await expect(fact('Проверка кода')).toHaveText('Нет');
 await expect(purchase.getByRole('link',{name:'На сайт курса'})).toHaveAttribute('href','/out/passport-free');
 expect(new URL(page.url()).searchParams.get('offer')).toBe('passport-free');
 await expect(page.locator('.coursePage>.srOnly[role=status]')).toContainText('Вводный тариф. Бесплатно');
 await purchase.getByRole('button',{name:'Добавить в сравнение'}).click();
 await expect(page).toHaveURL(/offers=passport-free/);
 await expect(page.getByRole('table')).toContainText('Бесплатно');
 await page.goBack();await expect(choose('passport-free')).toBeChecked();
 await choose('passport-from').check();
 await expect(purchase.locator('.coursePurchasePrice')).toHaveText('Цена не подтверждена');
 await expect(purchase).toContainText('Цена «от»');
 await expect(fact('Проверка кода')).toHaveText('Уточните у школы');
 await expect(purchase.getByRole('button',{name:'Добавить в сравнение'})).toHaveCount(0);
 await expect(purchase.getByRole('link',{name:'Проверить набор'})).toHaveAttribute('href','/out/passport-from');
 await choose('passport-closed').check();
 await expect(purchase).toContainText('Набор закрыт');
 await expect(fact('Проверка кода')).toHaveText('Есть');
 await expect(purchase.getByRole('button',{name:'Добавить в сравнение'})).toHaveCount(0);
 await expect(purchase.getByRole('link',{name:'Проверить набор'})).toHaveAttribute('href','/out/passport-closed');
 await page.reload();await expect(choose('passport-closed')).toBeChecked();
});

test('shared tariff selection is server rendered and invalid IDs fall back safely',async({browser})=>{
 test.skip(process.env.E2E_REAL_API==='1','Synthetic multi-offer fixture is local to the frontend API fixture.');
 const context=await browser.newContext({javaScriptEnabled:false});
 try{
  const page=await context.newPage();
  await page.goto('http://127.0.0.1:3091/courses/passport-multi?offer=passport-free');
  await expect(page.locator('input[name=course-offer][value=passport-free]')).toBeChecked();
  await expect(page.locator('.coursePurchase[data-selected=true] .coursePurchasePrice')).toHaveText('Бесплатно');
  await expect(page.locator('.coursePurchase[data-selected=true]').getByRole('link',{name:'На сайт курса'})).toHaveAttribute('href','/out/passport-free');
  await page.goto('http://127.0.0.1:3091/courses/passport-multi?offer=missing');
  await expect(page.locator('input[name=course-offer][value=passport-standard]')).toBeChecked();
 }finally{await context.close()}
});

test('missing topics and tariffs remain explicit without invented content or actions',async({page})=>{
 test.skip(process.env.E2E_REAL_API==='1','Synthetic empty-offer fixture is local to the frontend API fixture.');
 await page.setViewportSize({width:360,height:800});await page.goto('/courses/passport-empty');
 await expect(page.getByRole('heading',{name:'Тарифы уточняются'})).toBeVisible();
 await expect(page.getByRole('button',{name:'Добавить в сравнение'})).toHaveCount(0);
 await expect(page.locator('.courseTopics')).toHaveCount(0);
 await expect(page.locator('.courseProgram')).toContainText('Содержание программы уточняйте на сайте школы.');
 await expect(page.locator('.courseFacts').first()).toContainText('Требования к опыту не подтверждены');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

test('catalog details preserve the matched non-first tariff on a short screen',async({page,request})=>{
 test.skip(process.env.E2E_REAL_API==='1','Synthetic multi-offer fixture is local to the frontend API fixture.');
 const data=await (await request.get('http://127.0.0.1:8091/api/v1/courses/passport-multi')).json();
 const match=data.offers.find((i:{offer:{id:string}})=>i.offer.id==='passport-free');
 await page.route('**/api/v1/courses?*',route=>route.fulfill({json:{items:[match],total:1,page:1,page_size:12}}));
 await page.setViewportSize({width:320,height:568});await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses');
 const card=page.locator('.card');await expect(card).toHaveCount(1);
 for(const link of [card.locator('.titleButton'),card.getByRole('link',{name:'Подробнее'})])await expect(link).toHaveAttribute('href','/courses/passport-multi?offer=passport-free');
 await card.getByRole('link',{name:'Подробнее'}).click();
 await expect(page.locator('input[name=course-offer][value=passport-free]')).toBeChecked();
 await expect(page.locator('.coursePurchase[data-selected=true] .coursePurchasePrice')).toHaveText('Бесплатно');
 const purchase=page.locator('.coursePurchase[data-selected=true]');
 expect(await purchase.evaluate(node=>({maxHeight:getComputedStyle(node).maxHeight,overflow:getComputedStyle(node).overflowY}))).toEqual({maxHeight:'none',overflow:'visible'});
 await purchase.getByRole('link',{name:'На сайт курса'}).scrollIntoViewIfNeeded();
 await expect(purchase.getByRole('link',{name:'На сайт курса'})).toBeInViewport();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
});

for(const width of [390,1440])for(const theme of ['light','dark'] as const){
 test(`all tariff cards form an accessible vertical stack: ${width}px ${theme}`,async({page},info)=>{
 test.skip(process.env.E2E_REAL_API==='1','Synthetic multi-offer fixture is local to the frontend API fixture.');
 await page.setViewportSize({width,height:1000});await page.emulateMedia({colorScheme:theme,reducedMotion:'reduce'});
 await page.goto('/courses/passport-multi');
 const cards=page.locator('.coursePurchase');await expect(cards).toHaveCount(4);
 const boxes=await cards.evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,bottom:r.bottom}}));
 for(let i=1;i<boxes.length;i++){expect(boxes[i].y-boxes[i-1].bottom).toBeGreaterThanOrEqual(23);expect(boxes[i].x).toBe(boxes[0].x)}
 if(width===1440){
  expect(boxes[0].width).toBe(380);
  const intro=(await page.locator('.courseIntro').boundingBox())!,content=(await page.locator('.courseContent').boundingBox())!;
  expect(content.y-intro.y-intro.height).toBeGreaterThanOrEqual(31);
  expect(content.y-intro.y-intro.height).toBeLessThanOrEqual(33);
 }
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 await page.getByRole('radio',{name:'Полная программа',exact:true}).focus();await page.keyboard.press('ArrowDown');
 await expect(page.getByRole('radio',{name:'Вводный тариф'})).toBeChecked();
 await expect(page.locator('.courseConditionsTariff')).toContainText('Вводный тариф');
 if(width===390){
  // Jump from below the viewport directly above it: IO ratios remain zero.
  await page.getByRole('radio',{name:'Закрытый тариф'}).check();
  await page.locator('.coursePurchase[data-offer=passport-closed]').evaluate(node=>scrollTo(0,scrollY+node.getBoundingClientRect().top-innerHeight+100));
  await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
  expect((await page.locator('.coursePurchase[data-selected=true] .coursePrimaryAction').boundingBox())!.y).toBeGreaterThan(1000);
  await expect(page.locator('.courseDock')).toHaveCount(0);
  await page.evaluate(()=>scrollTo(0,document.documentElement.scrollHeight));
  const dock=page.locator('.courseDock');await expect(dock).toBeInViewport();
  await expect(dock.getByRole('link',{name:'Проверить набор'})).toHaveAttribute('href','/out/passport-closed');
  // A layout-driven boundary crossing must update without another scroll event.
  const action=page.locator('.coursePurchase[data-selected=true] .coursePrimaryAction');
  await action.evaluate(node=>{node.style.transform='translateY('+(20-node.getBoundingClientRect().top)+'px)'});
  await expect(dock).toHaveCount(0);
  await action.evaluate(node=>{node.style.transform=''});await expect(dock).toBeInViewport();

  await page.evaluate(()=>scrollTo(0,0));await expect(dock).toHaveCount(0);
 }

 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();expect(axe.violations.map(v=>({id:v.id,targets:v.nodes.map(n=>n.target)}))).toEqual([]);
 await page.evaluate(()=>scrollTo(0,0));await page.screenshot({path:info.outputPath(`tariffs-${width}-${theme}.png`),fullPage:true});
 });
}
