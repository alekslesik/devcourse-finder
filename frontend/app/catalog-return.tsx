'use client';
import {useEffect,useState} from 'react';
import {catalogContext,requestCatalogRestore} from '../lib/catalog-navigation';

export default function CatalogReturn() {
  const [url,setUrl]=useState('/courses');
  const [hasContext,setHasContext]=useState(false);
  useEffect(()=>{const context=catalogContext();if(context){setUrl(context.url);setHasContext(true);}},[]);
  return <a href={url} onClick={()=>{if(hasContext)requestCatalogRestore();}}>← {hasContext?'Вернуться к подбору':'Каталог'}</a>;
}
