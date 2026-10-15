import {chromium,test,expect,type BrowserContext} from '@playwright/test';
import {mkdtemp,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

type TabZoomAPI={tabs:{
 query:(query:object)=>Promise<Array<{id:number;url?:string}>>;
 setZoom:(id:number,factor:number)=>Promise<void>;
 getZoom:(id:number)=>Promise<number>;
}};

for(const theme of ['light','dark'] as const)test(`actual 200% tab zoom preserves keyboard search and three-offer comparison: ${theme}`,async({browserName},info)=>{
 test.skip(browserName!=='chromium','The test controller uses the Chromium extensions API.');
 const dir=await mkdtemp(join(tmpdir(),'catalog-tab-zoom-'));
 let context:BrowserContext|undefined;
 try{
  // A test-only extension page controls browser zoom without a background worker.
  // No CSS zoom, emulated viewport or deviceScaleFactor stands in for tab zoom.
  await writeFile(join(dir,'manifest.json'),JSON.stringify({manifest_version:3,name:'Acceptance tab zoom',version:'1.0',permissions:['tabs']}));
  await writeFile(join(dir,'zoom.html'),'<!doctype html><title>Acceptance zoom controller</title>');
  context=await chromium.launchPersistentContext(join(dir,'profile'),{
   channel:'chromium',executablePath:process.env.CHROMIUM_PATH||chromium.executablePath(),headless:true,viewport:null,colorScheme:theme,reducedMotion:'reduce',
   args:[`--disable-extensions-except=${dir}`,`--load-extension=${dir}`,'--window-size=1440,1000'],
  });
  const controller=await context.newPage();
  await controller.goto('chrome://extensions');
  const extension=controller.locator('extensions-item').filter({hasText:'Acceptance tab zoom'});
  try{await expect(extension).toHaveCount(1)}catch(error){
   // Some system builds disable command-line extension loading. Report the
   // missing capability explicitly; CI's bundled Chromium must never skip it.
   test.skip(!!process.env.CHROMIUM_PATH,'Selected system Chromium cannot load the tab-zoom extension; use the Playwright Chromium build or verify actual zoom manually.');
   throw error;
  }
  const id=await extension.getAttribute('id');
  await controller.goto(`chrome-extension://${id}/zoom.html`);
  const page=await context.newPage();await page.goto('http://127.0.0.1:3091/courses');
  await expect(page.locator('.card')).toHaveCount(12);
  const before=await page.evaluate(()=>({width:innerWidth,dpr:devicePixelRatio}));
  const zoom=await controller.evaluate(async()=>{
   const {tabs}=(globalThis as unknown as {chrome:TabZoomAPI}).chrome;
   const tab=(await tabs.query({})).find(t=>t.url?.startsWith('http://127.0.0.1:3091/courses'));
   if(!tab)throw new Error('Catalog tab not found');
   await tabs.setZoom(tab.id,2);return tabs.getZoom(tab.id);
  });
  expect(zoom).toBe(2);
  await expect.poll(()=>page.evaluate(()=>innerWidth)).toBe(Math.round(before.width/2));
  const after=await page.evaluate(()=>({width:innerWidth,dpr:devicePixelRatio}));
  expect(after.dpr).toBe(before.dpr*2);
  await expect(page.locator('html')).toHaveAttribute('data-theme',theme);
  const noOverflow=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await noOverflow();
  await page.getByRole('radio',{name:'Go',exact:true}).focus();await page.keyboard.press('Space');
  await page.getByRole('button',{name:'Найти обучение'}).focus();await page.keyboard.press('Enter');
  await expect(page.locator('.card')).toHaveCount(5);
  await expect(page.locator('.resultHeading')).toBeFocused();
  for(let i=0;i<3;i++){
   const choice=page.getByLabel('Сравнить тариф').nth(i);
   await choice.focus();await page.keyboard.press('Space');await expect(choice).toBeChecked();
  }
  const compare=page.getByRole('button',{name:'Сравнить →'});
  await compare.focus();await expect(compare).toBeInViewport();await page.keyboard.press('Enter');
  const region=page.getByRole('region',{name:'Сравнение тарифов, горизонтальная прокрутка'});
  await region.focus();await expect(region).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect.poll(()=>region.evaluate(node=>node.scrollLeft)).toBeGreaterThan(0);
  await noOverflow();
  await page.screenshot({path:info.outputPath(`comparison-200-${theme}.png`),fullPage:true});
  await page.getByRole('link',{name:'← Вернуться к подбору'}).focus();await page.keyboard.press('Enter');
  await expect(page.locator('.card')).toHaveCount(5);
  expect(new URL(page.url()).searchParams.get('language')).toBe('go');
  await page.locator('.card .titleButton').first().focus();await page.keyboard.press('Enter');
  await expect(page.locator('.courseSummary')).toBeVisible();
  await noOverflow();
  await page.getByRole('link',{name:'← Вернуться к подбору'}).focus();await page.keyboard.press('Enter');
  await expect(page.locator('.card')).toHaveCount(5);await noOverflow();
  const measurement=info.outputPath('actual-tab-zoom.json');
  await writeFile(measurement,JSON.stringify({theme,zoom,before,after},null,2));
  await info.attach('actual-tab-zoom.json',{path:measurement,contentType:'application/json'});
 }finally{await context?.close();await rm(dir,{recursive:true,force:true})}
});
