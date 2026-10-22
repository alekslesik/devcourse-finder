'use client';
import {useEffect,useState} from 'react';
import {catalogContext,requestCatalogRestore} from '../lib/catalog-navigation';

export default function CatalogReturn({label,current}:{label?:string;current?:boolean}={}) {
  const [url,setUrl]=useState('/courses');
  const [hasContext,setHasContext]=useState(false);
  useEffect(()=>{const context=catalogContext();if(context){setUrl(context.url);setHasContext(true);}},[]);
  return <a href={url} aria-current={current?'page':undefined} onClick={()=>{if(hasContext)requestCatalogRestore();}}>{label||('← '+(hasContext?'Вернуться к подбору':'Каталог'))}</a>;
}
