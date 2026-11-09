'use client';

import {useEffect} from 'react';
import {usePathname,useSearchParams} from 'next/navigation';
import {metrikaHost,metrikaId,metrikaTagURL} from '../lib/metrika';

type Metrika = ((...args:unknown[])=>void)&{a?:unknown[][];l?:number};
declare global {interface Window {ym?:Metrika}}

// The root layout persists across navigation. Keep initialization idempotent
// even when React replays an effect or the component remounts.
let lastURL:string|undefined;

export default function YandexMetrika(){
 const pathname=usePathname(),searchParams=useSearchParams();
 useEffect(()=>{
  if(location.hostname.replace(/^www\./,'')!==metrikaHost)return;
  try{
   if(!window.ym){
    const queue:Metrika=(...args)=>{(queue.a??=[]).push(args)};
    queue.l=Date.now();window.ym=queue;
   }
   const url=location.href;
   if(lastURL===undefined){
    window.ym(metrikaId,'init',{ssr:true,webvisor:true,clickmap:true,ecommerce:'dataLayer',referrer:document.referrer,url,accurateTrackBounce:true,trackLinks:true});
    lastURL=url;
    if(!Array.from(document.scripts).some(script=>script.src===metrikaTagURL)){
     const script=document.createElement('script');
     script.id='yandex-metrika';script.async=true;script.src=metrikaTagURL;
     document.head.appendChild(script);
    }
   }else if(lastURL!==url){
    window.ym(metrikaId,'hit',url,{referer:lastURL});lastURL=url;
   }
  }catch{
   // Analytics failures must not interrupt catalog navigation.
  }
 },[pathname,searchParams]);
 return null;
}
