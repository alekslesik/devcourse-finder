import {test,expect,type Page} from '@playwright/test';
import {randomBytes} from 'node:crypto';
import AxeBuilder from '@axe-core/playwright';
const fresh=()=>`v1.${randomBytes(32).toString('base64url')}`;
async function mockActions(page:Page,post:(action:string,body:Record<string,string>)=>Promise<{status:number;code:string}>){
 await page.route('**/api/v1/account/**',async route=>{const action=new URL(route.request().url()).pathname.split('/').at(-1)!;if(action==='csrf'){await route.fulfill({json:{token:fresh()}});return;}const result=await post(action,route.request().postDataJSON());await route.fulfill({status:result.status,json:{code:result.code,message:'Не удалось выполнить действие.'}});});
}
test.describe('controlled account forms',()=>{
 test.skip(process.env.E2E_REAL_API==='1','Production-like stack keeps account actions disabled');
 test('token link is private and opening it never performs an action',async({page},testInfo)=>{
  const bearer=fresh();const calls:string[]=[];let analytics=0;
  await page.route('https://devcoursefinder.ru/**',async route=>{const u=new URL(route.request().url());const response=await route.fetch({url:'http://127.0.0.1:3091'+u.pathname+u.search});await route.fulfill({response});});
  await page.route('https://mc.yandex.ru/**',route=>{analytics++;return route.abort()});
  await mockActions(page,async(action,body)=>{calls.push(action);expect(body.token).toBe(bearer);return {status:200,code:'completed'}});
  const response=await page.goto('https://devcoursefinder.ru/account/confirm-email?ignored=value#token='+bearer);
  expect(response?.headers()['referrer-policy']).toBe('no-referrer');expect(response?.headers()['x-robots-tag']).toContain('noindex');
  const confirm=page.getByRole('button',{name:'Подтвердить email',exact:true});await expect(confirm).toBeVisible();await expect(page).toHaveURL('https://devcoursefinder.ru/account/confirm-email');
  await page.screenshot({path:testInfo.outputPath('confirm-desktop.png'),fullPage:true});
  expect(calls).toHaveLength(0);expect(analytics).toBe(0);expect(await page.locator('noscript img').count()).toBe(0);expect(await page.evaluate(()=>Boolean(window.ym))).toBe(false);
  expect(await page.content()).not.toContain(bearer);expect(await page.evaluate(()=>JSON.stringify([localStorage,sessionStorage]))).not.toContain(bearer);
  await confirm.focus();await page.keyboard.press('Enter');await expect(page.getByRole('heading',{name:'Email подтверждён',exact:true})).toBeVisible();expect(calls).toEqual(['confirm-email']);expect(analytics).toBe(0);
 });
 test('reset validates matching passwords and submits only once',async({page})=>{
  const bearer=fresh(),value=randomBytes(20).toString('hex');let posts=0;let release!:()=>void;const gate=new Promise<void>(resolve=>{release=resolve});
  await mockActions(page,async(action,body)=>{posts++;expect(action).toBe('reset-password');expect(body).toEqual({token:bearer,password:value});await gate;return {status:200,code:'completed'}});
  await page.goto('/account/reset-password#token='+bearer);await page.getByLabel('Новый пароль',{exact:true}).fill(value);await page.getByLabel('Повторите пароль',{exact:true}).fill(value+'x');
  await page.getByRole('button',{name:'Сохранить новый пароль'}).click();await expect(page.locator('#account-error')).toContainText('не совпадают');expect(posts).toBe(0);
  await page.getByLabel('Повторите пароль',{exact:true}).fill(value);await page.locator('form').evaluate(form=>{(form as HTMLFormElement).requestSubmit();(form as HTMLFormElement).requestSubmit()});
  await expect.poll(()=>posts).toBe(1);await expect(page.getByRole('button',{name:'Отправляем…'})).toBeDisabled();release();await expect(page.getByRole('heading',{name:'Пароль изменён'})).toBeVisible();expect(posts).toBe(1);expect(await page.content()).not.toContain(bearer);expect(await page.locator('input[type=password]').count()).toBe(0);
 });
 test('email result stays uniform and mobile form is accessible',async({page},testInfo)=>{
  await page.setViewportSize({width:390,height:844});let posted:Record<string,string>|undefined;await mockActions(page,async(action,body)=>{expect(action).toBe('request-reset');posted=body;return {status:202,code:'accepted'}});
  await page.goto('/account/reset-password');await expect(page.getByLabel('Email',{exact:true})).toBeVisible();await page.getByLabel('Email',{exact:true}).fill('missing@example.test');await page.getByRole('button',{name:'Отправить письмо',exact:true}).click();
  await expect(page.getByRole('status')).toContainText('Если адрес подходит');expect(posted).toEqual({email:'missing@example.test'});await expect(page.getByRole('button',{name:/Отправить ещё раз через/})).toBeDisabled();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  for(const theme of ['light','dark']){await page.evaluate(value=>document.documentElement.setAttribute('data-theme',value),theme);await page.screenshot({path:testInfo.outputPath('reset-mobile-'+theme+'.png'),fullPage:true});const result=await new AxeBuilder({page}).analyze();expect(result.violations).toEqual([]);}
 });
 test('expired reset returns to email request and clears the bearer',async({page})=>{
  const bearer=fresh(),value=randomBytes(20).toString('hex');await mockActions(page,async()=>({status:400,code:'invalid_link'}));await page.goto('/account/reset-password#token='+bearer);
  await page.getByLabel('Новый пароль',{exact:true}).fill(value);await page.getByLabel('Повторите пароль',{exact:true}).fill(value);await page.getByRole('button',{name:'Сохранить новый пароль'}).click();await expect(page.locator('#account-error')).toContainText('устарела');await expect(page.getByLabel('Email',{exact:true})).toBeVisible();expect(await page.content()).not.toContain(bearer);
 });
 test('frontend rejects cross-origin requests before proxying',async({request})=>{const response=await request.post('/api/v1/account/request-reset',{headers:{Origin:'https://evil.example'},data:{email:'missing@example.test'}});expect(response.status()).toBe(403)});
});
test('production-like stack leaves public account actions disabled',async({page,request})=>{test.skip(process.env.E2E_REAL_API!=='1','Real stack only');const response=await page.goto('/account/reset-password');expect(response?.status()).toBe(404);const api=await request.get('/api/v1/account/csrf');expect(api.status()).toBe(404)});
