import {test,expect} from '@playwright/test';

test('free-only ignores price bounds and restores entered rubles after applying',async({page})=>{
 await page.goto('/courses');
 const min=page.getByLabel('От, ₽',{exact:true}),max=page.getByLabel('До, ₽',{exact:true});
 await min.fill('20000');await max.fill('10000');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(max).toHaveJSProperty('validationMessage','Максимальный бюджет должен быть не меньше минимального.');
 const free=page.getByRole('checkbox',{name:'Бесплатно',exact:true});
 await free.check();
 await expect(min).toBeDisabled();await expect(max).toBeDisabled();
 const request=page.waitForRequest(r=>new URL(r.url()).pathname==='/api/v1/courses'&&new URL(r.url()).searchParams.get('budget')==='free');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 const query=new URL((await request).url()).searchParams;
 expect(query.has('min')).toBe(false);expect(query.has('max')).toBe(false);
 await expect(page.locator('.card')).toHaveCount(4);
 for(const price of await page.locator('.card .price').allTextContents())expect(price).toBe('Бесплатно');
 await expect(page.getByRole('button',{name:'Убрать фильтр: Цена'})).toBeVisible();
 await expect(min).toHaveValue('20000');await expect(max).toHaveValue('10000');
 await free.uncheck();await expect(min).toBeEnabled();await expect(max).toBeEnabled();
 await max.fill('30000');
 const paidRequest=page.waitForRequest(r=>new URL(r.url()).pathname==='/api/v1/courses'&&new URL(r.url()).searchParams.get('max')==='3000000');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 const paid=new URL((await paidRequest).url()).searchParams;
 expect(paid.get('min')).toBe('2000000');expect(paid.has('budget')).toBe(false);
 await page.getByRole('button',{name:'Убрать фильтр: От',exact:true}).click();
 await expect(min).toHaveValue('');await expect(max).toHaveValue('30000');
});

test('free-only links, pagination, history, chip removal and reset stay in sync',async({page})=>{
 await page.goto('/courses?budget=free&min=50000&max=100000&page_size=1');
 const free=page.getByRole('checkbox',{name:'Бесплатно',exact:true});
 await expect(free).toBeChecked();await expect(page.getByLabel('От, ₽',{exact:true})).toBeDisabled();
 await expect(page.locator('.card')).toHaveCount(1);
 expect(new URL(page.url()).searchParams.has('min')).toBe(false);
 expect(new URL(page.url()).searchParams.has('max')).toBe(false);
 await page.getByRole('button',{name:'Далее →'}).click();
 await expect(page).toHaveURL(/page=2/);await expect(free).toBeChecked();
 await expect(page.locator('.card .price')).toHaveText('Бесплатно');
 await page.reload();await expect(free).toBeChecked();
 await page.getByRole('button',{name:'Убрать фильтр: Цена'}).click();
 await expect(free).not.toBeChecked();await expect(page.getByLabel('До, ₽',{exact:true})).toBeEnabled();
 expect(new URL(page.url()).searchParams.has('budget')).toBe(false);
 await page.goBack();await expect(free).toBeChecked();
 await page.getByRole('button',{name:'Сбросить',exact:true}).click();
 await expect(free).not.toBeChecked();await expect(page.getByLabel('От, ₽',{exact:true})).toHaveValue('');
 await expect(page).toHaveURL('/courses');
});

for(const width of [390,1440])test(`free-only control fits and works with keyboard at ${width}px`,async({page})=>{
 await page.setViewportSize({width,height:1000});await page.emulateMedia({colorScheme:'dark'});
 await page.goto('/courses');
 const free=page.getByRole('checkbox',{name:'Бесплатно',exact:true});
 await free.focus();await page.keyboard.press('Space');
 await expect(free).toBeChecked();await expect(page.getByLabel('До, ₽',{exact:true})).toBeDisabled();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
 await page.locator('#catalog-filters').screenshot({path:`test-results/free-budget-${width}.png`});
});
