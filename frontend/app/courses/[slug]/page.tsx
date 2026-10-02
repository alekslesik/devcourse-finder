import type {Metadata} from 'next';
import {notFound} from 'next/navigation';

type Course={id:string;slug:string;title:string;provider:string;language:string;summary:string;topics:string[];audience:string[];checked_at:string;status:string;demo:boolean};
type Offer={id:string;name:string;price:number|null;price_kind:string;price_checked_at:string;hours:number|null;weeks:number|null;review:boolean;mentor:boolean;schedule:string;enrollment:string};
type Item={offer:Offer;effective_price:number|null};
type CourseResponse={course:Course;offers:Item[]};

const api=process.env.API_URL||'http://api:8080';
const languageNames:Record<string,string>={go:'Go',python:'Python',java:'Java',javascript:'JavaScript'};
const audienceNames:Record<string,string>={none:'Начинающим с нуля',basics:'Тем, кто знает основы',projects:'Разработчикам с учебными проектами',working:'Работающим разработчикам',switch:'Переходящим с другого языка'};
const enrollmentNames:Record<string,string>={open:'Набор открыт',continuous:'Можно начать в любое время',closed:'Набор закрыт',unknown:'Статус набора уточняется'};

async function loadCourse(slug:string):Promise<CourseResponse>{
 const response=await fetch(`${api}/api/v1/courses/${encodeURIComponent(slug)}`,{cache:'no-store'});
 if(response.status===404)notFound();
 if(!response.ok)throw new Error('Не удалось загрузить программу');
 return response.json();
}

function formatDate(value:string){return new Intl.DateTimeFormat('ru-RU',{dateStyle:'long'}).format(new Date(value))}
function support(offer:Offer){return offer.mentor?'Персональный наставник':offer.review?'Проверка заданий':'Самостоятельное обучение'}
function price(item:Item){
 if(item.effective_price!==null)return item.effective_price===0?'Бесплатно':new Intl.NumberFormat('ru-RU',{style:'currency',currency:'RUB',maximumFractionDigits:0}).format(item.effective_price/100);
 const stale=item.offer.price!==null&&Date.now()-new Date(item.offer.price_checked_at).getTime()>30*24*60*60*1000;
 if(stale)return 'Цена устарела — уточните у источника';
 if(item.offer.price_kind==='from')return 'Цена указана «от» — уточните полную стоимость';
 return 'Цена неизвестна — уточните у источника';
}

export async function generateMetadata({params}:{params:Promise<{slug:string}>}):Promise<Metadata>{
 const {slug}=await params;
 const data=await loadCourse(slug);
 return {title:`${data.course.title} — ${data.course.provider} | DevCourse`,description:data.course.summary.slice(0,160),alternates:{canonical:`/courses/${data.course.slug}`}};
}

export default async function CoursePage({params}:{params:Promise<{slug:string}>}){
 const {slug}=await params;
 const {course,offers}=await loadCourse(slug);
 return <><header><a className="brand" href="/"><span className="brandIcon">&lt;/&gt;</span>devcourse<span className="brandDot">.</span></a><span className="headerNote">Ваш путь в разработку</span><a href="/courses" className="quiet">← Вернуться к каталогу</a></header>
  <main className="coursePage"><nav className="breadcrumbs" aria-label="Хлебные крошки"><a href="/courses">Каталог</a><span aria-hidden="true">/</span><span>{course.title}</span></nav><article className="courseDetail">
   <div className="eyebrow">{languageNames[course.language]||course.language} · {course.provider}</div><h1>{course.title}</h1>
   {course.demo&&<div className="demoBanner"><strong>Демонстрационная программа.</strong> Условия и цены вымышлены и предназначены для проверки сервиса.</div>}<p className="courseLead">{course.summary}</p>
   <div className="courseColumns"><section><h2>Кому подойдёт</h2><ul>{course.audience.map(value=><li key={value}>{audienceNames[value]||value}</li>)}</ul></section><section><h2>Что будете изучать</h2><div className="topics">{course.topics.map(topic=><span key={topic}>{topic}</span>)}</div></section></div>
   <section aria-labelledby="offers-heading"><div className="detailHeading"><div><div className="eyebrow">УСЛОВИЯ ОБУЧЕНИЯ</div><h2 id="offers-heading">Тарифы</h2></div><span>Проверено {formatDate(course.checked_at)}</span></div><div className="detailOffers">{offers.map(item=><article key={item.offer.id}>
    <h3>{item.offer.name}</h3><strong className="price">{price(item)}</strong><dl><div><dt>Поддержка</dt><dd>{support(item.offer)}</dd></div><div><dt>Срок</dt><dd>{item.offer.weeks?`${item.offer.weeks} нед.`:'Не указан'}</dd></div><div><dt>Нагрузка</dt><dd>{item.offer.hours?`${item.offer.hours} ч/нед.`:'Не указана'}</dd></div><div><dt>Набор</dt><dd>{enrollmentNames[item.offer.enrollment]||'Уточняется'}</dd></div></dl>
    {item.offer.enrollment==='closed'?<p className="unavailableOffer">Тариф сейчас недоступен для записи и сравнения.</p>:<a className="secondary linkButton" href={`/compare?offers=${encodeURIComponent(item.offer.id)}`}>Добавить в сравнение</a>}
    {item.offer.enrollment!=='closed'&&<a className="primary linkButton" href={`/out/${encodeURIComponent(item.offer.id)}`} target="_blank" rel="noopener noreferrer">Перейти к источнику ↗</a>}
   </article>)}</div></section>
   <p className="sourceNote">Данные проверены {formatDate(course.checked_at)}. Перед записью уточните цену, программу и статус набора на сайте источника. DevCourse не гарантирует трудоустройство.</p>
  </article></main></>;
}
