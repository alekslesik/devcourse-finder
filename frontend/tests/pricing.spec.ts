import {test,expect} from '@playwright/test';

test('AC-02: Go switch search submits the full budget in kopecks and preserves rubles in the form',async({page})=>{
 await page.goto('/courses');
 await expect(page.locator('.card')).toHaveCount(12);
 await page.getByRole('radio',{name:'Go',exact:true}).check();
 await page.getByLabel('Текущий опыт').selectOption('switch');
 await page.getByLabel('Ваша цель').selectOption('switch');
 await page.getByLabel('До, ₽',{exact:true}).fill('30000');
 const submitted=page.waitForRequest(request=>new URL(request.url()).pathname==='/api/v1/courses'&&new URL(request.url()).searchParams.get('goal')==='switch');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 const params=new URL((await submitted).url()).searchParams;
 expect(Object.fromEntries(params)).toMatchObject({language:'go',experience:'switch',goal:'switch',max:'3000000'});
 await expect(page.getByRole('region',{name:'Результаты поиска'})).toHaveAttribute('aria-busy','false');
 await expect(page.getByLabel('До, ₽',{exact:true})).toHaveValue('30000');
 await expect(page.getByLabel('Текущий опыт')).toHaveValue('switch');
 await expect(page).toHaveURL(/max=3000000/);
});

for(const [kind,label] of [['from','Цена от · уточните у школы'],['unknown','Уточнить цену']] as const){
 test(`AC-04: ${kind} price shows a qualification instead of an exact amount`,async({page})=>{
  await page.route('**/api/v1/courses*',async route=>{
   const response=await route.fetch();
   const body=await response.json();
   const item=body.items[0];
   item.offer.price_kind=kind;
   item.offer.price=kind==='from'?2000000:null;
   item.effective_price=null;
   await route.fulfill({json:{...body,items:[item],total:1}});
  });
  await page.goto('/courses');
  await expect(page.locator('.card')).toHaveCount(1);
  await expect(page.locator('.card .price')).toHaveText(label);
 });
}

for(const invalidity of ['stale','expired'] as const){
 test(`AC-06: ${invalidity} exact amount is not displayed as a current price`,async({page})=>{
  await page.route('**/api/v1/courses*',async route=>{
   const response=await route.fetch();
   const body=await response.json();
   const item=body.items[0];
   item.offer.price_kind='exact';
   item.offer.price=2000000;
   item.offer.price_checked_at=new Date(Date.now()-(invalidity==='stale'?31:0)*86400000).toISOString();
   item.offer.valid_until=invalidity==='expired'?new Date(Date.now()-86400000).toISOString():null;
   // Effective price is computed by Go. UI must respect its null result and
   // never resurrect the original stored amount as a confirmed price.
   item.effective_price=null;
   await route.fulfill({json:{...body,items:[item],total:1}});
  });
  await page.goto('/courses');
  await expect(page.locator('.card')).toHaveCount(1);
  await expect(page.locator('.card .price')).toHaveText('Уточнить цену');
 });
}


test('discovered courses do not invent support when evidence is unknown',async({page})=>{
 await page.route('**/api/v1/courses*',async route=>{
  const response=await route.fetch();
  const body=await response.json();
  const item=body.items[0];
  item.offer.support_known=false;
  item.offer.review=false;
  item.offer.mentor=false;
  await route.fulfill({json:{...body,items:[item],total:1}});
 });
 await page.goto('/courses');
 await expect(page.locator('.card .facts')).toContainText('Поддержка не подтверждена');
 await expect(page.locator('.card .facts')).not.toContainText('Самостоятельно');
 await page.route('**/api/v1/compare*',async route=>{
  const response=await route.fetch();
  const body=await response.json();
  for(const item of body){if(!item.unavailable){item.offer.support_known=false;item.offer.review=false;item.offer.mentor=false}}
  await route.fulfill({json:body});
 });
 await page.goto('/courses?compare=demo-go-1-standard');
 await expect(page.getByRole('dialog').getByRole('table')).toBeVisible();
 await expect(page.getByRole('row').filter({has:page.getByRole('rowheader',{name:'Проверка кода',exact:true})})).toContainText('Не подтверждено');
 await expect(page.getByRole('row').filter({has:page.getByRole('rowheader',{name:'Индивидуальные занятия',exact:true})})).toContainText('Не подтверждено');
});
