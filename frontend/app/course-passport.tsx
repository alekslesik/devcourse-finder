'use client';
import {useEffect,useRef,useState} from 'react';
import type {ReactNode} from 'react';
import {checked,duration,price,profiles,supportText,supportValue,workload,type Course,type Item} from '../lib/catalog';
import CatalogActions from './catalog-actions';
import CourseIcon from './course-icon';
import ExternalMark from './external-mark';
import ProviderName from './provider-name';

const languages:Record<string,string>={go:'Go',python:'Python',java:'Java',javascript:'JavaScript'};
const schedule:Record<string,string>={flexible:'В своём темпе',scheduled:'По расписанию',unknown:'Уточните у школы'};
const enrollment:Record<string,string>={open:'Открыт',continuous:'Постоянный',closed:'Закрыт',unknown:'Уточните у школы'};
type PassportCourse=Pick<Course,'title'|'provider'|'language'|'summary'|'audience'|'profile'|'topics'|'source'|'checked_at'|'demo'>;
type PassportItem=Pick<Item,'offer'|'effective_price'>;
function Fact({label,children,unknown=false}:{label:string;children:ReactNode;unknown?:boolean}){
 return <div className="courseFact"><dt>{label}</dt><dd>{unknown&&<CourseIcon kind="unknown"/>}<span>{children}</span></dd></div>;
}

export default function CoursePassport({course:c,offers,initialOfferId}:{course:PassportCourse;offers:PassportItem[];initialOfferId?:string}){
 const [offerId,setOfferId]=useState(initialOfferId||offers[0]?.offer.id||'');
 const [notice,setNotice]=useState('');
 const [section,setSection]=useState('course-overview');
 const [showDock,setShowDock]=useState(false);
 const primaryAction=useRef<HTMLAnchorElement>(null);
 const item=offers.find(i=>i.offer.id===offerId)||offers[0];
 const offer=item?.offer;
 const canCompare=!!offer&&['open','continuous'].includes(offer.enrollment);
 const actionLabel=canCompare?'На сайт курса':'Проверить набор';
 const courseDuration=offer?duration(offer):null;
 const courseWorkload=offer?workload(offer):null;
 const audience=(c.profile?.audience?.values||c.audience).filter(v=>v!=='unknown');
 const experience=audience.length?audience.map(v=>profiles[v]||v).join(' · '):'Аудитория не подтверждена';
 const prerequisite=c.profile?.prerequisites;
 const requirement=prerequisite?({none:'Предварительный опыт не нужен',basics:'Знание основ',projects:'Опыт создания проектов',working:'Опыт работы разработчиком'}[prerequisite.level]+(prerequisite.level==='none'?'':prerequisite.scope==='course_language'?' на языке курса':' в программировании')):'Требования к опыту не подтверждены';
 useEffect(()=>{
  if(!('IntersectionObserver' in window)||!primaryAction.current)return;
  const observer=new IntersectionObserver(([entry])=>setShowDock(!entry.isIntersecting&&entry.boundingClientRect.bottom<0));
  observer.observe(primaryAction.current);return ()=>observer.disconnect();
 },[offer?.id]);
 useEffect(()=>{
  const nodes=['course-overview','course-conditions','course-sources'].map(id=>document.getElementById(id)).filter((node):node is HTMLElement=>!!node);
  let frame=0;
  function update(){
   frame=0;
   const atEnd=scrollY+innerHeight>=document.documentElement.scrollHeight-2;
   const current=atEnd?nodes.at(-1):[...nodes].reverse().find(node=>node.getBoundingClientRect().top<=innerHeight*.25)||nodes[0];
   if(current)setSection(current.id);
  }
  const queue=()=>{if(!frame)frame=requestAnimationFrame(update)};
  window.addEventListener('scroll',queue,{passive:true});window.addEventListener('resize',queue);update();
  return ()=>{cancelAnimationFrame(frame);window.removeEventListener('scroll',queue);window.removeEventListener('resize',queue)};
 },[]);
 function selectOffer(id:string){
  const next=offers.find(i=>i.offer.id===id);if(!next)return;
  setOfferId(id);setShowDock(false);setNotice(next.offer.name+'. '+price(next));
  const url=new URL(location.href);url.searchParams.set('offer',id);history.replaceState(history.state,'',url);
 }
 return <>
  <div className="courseLayout">
   <section className="courseIntro" id="course-overview" aria-labelledby="course-title" tabIndex={-1}>
    <div className="courseIdentity"><ProviderName name={c.provider}/><span className={'languageBadge '+c.language}>{languages[c.language]||c.language}</span></div>
    <h1 id="course-title">{c.title}</h1><p className="courseLead">{c.summary}</p>
    {c.demo&&<p className="demoBanner">Демонстрационная программа: название и условия вымышлены.</p>}
   </section>
   <aside className="courseTariffs" data-single={offers.length<=1} id="course-tariffs" tabIndex={-1} aria-label="Тарифы курса">
    <fieldset className="courseTariffList"><legend className="srOnly">Тарифы курса</legend>
    {offers.map(entry=>{
     const tariff=entry.offer,selected=tariff.id===offer?.id,comparable=['open','continuous'].includes(tariff.enrollment);
     return <section key={tariff.id} className="courseSummary coursePurchase" data-offer={tariff.id} data-selected={selected} aria-labelledby={'tariff-'+tariff.id}>
     <h2 id={'tariff-'+tariff.id}>{offers.length>1?<label className="courseTariffChoice"><input type="radio" name="course-offer" value={tariff.id} checked={selected} onChange={()=>selectOffer(tariff.id)} aria-controls="course-conditions"/><span>{tariff.name}</span></label>:tariff.name}</h2>
     <div className="coursePriceBlock"><strong className="coursePurchasePrice" data-unconfirmed={entry.effective_price===null}>{price(entry)}</strong><span>{entry.effective_price===null?'Полная стоимость уточняется':entry.effective_price===0?'Бесплатный тариф':'за весь курс'}</span></div>
     {entry.effective_price===null&&<p className="coursePriceNote">{tariff.price_kind==='from'?'Цена «от»: уточните полную стоимость у школы.':'Актуальная полная стоимость не подтверждена. Уточните её у школы.'}</p>}
     <ul className="coursePurchaseFacts">
      <li><CourseIcon kind="clock"/><span>{duration(tariff)||'Длительность уточняется'}</span></li>
      <li><CourseIcon kind="calendar"/><span>{schedule[tariff.schedule]||schedule.unknown}</span></li>
      <li><CourseIcon kind="people"/><span>{tariff.enrollment==='open'?'Набор открыт':tariff.enrollment==='continuous'?'Постоянный набор':tariff.enrollment==='closed'?'Набор закрыт':'Набор уточняется'}</span></li>
     </ul>
     <a ref={selected?primaryAction:undefined} className="primary linkButton coursePrimaryAction" href={'/out/'+encodeURIComponent(tariff.id)} target="_blank" rel="noopener noreferrer">{comparable?'На сайт курса':'Проверить набор'} <ExternalMark/></a>
     {comparable&&<div className="coursePurchaseCompare"><CatalogActions key={tariff.id} id={tariff.id}/></div>}
     <small className="coursePriceChecked">Цена проверена: {checked(tariff.price_checked_at)}</small>
     {offers.length>1&&<small className="courseTariffStatus">{selected?'Подробные условия этого тарифа показаны ниже':'Выберите тариф, чтобы посмотреть подробные условия'}</small>}
     </section>;
    })}
    {!offers.length&&<section className="courseSummary coursePurchase" aria-labelledby="course-purchase-title">
     <h2 id="course-purchase-title">Тарифы уточняются</h2><p>Актуальные предложения смотрите на сайте школы.</p>
     <a className="primary linkButton" href={c.source} target="_blank" rel="noopener noreferrer">Программа на сайте школы <ExternalMark/></a>
    </section>}
    </fieldset>
   </aside>
   <div className="courseContent">
    <nav className="courseSections" aria-label="Разделы курса">{[['course-overview','Обзор'],['course-conditions','Условия'],['course-sources','Источник']].map(([id,label])=><a key={id} href={'#'+id} aria-current={section===id?'location':undefined} onClick={()=>setSection(id)}>{label}</a>)}</nav>
    <section className="courseConditions" id="course-conditions" tabIndex={-1} aria-labelledby="course-audience-title">
     <h2 id="course-audience-title"><CourseIcon kind="person"/>Для кого подходит</h2>
     <dl className="courseFacts"><Fact label="Кому подходит" unknown={!audience.length}>{experience}</Fact><Fact label="Требования к опыту" unknown={!prerequisite}>{requirement}</Fact></dl>
     <h2><CourseIcon kind="code"/>Формат и поддержка</h2>
     {offers.length>1&&<p className="courseConditionsTariff">Условия тарифа: <strong>{offer?.name}</strong></p>}
     <dl className="courseFacts">
      <Fact label="Длительность" unknown={!courseDuration}>{courseDuration||'Уточните у школы'}</Fact>
      <Fact label="Нагрузка" unknown={!courseWorkload}>{courseWorkload||'Уточните у школы'}</Fact>
      <Fact label="Расписание" unknown={!offer||offer.schedule==='unknown'}>{offer?(schedule[offer.schedule]||schedule.unknown):schedule.unknown}</Fact>
      <Fact label="Проверка кода" unknown={!offer||supportValue(offer,'review')===null}>{offer?supportText(offer,'review'):'Уточните у школы'}</Fact>
      <Fact label="Индивидуальные занятия" unknown={!offer||supportValue(offer,'mentor')===null}>{offer?supportText(offer,'mentor'):'Уточните у школы'}</Fact>
      {offer?.learning_conditions?.community&&<Fact label="Учебное сообщество">{offer.learning_conditions.community.available?'Есть':'Нет'}</Fact>}
      <Fact label="Набор" unknown={!offer||offer.enrollment==='unknown'}>{offer?(enrollment[offer.enrollment]||enrollment.unknown):enrollment.unknown}</Fact>
     </dl>
    </section>
    <section className="courseProgram" aria-labelledby="course-program-title">
     <h2 id="course-program-title"><CourseIcon kind="book"/>Программа курса</h2>
     {c.topics.length>0&&<ul className="courseTopics">{c.topics.map(topic=><li key={topic}>{topic}</li>)}</ul>}
     <p>{c.topics.length?'Полное содержание и актуальные условия — у школы.':'Содержание программы уточняйте на сайте школы.'}</p>
     <a className="secondary linkButton courseProgramLink" href={c.source} target="_blank" rel="noopener noreferrer">Программа на сайте школы <ExternalMark/></a>
    </section>
    <section className="courseSources" id="course-sources" tabIndex={-1} aria-labelledby="course-sources-title">
     <h2 id="course-sources-title"><CourseIcon kind="link"/>Источник</h2>
     <a className="courseSourceLink" href={c.source} target="_blank" rel="noopener noreferrer">Опубликованная программа поставщика <ExternalMark/></a>
     <p>Условия проверены {checked(c.checked_at)}. Проверка опубликованной программы не означает прохождение курса. Срок доступа и ограничения тарифа уточняйте у поставщика.</p>
    </section>
   </div>
  </div>
  <p className="srOnly" role="status">{notice}</p>
  {showDock&&item&&offer&&<div className="courseSummary courseDock" role="region" aria-label="Быстрые действия по тарифу"><div><strong>{price(item)}</strong><small>{offers.length>1?offer.name:item.effective_price===null?'Уточните у школы':item.effective_price===0?'Бесплатный тариф':'за весь курс'}</small></div><a className="primary linkButton" href={'/out/'+encodeURIComponent(offer.id)} target="_blank" rel="noopener noreferrer">{actionLabel} <ExternalMark/></a></div>}
 </>;
}
