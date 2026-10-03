import type {MetadataRoute} from 'next';
import type {Response} from '../lib/catalog';
export const dynamic='force-dynamic';
export default async function sitemap():Promise<MetadataRoute.Sitemap>{
 const site=process.env.SITE_URL||'http://localhost:3000';
 const origin=process.env.API_URL||'http://api:8080';
 const entries:MetadataRoute.Sitemap=[{url:site},{url:site+'/courses'},{url:site+'/about'}];
 let page=1;
 do{const response=await fetch(origin+'/api/v1/courses?include_closed=true&page_size=48&page='+page,{cache:'no-store'});if(!response.ok)throw Error('Catalog unavailable');const data:Response=await response.json();for(const item of data.items||[])entries.push({url:site+'/courses/'+encodeURIComponent(item.course.slug),lastModified:item.course.checked_at});if(page*48>=data.total)break;page++}while(true);
 return entries;
}
