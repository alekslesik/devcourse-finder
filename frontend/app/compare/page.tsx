import type {Metadata} from 'next';
import {socialMetadata} from '../../lib/social-metadata';
import Selection from './selection';
import {comparison} from '../../lib/api';
import ComparisonTable from '../comparison-table';
export const dynamic='force-dynamic';
export const metadata:Metadata={...socialMetadata('Сравнение тарифов — DevCourse','Сравнение условий выбранных тарифов обучения.','/compare'),robots:{index:false,follow:true}};
export default async function ComparePage({searchParams}:{searchParams:Promise<{offers?:string|string[]}>}){
 const raw=(await searchParams).offers;const ids=[...new Set((typeof raw==='string'?raw:'').split(',').filter(Boolean))];
 if(!ids.length||ids.length>3)return <main className="document">{ids.length<=3&&<Selection ids={ids}/>}<h1>Сравнение тарифов</h1><p>{ids.length>3?'Можно сравнить максимум три тарифа.':'Выберите от одного до трёх тарифов в каталоге.'}</p><a href="/courses">Вернуться в каталог</a></main>;
 const items=await comparison(ids);
 return <main className="document"><a href="/courses">← Каталог</a><Selection ids={ids}/><h1>Сравнение тарифов</h1><p>Скопируйте адрес страницы, чтобы поделиться сравнением. На узком экране таблицу можно прокрутить горизонтально.</p><ComparisonTable items={items} ids={ids}/></main>;
}
