import {test,expect} from '@playwright/test';

for(const width of [390,1440])test(`hero language shortcuts lead to route filters at ${width}px and preserve search criteria`,async({page})=>{
 await page.setViewportSize({width,height:844});
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/courses?goal=try&sort=price_asc&page=2');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await expect(page.getByRole('complementary',{name:'Ваш маршрут'})).toBeVisible();
 await expect(page.getByRole('button',{name:/^(Скрыть фильтры|Фильтры)/})).toHaveCount(0);
 await expect(page.getByRole('link',{name:'Посмотреть программы'})).toHaveCount(0);
 await page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true}).click();
 expect(new URL(page.url()).searchParams.has('language')).toBe(false);
 await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
 await expect.poll(()=>page.locator('#catalog-filters').evaluate(node=>Math.round(node.getBoundingClientRect().top))).toBe(24);
 await expect(page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true})).toHaveAttribute('aria-pressed','true');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await page.getByRole('button',{name:'Найти обучение'}).click();
 await expect(page).toHaveURL(/language=python/);
 expect(new URL(page.url()).searchParams.get('goal')).toBe('try');
 expect(new URL(page.url()).searchParams.get('sort')).toBe('price_asc');
 expect(new URL(page.url()).searchParams.has('page')).toBe(false);
 await page.goBack();await expect(page.getByRole('radio',{name:'Все языки',exact:true})).toBeChecked();
});
