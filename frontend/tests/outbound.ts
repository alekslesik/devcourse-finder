import {expect,type Page} from '@playwright/test';

export async function expectOutboundPopup(page:Page,providerURL:string,click:()=>Promise<void>){
 const context=page.context();
 const stubURL=new URL('/about?provider-stub=1',page.url()).href;
 // Playwright routes intercept the first URL of a redirect chain. Fetch the
 // real /out response without following it, verify the provider destination,
 // then let the browser follow its 302 to a local page instead of the Internet.
 await context.route('**/out/**',async route=>{
  const response=await route.fetch({maxRedirects:0});
  expect(response.status()).toBe(302);
  expect(response.headers()['location']).toBe(providerURL);
  await route.fulfill({response,headers:{...response.headers(),location:stubURL}});
 });
 const popupPromise=page.waitForEvent('popup');
 await click();
 const popup=await popupPromise;
 await expect(popup).toHaveURL(stubURL);
 await expect(popup.getByRole('heading',{level:1})).toBeVisible();
 await popup.close();
 await context.unroute('**/out/**');
}
