import type {MetadataRoute} from 'next';
import type {Response} from '../lib/catalog';
import {siteOrigin} from '../lib/social-metadata';

export const dynamic='force-dynamic';
export default async function sitemap():Promise<MetadataRoute.Sitemap>{
 const origin=process.env.API_URL||'http://api:8080';
 // /courses is a noindex copy of the interactive home catalog.
 const entries:MetadataRoute.Sitemap=[
  {url:new URL('/',siteOrigin).href},
  {url:new URL('/about',siteOrigin).href},
 ];
 const seen=new Set<string>();
 let page=1;
 do{
  const response=await fetch(origin+'/api/v1/courses?include_closed=true&page_size=48&page='+page,{cache:'no-store'});
  if(!response.ok)throw Error('Catalog unavailable');
  const data:Response=await response.json();
  for(const {course} of data.items||[]){
   if(course.status!=='published'||course.demo||seen.has(course.slug))continue;
   seen.add(course.slug);
   entries.push({url:new URL('/courses/'+encodeURIComponent(course.slug),siteOrigin).href,lastModified:course.checked_at});
  }
  if(page*48>=data.total)break;
  page++;
 }while(true);
 return entries;
}
