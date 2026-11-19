import 'server-only';
import {randomBytes} from 'node:crypto';
import {isIP} from 'node:net';
import {NextRequest,NextResponse} from 'next/server';
import {actionSignature} from '../../../../../lib/account-proxy.mjs';
export const runtime='nodejs';
export const dynamic='force-dynamic';
const cookieName='__Host-dcf-action-csrf';
const actions=new Set(['csrf','request-confirmation','request-reset','confirm-email','reset-password']);
const privateHeaders={'Cache-Control':'private, no-store, max-age=0','Referrer-Policy':'no-referrer','X-Content-Type-Options':'nosniff','X-Frame-Options':'DENY'};
const failure=(status:number,code:string,message:string)=>NextResponse.json({code,message},{status,headers:privateHeaders});
async function proxy(request:NextRequest,{params}:{params:Promise<{action:string}>}){
 if(process.env.AUTH_ACTIONS_ENABLED!=='true')return failure(404,'not_found','Страница не найдена.');
 const {action}=await params;const path='/api/v1/account/'+action;
 if(!actions.has(action)||request.nextUrl.pathname!==path||request.nextUrl.search)return failure(404,'not_found','Страница не найдена.');
 if((action==='csrf'&&request.method!=='GET')||(action!=='csrf'&&request.method!=='POST'))return failure(405,'method_not_allowed','Действие требует явного подтверждения.');
 try{
  const publicOrigin=new URL(process.env.AUTH_PUBLIC_ORIGIN??'');
  if(publicOrigin.protocol!=='https:'||publicOrigin.username||publicOrigin.password||publicOrigin.search||publicOrigin.hash||!['','/'].includes(publicOrigin.pathname))throw Error();
  const origin=request.headers.get('origin')??'';
  const site=request.headers.get('sec-fetch-site')??'';
  if((site&&site!=='same-origin')||(origin&&origin!==publicOrigin.origin)||(request.method==='POST'&&origin!==publicOrigin.origin))return failure(403,'invalid_origin','Откройте форму на сайте сервиса.');
  const encoded=process.env.AUTH_GATEWAY_SECRET??'';const key=Buffer.from(encoded,'base64');
  if(key.length!==32||key.toString('base64')!==encoded)throw Error();
  const target=new URL(process.env.API_URL??'http://api:8080');
  if(target.username||target.password||target.search||target.hash||!['','/'].includes(target.pathname)||(!['api','localhost','127.0.0.1','[::1]'].includes(target.hostname)&&target.protocol!=='https:')||!['http:','https:'].includes(target.protocol))throw Error();
  let body=Buffer.alloc(0);
  if(request.method==='POST'){
   const reader=request.body?.getReader();const chunks:Uint8Array[]=[];let length=0;
   if(reader)try{for(;;){const {done,value}=await reader.read();if(done)break;length+=value.byteLength;if(length>8192){await reader.cancel();return failure(413,'invalid_request','Запрос слишком большой.');}chunks.push(value);}}finally{reader.releaseLock();}
   body=Buffer.concat(chunks);
  }
  const candidate=(request.headers.get('x-forwarded-for')??'').split(',').at(-1)?.trim()??'';
  const ip=process.env.AUTH_TRUSTED_INGRESS==='true'&&isIP(candidate)?candidate:'unknown';
  const time=Math.floor(Date.now()/1000).toString(),nonce=randomBytes(16).toString('hex');
  const csrf=request.headers.get('x-csrf-token')??'';
  const cookie=request.cookies.get(cookieName)?.value??'';
  if(csrf.length>100||cookie.length>100)return failure(403,'invalid_csrf','Обновите форму и повторите попытку.');
  const headers=new Headers({'X-DCF-Action-Time':time,'X-DCF-Action-Nonce':nonce,'X-DCF-Action-IP':ip,'X-DCF-Action-Signature':actionSignature(key,{method:request.method,path,time,nonce,ip,origin,csrf,cookie},body)});
  if(origin)headers.set('Origin',origin);if(site)headers.set('Sec-Fetch-Site',site);if(csrf)headers.set('X-CSRF-Token',csrf);if(cookie)headers.set('Cookie',cookieName+'='+cookie);
  if(request.method==='POST')headers.set('Content-Type',request.headers.get('content-type')??'');
  const response=await fetch(new URL(path,target),{method:request.method,headers,body:request.method==='POST'?new Uint8Array(body):undefined,cache:'no-store',redirect:'error',signal:AbortSignal.timeout(15000)});
  const outgoing=new Headers(privateHeaders);outgoing.set('Content-Type','application/json; charset=utf-8');
  const setCookie=response.headers.get('set-cookie');if(setCookie?.startsWith(cookieName+'='))outgoing.set('Set-Cookie',setCookie);
  const retry=response.headers.get('retry-after');if(retry&&/^\d{1,4}$/.test(retry))outgoing.set('Retry-After',retry);
  return new NextResponse(response.body,{status:response.status,headers:outgoing});
 }catch{return failure(503,'unavailable','Сервис временно недоступен. Попробуйте позже.');}
}
export const GET=proxy;export const POST=proxy;
