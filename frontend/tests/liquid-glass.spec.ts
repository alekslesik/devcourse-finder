import {test,expect} from '@playwright/test';

test('hero language shortcuts apply live API filters and preserve the search criteria',async({page})=>{
 await page.goto('/courses?goal=try&sort=price_asc&page=2');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 const request=page.waitForRequest(r=>r.url().includes('/api/v1/courses?')&&new URL(r.url()).searchParams.get('language')==='python');
 await page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true}).click();
 const query=new URL((await request).url()).searchParams;
 expect(query.get('goal')).toBe('try');expect(query.get('sort')).toBe('price_asc');expect(query.has('page')).toBe(false);
 await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
 await expect(page.getByRole('group',{name:'Выберите язык разработки'}).getByRole('button',{name:'Python',exact:true})).toHaveAttribute('aria-pressed','true');
 await expect(page.locator('.results')).toHaveAttribute('aria-busy','false');
 await page.goBack();await expect(page.getByRole('radio',{name:'Все языки',exact:true})).toBeChecked();
});
