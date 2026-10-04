import {test,expect} from '@playwright/test';

type DelayedSearch={waiting:boolean;release:()=>void};
type RaceWindow=typeof window & {delayedSearch:DelayedSearch};

for(const outcome of ['success','http-error','processing-error'] as const){
 test(`AC-17: late ${outcome} from the first search cannot replace the second`,async({page})=>{
  await page.addInitScript(outcome=>{
   const nativeFetch=window.fetch.bind(window);
   let release!:()=>void;
   const gate=new Promise<void>(resolve=>{release=resolve});
   const control:DelayedSearch={waiting:false,release};
   (window as RaceWindow).delayedSearch=control;
   window.fetch=async(input,init)=>{
    const response=await nativeFetch(input,init);
    const url=new URL(input instanceof Request?input.url:String(input),location.href);
    if(url.pathname!=='/api/v1/courses'||url.searchParams.get('language')!=='go')return response;
    // Keep the real response, but defer its handoff after its body has arrived.
    // Abort cannot undo application processing already queued after transport.
    const body=await response.text();
    control.waiting=true;
    await gate;
    if(outcome==='processing-error')throw Error('Late response processing failed');
    return new Response(body,{status:outcome==='http-error'?503:response.status,headers:response.headers});
   };
  },outcome);
  const staleSearchEvents:string[]=[];
  page.on('request',request=>{
   if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1/events'){
    const event=request.postDataJSON();
    if(event.kind==='search'&&event.language==='go')staleSearchEvents.push(event.id);
   }
  });
  await page.goto('/courses');
  await expect(page.locator('.card')).toHaveCount(12);
  await page.getByRole('radio',{name:'Go',exact:true}).check();
  await page.getByLabel('Ваша цель').selectOption('try');
  await page.getByRole('button',{name:'Найти обучение'}).click();
  await expect.poll(()=>page.evaluate(()=>(window as RaceWindow).delayedSearch.waiting)).toBe(true);
  await expect(page.getByRole('region',{name:'Результаты поиска'})).toHaveAttribute('aria-busy','true');
  await page.getByRole('radio',{name:'Python',exact:true}).check();
  await page.getByRole('button',{name:'Найти обучение'}).click();
  const currentCard=page.locator('.card');
  await expect(currentCard).toHaveCount(1);
  await expect(currentCard.locator('.languageBadge')).toHaveText('Python');
  const currentTitle=await currentCard.getByRole('heading').innerText();
  await page.evaluate(async()=>{
   (window as RaceWindow).delayedSearch.release();
   // Let the deferred promise chain and React's render finish before checking.
   await new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve())));
  });
  await expect(currentCard).toHaveCount(1);
  await expect(currentCard.getByRole('heading')).toHaveText(currentTitle);
  await expect(currentCard.locator('.languageBadge')).toHaveText('Python');
  await expect(page.locator('main').getByRole('alert')).toHaveCount(0);
  await expect(page.getByRole('region',{name:'Результаты поиска'})).toHaveAttribute('aria-busy','false');
  await expect(page.getByRole('radio',{name:'Python',exact:true})).toBeChecked();
  await expect(page.getByLabel('Ваша цель')).toHaveValue('try');
  await expect(page).toHaveURL(/\/courses\?.*language=python/);
  expect(staleSearchEvents).toEqual([]);
 });
}
