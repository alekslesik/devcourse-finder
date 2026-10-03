import {cache} from 'react';
import {notFound} from 'next/navigation';
import type {Course,Item,ComparisonItem} from './catalog';
const origin=process.env.API_URL||'http://api:8080';
export const course=cache(async(slug:string):Promise<{course:Course;offers:Item[]}>=>{
 const response=await fetch(origin+'/api/v1/courses/'+encodeURIComponent(slug),{cache:'no-store'});
 if(response.status===404)notFound();
 if(!response.ok)throw Error('Каталог временно недоступен');
 const data=await response.json();if(data.course.status!=='published')notFound();return data;
});
export async function comparison(ids:string[]):Promise<ComparisonItem[]>{
 const response=await fetch(origin+'/api/v1/compare?'+new URLSearchParams({offer_ids:ids.join(',')}),{cache:'no-store'});
 if(!response.ok)throw Error('Сравнение временно недоступно');return response.json();
}
