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
