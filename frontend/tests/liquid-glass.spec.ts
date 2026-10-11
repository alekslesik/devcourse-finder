import {test,expect} from '@playwright/test';

for(const width of [390,1440])test(`hero language shortcuts lead to route filters at ${width}px and preserve search criteria`,async({page})=>{
 await page.setViewportSize({width,height:844});
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses?goal=try&sort=price_asc&page=2');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await expect(page.getByRole('complementary',{name:'Ваш маршрут'})).toBeVisible();
 await expect(page.getByRole('button',{name:/^(Скрыть фильтры|Фильтры)/})).toHaveCount(0);
 await expect(page.getByRole('link',{name:'Посмотреть программы'})).toHaveCount(0);
 const request=page.waitForRequest(r=>r.url().includes('/api/v1/courses?')&&new URL(r.url()).searchParams.get('language')==='python');
 await page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true}).click();
 const query=new URL((await request).url()).searchParams;
 expect(query.get('goal')).toBe('try');expect(query.get('sort')).toBe('price_asc');expect(query.has('page')).toBe(false);
 await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
 await expect.poll(()=>page.locator('#catalog-filters').evaluate(node=>Math.round(node.getBoundingClientRect().top))).toBe(24);
 await expect(page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true})).toHaveAttribute('aria-pressed','true');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await page.goBack();await expect(page.getByRole('radio',{name:'Все языки',exact:true})).toBeChecked();
});
