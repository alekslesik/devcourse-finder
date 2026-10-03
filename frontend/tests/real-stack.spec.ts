import {test,expect} from '@playwright/test';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const exec=promisify(execFile);
const compose=async(...args:string[])=>{
 const project=process.env.COMPOSE_PROJECT_NAME;
 if(!project?.startsWith('devcourse-e2e-'))throw Error('Real tests require the isolated scripts/e2e-real.sh project');
 return exec('docker',['compose','-f','compose.yaml','-f','compose.e2e.yaml','-p',project,...args],{cwd:'..'});
};

test('real API outage preserves filters and retry recovers after restart',async({page,request})=>{
 test.setTimeout(60000);
 await page.goto('/courses?language=python&goal=try');
 await expect(page.locator('.card')).toHaveCount(1);
 try{
  await compose('stop','api');
  await page.getByRole('button',{name:'Найти обучение'}).click();
  await expect(page.locator('main').getByRole('alert')).toContainText('Каталог временно недоступен',{timeout:20000});
  await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
  await expect(page.getByLabel('Ваша цель')).toHaveValue('try');
 }finally{
  await compose('start','api');
 }
 await expect.poll(async()=>{
  try{return (await request.get('http://127.0.0.1:8091/health/ready')).status()}catch{return 0}
 },{timeout:15000}).toBe(200);
 await page.getByRole('button',{name:'Повторить',exact:true}).click();
 await expect(page.locator('.card')).toHaveCount(1);
 await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
});

test('real database hides draft/archive and persists browser search and outbound events',async({page,context})=>{
 for(const slug of ['draft-course','archived-course','absent-course']){
  const response=await page.goto('/courses/'+slug);
  expect(response?.status()).toBe(404);
 }
 const searchCount=async()=>Number((await compose('exec','-T','db','psql','-U','devcourse','-d','devcourse_e2e_test','-tAc',"SELECT count(*) FROM events WHERE kind='search' AND language='go' AND goal='try'")).stdout.trim());
 const outboundCount=async()=>Number((await compose('exec','-T','db','psql','-U','devcourse','-d','devcourse_e2e_test','-tAc',"SELECT count(*) FROM events WHERE kind='outbound' AND course_id='demo-go-1'")).stdout.trim());
 const searchesBefore=await searchCount();
 await page.goto('/courses?language=go&goal=try');
 await expect(page.locator('.card')).toHaveCount(1);
 await expect.poll(searchCount).toBeGreaterThan(searchesBefore);
 const outboundBefore=await outboundCount();
 await context.route('https://example.com/**',route=>route.fulfill({body:'Provider website'}));
 await page.getByRole('link',{name:'Подробнее'}).click();
 const popupPromise=page.waitForEvent('popup');
 await page.getByRole('link',{name:'На сайт курса'}).click();
 const popup=await popupPromise;
 await expect(popup).toHaveURL('https://example.com/devcourse-demo/demo-go-1/enroll');
 await expect.poll(outboundCount).toBeGreaterThan(outboundBefore);
 await popup.close();
});
